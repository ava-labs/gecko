// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package conflicts

import (
	"math"

	"github.com/ava-labs/avalanchego/ids"
)

// transitionNode contains data about a processing transition
type transitionNode struct {
	// [txIDs] is the set of processing txs that contain this transition.
	txIDs ids.Set

	// [restrictions] is the set of transaction IDs that require this
	// transition to be performed in that transaction's epoch or a
	// later one.
	restrictions ids.Set

	// [dependents] is the set of transactions that require this
	// transition to be performed before the transaction can be accepted.
	dependents ids.Set

	// [missingDependencies] is the set of transitions that must
	// be performed before this transition can be accepted.
	missingDependencies ids.Set
}

// Conflicts implements the [snowstorm.Conflicts] interface
type Conflicts struct {
	// tracks the currently processing txs. This includes
	// transactions in [acceptable] and [rejectable].
	// Transaction ID --> The transaction with that ID.
	txs map[ids.ID]Tx

	// tracks the currently processing transitions
	// Transition ID --> Data about that transition.
	transitionNodes map[ids.ID]transitionNode

	// tracks which txs consume which utxos.
	// UTXO ID --> IDs of transactions that consume the UTXO.
	utxos map[ids.ID]ids.Set

	// conditionallyAccepted tracks the set of txIDs that have been
	// conditionally accepted, but not returned as fully accepted.
	// A transaction is conditionally accepted if it has received enough
	// consecutive successful polls but has unmet dependencies.
	conditionallyAccepted ids.Set

	// acceptableIDs is the set of IDs of transitions that are in a
	// transaction in [acceptable].
	acceptableIDs ids.Set

	// acceptable is the set of txs that may be accepted.
	acceptable []Tx

	// rejectableIDs is the set of IDs of transactions in [rejectable].
	rejectableIDs ids.Set

	// rejectable is the set of txs that may be rejected.
	rejectable []Tx
}

// New returns a new Conflict Manager
func New() *Conflicts {
	return &Conflicts{
		txs:             make(map[ids.ID]Tx),
		utxos:           make(map[ids.ID]ids.Set),
		transitionNodes: make(map[ids.ID]transitionNode),
	}
}

// Add this tx to the conflict set.
// Assumes: Add has not already been called with argument [tx].
func (c *Conflicts) Add(tx Tx) {
	// Mark that this tx is processing
	txID := tx.ID()
	c.txs[txID] = tx

	// Mark that this transaction contains this transition
	tr := tx.Transition()
	trID := tr.ID()

	tn := c.transitionNodes[trID]
	tn.txIDs.Add(txID)

	// Mark the UTXOs that this transition consumes
	for _, inputID := range tr.InputIDs() {
		spenders := c.utxos[inputID]
		spenders.Add(txID)
		c.utxos[inputID] = spenders
	}

	// Mark the transitions that this transaction depends
	// on being accepted in an epoch >= this transaction's epoch
	for _, restrictionID := range tx.Restrictions() {
		restrictionNode := c.transitionNodes[restrictionID]
		restrictionNode.restrictions.Add(txID)
		c.transitionNodes[restrictionID] = restrictionNode
	}

	// Mark this transition's dependencies
	missingDependencies := tr.Dependencies()
	for _, dependencyID := range missingDependencies {
		dependentNode := c.transitionNodes[dependencyID]
		dependentNode.dependents.Add(txID)
		c.transitionNodes[dependencyID] = dependentNode
	}

	tn.missingDependencies.Add(missingDependencies...)
	// The transition node is written back
	c.transitionNodes[trID] = tn
}

// Processing returns true if transition [trID] is processing.
func (c *Conflicts) Processing(trID ids.ID) bool {
	_, exists := c.transitionNodes[trID]
	return exists
}

