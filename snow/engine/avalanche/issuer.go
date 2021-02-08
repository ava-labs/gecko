// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avalanche

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
)

// issuer issues [vtx] into consensus after its dependencies are met.
type issuer struct {
	t *Transitive
	// [vtx] attempting to be issued to consensus by this issuer
	vtx avalanche.Vertex
	// if [updatedEpoch] is true, any transactions that fail verification
	// will be restricted into the current epoch. Otherwise, transactions
	// that fail verification will simply be ignored.
	updatedEpoch      bool
	issued, abandoned bool
	// [vtxDeps] is the set of vertices that must be issued
	// before [vtx] can be issued to consensus.
	vtxDeps ids.Set

	// Note:
	// [trIssuer] must return all unfulfilled dependencies (including those
	// that have been issued into the epoch of [vtx]) in order to be notified
	// if any of these unfulfilled dependencies are accepted in a future epoch
	// such that we can abandon [vtx].
	// [trDeps] and [unfulfilledDeps] are kept as two separate
	// sets so that trIssuer's Dependency call is O(1) instead of requiring
	// the union of the set of fulfilled and unfulfilled transitions.

	// [trDeps] is the set of transitions that the transactions
	// in [vtx] depend on and must be issued to consensus in the
	// same epoch as [vtx] or accepted in an epoch <= [vtx].
	trDeps ids.Set
	// [unfulfilledDeps] is the set of transitions in [trDeps]
	// that have not yet fulfilled the above requirements.
	unfulfilledDeps ids.Set
}

// Register that a vertex we were waiting on has been issued to consensus.
func (i *issuer) FulfillVtx(id ids.ID) {
	i.vtxDeps.Remove(id)
	i.Update()
}

// Register that a transition we were waiting on has been issued to consensus.
func (i *issuer) FulfillTr(id ids.ID) {
	i.unfulfilledDeps.Remove(id)
	i.Update()
}

// Abandon this attempt to issue
func (i *issuer) Abandon() {
	if !i.abandoned {
		vtxID := i.vtx.ID()
		i.t.pending.Remove(vtxID)
		i.abandoned = true
		i.t.vtxBlocked.Abandon(vtxID) // Inform vertices waiting on this vtx that it won't be issued
	}
}

