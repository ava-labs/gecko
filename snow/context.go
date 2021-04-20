// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snow

import (
	"sync"
	"time"

	stdatomic "sync/atomic"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/api/keystore"
	"github.com/ava-labs/avalanchego/chains/atomic"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/timer"
)

// EventDispatcher ...
type EventDispatcher interface {
	Issue(ctx *Context, containerID ids.ID, container []byte) error
	// If the returned error is non-nil, the chain associated with [ctx] should shut
	// down and not commit [container] or any other container to its database as accepted.
	// Accept must be called before [containerID] is committed to the VM as accepted.
	Accept(ctx *Context, containerID ids.ID, container []byte) error
	Reject(ctx *Context, containerID ids.ID, container []byte) error
}

// AliasLookup ...
type AliasLookup interface {
	Lookup(alias string) (ids.ID, error)
	PrimaryAlias(id ids.ID) (string, error)
}

// SubnetLookup ...
type SubnetLookup interface {
	SubnetID(chainID ids.ID) (ids.ID, error)
}

// Context is information about the current execution.
// [NetworkID] is the ID of the network this context exists within.
// [ChainID] is the ID of the chain this context exists within.
// [NodeID] is the ID of this node
type Context struct {
	NetworkID uint32
	SubnetID  ids.ID
	ChainID   ids.ID
	NodeID    ids.ShortID

	XChainID    ids.ID
	AVAXAssetID ids.ID

	Log                 logging.Logger
	DecisionDispatcher  EventDispatcher
	ConsensusDispatcher EventDispatcher
	Lock                sync.RWMutex
	Keystore            keystore.BlockchainKeystore
	SharedMemory        atomic.SharedMemory
	BCLookup            AliasLookup
	SNLookup            SubnetLookup
	Namespace           string
	Metrics             prometheus.Registerer

	// Epoch management
	EpochFirstTransition time.Time
	EpochDuration        time.Duration
	Clock                timer.Clock

	// Non-zero iff this chain bootstrapped. Should only be accessed atomically.
	bootstrapped uint32
}

// IsBootstrapped returns true iff this chain is done bootstrapping
func (ctx *Context) IsBootstrapped() bool {
	return stdatomic.LoadUint32(&ctx.bootstrapped) > 0
}

// Bootstrapped marks this chain as done bootstrapping
func (ctx *Context) Bootstrapped() {
	stdatomic.StoreUint32(&ctx.bootstrapped, 1)
}

// Epoch this context thinks it's in based on the wall clock time.
func (ctx *Context) Epoch() uint32 {
	now := ctx.Clock.Time()
	timeSinceFirstEpochTransition := now.Sub(ctx.EpochFirstTransition)
	epochsSinceFirstTransition := timeSinceFirstEpochTransition / ctx.EpochDuration
	currentEpoch := epochsSinceFirstTransition + 1
	if currentEpoch < 0 {
		return 0
	}
	return uint32(currentEpoch)
}

// DefaultContextTest ...
func DefaultContextTest() *Context {
	aliaser := &ids.Aliaser{}
	aliaser.Initialize()
	return &Context{
		NetworkID:           0,
		SubnetID:            ids.Empty,
		ChainID:             ids.Empty,
		NodeID:              ids.ShortEmpty,
		Log:                 logging.NoLog{},
		DecisionDispatcher:  emptyEventDispatcher{},
		ConsensusDispatcher: emptyEventDispatcher{},
		BCLookup:            aliaser,
		Namespace:           "",
		Metrics:             prometheus.NewRegistry(),
	}
}

type emptyEventDispatcher struct{}

func (emptyEventDispatcher) Issue(*Context, ids.ID, []byte) error  { return nil }
func (emptyEventDispatcher) Accept(*Context, ids.ID, []byte) error { return nil }
func (emptyEventDispatcher) Reject(*Context, ids.ID, []byte) error { return nil }
