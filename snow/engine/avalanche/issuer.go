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
	t                 *Transitive
	vtx               avalanche.Vertex
	updatedEpoch      bool
	issued, abandoned bool
	vtxDeps, txDeps   ids.Set
}

// Register that a vertex we were waiting on has been issued to consensus.
func (i *issuer) FulfillVtx(id ids.ID) {
	i.vtxDeps.Remove(id)
	i.Update()
}

// Register that a transaction we were waiting on has been issued to consensus.
func (i *issuer) FulfillTx(id ids.ID) {
	i.txDeps.Remove(id)
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
	if i.abandoned || i.issued || i.vtxDeps.Len() != 0 || i.txDeps.Len() != 0 || i.t.Consensus.VertexIssued(i.vtx) || i.t.errs.Errored() {
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
		err = i.t.batch(
			epoch,
			validTransitions,
			[][]ids.ID{invalidTransitions},
			false, // force
			false, // empty
			true,  // updatedEpoch
		)
		i.t.errs.Add(err)
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

	// Notify vertices waiting on this one that it (and its transactions) have been issued.
	i.t.vtxBlocked.Fulfill(vtxID)
	for _, tx := range txs {
		i.t.txBlocked.Fulfill(tx.Transition().ID())
	}

	// Issue a repoll
	i.t.errs.Add(i.t.repoll())
}

type vtxIssuer struct{ i *issuer }

func (vi *vtxIssuer) Dependencies() ids.Set { return vi.i.vtxDeps }
func (vi *vtxIssuer) Fulfill(id ids.ID)     { vi.i.FulfillVtx(id) }
func (vi *vtxIssuer) Abandon(ids.ID)        { vi.i.Abandon() }
func (vi *vtxIssuer) Update()               { vi.i.Update() }

type txIssuer struct{ i *issuer }

func (ti *txIssuer) Dependencies() ids.Set { return ti.i.txDeps }
func (ti *txIssuer) Fulfill(id ids.ID)     { ti.i.FulfillTx(id) }
func (ti *txIssuer) Abandon(ids.ID)        { ti.i.Abandon() }
func (ti *txIssuer) Update()               { ti.i.Update() }