// IsVirtuous checks the currently processing txs for conflicts.
// [tx] need not be is processing.
func (c *Conflicts) IsVirtuous(tx Tx) bool {
	// Check whether this transaction consumes the same state as a different
	// processing transaction
	txID := tx.ID()
	tr := tx.Transition()
	for _, inputID := range tr.InputIDs() { // For each piece of state [tx] consumes
		spenders := c.utxos[inputID] // txs that consume this state
		if numSpenders := spenders.Len(); numSpenders > 1 ||
			(numSpenders == 1 && !spenders.Contains(txID)) {
			return false // another tx consumes the same state
		}
	}

	// Check if there are any transactions that attempt to restrict this
	// transition to at least their epoch.
	epoch := tx.Epoch()
	trID := tr.ID()
	tn := c.transitionNodes[trID]

	for restrictorID := range tn.restrictions {
		restrictor := c.txs[restrictorID]
		restrictorEpoch := restrictor.Epoch()
		if restrictorEpoch > epoch {
			return false
		}
	}

	// Check if this transaction is marked as attempting to restrict other txs
	// to at least this epoch.
	for _, restrictedTransitionID := range tx.Restrictions() {
		// Iterate over the set of transactions that [tx] requires to be
		// performed in [epoch] or a later one
		for restrictionID := range c.transitionNodes[restrictedTransitionID].txIDs {
			restriction := c.txs[restrictionID]
			restrictionEpoch := restriction.Epoch()
			if restrictionEpoch < epoch {
				return false
			}
		}
	}
	return true
}

// Conflicts returns the processing txs that conflict with [tx].
// [tx] need not be processing.
func (c *Conflicts) Conflicts(tx Tx) []Tx {
	txID := tx.ID()
	var conflictSet ids.Set
	conflictSet.Add(txID)
	conflicts := []Tx(nil)
	tr := tx.Transition()

	// Add to the conflict set transactions that consume the same state as [tx]
	for _, inputID := range tr.InputIDs() {
		spenders := c.utxos[inputID]
		for spenderID := range spenders {
			if conflictSet.Contains(spenderID) { // already have this tx as a conflict
				continue
			}
			conflictSet.Add(spenderID)
			conflicts = append(conflicts, c.txs[spenderID])
		}
	}

	// Add to the conflict set transactions that require the transition
	// performed by [tx] to be in at least their epoch, but which are in an
	// epoch after [tx]'s
	epoch := tx.Epoch()
	trID := tr.ID()
	tn := c.transitionNodes[trID]
	for restrictorID := range tn.restrictions {
		if conflictSet.Contains(restrictorID) {
			continue
		}
		restrictor := c.txs[restrictorID]
		restrictorEpoch := restrictor.Epoch()
		if restrictorEpoch > epoch {
			conflictSet.Add(restrictorID)
			conflicts = append(conflicts, restrictor)
		}
	}

	// Add to the conflict set transactions that perform a transition that [tx]
	// requires to occur in [epoch] or later, but which are in an earlier epoch.
	for _, restrictedTransitionID := range tx.Restrictions() {
		for restrictionID := range c.transitionNodes[restrictedTransitionID].txIDs {
			if conflictSet.Contains(restrictionID) {
				continue
			}
			restriction := c.txs[restrictionID]
			restrictionEpoch := restriction.Epoch()
			if restrictionEpoch < epoch {
				conflictSet.Add(restrictionID)
				conflicts = append(conflicts, restriction)
			}
		}
	}
	return conflicts
}

// Accept notifies this conflict manager that tx [txID] has been conditionally
// accepted. That is, assuming all the txs this tx depends on are
// accepted, then this tx should be accepted as well.
func (c *Conflicts) Accept(txID ids.ID) {
	tx, exists := c.txs[txID]
	if !exists {
		return
	}

	tr := tx.Transition()
	trID := tr.ID()
	tn := c.transitionNodes[trID]

	acceptable := true
	for dependency := range tn.missingDependencies {
		if !c.acceptableIDs.Contains(dependency) {
			// [tx] is not acceptable because a transition it requires to have
			// been performed has not yet been
			acceptable = false
			break
		}
	}
	if acceptable {
		c.acceptableIDs.Add(trID)
		c.acceptable = append(c.acceptable, tx)
	} else {
		c.conditionallyAccepted.Add(txID)
	}
}

// Updateable implements the [snowstorm.Conflicts] interface
func (c *Conflicts) Updateable() ([]Tx, []Tx) {
	acceptable := make([]Tx, 0, len(c.acceptable))
	rejectable := make([]Tx, 0)
	done := false
	for !done {
		acceptableInner := c.updateAccepted()
		rejectableInner := c.updateRejected()
		acceptable = append(acceptable, acceptableInner...)
		rejectable = append(rejectable, rejectableInner...)
		done = len(acceptableInner)+len(rejectableInner) == 0
	}

	c.acceptableIDs.Clear()
	c.rejectableIDs.Clear()
	return acceptable, rejectable
}

