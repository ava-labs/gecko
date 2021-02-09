// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowstorm

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/utils/formatting"

	sbcon "github.com/ava-labs/avalanchego/snow/consensus/snowball"
)

// DirectedFactory implements Factory by returning a directed struct
type DirectedFactory struct{}

// New implements Factory
func (DirectedFactory) New() Consensus { return &Directed{} }

// Directed is an implementation of a multi-color, non-transitive, snowball
// instance
type Directed struct {
	// metrics that describe this consensus instance
	metrics

	// context that this consensus instance is executing in
	ctx *snow.Context

	// conflicts that this consensus instance is using to determine the shape of
	// the conflict graph
	conflicts Conflicts

	// params describes how this instance was parameterized
	params sbcon.Parameters

	// each element of preferences is the ID of a transaction that is preferred
	preferences ids.Set

	// each element of virtuous is the ID of a transaction that is virtuous
	virtuous ids.Set

	// each element is in the virtuous set and is still being voted on
	virtuousVoting ids.Set

	// number of times RecordPoll has been called
	currentVote int

	// Key: Transaction ID
	// Value: Node that represents this transaction in the conflict graph
	txs map[ids.ID]*directedTx
}

type directedTx struct {
	snowball

	// pendingAccept identifies if this transaction has been marked as accepted
	// once its transitive dependencies have also been accepted
	pendingAccept bool

	// tx is the actual transaction this node represents
	tx choices.Decidable

	// ins is the set of txIDs that this tx conflicts with that are less
	// preferred than this tx
	ins ids.Set

	// outs is the set of txIDs that this tx conflicts with that are more
	// preferred than this tx
	outs ids.Set
}

// Initialize implements the Consensus interface
func (dg *Directed) Initialize(
	ctx *snow.Context,
	conflicts Conflicts,
	params sbcon.Parameters,
) error {
	dg.ctx = ctx
	dg.conflicts = conflicts
	dg.params = params
	dg.txs = make(map[ids.ID]*directedTx)

	if err := dg.metrics.Initialize(params.Namespace, params.Metrics); err != nil {
		return fmt.Errorf("failed to initialize metrics: %w", err)
	}
	return params.Valid()
}

// Parameters implements the Snowstorm interface
func (dg *Directed) Parameters() sbcon.Parameters { return dg.params }

// Virtuous implements the ConflictGraph interface
func (dg *Directed) Virtuous() ids.Set { return dg.virtuous }

// Preferences implements the ConflictGraph interface
func (dg *Directed) Preferences() ids.Set { return dg.preferences }

// Quiesce implements the ConflictGraph interface
func (dg *Directed) Quiesce() bool {
	numVirtuous := dg.virtuousVoting.Len()
	dg.ctx.Log.Verbo("Conflict graph has %d voting virtuous transactions",
		numVirtuous)
	return numVirtuous == 0
}

// Finalized implements the ConflictGraph interface
func (dg *Directed) Finalized() bool {
	numPreferences := dg.preferences.Len()
	dg.ctx.Log.Verbo("Conflict graph has %d preferred transactions",
		numPreferences)
	return numPreferences == 0
}

// IsVirtuous implements the Consensus interface
func (dg *Directed) IsVirtuous(tx choices.Decidable) (bool, error) {
	// If the tx is currently processing, we should just return if it is
	// registered as rogue or not.
	if node, exists := dg.txs[tx.ID()]; exists {
		return !node.rogue, nil
	}

	return dg.conflicts.IsVirtuous(tx)
}

// Conflicts implements the Consensus interface
func (dg *Directed) Conflicts(tx choices.Decidable) (ids.Set, error) {
	if node, exists := dg.txs[tx.ID()]; exists {
		// If the tx is currently processing, the conflicting txs are just the
		// union of the inbound conflicts and the outbound conflicts.
		var conflicts ids.Set
		// Avoid performing a memory allocation since the set is normally empty.
		if numConflicts := node.ins.Len() + node.outs.Len(); numConflicts > 0 {
			conflicts = make(ids.Set, numConflicts)
			conflicts.Union(node.ins)
			conflicts.Union(node.outs)
		}
		return conflicts, nil
	}

	// If the tx isn't currently processing, the conflicting txs are the
	// union of all the txs that spend an input that this tx spends.
	txConflicts, err := dg.conflicts.Conflicts(tx)
	if err != nil {
		return nil, err
	}

	conflicts := make(ids.Set, len(txConflicts))
	for _, conflict := range txConflicts {
		conflicts.Add(conflict.ID())
	}
	return conflicts, nil
}

