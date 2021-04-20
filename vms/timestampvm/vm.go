// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package timestampvm

import (
	"errors"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/engine/snowman/block"
	"github.com/ava-labs/avalanchego/vms/components/core"
)

const (
	dataLen      = 32
	codecVersion = 0
)

var (
	errNoPendingBlocks = errors.New("there is no block to propose")
	errBadGenesisBytes = errors.New("genesis data should be bytes (max length 32)")

	_ block.ChainVM   = &VM{}
	_ common.StaticVM = &VM{}
)

// VM implements the snowman.VM interface
// Each block in this chain contains a Unix timestamp
// and a piece of data (a string)
type VM struct {
	core.SnowmanVM
	codec codec.Manager
	// Proposed pieces of data that haven't been put into a block and proposed yet
	mempool [][dataLen]byte
}

// Initialize this vm
// [ctx] is this vm's context
// [db] is this vm's database
// [toEngine] is used to notify the consensus engine that new blocks are
//   ready to be added to consensus
// The data in the genesis block is [genesisData]
func (vm *VM) Initialize(
	ctx *snow.Context,
	db database.Database,
	genesisData []byte,
	toEngine chan<- common.Message,
	_ []*common.Fx,
) error {
	if err := vm.SnowmanVM.Initialize(ctx, db, vm.ParseBlock, toEngine); err != nil {
		ctx.Log.Error("error initializing SnowmanVM: %v", err)
		return err
	}
	c := linearcodec.NewDefault()
	manager := codec.NewDefaultManager()
	if err := manager.RegisterCodec(codecVersion, c); err != nil {
		return err
	}
	vm.codec = manager

	// If database is empty, create it using the provided genesis data
	if !vm.DBInitialized() {
		if len(genesisData) > dataLen {
			return errBadGenesisBytes
		}

		// genesisData is a byte slice but each block contains an byte array
		// Take the first [dataLen] bytes from genesisData and put them in an array
		var genesisDataArr [dataLen]byte
		copy(genesisDataArr[:], genesisData)

		// Create the genesis block
		// Timestamp of genesis block is 0. It has no parent.
		genesisBlock, err := vm.NewBlock(ids.Empty, 0, genesisDataArr, time.Unix(0, 0))
		if err != nil {
			vm.Ctx.Log.Error("error while creating genesis block: %v", err)
			return err
		}

		if err := vm.SaveBlock(vm.DB, genesisBlock); err != nil {
			vm.Ctx.Log.Error("error while saving genesis block: %v", err)
			return err
		}

		// Accept the genesis block
		// Sets [vm.lastAccepted] and [vm.preferred]
		if err := genesisBlock.Accept(); err != nil {
			return fmt.Errorf("error accepting genesis block: %w", err)
		}

		if err := vm.SetDBInitialized(); err != nil {
			return fmt.Errorf("error while setting db to initialized: %w", err)
		}

		// Flush VM's database to underlying db
		if err := vm.DB.Commit(); err != nil {
			vm.Ctx.Log.Error("error while committing db: %v", err)
			return err
		}
	}
	return nil
}

// CreateHandlers returns a map where:
// Keys: The path extension for this VM's API (empty in this case)
// Values: The handler for the API
func (vm *VM) CreateHandlers() (map[string]*common.HTTPHandler, error) {
	handler, err := vm.NewHandler("timestamp", &Service{vm})
	return map[string]*common.HTTPHandler{
		"": handler,
	}, err
}

// CreateStaticHandlers returns a map where:
// Keys: The path extension for this VM's static API
// Values: The handler for that static API
// We return nil because this VM has no static API
func (vm *VM) CreateStaticHandlers() (map[string]*common.HTTPHandler, error) { return nil, nil }

// Health implements the common.VM interface
func (vm *VM) HealthCheck() (interface{}, error) { return nil, nil }

// BuildBlock returns a block that this vm wants to add to consensus
func (vm *VM) BuildBlock() (snowman.Block, error) {
	if len(vm.mempool) == 0 { // There is no block to be built
		return nil, errNoPendingBlocks
	}

	// Get the value to put in the new block
	value := vm.mempool[0]
	vm.mempool = vm.mempool[1:]

	// Notify consensus engine that there are more pending data for blocks
	// (if that is the case) when done building this block
	if len(vm.mempool) > 0 {
		defer vm.NotifyBlockReady()
	}

	preferredIntf, err := vm.GetBlock(vm.Preferred())
	if err != nil {
		return nil, fmt.Errorf("couldn't get preferred block")
	}
	preferredHeight := preferredIntf.(*Block).Height()

	// Build the block
	block, err := vm.NewBlock(vm.Preferred(), preferredHeight+1, value, time.Now())
	if err != nil {
		return nil, err
	}
	return block, nil
}

// proposeBlock appends [data] to [p.mempool].
// Then it notifies the consensus engine
// that a new block is ready to be added to consensus
// (namely, a block with data [data])
func (vm *VM) proposeBlock(data [dataLen]byte) {
	vm.mempool = append(vm.mempool, data)
	vm.NotifyBlockReady()
}

// ParseBlock parses [bytes] to a snowman.Block
// This function is used by the vm's state to unmarshal blocks saved in state
func (vm *VM) ParseBlock(bytes []byte) (snowman.Block, error) {
	block := &Block{}
	_, err := vm.codec.Unmarshal(bytes, block)
	block.Initialize(bytes, &vm.SnowmanVM)
	return block, err
}

// NewBlock returns a new Block where:
// - the block's parent is [parentID]
// - the block's data is [data]
// - the block's timestamp is [timestamp]
// The block is persisted in storage
func (vm *VM) NewBlock(parentID ids.ID, height uint64, data [dataLen]byte, timestamp time.Time) (*Block, error) {
	block := &Block{
		Block:     core.NewBlock(parentID, height),
		Data:      data,
		Timestamp: timestamp.Unix(),
	}
	blockBytes, err := vm.codec.Marshal(codecVersion, block)
	if err != nil {
		return nil, err
	}
	block.Initialize(blockBytes, &vm.SnowmanVM)
	return block, nil
}
