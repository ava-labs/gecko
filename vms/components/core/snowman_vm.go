// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"errors"

	"github.com/gorilla/rpc/v2"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/vms/components/state"
)

var (
	errBadData = errors.New("got unexpected value from database")
)

// If the status of this ID is not choices.Accepted,
// the db has not yet been initialized
var dbInitializedID = ids.NewID([32]byte{'d', 'b', ' ', 'i', 'n', 'i', 't'})

// SnowmanVM provides the core functionality shared by most snowman vms
type SnowmanVM struct {
	State SnowmanState

	// VersionDB on top of underlying database
	// Important note: In order for writes to [DB] to be persisted,
	// DB.Commit() must be called
	// We use a versionDB here so user can do atomic commits as they see fit
	DB *versiondb.Database

	// The context of this vm
	Ctx *snow.Context

	// ID of the preferred block
	preferred ids.ID

	// ID of the last accepted block
	LastAcceptedID ids.ID

	// unmarshals bytes to a block
	unmarshalBlockFunc func([]byte) (snowman.Block, error)

	// channel to send messages to the consensus engine
	ToEngine chan<- common.Message
}

// SetPreference sets the block with ID [ID] as the preferred block
func (svm *SnowmanVM) SetPreference(ID ids.ID) { svm.preferred = ID }

// Preferred returns the ID of the preferred block
func (svm *SnowmanVM) Preferred() ids.ID { return svm.preferred }

// LastAccepted returns the block most recently accepted
func (svm *SnowmanVM) LastAccepted() ids.ID { return svm.LastAcceptedID }

// ParseBlock parses [bytes] to a block
func (svm *SnowmanVM) ParseBlock(bytes []byte) (snowman.Block, error) {
	return svm.unmarshalBlockFunc(bytes)
}

// GetBlock returns the block with ID [ID]
func (svm *SnowmanVM) GetBlock(ID ids.ID) (snowman.Block, error) {
	block, err := svm.State.Get(svm.DB, state.BlockTypeID, ID)
	if err != nil {
		return nil, err
	}
	if block, ok := block.(snowman.Block); ok {
		return block, nil
	}
	return nil, errBadData // Should never happen
}

// Bootstrapping marks this VM as bootstrapping
func (svm *SnowmanVM) Bootstrapping() error { return nil }

// Bootstrapped marks this VM as bootstrapped
func (svm *SnowmanVM) Bootstrapped() error { return nil }

// Shutdown this vm
func (svm *SnowmanVM) Shutdown() error {
	if svm.DB == nil {
		return nil
	}

	// flush DB
	if err := svm.DB.Commit(); err != nil {
		return err
	}

	// close underlying database
	if err := svm.DB.GetDatabase().Close(); err != nil {
		return err
	}
	return svm.DB.Close() // close versionDB
}

// DBInitialized returns true iff [svm]'s database has values in it already
func (svm *SnowmanVM) DBInitialized() bool {
	status := svm.State.GetStatus(svm.DB, dbInitializedID)
	return status == choices.Accepted
}

// SetDBInitialized marks the database as initialized
func (svm *SnowmanVM) SetDBInitialized() error {
	return svm.State.PutStatus(svm.DB, dbInitializedID, choices.Accepted)
}

// SaveBlock saves [block] to state
func (svm *SnowmanVM) SaveBlock(block snowman.Block) error {
	return svm.State.Put(svm.DB, state.BlockTypeID, block.ID(), block)
}

// NotifyBlockReady tells the consensus engine that a new block
// is ready to be created
func (svm *SnowmanVM) NotifyBlockReady() {
	select {
	case svm.ToEngine <- common.PendingTxs:
	default:
		svm.Ctx.Log.Debug("dropping message to consensus engine")
	}
}

// NewHandler returns a new Handler for a service where:
//   * The handler's functionality is defined by [service]
//     [service] should be a gorilla RPC service (see https://www.gorillatoolkit.org/pkg/rpc/v2)
//   * The name of the service is [name]
//   * The LockOption is the first element of [lockOption]
//     By default the LockOption is WriteLock
//     [lockOption] should have either 0 or 1 elements. Elements beside the first are ignored.
func (svm *SnowmanVM) NewHandler(name string, service interface{}, lockOption ...common.LockOption) (*common.HTTPHandler, error) {
	server := rpc.NewServer()
	server.RegisterCodec(json.NewCodec(), "application/json")
	server.RegisterCodec(json.NewCodec(), "application/json;charset=UTF-8")
	if err := server.RegisterService(service, name); err != nil {
		return nil, err
	}

	var lock common.LockOption = common.WriteLock
	if len(lockOption) != 0 {
		lock = lockOption[0]
	}
	return &common.HTTPHandler{LockOptions: lock, Handler: server}, nil
}

// Initialize this vm.
// If there is data in [db], sets [svm.lastAccepted] using data in the database,
// and sets [svm.preferred] to the last accepted block.
func (svm *SnowmanVM) Initialize(
	ctx *snow.Context,
	db database.Database,
	unmarshalBlockFunc func([]byte) (snowman.Block, error),
	toEngine chan<- common.Message,
) error {
	svm.Ctx = ctx
	svm.ToEngine = toEngine
	svm.DB = versiondb.New(db)

	var err error
	svm.State, err = NewSnowmanState(unmarshalBlockFunc)
	if err != nil {
		return err
	}

	if svm.DBInitialized() {
		if svm.LastAcceptedID, err = svm.State.GetLastAccepted(svm.DB); err != nil {
			return err
		}
		svm.preferred = svm.LastAcceptedID
	}

	return nil
}
