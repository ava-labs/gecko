// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/components/core"
)

// Commit being accepted results in the proposal of its parent (which must be a proposal block)
// being enacted.
type Commit struct {
	DoubleDecisionBlock `serialize:"true"`
}

// Verify this block performs a valid state transition.
//
// The parent block must either be a proposal
//
// This function also sets the onCommit databases if the verification passes.
func (c *Commit) Verify() error {
	if err := c.DoubleDecisionBlock.Verify(); err != nil {
		if err := c.Reject(); err != nil {
			c.vm.Ctx.Log.Error("failed to reject commit block %s due to %s", c.ID(), err)
		}
		return err
	}

	// the parent of an Commit block should always be a proposal
	parent, ok := c.parentBlock().(*ProposalBlock)
	if !ok {
		if err := c.Reject(); err != nil {
			c.vm.Ctx.Log.Error("failed to reject commit block %s due to %s", c.ID(), err)
		}
		return errInvalidBlockType
	}

	c.onAcceptDB, c.onAcceptFunc = parent.onCommit()

	c.vm.currentBlocks[c.ID()] = c
	c.parentBlock().addChild(c)
	return nil
}

// newCommitBlock returns a new *Commit block where the block's parent, a
// proposal block, has ID [parentID].
func (vm *VM) newCommitBlock(parentID ids.ID, height uint64) (*Commit, error) {
	commit := &Commit{
		DoubleDecisionBlock: DoubleDecisionBlock{
			CommonDecisionBlock: CommonDecisionBlock{
				CommonBlock: CommonBlock{
					Block: core.NewBlock(parentID, height),
					vm:    vm,
				},
			},
		},
	}

	// We serialize this block as a Block so that it can be deserialized into a
	// Block
	blk := Block(commit)
	bytes, err := Codec.Marshal(codecVersion, &blk)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal commit block: %w", err)
	}
	commit.Block.Initialize(bytes, vm.SnowmanVM)
	return commit, nil
}