func (c *Conflicts) updateAccepted() []Tx {
	acceptable := c.acceptable
	c.acceptable = nil

	// This loop does cleanup for the txs which are about to be accepted,
	// marks conditionally acceptable txs with all dependencies met as
	// acceptable, and marks conflicting txs as rejectable
	for _, tx := range acceptable {
		txID := tx.ID()
		tr := tx.Transition()
		trID := tr.ID()
		epoch := tx.Epoch()
		tn := c.transitionNodes[trID]

		// Remove the accepted transaction from the processing tx set
		delete(c.txs, txID)

		// Remove from the transitions map
		tn.txIDs.Remove(txID)

		// Remove [tx] from the UTXO map
		c.removeInputs(txID, tr)

		// Remove from the restrictions
		c.removeRestrictions(tx)

		// Go though the dependent transactions to accept or reject them
		// according to the epochs
		c.notifyDependentsAccept(epoch, trID, &tn)

		// If the last transaction attempting to perform [trID] has been
		// accepted, then remove it from the transition map.
		if tn.txIDs.Len() == 0 {
			delete(c.transitionNodes, trID)
		}

		// Since we are accepting this transaction, all the transactions that
		// conflict with this transaction should be rejected
		c.rejectConflicts(tx)
	}
	return acceptable
}

// removeInputs of this transition from the UTXO set
func (c *Conflicts) removeInputs(txID ids.ID, tr Transition) {
	// Remove [tx] from the UTXO map
	for _, inputID := range tr.InputIDs() {
		spenders := c.utxos[inputID]
		spenders.Remove(txID)
		if spenders.Len() == 0 {
			delete(c.utxos, inputID)
		}
		// Note: Updating the utxos map isn't needed here because the utxo set
		// is a map, which wraps a pointer.
	}
}

// removeRestrictions that this transaction names
func (c *Conflicts) removeRestrictions(tx Tx) {
	txID := tx.ID()
	// Remove [tx] from the restrictions
	for _, restrictionID := range tx.Restrictions() {
		restrictorNode := c.transitionNodes[restrictionID]
		restrictorNode.restrictions.Remove(txID)
		// Note: Updating the transitionNodes map isn't needed here because the
		// restrictions set is a map, which wraps a pointer.
	}
}

// Remove the dependency pointers of the transactions that depended on [tx]'s
// transition. If a transaction has been conditionally accepted and it has no
// dependencies left, then it should be marked as acceptable. Reject
// dependencies that are no longer valid to be accepted.
func (c *Conflicts) notifyDependentsAccept(epoch uint32, trID ids.ID, tn *transitionNode) {
outerLoop:
	// [tn.dependents] is the set of transactions that require
	// [transition] to be performed before they are accepted
	for dependentTxID := range tn.dependents {
		dependentTx := c.txs[dependentTxID]
		dependentEpoch := dependentTx.Epoch()

		dependentTransition := dependentTx.Transition()
		dependentTransitionID := dependentTransition.ID()

		// Mark that this dependency has been fulfilled.
		dependentTransitionNode := c.transitionNodes[dependentTransitionID]
		dependentTransitionNode.missingDependencies.Remove(trID)
		// Note: Updating the transitionNodes map isn't needed here because the
		// missingDependencies set is a map, which wraps a pointer.

		// If the dependent transaction has already been marked for rejection,
		// we can't reject it again.
		if c.rejectableIDs.Contains(dependentTxID) {
			continue
		}

		// If [dependentTx] requires [tx]'s transition to happen before [epoch],
		// then [dependent] can no longer be accepted.
		if dependentEpoch < epoch {
			// If this tx was previously conditionally accepted, it should no
			// longer be treated as such.
			c.conditionallyAccepted.Remove(dependentTxID)

			c.rejectableIDs.Add(dependentTxID)
			c.rejectable = append(c.rejectable, dependentTx)
			continue
		}

		// If the dependent transaction hasn't been accepted by consensus yet,
		// it shouldn't be marked as accepted.
		if !c.conditionallyAccepted.Contains(dependentTxID) {
			continue
		}

		// [dependent] has been conditionally accepted.
		// Check whether all of its transition dependencies are met
		for dependentTransitionID := range dependentTransitionNode.missingDependencies {
			if !c.acceptableIDs.Contains(dependentTransitionID) {
				continue outerLoop
			}
		}

		// [dependentTx] has been conditionally accepted and its transition
		// dependencies have been met.
		c.conditionallyAccepted.Remove(dependentTxID)
		c.acceptableIDs.Add(dependentTransitionID)
		c.acceptable = append(c.acceptable, dependentTx)
	}

	// Make sure that future checks of the dependents doesn't result in multiple
	// iterations, as the dependents should already have had their missins
	// dependencies updated.
	tn.dependents.Clear()
}

