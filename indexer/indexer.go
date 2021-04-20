package indexer

import (
	"fmt"
	"io"
	"math"
	"sync"

	"github.com/ava-labs/avalanchego/api/server"
	"github.com/ava-labs/avalanchego/network"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/ava-labs/avalanchego/utils/wrappers"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/codec/reflectcodec"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/engine/snowman"
	"github.com/ava-labs/avalanchego/snow/triggers"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/gorilla/rpc/v2"
)

const (
	indexNamePrefix = "index-"
	codecVersion    = uint16(0)
	// Max size, in bytes, of something serialized by this indexer
	// Assumes no containers are larger than math.MaxUint32
	// wrappers.IntLen accounts for the size of the container bytes
	// wrappers.LongLen accounts for the timestamp of the container
	// hashing.HashLen accounts for the container ID
	// wrappers.ShortLen accounts for the codec version
	codecMaxSize = int(network.DefaultMaxMessageSize) + wrappers.IntLen + wrappers.LongLen + hashing.HashLen + wrappers.ShortLen
)

var (
	txPrefix                = byte(0x01)
	vtxPrefix               = byte(0x02)
	blockPrefix             = byte(0x03)
	isIncompletePrefix      = byte(0x04)
	previouslyIndexedPrefix = byte(0x05)
	hasRunKey               = []byte{0x07}
)

var (
	_ Indexer = &indexer{}
)

// Config for an indexer
type Config struct {
	DB                                      database.Database
	Log                                     logging.Logger
	IndexingEnabled                         bool
	AllowIncompleteIndex                    bool
	DecisionDispatcher, ConsensusDispatcher *triggers.EventDispatcher
	APIServer                               server.RouteAdder
	ShutdownF                               func()
}

// Indexer causes accepted containers for a given chain
// to be indexed by their ID and by the order in which
// they were accepted by this node.
// Indexer is threadsafe.
type Indexer interface {
	RegisterChain(name string, ctx *snow.Context, engine common.Engine)
	// Close will do nothing and return nil after the first call
	io.Closer
}

// NewIndexer returns a new Indexer and registers a new endpoint on the given API server.
func NewIndexer(config Config) (Indexer, error) {
	indexer := &indexer{
		codec:                codec.NewManager(codecMaxSize),
		log:                  config.Log,
		db:                   config.DB,
		allowIncompleteIndex: config.AllowIncompleteIndex,
		indexingEnabled:      config.IndexingEnabled,
		consensusDispatcher:  config.ConsensusDispatcher,
		decisionDispatcher:   config.DecisionDispatcher,
		txIndices:            map[ids.ID]Index{},
		vtxIndices:           map[ids.ID]Index{},
		blockIndices:         map[ids.ID]Index{},
		routeAdder:           config.APIServer,
		shutdownF:            config.ShutdownF,
	}
	if err := indexer.codec.RegisterCodec(
		codecVersion,
		linearcodec.New(reflectcodec.DefaultTagName, math.MaxUint32),
	); err != nil {
		return nil, fmt.Errorf("couldn't register codec: %s", err)
	}
	hasRun, err := indexer.hasRun()
	if err != nil {
		return nil, err
	}
	indexer.hasRunBefore = hasRun
	return indexer, indexer.markHasRun()
}

// indexer implements Indexer
type indexer struct {
	codec  codec.Manager
	clock  timer.Clock
	lock   sync.RWMutex
	log    logging.Logger
	db     database.Database
	closed bool

	// Called in a goroutine on shutdown
	shutdownF func()

	// true if this is not the first run using this database
	hasRunBefore bool

	// Used to add API endpoint for new indices
	routeAdder server.RouteAdder

	// If true, allow running in such a way that could allow the creation
	// of an index which could be missing accepted containers.
	allowIncompleteIndex bool

	// If false, don't create index for a chain when RegisterChain is called
	indexingEnabled bool

	// Chain ID --> index of blocks of that chain (if applicable)
	blockIndices map[ids.ID]Index
	// Chain ID --> index of vertices of that chain (if applicable)
	vtxIndices map[ids.ID]Index
	// Chain ID --> index of txs of that chain (if applicable)
	txIndices map[ids.ID]Index

	// Notifies of newly accepted blocks and vertices
	consensusDispatcher *triggers.EventDispatcher
	// Notifies of newly accepted transactions
	decisionDispatcher *triggers.EventDispatcher
}