// Issued implements the Consensus interface
func (dg *Directed) Issued(tx choices.Decidable) bool {
	// If the tx is either Accepted or Rejected, then it must have been issued
	// previously.
	if tx.Status().Decided() {
		return true
	}

	// If the tx is currently processing, then it must have been issued.
	_, ok := dg.txs[tx.ID()]
	return ok
}

// Add implements the Consensus interface
func (dg *Directed) Add(tx choices.Decidable) error {
	if dg.Issued(tx) {
		// If the tx was previously inserted, it shouldn't be re-inserted.
		return nil
	}

	conflicts, err := dg.conflicts.Conflicts(tx)
	if err != nil {
		return err
	}

	txID := tx.ID()

	// Notify the metrics that this transaction is being issued.
	dg.metrics.Issued(txID)

	txNode := &directedTx{tx: tx}
	for _, conflict := range conflicts {
		conflictID := conflict.ID()

		// This conflicting tx can't be virtuous anymore. So, we attempt to
		// remove it from all of the virtuous sets.
		dg.virtuous.Remove(conflictID)
		dg.virtuousVoting.Remove(conflictID)

		// The conflicting tx must be preferred over this tx. We know this
		// because this tx currently has a bias of 0 and the tie goes to the tx
		// whose bias was updated first.
		txNode.outs.Add(conflictID)
		txNode.rogue = true

		conflictNode := dg.txs[conflictID]
		conflictNode.ins.Add(txID)
		conflictNode.rogue = true
	}

	if !txNode.rogue {
		// If this tx is currently virtuous, add it to the virtuous sets
		dg.virtuous.Add(txID)
		dg.virtuousVoting.Add(txID)

		// If a tx is virtuous, it must be preferred.
		dg.preferences.Add(txID)
	}

	// Add this tx to the set of currently processing txs
	dg.txs[txID] = txNode
	return dg.conflicts.Add(tx)
}

// RecordPoll implements the Consensus interface
func (dg *Directed) RecordPoll(votes ids.Bag) (bool, error) {
	// Increase the vote ID. This is only updated here and is used to reset the
	// confidence values of transactions lazily.
	dg.currentVote++

	// We only want to iterate over txs that received alpha votes
	votes.SetThreshold(dg.params.Alpha)

	// Get the set of IDs that meet this alpha threshold
	metThreshold := votes.Threshold()

	// This flag tracks if the Avalanche instance needs to recompute its
	// frontiers. Frontiers only need to be recalculated if preferences change
	// or if a tx was accepted.
	changed := false

	// Update the confidence values based on the provided votes.
	for txID := range metThreshold {
		// Get the node this tx represents
		txNode, exist := dg.txs[txID]
		if !exist {
			// This tx may have already been decided because of tx dependencies.
			// If this is the case, we can just drop the vote.
			continue
		}

		// Update the confidence values.
		txNode.RecordSuccessfulPoll(dg.currentVote)

		// If the tx can be accepted, then we should notify the conflict manager
		// to defer its acceptance until its dependencies are decided. If this
		// tx was already marked to be accepted, we shouldn't register it again.
		if !txNode.pendingAccept &&
			txNode.Finalized(dg.params.BetaVirtuous, dg.params.BetaRogue) {
			// Mark that this tx is pending acceptance so acceptance is only
			// registered once.
			txNode.pendingAccept = true

			// This tx is no longer being voted on, so we remove it from the
			// voting set. This ensures that virtuous txs built on top of rogue
			// txs don't force the node to treat the rogue tx as virtuous.
			dg.virtuousVoting.Remove(txID)

			// Mark this transaction as being confitionally accepted
			dg.conflicts.Accept(txID)
		}

		// Make sure that edges are directed correctly.
		for conflictID := range txNode.outs {
			conflict := dg.txs[conflictID]
			if txNode.numSuccessfulPolls <= conflict.numSuccessfulPolls {
				// The edge is already pointing in the correct direction.
				continue
			}

			// Change the edge direction according to the conflict tx
			conflict.ins.Remove(txID)
			conflict.outs.Add(txID)
			dg.preferences.Remove(conflictID) // This conflict has an outbound edge

			// Change the edge direction according to this tx
			txNode.ins.Add(conflictID)
			txNode.outs.Remove(conflictID)
			if txNode.outs.Len() == 0 {
				// If this tx doesn't have any outbound edges, it's preferred
				dg.preferences.Add(txID)
			}

			// The instance's preferences have changed.
			changed = true
		}
	}

	shouldWork := true
	for shouldWork {
		acceptable, rejectable := dg.conflicts.Updateable()
		for _, toAccept := range acceptable {
			toAcceptID := toAccept.ID()

			// We can remove the accepted tx from the graph.
			delete(dg.txs, toAcceptID)

			// This tx is now accepted, so it shouldn't be part of the virtuous
			// set or the preferred set. Its status as Accepted implies these
			// descriptions.
			dg.virtuous.Remove(toAcceptID)
			dg.preferences.Remove(toAcceptID)

			// Update the metrics to account for this transaction's acceptance
			dg.metrics.Accepted(toAcceptID)

			// Accept the transaction
			if err := toAccept.Accept(); err != nil {
				return false, err
			}
		}
		for _, toReject := range rejectable {
			toRejectID := toReject.ID()
			toRejectNode := dg.txs[toRejectID]

			// We can remove the rejected tx from the graph.
			delete(dg.txs, toRejectID)

			// While it's statistically unlikely that something being rejected
			// is preferred, it is handled for completion.
			dg.preferences.Remove(toRejectID)

			// remove the edge between this node and all its neighbors
			dg.removeConflict(toRejectID, toRejectNode.ins)
			dg.removeConflict(toRejectID, toRejectNode.outs)

			// Update the metrics to account for this transaction's rejection
			dg.metrics.Rejected(toRejectID)

			// Reject the transaction
			if err := toReject.Reject(); err != nil {
				return false, err
			}
		}

		// If a status has been changed. We must perform a second iteration
		shouldWork = len(acceptable)+len(rejectable) > 0

		// If a status has been changed. So the frontiers must be recalculated.
		changed = changed || shouldWork
	}
	return changed, nil
}