// rejectConflicts of [tx]
func (c *Conflicts) rejectConflicts(tx Tx) {
	conflictTxs := c.Conflicts(tx)
	for _, conflictTx := range conflictTxs {
		if conflictTxID := conflictTx.ID(); !c.rejectableIDs.Contains(conflictTxID) {
			// If this tx was previously conditionally accepted, it should no
			// longer be treated as such.
			c.conditionallyAccepted.Remove(conflictTxID)

			c.rejectableIDs.Add(conflictTxID)
			c.rejectable = append(c.rejectable, conflictTx)
		}
	}
}

// Cleanup the transactions which are about to be rejected and mark transactions
// as rejectable if they have a dependency/restriction that can't be met.
func (c *Conflicts) updateRejected() []Tx {
	rejectable := c.rejectable
	c.rejectable = nil
	for _, tx := range rejectable {
		txID := tx.ID()
		tr := tx.Transition()
		trID := tr.ID()
		tn := c.transitionNodes[trID]

		// Remove [tx] from the processing tx set
		delete(c.txs, txID)

		// Remove [tx] from set of transactions that perform this transition
		tn.txIDs.Remove(txID)
		// Note: Updating the transitionNodes map isn't needed here because the
		// txIDs set is a map, which wraps a pointer.

		// Remove [tx] from the UTXO map
		c.removeInputs(txID, tr)

		// Remove [tx] from the restrictions
		c.removeRestrictions(tx)

		// Remove transactions that are no longer valid
		c.rejectInvalidDependents(&tn)

		// Since this transaction is being rejected, it's dependencies no longer
		// need to notify it.
		c.removeDependents(txID, &tn)

		// If the last transaction attempting to perform [trID] has been
		// rejected, then remove it from the transition map.
		if tn.txIDs.Len() == 0 {
			delete(c.transitionNodes, trID)
		}
	}

	return rejectable
}

// Mark as rejectable txs that require the transition to occur earlier than it
// can.
func (c *Conflicts) rejectInvalidDependents(tn *transitionNode) {
	if len(tn.dependents) == 0 {
		return
	}

	// Calculate the earliest epoch in which the transition may occur.
	lowestRemainingEpoch := uint32(math.MaxUint32)
	for otherTxID := range tn.txIDs {
		otherTx := c.txs[otherTxID]
		otherEpoch := otherTx.Epoch()
		if otherEpoch < lowestRemainingEpoch {
			lowestRemainingEpoch = otherEpoch
		}
	}

	for dependentTxID := range tn.dependents {
		dependentTx := c.txs[dependentTxID]
		dependentEpoch := dependentTx.Epoch()
		if dependentEpoch < lowestRemainingEpoch && !c.rejectableIDs.Contains(dependentTxID) {
			// If this tx was previously conditionally accepted, it should no
			// longer be treated as such.
			c.conditionallyAccepted.Remove(dependentTxID)

			c.rejectableIDs.Add(dependentTxID)
			c.rejectable = append(c.rejectable, dependentTx)
		}
	}
}

// For each missing dependency of this transition, mark that [tx] is no longer a
// dependent
func (c *Conflicts) removeDependents(txID ids.ID, tn *transitionNode) {
	for dependency := range tn.missingDependencies {
		dependentTransitionNode := c.transitionNodes[dependency]
		dependentTransitionNode.dependents.Remove(txID)
	}
}
