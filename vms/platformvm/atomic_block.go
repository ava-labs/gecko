// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"errors"
	"fmt"

	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/vms/components/core"
)

var (
	errConflictingParentTxs = errors.New("block contains a transaction that conflicts with a transaction in a parent block")
)

// AtomicBlock being accepted results in the transaction contained in the
// block to be accepted and committed to the chain.
type AtomicBlock struct {
	CommonDecisionBlock `serialize:"true"`

	Tx Tx `serialize:"true" json:"tx"`

	inputs ids.Set
}

// initialize this block
func (ab *AtomicBlock) initialize(vm *VM, bytes []byte) error {
	if err := ab.CommonDecisionBlock.initialize(vm, bytes); err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}
	unsignedBytes, err := vm.codec.Marshal(codecVersion, &ab.Tx.UnsignedTx)
	if err != nil {
		return fmt.Errorf("failed to marshal unsigned tx: %w", err)
	}
	signedBytes, err := ab.vm.codec.Marshal(codecVersion, &ab.Tx)
	if err != nil {
		return fmt.Errorf("failed to marshal tx: %w", err)
	}
	ab.Tx.Initialize(unsignedBytes, signedBytes)
	return nil
}

// Reject implements the snowman.Block interface
func (ab *AtomicBlock) conflicts(s ids.Set) bool {
	if ab.Status() == choices.Accepted {
		return false
	}
	if ab.inputs.Overlaps(s) {
		return true
	}
	return ab.parentBlock().conflicts(s)
}

// Verify this block performs a valid state transition.
//
// The parent block must be a proposal
//
// This function also sets onAcceptDB database if the verification passes.
func (ab *AtomicBlock) Verify() error {
	if err := ab.CommonDecisionBlock.Verify(); err != nil {
		if err := ab.Reject(); err != nil {
			ab.vm.Ctx.Log.Error("failed to reject atomic block %s due to %s", ab.ID(), err)
		}
		return err
	}

	tx, ok := ab.Tx.UnsignedTx.(UnsignedAtomicTx)
	if !ok {
		return errWrongTxType
	}
	ab.inputs = tx.InputUTXOs()

	parentBlock := ab.parentBlock()
	if parentBlock.conflicts(ab.inputs) {
		return errConflictingParentTxs
	}

	// AtomicBlock is not a modifier on a proposal block, so its parent must be
	// a decision.
	parent, ok := parentBlock.(decision)
	if !ok {
		return errInvalidBlockType
	}

	pdb := parent.onAccept()

	ab.onAcceptDB = versiondb.New(pdb)
	if err := tx.SemanticVerify(ab.vm, ab.onAcceptDB, &ab.Tx); err != nil {
		ab.vm.droppedTxCache.Put(ab.Tx.ID(), err.Error()) // cache tx as dropped
		return fmt.Errorf("tx %s failed semantic verification: %w", tx.ID(), err)
	}
	txBytes := ab.Tx.Bytes()
	if err := ab.vm.putTx(ab.onAcceptDB, ab.Tx.ID(), txBytes); err != nil {
		return fmt.Errorf("failed to put tx %s: %w", tx.ID(), err)
	} else if err := ab.vm.putStatus(ab.onAcceptDB, ab.Tx.ID(), Committed); err != nil {
		return fmt.Errorf("failed to put status of tx %s: %w", tx.ID(), err)
	}

	ab.vm.currentBlocks[ab.ID()] = ab
	ab.parentBlock().addChild(ab)
	return nil
}

// Accept implements the snowman.Block interface
func (ab *AtomicBlock) Accept() error {
	ab.vm.Ctx.Log.Verbo("Accepting Atomic Block %s at height %d with parent %s", ab.ID(), ab.Height(), ab.ParentID())

	tx, ok := ab.Tx.UnsignedTx.(UnsignedAtomicTx)
	if !ok {
		return errWrongTxType
	}

	if err := ab.CommonBlock.Accept(); err != nil {
		return fmt.Errorf("failed to accept CommonBlock of %s: %w", ab.ID(), err)
	}

	// Update the state of the chain in the database
	if err := ab.onAcceptDB.Commit(); err != nil {
		return fmt.Errorf("failed to commit onAcceptDB for block %s: %w", ab.ID(), err)
	}

	batch, err := ab.vm.DB.CommitBatch()
	if err != nil {
		return fmt.Errorf("failed to commit VM's database for block %s: %w", ab.ID(), err)
	}
	defer ab.vm.DB.Abort()

	if err := tx.Accept(ab.vm.Ctx, batch); err != nil {
		return fmt.Errorf("failed to atomically accept tx %s in block %s: %w", tx.ID(), ab.ID(), err)
	}

	for _, child := range ab.children {
		child.setBaseDatabase(ab.vm.DB)
	}
	if ab.onAcceptFunc != nil {
		if err := ab.onAcceptFunc(); err != nil {
			return fmt.Errorf("failed to execute onAcceptFunc of %s: %w", ab.ID(), err)
		}
	}

	ab.free()
	return nil
}

// Reject implements the snowman.Block interface
func (ab *AtomicBlock) Reject() error {
	ab.vm.Ctx.Log.Verbo("Rejecting Atomic Block %s at height %d with parent %s", ab.ID(), ab.Height(), ab.ParentID())

	if err := ab.vm.mempool.IssueTx(&ab.Tx); err != nil {
		ab.vm.Ctx.Log.Debug("failed to reissue tx %q due to: %s", ab.Tx.ID(), err)
	}
	return ab.CommonDecisionBlock.Reject()
}

// newAtomicBlock returns a new *AtomicBlock where the block's parent, a
// decision block, has ID [parentID].
func (vm *VM) newAtomicBlock(parentID ids.ID, height uint64, tx Tx) (*AtomicBlock, error) {
	ab := &AtomicBlock{
		CommonDecisionBlock: CommonDecisionBlock{
			CommonBlock: CommonBlock{
				Block: core.NewBlock(parentID, height),
				vm:    vm,
			},
		},
		Tx: tx,
	}

	// We serialize this block as a Block so that it can be deserialized into a
	// Block
	blk := Block(ab)
	bytes, err := Codec.Marshal(codecVersion, &blk)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block: %w", err)
	}
	ab.Block.Initialize(bytes, vm.SnowmanVM)
	return ab, nil
}
