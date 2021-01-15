// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avalanche

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/vertex"
)

// Voter records chits received from [vdr] once its dependencies are met.
type voter struct {
	t         *Transitive
	vdr       ids.ShortID
	requestID uint32
	response  []ids.ID
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

type vtx struct {
	trs []conflicts.Transition
	rts [][]ids.ID
}

func (v *voter) Update() {
	if v.deps.Len() != 0 || v.t.errs.Errored() {
		return
	}

	results, finished := v.t.polls.Vote(v.requestID, v.vdr, v.response)
	if !finished {
		return
	}
	results, err := v.bubbleVotes(results)
	if err != nil {
		v.t.errs.Add(err)
		return
	}

	v.t.Ctx.Log.Debug("Finishing poll with:\n%s", &results)
	if err := v.t.Consensus.RecordPoll(results); err != nil {
		v.t.errs.Add(err)
		return
	}

	epochs := make(map[uint32]vtx, 2)
	orphans := v.t.Consensus.Orphans()
	for orphanID := range orphans {
		tx, err := v.t.Consensus.GetTx(orphanID)
		if err != nil {
			v.t.Ctx.Log.Error("Failed to fetch %s during attempted re-issuance",
				orphanID)
			continue
		}
		epoch := tx.Epoch()
		vt, exists := epochs[epoch]
		if !exists {
			vt.trs = make([]conflicts.Transition, 0, orphans.Len())
			vt.rts = make([][]ids.ID, 0, orphans.Len())
		}
		if rts := tx.Restrictions(); len(rts) > 0 {
			vt.rts = append(vt.rts, rts)
		} else {
			vt.trs = append(vt.trs, tx.Transition())
		}
		epochs[epoch] = vt
	}
	for epoch, vt := range epochs {
		v.t.Ctx.Log.Debug("Re-issuing %d transitions and %d restriction groups in to epoch %d",
			len(vt.trs), len(vt.rts), epoch)
		if err := v.t.batch(epoch, vt.trs, vt.rts, true /*=force*/, false /*empty*/, false /*=updatedEpoch*/); err != nil {
			v.t.errs.Add(err)
			return
		}
	}

	if v.t.Consensus.Finalized() {
		v.t.Ctx.Log.Debug("Avalanche engine can quiesce")
		return
	}

	v.t.Ctx.Log.Debug("Avalanche engine can't quiesce")
	v.t.errs.Add(v.t.repoll())
}

func (v *voter) bubbleVotes(votes ids.UniqueBag) (ids.UniqueBag, error) {
	bubbledVotes := ids.UniqueBag{}
	vertexHeap := vertex.NewHeap()
	for vote := range votes {
		vtx, err := v.t.Manager.Get(vote)
		if err != nil {
			continue
		}
		vertexHeap.Push(vtx)
	}

	for vertexHeap.Len() > 0 {
		vtx := vertexHeap.Pop()
		vtxID := vtx.ID()
		set := votes.GetSet(vtxID)
		status := vtx.Status()

		if !status.Fetched() {
			v.t.Ctx.Log.Verbo("Dropping %d vote(s) for %s because the vertex is unknown",
				set.Len(), vtxID)
			bubbledVotes.RemoveSet(vtx.ID())
			continue
		}

		if status.Decided() {
			v.t.Ctx.Log.Verbo("Dropping %d vote(s) for %s because the vertex is decided",
				set.Len(), vtxID)
			bubbledVotes.RemoveSet(vtx.ID())
			continue
		}

		if v.t.Consensus.VertexIssued(vtx) {
			v.t.Ctx.Log.Verbo("Applying %d vote(s) for %s", set.Len(), vtx.ID())
			bubbledVotes.UnionSet(vtx.ID(), set)
		} else {
			v.t.Ctx.Log.Verbo("Bubbling %d vote(s) for %s because the vertex isn't issued",
				set.Len(), vtx.ID())
			bubbledVotes.RemoveSet(vtx.ID()) // Remove votes for this vertex because it hasn't been issued

			parents, err := vtx.Parents()
			if err != nil {
				return bubbledVotes, err
			}
			for _, parentVtx := range parents {
				bubbledVotes.UnionSet(parentVtx.ID(), set)
				vertexHeap.Push(parentVtx)
			}
		}
	}

	return bubbledVotes, nil
}