// Issue the poll when all dependencies are met
func (i *issuer) Update() {
	if i.abandoned || i.issued || i.vtxDeps.Len() != 0 || i.unfulfilledDeps.Len() != 0 || i.t.Consensus.VertexIssued(i.vtx) || i.t.errs.Errored() {
		return
	}
	// All dependencies have been met
	i.issued = true

	vtxID := i.vtx.ID()
	i.t.pending.Remove(vtxID) // Remove from set of vertices waiting to be issued.

	// Make sure the transactions in this vertex are valid
	txs, err := i.vtx.Txs()
	if err != nil {
		i.t.errs.Add(err)
		return
	}
	validTransitions := make([]conflicts.Transition, 0, len(txs))
	invalidTransitions := []ids.ID(nil)
	if i.updatedEpoch {
		invalidTransitions = make([]ids.ID, 0, len(txs))
	}
	unissuedTransitions := make([]conflicts.Transition, 0, len(txs))
	for _, tx := range txs {
		transition := tx.Transition()
		if err := tx.Verify(); err != nil {
			i.t.Ctx.Log.Debug("Transaction %s failed verification due to %s", tx.ID(), err)
			if i.updatedEpoch {
				invalidTransitions = append(invalidTransitions, transition.ID())
			}
			continue
		}

		validTransitions = append(validTransitions, transition)
		if !i.t.Consensus.TransitionProcessing(transition.ID()) {
			unissuedTransitions = append(unissuedTransitions, transition)
		}
	}

	epoch, err := i.vtx.Epoch()
	if err != nil {
		i.t.errs.Add(err)
		return
	}

	// Some of the transactions weren't valid. Abandon this vertex.
	// Take the valid transactions and issue a new vertex with them.
	if len(validTransitions) != len(txs) {
		i.t.Ctx.Log.Debug("Abandoning %s due to failed transaction verification", vtxID)
		if i.updatedEpoch {
			err = i.t.batch(
				epoch,
				validTransitions,
				[][]ids.ID{invalidTransitions},
				false, // force
				false, // empty
				true,  // updatedEpoch
			)
		} else {
			err = i.t.batch(
				epoch,
				validTransitions,
				nil,   // restrictions
				false, // force
				false, // empty
				true,  // updatedEpoch
			)
		}
		i.t.errs.Add(err)
		i.t.abandonedVertices = true
		i.t.vtxBlocked.Abandon(vtxID)
		return
	}

	currentEpoch := i.t.Ctx.Epoch()
	// Make sure that the first time these transitions are issued, they are
	// being issued into the current epoch. This enforces that nodes will prefer
	// their current epoch
	if epoch != currentEpoch && len(unissuedTransitions) > 0 {
		i.t.Ctx.Log.Debug("Reissuing transitions from epoch %d into epoch %d", epoch, currentEpoch)
		if err := i.t.batch(
			currentEpoch,
			unissuedTransitions,
			nil,   // restrictions
			true,  // force
			false, // empty
			true,  // updatedEpoch
		); err != nil {
			i.t.errs.Add(err)
			return
		}
	}
	if epoch > currentEpoch {
		i.t.Ctx.Log.Debug("Dropping vertex from future epoch:\n%s", vtxID)
		i.t.abandonedVertices = true
		i.t.vtxBlocked.Abandon(vtxID)
		return
	}

	i.t.Ctx.Log.Verbo("Adding vertex to consensus:\n%s", i.vtx)

	// Add this vertex to consensus.
	if err := i.t.Consensus.Add(i.vtx); err != nil {
		i.t.errs.Add(err)
		return
	}

	// Issue a poll for this vertex.
	p := i.t.Consensus.Parameters()
	vdrs, err := i.t.Validators.Sample(p.K) // Validators to sample

	vdrBag := ids.ShortBag{} // Validators to sample repr. as a set
	for _, vdr := range vdrs {
		vdrBag.Add(vdr.ID())
	}

	vdrSet := ids.ShortSet{}
	vdrSet.Add(vdrBag.List()...)

	i.t.RequestID++
	if err == nil && i.t.polls.Add(i.t.RequestID, vdrBag) {
		i.t.Sender.PushQuery(vdrSet, i.t.RequestID, vtxID, i.vtx.Bytes())
	} else if err != nil {
		i.t.Ctx.Log.Error("Query for %s was dropped due to an insufficient number of validators", vtxID)
	}

	// Notify vertices waiting on this one that it (and its transitions) have been issued.
	i.t.vtxBlocked.Fulfill(vtxID)
	for _, tx := range txs {
		trID := tx.Transition().ID()
		delete(i.t.missingTransitions[epoch], trID)
		if len(i.t.missingTransitions[epoch]) == 0 {
			delete(i.t.missingTransitions, epoch)
		}
		i.t.trBlocked.markIssued(trID, epoch)
	}
	i.t.updateMissingTransitions()

	// Issue a repoll
	i.t.errs.Add(i.t.repoll())
}

type vtxIssuer struct{ i *issuer }

func (vi *vtxIssuer) Dependencies() ids.Set { return vi.i.vtxDeps }
func (vi *vtxIssuer) Fulfill(id ids.ID)     { vi.i.FulfillVtx(id) }
func (vi *vtxIssuer) Abandon(ids.ID)        { vi.i.Abandon() }
func (vi *vtxIssuer) Update()               { vi.i.Update() }

type trIssuer struct{ i *issuer }

func (ti *trIssuer) Dependencies() ids.Set { return ti.i.trDeps }
func (ti *trIssuer) Fulfill(id ids.ID)     { ti.i.FulfillTr(id) }
func (ti *trIssuer) Abandon(ids.ID)        { ti.i.Abandon() }
func (ti *trIssuer) Update()               { ti.i.Update() }
