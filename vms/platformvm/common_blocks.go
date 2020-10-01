// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"errors"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/vms/components/core"
)

// When one stakes, one must specify the time one will start to validate and
// the time one will stop validating. The latter must be after the former, and both
// times must be in the future.
//
// When one wants to start staking:
// * They issue a transaction to that effect to an existing staker.
// * The staker checks whether the specified "start staking time" is in the past relative to
//   their wall clock.
//   ** If so, the staker ignores the transaction
//   ** If not, the staker issues a proposal block (see below) on behalf of the would-be staker.
//
// When one is done staking:
// * The staking set decides whether the staker should receive either:
//   ** Only the tokens that the staker put up as a bond
//	 ** The tokens the staker put up as a bond, and also a reward for staking
//
// This chain has three types of blocks:
// 1. A proposal block
//	- Contains a proposal to do one of the following:
//    * Change the chain time from t to t'. (This doubles as
//      proposing to update the staking set.)
//      ** It must be that: t' > t
//      ** It must be that: t' <= [time at which the next staker stops staking]
//    * Reward a staker upon their leaving the staking pool
//    	** It must be that chain time == [time for this staker to stop staking]
//    	** It must be that this staker is the next staker to stop staking
//    * Add a staker to the staking pool
//      ** It must be that: staker.startTime < chain time
//	- A proposal block is always followed by either a commit block or a rejection block
// 2. A commit block
//	- Does one of the following:
//    * Approve a proposal to change the chain time from t to t'
//      ** This should be the initial preference if t' <= Wall clock time
//    * Approve a proposal to reward for a staker upon their leaving the staking pool
//      ** It must be that: chain time == [time for this staker to stop staking]
//      ** It must be that: this staker is the next staker to stop staking
//    * Approve a proposal to add a staker to the staking pool
//      ** This should be the initial preference if staker.startTime > Wall clock
//         time
//	- A commit block must always be preceded on the chain by the proposal block whose
//	  proposal is being commited
// 3. A rejection block
//  - Does one of the following:
//    * Reject a proposal to change the chain time from t to t' (therefore keeping it at t)
//      ** This should be the initial preference if t' > [this node's wall clock time + Delta],
//         where Delta is our synchrony assumption
//    * Reject a proposal to reward for a staker upon their leaving the staking pool.
//      ** The staker only has their bond (locked tokens) returned
//      ** This should be the initial preference if the staker has had < Chi uptime
//      ** It must be that: t == [time for this staker to stop staking]
//      ** It must be that: this staker is the next staker to stop staking
//    * Reject a proposal to add a staker to the staking set.
//		** Increase the timestamp to the would-be staker's start time
//      ** This should be the initial preference if staker.startTime <= Wall clock
//         time
//  - A rejection block must always be preceded on the chain by the proposal block whose
//	  proposal is being rejected

var (
	errInvalidBlockType = errors.New("invalid block type")
)

// Block is the common interface that all staking blocks must have
type Block interface {
	snowman.Block

	// initialize this block's non-serialized fields.
	// This method should be called when a block is unmarshaled from bytes.
	// [vm] is the vm the block exists in
	// [bytes] is the byte representation of this block
	initialize(vm *VM, bytes []byte) error

	// This block's height
	Height() uint64

	conflicts(ids.Set) bool

	// parentBlock returns the parent block, similarly to Parent. However, it
	// provides the more specific staking.Block interface.
	parentBlock() Block

	// addChild notifies this block that it has a child block building on
	// its database. When this block commits its database, it should set the
	// child's database to the former's underlying database instance. This ensures that
	// the database versions do not recurse the length of the chain.
	addChild(Block)

	// free all the references of this block from the vm's memory
	free()

	// Set the database underlying the block's versiondb's to [db]
	setBaseDatabase(db database.Database)
}

// A decision block (either Commit, Abort, or DecisionBlock.) represents a
// decision to either commit (accept) or abort (reject) the changes specified in
// its parent, if its parent is a proposal. Otherwise, the changes are committed
// immediately.
type decision interface {
	// This function should only be called after Verify is called.
	// returns a database that contains the state of the chain if this block is
	// accepted.
	onAccept() database.Database
}

// CommonBlock contains the fields common to all blocks of the Platform Chain
type CommonBlock struct {
	*core.Block `serialize:"true"`
	vm          *VM

	// This block's children
	children []Block
}

// Reject implements the snowman.Block interface
func (cb *CommonBlock) Reject() error {
	defer cb.free() // remove this block from memory

	return cb.Block.Reject()
}

