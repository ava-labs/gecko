// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"errors"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/vms/components/missing"
)

var (
	errBlockNil = errors.New("block is nil")
	errRejected = errors.New("block is rejected")
)

// Block contains fields and methods common to block's in a Snowman blockchain.
// Block is meant to be a building-block (pun intended).
// When you write a VM, your blocks can (and should) embed a core.Block
// to take care of some bioler-plate code.
// Block's methods can be over-written by structs that embed this struct.
type Block struct {
	Metadata
	PrntID ids.ID `serialize:"true" json:"parentID"` // parent's ID
	Hght   uint64 `serialize:"true" json:"height"`   // This block's height. The genesis block is at height 0.
	VM     *SnowmanVM
}

// Initialize sets [b.bytes] to [bytes], sets [b.id] to hash([b.bytes])
// Checks if [b]'s status is already stored in state. If so, [b] gets that status.
// Otherwise [b]'s status is Unknown.
func (b *Block) Initialize(bytes []byte, vm *SnowmanVM) {
	b.VM = vm
	b.Metadata.Initialize(bytes)
	b.SetStatus(choices.Unknown) // don't set status until it is queried
}

// ParentID returns [b]'s parent's ID
func (b *Block) ParentID() ids.ID { return b.PrntID }

// Height returns this block's height. The genesis block has height 0.
func (b *Block) Height() uint64 { return b.Hght }

// ParentBlock returns [b]'s parent.
// If [b]'s parent can't be found, returns a missing.Block with the parent's ID.
func (b *Block) ParentBlock() snowman.Block {
	parentID := b.ParentID()
	parent, err := b.VM.GetBlock(parentID)
	if err != nil {
		return &missing.Block{BlkID: parentID}
	}
	return parent
}

// Parent returns the ID of [b]'s parent
func (b *Block) Parent() ids.ID {
	return b.ParentID()
}

// Accept sets this block's status to Accepted and sets lastAccepted to this
// block's ID and saves this info to b.vm.DB
// Recall that b.vm.DB.Commit() must be called to persist to the DB
func (b *Block) Accept() error {
	b.SetStatus(choices.Accepted) // Change state of this block
	blkID := b.ID()

	// Persist data
	if err := b.VM.State.PutStatus(b.VM.DB, blkID, choices.Accepted); err != nil {
		return err
	}
	if err := b.VM.State.PutLastAccepted(b.VM.DB, blkID); err != nil {
		return err
	}

	b.VM.LastAcceptedID = blkID // Change state of VM
	return nil
}

// Reject sets this block's status to Rejected and saves the status in state
// Recall that b.vm.DB.Commit() must be called to persist to the DB
func (b *Block) Reject() error {
	b.SetStatus(choices.Rejected)
	return b.VM.State.PutStatus(b.VM.DB, b.ID(), choices.Rejected)
}

// Status returns the status of this block
func (b *Block) Status() choices.Status {
	// See if [b]'s status field already has a value
	if status := b.Metadata.Status(); status != choices.Unknown {
		return status
	}
	// If not, check the state
	status := b.VM.State.GetStatus(b.VM.DB, b.ID())
	b.SetStatus(status)
	return status
}

// Verify returns:
// 1) true if the block is accepted
// 2) nil if this block is valid
func (b *Block) Verify() (bool, error) {
	if b == nil {
		return false, errBlockNil
	}

	// Check if [b] has already been accepted/rejected
	switch status := b.Status(); status {
	case choices.Accepted:
		return true, nil
	case choices.Rejected:
		return false, errRejected
	}
	return false, nil
}

// NewBlock returns a new *Block
func NewBlock(parentID ids.ID, height uint64) *Block {
	return &Block{PrntID: parentID, Hght: height}
}
