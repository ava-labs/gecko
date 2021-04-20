// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"fmt"

	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/components/core"
)

// StandardBlock being accepted results in the transactions contained in the
// block to be accepted and committed to the chain.
type StandardBlock struct {
	SingleDecisionBlock `serialize:"true"`

	Txs []*Tx `serialize:"true" json:"txs"`
}

// initialize this block
func (sb *StandardBlock) initialize(vm *VM, bytes []byte) error {
	if err := sb.SingleDecisionBlock.initialize(vm, bytes); err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}
	for _, tx := range sb.Txs {
		if err := tx.Sign(vm.codec, nil); err != nil {
			return fmt.Errorf("failed to sign block: %w", err)
		}
	}
	return nil
}

// Verify this block performs a valid state transition.
//
// The parent block must be a proposal
//
// This function also sets onAcceptDB database if the verification passes.
func (sb *StandardBlock) Verify() error {
	if err := sb.SingleDecisionBlock.Verify(); err != nil {
		if err := sb.Reject(); err != nil {
			sb.vm.Ctx.Log.Error("failed to reject standard block %s due to %s", sb.ID(), err)
		}
		return err
	}

	parentBlock := sb.parentBlock()
	// StandardBlock is not a modifier on a proposal block, so its parent must
	// be a decision.
	parent, ok := parentBlock.(decision)
	if !ok {
		if err := sb.Reject(); err != nil {
			sb.vm.Ctx.Log.Error("failed to reject standard block %s due to %s", sb.ID(), err)
		}
		return errInvalidBlockType
	}

	pdb := parent.onAccept()

	sb.onAcceptDB = versiondb.New(pdb)
	funcs := make([]func() error, 0, len(sb.Txs))
	for _, tx := range sb.Txs {
		utx, ok := tx.UnsignedTx.(UnsignedDecisionTx)
		if !ok {
			return errWrongTxType
		}
		txID := tx.ID()
		onAccept, err := utx.SemanticVerify(sb.vm, sb.onAcceptDB, tx)
		if err != nil {
			sb.vm.droppedTxCache.Put(txID, err.Error()) // cache tx as dropped
			if err := sb.Reject(); err != nil {
				sb.vm.Ctx.Log.Error("failed to reject standard block %s due to %s", sb.ID(), err)
			}
			return err
		}
		if txBytes, err := sb.vm.codec.Marshal(codecVersion, tx); err != nil {
			return fmt.Errorf("failed to marshal tx %s: %w", txID, err)
		} else if err := sb.vm.putTx(sb.onAcceptDB, txID, txBytes); err != nil {
			return fmt.Errorf("failed to put tx %s: %w", txID, err)
		} else if err := sb.vm.putStatus(sb.onAcceptDB, txID, Committed); err != nil {
			return fmt.Errorf("failed to put tx %s status: %w", txID, err)
		} else if onAccept != nil {
			funcs = append(funcs, onAccept)
		}
	}

	if numFuncs := len(funcs); numFuncs == 1 {
		sb.onAcceptFunc = funcs[0]
	} else if numFuncs > 1 {
		sb.onAcceptFunc = func() error {
			for _, f := range funcs {
				if err := f(); err != nil {
					return fmt.Errorf("failed to execute onAcceptFunc: %w", err)
				}
			}
			return nil
		}
	}

	sb.vm.currentBlocks[sb.ID()] = sb
	sb.parentBlock().addChild(sb)
	return nil
}

// Reject implements the snowman.Block interface
func (sb *StandardBlock) Reject() error {
	sb.vm.Ctx.Log.Verbo("Rejecting Standard Block %s at height %d with parent %s", sb.ID(), sb.Height(), sb.ParentID())

	for _, tx := range sb.Txs {
		if err := sb.vm.mempool.IssueTx(tx); err != nil {
			sb.vm.Ctx.Log.Debug("failed to reissue tx %q due to: %s", tx.ID(), err)
		}
	}
	return sb.SingleDecisionBlock.Reject()
}

// newStandardBlock returns a new *StandardBlock where the block's parent, a
// decision block, has ID [parentID].
func (vm *VM) newStandardBlock(parentID ids.ID, height uint64, txs []*Tx) (*StandardBlock, error) {
	sb := &StandardBlock{
		SingleDecisionBlock: SingleDecisionBlock{
			CommonDecisionBlock: CommonDecisionBlock{
				CommonBlock: CommonBlock{
					Block: core.NewBlock(parentID, height),
					vm:    vm,
				},
			},
		},
		Txs: txs,
	}

	// We serialize this block as a Block so that it can be deserialized into a
	// Block
	blk := Block(sb)
	bytes, err := vm.codec.Marshal(codecVersion, &blk)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block: %w", err)
	}
	sb.Block.Initialize(bytes, vm.SnowmanVM)
	return sb, nil
}