// free removes this block from memory
func (cb *CommonBlock) free() {
	delete(cb.vm.currentBlocks, cb.ID().Key())
	cb.children = nil
}

// Reject implements the snowman.Block interface
func (cb *CommonBlock) conflicts(s ids.Set) bool {
	if cb.Status() == choices.Accepted {
		return false
	}
	return cb.parentBlock().conflicts(s)
}

// parentBlock returns this block's parent
func (cb *CommonBlock) parentBlock() Block {
	// Get the parent from database
	parentID := cb.ParentID()
	parent, err := cb.vm.getBlock(parentID)
	if err != nil {
		return nil
	}
	return parent.(Block)
}

// addChild adds [child] as a child of this block
func (cb *CommonBlock) addChild(child Block) { cb.children = append(cb.children, child) }

// CommonDecisionBlock contains the fields and methods common to all decision blocks
type CommonDecisionBlock struct {
	CommonBlock `serialize:"true"`

	// state of the chain if this block is accepted
	onAcceptDB *versiondb.Database

	// to be executed if this block is accepted
	onAcceptFunc func() error
}

// initialize this block
func (cdb *CommonDecisionBlock) initialize(vm *VM, bytes []byte) error {
	cdb.vm = vm
	cdb.Block.Initialize(bytes, vm.SnowmanVM)
	return nil
}

// setBaseDatabase sets this block's base database to [db]
func (cdb *CommonDecisionBlock) setBaseDatabase(db database.Database) {
	if err := cdb.onAcceptDB.SetDatabase(db); err != nil {
		cdb.vm.Ctx.Log.Error("problem while setting base database: %s", err)
	}
}

// onAccept returns:
// 1) The state of the chain if this block is accepted
// 2) The function to execute if this block is accepted
func (cdb *CommonDecisionBlock) onAccept() database.Database {
	if cdb.Status().Decided() {
		return cdb.vm.DB
	}
	return cdb.onAcceptDB
}

// SingleDecisionBlock contains the accept for standalone decision blocks
type SingleDecisionBlock struct {
	CommonDecisionBlock `serialize:"true"`
}

// Accept implements the snowman.Block interface
func (sdb *SingleDecisionBlock) Accept() error {
	sdb.VM.Ctx.Log.Verbo("Accepting block with ID %s", sdb.ID())

	if err := sdb.CommonBlock.Accept(); err != nil {
		return err
	}

	// Update the state of the chain in the database
	if err := sdb.onAcceptDB.Commit(); err != nil {
		sdb.vm.Ctx.Log.Warn("unable to commit onAcceptDB")
		return err
	}
	if err := sdb.vm.DB.Commit(); err != nil {
		sdb.vm.Ctx.Log.Warn("unable to commit vm's DB")
		return err
	}

	for _, child := range sdb.children {
		child.setBaseDatabase(sdb.vm.DB)
	}
	if sdb.onAcceptFunc != nil {
		if err := sdb.onAcceptFunc(); err != nil {
			return err
		}
	}

	sdb.free()
	return nil
}

// DoubleDecisionBlock contains the accept for a pair of blocks
type DoubleDecisionBlock struct {
	CommonDecisionBlock `serialize:"true"`
}

// Accept implements the snowman.Block interface
func (ddb *DoubleDecisionBlock) Accept() error {
	ddb.VM.Ctx.Log.Verbo("Accepting block with ID %s", ddb.ID())

	parent, ok := ddb.parentBlock().(*ProposalBlock)
	if !ok {
		ddb.vm.Ctx.Log.Error("double decision block should only follow a proposal block")
		return errInvalidBlockType
	}

	if err := parent.CommonBlock.Accept(); err != nil {
		return err
	}

	if err := ddb.CommonBlock.Accept(); err != nil {
		return err
	}

	// Update the state of the chain in the database
	if err := ddb.onAcceptDB.Commit(); err != nil {
		ddb.vm.Ctx.Log.Warn("unable to commit onAcceptDB: %s", err)
		return err
	}
	if err := ddb.vm.DB.Commit(); err != nil {
		ddb.vm.Ctx.Log.Warn("unable to commit vm's DB: %s", err)
		return err
	}

	for _, child := range ddb.children {
		child.setBaseDatabase(ddb.vm.DB)
	}
	if ddb.onAcceptFunc != nil {
		if err := ddb.onAcceptFunc(); err != nil {
			ddb.vm.Ctx.Log.Warn("error executing OnAcceptFunc(): %s", err)
			return err
		}
	}

	// remove this block and its parent from memory
	parent.free()
	ddb.free()
	return nil
}