func (dg *Directed) removeConflict(txID ids.ID, neighborIDs ids.Set) {
	for neighborID := range neighborIDs {
		neighbor, exists := dg.txs[neighborID]
		if !exists {
			// If the neighbor doesn't exist, they may have already been
			// rejected, so this mapping can be skipped.
			continue
		}

		// Remove any edge to this tx.
		delete(neighbor.ins, txID)
		delete(neighbor.outs, txID)

		if neighbor.outs.Len() == 0 {
			// If this tx should now be preferred, make sure its status is
			// updated.
			dg.preferences.Add(neighborID)
		}
	}
}

func (dg *Directed) String() string {
	nodes := make([]*snowballNode, len(dg.txs))
	i := 0
	for _, txNode := range dg.txs {
		nodes[i] = &snowballNode{
			txID:               txNode.tx.ID(),
			numSuccessfulPolls: txNode.numSuccessfulPolls,
			confidence:         txNode.Confidence(dg.currentVote),
		}
		i++
	}

	// Sort the nodes so that the string representation is canonical
	sortSnowballNodes(nodes)

	sb := strings.Builder{}
	sb.WriteString("DG(")

	format := fmt.Sprintf(
		"\n    Choice[%s] = ID: %%50s %%s",
		formatting.IntFormat(len(nodes)-1))
	for i, txNode := range nodes {
		sb.WriteString(fmt.Sprintf(format, i, txNode.txID, txNode))
	}

	if len(nodes) > 0 {
		sb.WriteString("\n")
	}
	sb.WriteString(")")
	return sb.String()
}

type snowballNode struct {
	txID               ids.ID
	numSuccessfulPolls int
	confidence         int
}

func (sb *snowballNode) String() string {
	return fmt.Sprintf(
		"SB(NumSuccessfulPolls = %d, Confidence = %d)",
		sb.numSuccessfulPolls,
		sb.confidence)
}

type sortSnowballNodeData []*snowballNode

func (sb sortSnowballNodeData) Less(i, j int) bool {
	return bytes.Compare(sb[i].txID[:], sb[j].txID[:]) == -1
}
func (sb sortSnowballNodeData) Len() int      { return len(sb) }
func (sb sortSnowballNodeData) Swap(i, j int) { sb[j], sb[i] = sb[i], sb[j] }

func sortSnowballNodes(nodes []*snowballNode) {
	sort.Sort(sortSnowballNodeData(nodes))
}