// Assumes [engine]'s context lock is not held
func (i *indexer) RegisterChain(name string, ctx *snow.Context, engine common.Engine) {
	i.lock.Lock()
	defer i.lock.Unlock()

	if i.closed {
		i.log.Debug("not registering chain %s because indexer is closed", name)
		return
	} else if ctx.SubnetID != constants.PrimaryNetworkID {
		i.log.Debug("not registering chain %s because it's not in primary network", name)
		return
	}

	chainID := ctx.ChainID
	if i.blockIndices[chainID] != nil || i.txIndices[chainID] != nil || i.vtxIndices[chainID] != nil {
		i.log.Warn("chain %s is already being indexed", chainID)
		return
	}

	// If the index is incomplete, make sure that's OK. Otherwise, cause node to die.
	isIncomplete, err := i.isIncomplete(chainID)
	if err != nil {
		i.log.Error("couldn't get whether chain %s is incomplete: %s", name, err)
		if err := i.close(); err != nil {
			i.log.Error("error while closing indexer: %s", err)
		}
		return
	}

	// See if this chain was indexed in a previous run
	previouslyIndexed, err := i.previouslyIndexed(chainID)
	if err != nil {
		i.log.Error("couldn't get whether chain %s was previously indexed: %s", name, err)
		if err := i.close(); err != nil {
			i.log.Error("error while closing indexer: %s", err)
		}
		return
	}

	if !i.indexingEnabled { // Indexing is disabled
		if previouslyIndexed && !i.allowIncompleteIndex {
			// We indexed this chain in a previous run but not in this run.
			// This would create an incomplete index, which is not allowed, so exit.
			i.log.Fatal("running would cause index %s would become incomplete but incomplete indices are disabled", name)
			if err := i.close(); err != nil {
				i.log.Error("error while closing indexer: %s", err)
			}
			return
		}

		// Creating an incomplete index is allowed. Mark index as incomplete.
		err := i.markIncomplete(chainID)
		if err == nil {
			return
		}
		i.log.Fatal("couldn't mark chain %s as incomplete: %s", name, err)
		if err := i.close(); err != nil {
			i.log.Error("error while closing indexer: %s", err)
		}
		return
	}

	if !i.allowIncompleteIndex && isIncomplete && (previouslyIndexed || i.hasRunBefore) {
		i.log.Fatal("index %s is incomplete but incomplete indices are disabled. Shutting down", name)
		if err := i.close(); err != nil {
			i.log.Error("error while closing indexer: %s", err)
		}
		return
	}

	// Mark that in this run, this chain was indexed
	if err := i.markPreviouslyIndexed(chainID); err != nil {
		i.log.Error("couldn't mark chain %s as indexed: %s", name, err)
		if err := i.close(); err != nil {
			i.log.Error("error while closing indexer: %s", err)
		}
		return
	}

	switch engine.(type) {
	case snowman.Engine:
		index, err := i.registerChainHelper(chainID, blockPrefix, name, "block", i.consensusDispatcher)
		if err != nil {
			i.log.Fatal("couldn't create block index for %s: %s", name, err)
			if err := i.close(); err != nil {
				i.log.Error("error while closing indexer: %s", err)
			}
			return
		}
		i.blockIndices[chainID] = index
	case avalanche.Engine:
		vtxIndex, err := i.registerChainHelper(chainID, vtxPrefix, name, "vtx", i.consensusDispatcher)
		if err != nil {
			i.log.Fatal("couldn't create vertex index for %s: %s", name, err)
			if err := i.close(); err != nil {
				i.log.Error("error while closing indexer: %s", err)
			}
			return
		}
		i.vtxIndices[chainID] = vtxIndex

		txIndex, err := i.registerChainHelper(chainID, txPrefix, name, "tx", i.decisionDispatcher)
		if err != nil {
			i.log.Fatal("couldn't create tx index for %s: %s", name, err)
			if err := i.close(); err != nil {
				i.log.Error("error while closing indexer: %s", err)
			}
			return
		}
		i.txIndices[chainID] = txIndex
	default:
		i.log.Error("got unexpected engine type %T", engine)
		if err := i.close(); err != nil {
			i.log.Error("error while closing indexer: %s", err)
		}
		return
	}

}

