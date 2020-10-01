// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowman

import (
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/vms/components/missing"
)

// Voter records chits received from [vdr] once its dependencies are met.
type voter struct {
	t         *Transitive
	vdr       ids.ShortID
	requestID uint32
	response  ids.ID
	deps      ids.Set
}

func (v *voter) Dependencies() ids.Set { return v.deps }

// Mark that a dependency has been met.
func (v *voter) Fulfill(id ids.ID) {
	v.deps.Remove(id)
	v.Update()
}

// Abandon this attempt to record chits.
func (v *voter) Abandon(id ids.ID) { v.Fulfill(id) }

func (v *voter) Update() {
	if v.deps.Len() != 0 || v.t.errs.Errored() {
		return
	}

	results := ids.Bag{}
	finished := false
	if v.response.IsZero() {
		results, finished = v.t.polls.Drop(v.requestID, v.vdr)
	} else {
		results, finished = v.t.polls.Vote(v.requestID, v.vdr, v.response)
	}

	if !finished {
		return
	}

	// To prevent any potential deadlocks with un-disclosed dependencies, votes
	// must be bubbled to the nearest valid block
	results = v.bubbleVotes(results)

	v.t.Ctx.Log.Debug("Finishing poll [%d] with:\n%s", v.requestID, &results)
	accepted, rejected, err := v.t.Consensus.RecordPoll(results)
	if err != nil {
		v.t.errs.Add(err)
		return
	}
	// Unpin accepted and rejected blocks from memory
	for _, acceptedID := range accepted.List() {
		acceptedIDKey := acceptedID.Key()
		v.t.decidedCache.Put(acceptedID, nil)
		v.t.droppedCache.Evict(acceptedID)       // Remove from dropped cache, if it was in there
		blk, ok := v.t.processing[acceptedIDKey] // The block we're accepting
		if !ok {
			err := fmt.Errorf("couldn't find accepted block %s in processing list. Block not saved to VM's database", acceptedID)
			v.t.errs.Add(err)
			return
		} else if err := v.t.VM.SaveBlock(blk); err != nil { // Save accepted block in VM's database
			err := fmt.Errorf("couldn't save block %s to VM's database: %s", acceptedID, err)
			v.t.errs.Add(err)
			return
		}
		delete(v.t.processing, acceptedID.Key())
	}
	for _, rejectedID := range rejected.List() {
		v.t.decidedCache.Put(rejectedID, nil)
		v.t.droppedCache.Evict(rejectedID) // Remove from dropped cache, if it was in there
		delete(v.t.processing, rejectedID.Key())
	}
	v.t.numProcessing.Set(float64(len(v.t.processing)))

	v.t.VM.SetPreference(v.t.Consensus.Preference())

	if v.t.Consensus.Finalized() {
		v.t.Ctx.Log.Debug("Snowman engine can quiesce")
		return
	}

	v.t.Ctx.Log.Debug("Snowman engine can't quiesce")
	v.t.repoll()
}

// We assume that the number of blocks that require votes to be bubbled
// is sufficiently small to fit into the dropped cache.
func (v *voter) bubbleVotes(votes ids.Bag) ids.Bag {
	bubbledVotes := ids.Bag{}
	var err error
	for _, vote := range votes.List() {
		count := votes.Count(vote)
		blk, ok := v.t.processing[vote.Key()] // Check if the block is non-dropped and processing
		if !ok {
			blkIntf, ok := v.t.droppedCache.Get(vote) // See if the block was dropped
			if ok {                                   // The block was dropped before but we still have it.
				blk = blkIntf.(snowman.Block)
			} else { // Couldn't find the block that this vote is for. Skip it.
				continue
			}
		}

		for blk.Status().Fetched() && !v.t.Consensus.Issued(blk) {
			blkID := blk.Parent()
			blk, err = v.t.GetBlock(blkID)
			if err != nil {
				blk = &missing.Block{BlkID: blkID}
			}
		}

		if !blk.Status().Decided() && v.t.Consensus.Issued(blk) {
			bubbledVotes.AddCount(blk.ID(), count)
		}
	}
	return bubbledVotes
}
