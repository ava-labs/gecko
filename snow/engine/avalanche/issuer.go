// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avalanche

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm"
)

// issuer issues [vtx] into consensus after its dependencies are met.
type issuer struct {
	t                 *Transitive
	vtx               avalanche.Vertex
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
	validTxs := make([]snowstorm.Tx, 0, len(txs))
	for _, tx := range txs {
		if err := tx.Verify(); err != nil {
			i.t.Ctx.Log.Debug("Transaction %s failed verification due to %s", tx.ID(), err)
		} else {
			validTxs = append(validTxs, tx)
		}
	}

	// Some of the transactions weren't valid. Abandon this vertex.
	// Take the valid transactions and issue a new vertex with them.
	if len(validTxs) != len(txs) {
		i.t.Ctx.Log.Debug("Abandoning %s due to failed transaction verification", vtxID)
		if _, err := i.t.batch(validTxs, false /*=force*/, false /*=empty*/, false /*=limit*/); err != nil {
			i.t.errs.Add(err)
		}
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
		i.t.txBlocked.Fulfill(tx.ID())
	}

	// Issue a repoll
	i.t.repoll()
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