func (i *indexer) registerChainHelper(
	chainID ids.ID,
	prefixEnd byte,
	name, endpoint string,
	dispatcher *triggers.EventDispatcher,
) (Index, error) {
	prefix := make([]byte, hashing.HashLen+wrappers.ByteLen)
	copy(prefix, chainID[:])
	prefix[hashing.HashLen] = prefixEnd
	indexDB := prefixdb.New(prefix, i.db)
	index, err := newIndex(indexDB, i.log, i.codec, i.clock)
	if err != nil {
		_ = indexDB.Close()
		return nil, err
	}

	// Register index to learn about new accepted vertices
	if err := dispatcher.RegisterChain(chainID, fmt.Sprintf("%s%s", indexNamePrefix, chainID), index, true); err != nil {
		_ = index.Close()
		return nil, err
	}

	// Create an API endpoint for this index
	apiServer := rpc.NewServer()
	codec := json.NewCodec()
	apiServer.RegisterCodec(codec, "application/json")
	apiServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	if err := apiServer.RegisterService(&service{Index: index}, "index"); err != nil {
		_ = index.Close()
		return nil, err
	}
	handler := &common.HTTPHandler{LockOptions: common.NoLock, Handler: apiServer}
	if err := i.routeAdder.AddRoute(handler, &sync.RWMutex{}, "index/"+name, "/"+endpoint, i.log); err != nil {
		_ = index.Close()
		return nil, err
	}
	return index, nil
}

// Close this indexer. Stops indexing all chains.
// Closes [i.db]. Assumes Close is only called after
// the node is done making decisions.
// Calling Close after it has been called does nothing.
func (i *indexer) Close() error {
	i.lock.Lock()
	defer i.lock.Unlock()

	return i.close()
}

func (i *indexer) close() error {
	if i.closed {
		return nil
	}
	i.closed = true

	errs := &wrappers.Errs{}
	for chainID, txIndex := range i.txIndices {
		errs.Add(
			txIndex.Close(),
			i.decisionDispatcher.DeregisterChain(chainID, fmt.Sprintf("%s%s", indexNamePrefix, chainID)),
		)
	}
	for chainID, vtxIndex := range i.vtxIndices {
		errs.Add(
			vtxIndex.Close(),
			i.consensusDispatcher.DeregisterChain(chainID, fmt.Sprintf("%s%s", indexNamePrefix, chainID)),
		)
	}
	for chainID, blockIndex := range i.blockIndices {
		errs.Add(
			blockIndex.Close(),
			i.consensusDispatcher.DeregisterChain(chainID, fmt.Sprintf("%s%s", indexNamePrefix, chainID)),
		)
	}
	errs.Add(i.db.Close())

	go i.shutdownF()
	return errs.Err
}

func (i *indexer) markIncomplete(chainID ids.ID) error {
	key := make([]byte, hashing.HashLen+wrappers.ByteLen)
	copy(key, chainID[:])
	key[hashing.HashLen] = isIncompletePrefix
	return i.db.Put(key, nil)
}

// Returns true if this chain is incomplete
func (i *indexer) isIncomplete(chainID ids.ID) (bool, error) {
	key := make([]byte, hashing.HashLen+wrappers.ByteLen)
	copy(key, chainID[:])
	key[hashing.HashLen] = isIncompletePrefix
	return i.db.Has(key)
}

func (i *indexer) markPreviouslyIndexed(chainID ids.ID) error {
	key := make([]byte, hashing.HashLen+wrappers.ByteLen)
	copy(key, chainID[:])
	key[hashing.HashLen] = previouslyIndexedPrefix
	return i.db.Put(key, nil)
}

// Returns true if this chain is incomplete
func (i *indexer) previouslyIndexed(chainID ids.ID) (bool, error) {
	key := make([]byte, hashing.HashLen+wrappers.ByteLen)
	copy(key, chainID[:])
	key[hashing.HashLen] = previouslyIndexedPrefix
	return i.db.Has(key)
}

// Mark that the node has run at least once
func (i *indexer) markHasRun() error {
	return i.db.Put(hasRunKey, nil)
}

// Returns true if the node has run before
func (i *indexer) hasRun() (bool, error) {
	return i.db.Has(hasRunKey)
}
