// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package timestampvm

import (
	"fmt"
	"testing"

	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/formatting"
)

var blockchainID = ids.NewID([32]byte{1, 2, 3})

// Utility function to assert that [block] has:
// * Parent with ID [parentID]
// * Data [expectedData]
// * Verify() returns nil iff passesVerify == true
func assertBlock(block *Block, parentID ids.ID, expectedData [dataLen]byte, passesVerify bool) error {
	if !block.ParentID().Equals(parentID) {
		return fmt.Errorf("expect parent ID to be %s but was %s", parentID, block.ParentID())
	}
	if block.Data != expectedData {
		return fmt.Errorf("expected data to be %v but was %v", expectedData, block.Data)
	}
	if block.Verify() != nil && passesVerify {
		return fmt.Errorf("expected block to pass verification but it fails")
	}
	if block.Verify() == nil && !passesVerify {
		return fmt.Errorf("expected block to fail verification but it passes")
	}
	return nil
}

// Assert that after initialization, the vm has the state we expect
func TestGenesis(t *testing.T) {
	// Initialize the vm
	db := memdb.New()
	msgChan := make(chan common.Message, 1)
	vm := &VM{}
	ctx := snow.DefaultContextTest()
	ctx.ChainID = blockchainID
	vm.Initialize(ctx, db, []byte{0, 0, 0, 0, 0}, msgChan, nil)

	// Verify that the db is initialized
	if !vm.DBInitialized() {
		t.Fatal("db should be initialized")
	}

	// Get lastAccepted
	lastAccepted := vm.LastAccepted()
	if lastAccepted.IsZero() {
		t.Fatal("lastAccepted should not be empty")
	}

	// Verify that getBlock returns the genesis block, and the genesis block
	// is the type we expect
	genesisSnowmanBlock, err := vm.GetBlock(lastAccepted) // genesisBlock as snowman.Block
	if err != nil {
		t.Fatalf("couldn't get genesisBlock: %s", err)
	}
	genesisBlock, ok := genesisSnowmanBlock.(*Block) // type assert that genesisBlock is a *Block
	if !ok {
		t.Fatal("type of genesisBlock should be *Block")
	}

	// Verify that the genesis block has the data we expect
	if err := assertBlock(genesisBlock, ids.Empty, [32]byte{0, 0, 0, 0, 0}, true); err != nil {
		t.Fatal(err)
	}
}

func TestHappyPath(t *testing.T) {
	// Initialize the vm
	db := memdb.New()
	msgChan := make(chan common.Message, 1)
	vm := &VM{}
	ctx := snow.DefaultContextTest()
	ctx.ChainID = blockchainID
	if err := vm.Initialize(ctx, db, []byte{0, 0, 0, 0, 0}, msgChan, nil); err != nil {
		t.Fatal(err)
	}

	genesisBlock, err := vm.GetBlock(vm.LastAccepted())
	if err != nil {
		t.Fatal("could not get genesis block")
	}
	// in an actual execution, the engine would set the preference
	vm.SetPreference(genesisBlock.ID())

	ctx.Lock.Lock()
	vm.proposeBlock([dataLen]byte{0, 0, 0, 0, 1}) // propose a value
	ctx.Lock.Unlock()

	select { // assert there is a pending tx message to the engine
	case msg := <-msgChan:
		if msg != common.PendingTxs {
			t.Fatal("Wrong message")
		}
	default:
		t.Fatal("should have been pendingTxs message on channel")
	}

	// build the block
	ctx.Lock.Lock()
	snowmanBlock2, err := vm.BuildBlock()
	if err != nil {
		t.Fatalf("problem building block: %s", err)
	}
	if err := snowmanBlock2.Verify(); err != nil {
		t.Fatal(err)
	} else if err := snowmanBlock2.Accept(); err != nil { // accept the block
		t.Fatalf("couldn't accept block: %s", err)
	} else if err := vm.SaveBlock(snowmanBlock2); err != nil { // normally the engine would do this
		t.Fatalf("couldn't save block: %s", err)
	}
	vm.SetPreference(snowmanBlock2.ID())

	// Should be the block we just accepted
	snowmanBlock2, err = vm.GetBlock(vm.LastAccepted())
	if err != nil {
		t.Fatal("couldn't get block")
	}
	block2, ok := snowmanBlock2.(*Block)
	if !ok {
		t.Fatal("genesis block should be type *Block")
	}
	// Assert the block we accepted has the data we expect
	if err := assertBlock(block2, genesisBlock.ID(), [dataLen]byte{0, 0, 0, 0, 1}, true); err != nil {
		t.Fatal(err)
	}

	vm.proposeBlock([dataLen]byte{0, 0, 0, 0, 2}) // propose a block
	ctx.Lock.Unlock()

	select { // verify there is a pending tx message to the engine
	case msg := <-msgChan:
		if msg != common.PendingTxs {
			t.Fatal("Wrong message")
		}
	default:
		t.Fatal("should have been pendingTxs message on channel")
	}

	ctx.Lock.Lock()

	// build the block
	if block, err := vm.BuildBlock(); err != nil {
		t.Fatalf("problem building block: %s", err)
	} else {
		if err := block.Verify(); err != nil {
			t.Fatal(err)
		} else if err := block.Accept(); err != nil { // accept the block
			t.Fatalf("couldn't accept block: %s", err)
		} else if err := vm.SaveBlock(block); err != nil { // normally the engine would do this
			t.Fatalf("couldn't save block: %s", err)
		}
		vm.SetPreference(block.ID()) // normally the engine would do this
	}

	// The block we just accepted
	snowmanBlock3, err := vm.GetBlock(vm.LastAccepted())
	if err != nil {
		t.Fatal("couldn't get block")
	}
	block3, ok := snowmanBlock3.(*Block)
	if !ok {
		t.Fatal("genesis block should be type *Block")
	}
	// Assert the block we accepted has the data we expect
	if err := assertBlock(block3, snowmanBlock2.ID(), [dataLen]byte{0, 0, 0, 0, 2}, true); err != nil {
		t.Fatal(err)
	}

	// Next, check the blocks we added are there
	if block2FromState, err := vm.GetBlock(block2.ID()); err != nil {
		t.Fatal(err)
	} else if !block2FromState.ID().Equals(block2.ID()) {
		t.Fatal("expected IDs to match but they don't")
	}
	if block3FromState, err := vm.GetBlock(block3.ID()); err != nil {
		t.Fatal(err)
	} else if !block3FromState.ID().Equals(block3.ID()) {
		t.Fatal("expected IDs to match but they don't")
	}

	ctx.Lock.Unlock()
}

func TestMakeStringFrom32Bytes(t *testing.T) {
	bytes := [32]byte{'w', 'o', 'o'}
	bytesFormatter := formatting.CB58{Bytes: bytes[:]}
	t.Log(bytesFormatter.String())
}

func TestService(t *testing.T) {
	// Initialize the vm
	db := memdb.New()
	msgChan := make(chan common.Message, 1)
	vm := &VM{}
	ctx := snow.DefaultContextTest()
	ctx.ChainID = blockchainID
	if err := vm.Initialize(ctx, db, []byte{0, 0, 0, 0, 0}, msgChan, nil); err != nil {
		t.Fatal(err)
	}

	service := Service{vm}
	if err := service.GetBlock(nil, &GetBlockArgs{}, &GetBlockReply{}); err != nil {
		t.Fatal(err)
	}
}
