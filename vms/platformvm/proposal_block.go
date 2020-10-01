// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/vms/components/core"
)

// ProposalBlock is a proposal to change the chain's state.
// A proposal may be to:
// 	1. Advance the chain's timestamp (*AdvanceTimeTx)
//  2. Remove a staker from the staker set (*RewardStakerTx)
//  3. Add a new staker to the set of pending (future) stakers (*AddStakerTx)
// The proposal will be enacted (change the chain's state) if the proposal block
// is accepted and followed by an accepted Commit block
type ProposalBlock struct {
	CommonBlock `serialize:"true"`

	Tx Tx `serialize:"true" json:"tx"`

	// The database that the chain will have if this block's proposal is committed
	onCommitDB *versiondb.Database
	// The database that the chain will have if this block's proposal is aborted
	onAbortDB *versiondb.Database
	// The function to execute if this block's proposal is committed
	onCommitFunc func() error
	// The function to execute if this block's proposal is aborted
	onAbortFunc func() error
}

// Accept implements the snowman.Block interface
func (pb *ProposalBlock) Accept() error {
	pb.SetStatus(choices.Accepted)
	pb.VM.LastAcceptedID = pb.ID()
	return nil
}

// Initialize this block.
// Sets [pb.vm] to [vm] and populates non-serialized fields
// This method should be called when a block is unmarshaled from bytes
func (pb *ProposalBlock) initialize(vm *VM, bytes []byte) error {
	pb.vm = vm
	pb.Block.Initialize(bytes, vm.SnowmanVM)

	unsignedBytes, err := pb.vm.codec.Marshal(&pb.Tx.UnsignedTx)
	if err != nil {
		return err
	}
	signedBytes, err := pb.vm.codec.Marshal(&pb.Tx)
	if err != nil {
		return err
	}
	pb.Tx.Initialize(unsignedBytes, signedBytes)
	return nil
}

// setBaseDatabase sets this block's base database to [db]
func (pb *ProposalBlock) setBaseDatabase(db database.Database) {
	if err := pb.onCommitDB.SetDatabase(db); err != nil {
		pb.vm.Ctx.Log.Error("problem while setting base database: %s", err)
	}
	if err := pb.onAbortDB.SetDatabase(db); err != nil {
		pb.vm.Ctx.Log.Error("problem while setting base database: %s", err)
	}
}

// onCommit should only be called after Verify is called.
// onCommit returns:
//   1. A database that contains the state of the chain assuming this proposal
//      is enacted. (That is, if this block is accepted and followed by an
//      accepted Commit block.)
//   2. A function be be executed when this block's proposal is committed.
//      This function should not write to state.
func (pb *ProposalBlock) onCommit() (*versiondb.Database, func() error) {
	return pb.onCommitDB, pb.onCommitFunc
}

// onAbort should only be called after Verify is called.
// onAbort returns a database that contains the state of the chain assuming this
// block's proposal is rejected. (That is, if this block is accepted and
// followed by an accepted Abort block.)
func (pb *ProposalBlock) onAbort() (*versiondb.Database, func() error) {
	return pb.onAbortDB, pb.onAbortFunc
}

// Verify this block is valid.
//
// The parent block must either be a Commit or an Abort block.
//
// If this block is valid, this function also sets pas.onCommit and pas.onAbort.
func (pb *ProposalBlock) Verify() error {
	tx, ok := pb.Tx.UnsignedTx.(UnsignedProposalTx)
	if !ok {
		return errWrongTxType
	}

	parentIntf := pb.parentBlock()

	// The parent of a proposal block (ie this block) must be a decision block
	parent, ok := parentIntf.(decision)
	if !ok {
		if err := pb.Reject(); err == nil {
			if err := pb.vm.DB.Commit(); err != nil {
				pb.vm.Ctx.Log.Error("error committing Proposal block as rejected: %s", err)
			}
		} else {
			pb.vm.DB.Abort()
		}
		return errInvalidBlockType
	}

	// pdb is the database if this block's parent is accepted
	pdb := parent.onAccept()

	txID := tx.ID()

	var err TxError
	pb.onCommitDB, pb.onAbortDB, pb.onCommitFunc, pb.onAbortFunc, err = tx.SemanticVerify(pb.vm, pdb, &pb.Tx)
	if err != nil {
		pb.vm.droppedTxCache.Put(txID, nil) // cache tx as dropped
		// If this block's transaction proposes to advance the timestamp, the transaction may fail
		// verification now but be valid in the future, so don't (permanently) mark the block as rejected.
		if !err.Temporary() {
			if err := pb.Reject(); err == nil {
				if err := pb.vm.DB.Commit(); err != nil {
					pb.vm.Ctx.Log.Error("error committing Proposal block as rejected: %s", err)
				}
			} else {
				pb.vm.DB.Abort()
			}
		}
		return err
	}

	txBytes := tx.Bytes()
	if err := pb.vm.putTx(pb.onCommitDB, txID, txBytes); err != nil {
		return err
	}
	if err := pb.vm.putStatus(pb.onCommitDB, txID, Committed); err != nil {
		return err
	}

	if err := pb.vm.putTx(pb.onAbortDB, txID, txBytes); err != nil {
		return err
	}
	if err := pb.vm.putStatus(pb.onAbortDB, txID, Aborted); err != nil {
		return err
	}

	pb.vm.currentBlocks[pb.ID().Key()] = pb
	parentIntf.addChild(pb)
	return nil
}

// Options returns the possible children of this block in preferential order.
func (pb *ProposalBlock) Options() ([2]snowman.Block, error) {
	blockID := pb.ID()

	commit, err := pb.vm.newCommitBlock(blockID, pb.Height()+1)
	if err != nil {
		return [2]snowman.Block{}, err
	}
	abort, err := pb.vm.newAbortBlock(blockID, pb.Height()+1)
	if err != nil {
		return [2]snowman.Block{}, err
	}

	tx, ok := pb.Tx.UnsignedTx.(UnsignedProposalTx)
	if !ok {
		return [2]snowman.Block{}, errWrongTxType
	}

	if tx.InitiallyPrefersCommit(pb.vm) {
		return [2]snowman.Block{commit, abort}, nil
	}
	return [2]snowman.Block{abort, commit}, nil
}

// newProposalBlock creates a new block that proposes to issue a transaction.
// The parent of this block has ID [parentID]. The parent must be a decision block.
// Returns nil if there's an error while creating this block
func (vm *VM) newProposalBlock(parentID ids.ID, height uint64, tx Tx) (*ProposalBlock, error) {
	pb := &ProposalBlock{
		CommonBlock: CommonBlock{
			Block: core.NewBlock(parentID, height),
			vm:    vm,
		},
		Tx: tx,
	}

	// We marshal the block in this way (as a Block) so that we can unmarshal
	// it into a Block (rather than a *ProposalBlock)
	block := Block(pb)
	bytes, err := Codec.Marshal(&block)
	if err != nil {
		return nil, err
	}
	pb.Initialize(bytes, vm.SnowmanVM)
	return pb, nil
}
