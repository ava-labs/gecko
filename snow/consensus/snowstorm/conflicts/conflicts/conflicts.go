// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package conflicts

import (
	"errors"
	"math"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
)

var (
	errInvalidTxType = errors.New("invalid tx type")
)

type Conflicts struct {
	// track the currently processing txs. This includes
	// transactions that have been added to the acceptable and rejectable
	// queues.
	// Transaction ID --> The transaction with that ID
	txs map[ids.ID]Tx

	// transitions tracks the currently processing txIDs for a transition.
	// Transition ID --> IDs of transactions that perform the transition
	transitions map[ids.ID]ids.Set

	// track which txs are currently consuming which utxos.
	// UTXO ID --> IDs of transactions that consume the UTXO
	utxos map[ids.ID]ids.Set

	// track which txs are attempting to be restricted.
	// Transition ID --> IDs of transactions that require the transition
	// to be performed in the transaction's epoch or a later one if
	// the transaction is to be accepted.
	restrictions map[ids.ID]ids.Set

	// track which txs are blocking on others.
	// Transition ID --> IDs of transactions that need the transition to be
	// performed before the transaction is accepted.
	dependencies map[ids.ID]ids.Set

	// conditionallyAccepted tracks the set of txIDs that have been
	// conditionally accepted, but not returned as fully accepted.
	conditionallyAccepted ids.Set

	// accepted tracks the set of txIDs that have been added to the acceptable
	// queue.
	accepted ids.Set

	// track txs that have been marked as ready to accept
	acceptable []choices.Decidable

	// track txs that have been marked as ready to reject
	rejectable []choices.Decidable
}

func New() *Conflicts {
	return &Conflicts{
		txs:          make(map[ids.ID]Tx),
		transitions:  make(map[ids.ID]ids.Set),
		utxos:        make(map[ids.ID]ids.Set),
		restrictions: make(map[ids.ID]ids.Set),
		dependencies: make(map[ids.ID]ids.Set),
	}
}

// Add this tx to the conflict set. If this tx is of the correct type, this tx
// will be added to the set of processing txs. It is assumed this tx wasn't
// already processing. This will mark the consumed utxos and register a rejector
// that will be notified if a dependency of this tx was rejected.
func (c *Conflicts) Add(txIntf choices.Decidable) error {
	tx, ok := txIntf.(Tx)
	if !ok {
		return errInvalidTxType
	}

	txID := tx.ID()
	c.txs[txID] = tx

	transitionID := tx.TransitionID()
	txIDs := c.transitions[transitionID]
	txIDs.Add(txID)
	c.transitions[transitionID] = txIDs

	for _, inputID := range tx.InputIDs() {
		spenders := c.utxos[inputID]
		spenders.Add(txID)
		c.utxos[inputID] = spenders
	}

	for _, restrictionID := range tx.Restrictions() {
		restrictors := c.restrictions[restrictionID]
		restrictors.Add(txID)
		c.restrictions[restrictionID] = restrictors
	}

	for _, dependencyID := range tx.Dependencies() {
		dependents := c.dependencies[dependencyID]
		dependents.Add(txID)
		c.dependencies[dependencyID] = dependents
	}
	return nil
}

// IsVirtuous checks the currently processing txs for conflicts. It is allowed
// to call this function with txs that aren't yet processing or currently
// processing txs.
func (c *Conflicts) IsVirtuous(txIntf choices.Decidable) (bool, error) {
	tx, ok := txIntf.(Tx)
	if !ok {
		return false, errInvalidTxType
	}

	// Check whether this transaction consumes the same state as
	// a different processing transaction
	txID := tx.ID()
	for _, inputID := range tx.InputIDs() { // For each piece of state [tx] consumes
		spenders := c.utxos[inputID] // txs that consume this state
		if numSpenders := spenders.Len(); numSpenders > 1 ||
			(numSpenders == 1 && !spenders.Contains(txID)) {
			return false, nil
		}
	}

	// Check if other transactions have marked as attempting to restrict this tx
	// to at least their epoch.
	epoch := tx.Epoch()
	transitionID := tx.TransitionID()
	// [restictors] is the set of transactions that require [tx]
	// to be performed in a later epoch
	restrictors := c.restrictions[transitionID]
	for restrictorID := range restrictors {
		restrictor := c.txs[restrictorID]
		restrictorEpoch := restrictor.Epoch()
		if restrictorEpoch > epoch {
			return false, nil
		}
	}

	// Check if this transaction has marked as attempting to restrict other txs
	// to at least this epoch.
	for _, restrictedTransitionID := range tx.Restrictions() {
		// [restrictionIDs] is a set of transactions that [tx] requires
		// to be performed in [epoch] or a later one
		restrictionIDs := c.transitions[restrictedTransitionID]
		for restrictionID := range restrictionIDs {
			restriction := c.txs[restrictionID]
			restrictionEpoch := restriction.Epoch()
			if restrictionEpoch < epoch {
				return false, nil
			}
		}
	}
	return true, nil
}

// Conflicts returns the collection of txs that are currently processing that
// conflict with the provided tx. It is allowed to call with function with txs
// that aren't yet processing or currently processing txs.
func (c *Conflicts) Conflicts(txIntf choices.Decidable) ([]choices.Decidable, error) {
	tx, ok := txIntf.(Tx)
	if !ok {
		return nil, errInvalidTxType
	}

	txID := tx.ID()
	var conflictSet ids.Set
	conflictSet.Add(txID)
	conflicts := []choices.Decidable(nil)

	// Add to the conflict set transactions that consume the same state as [tx]
	for _, inputID := range tx.InputIDs() {
		spenders := c.utxos[inputID]
		for spenderID := range spenders {
			if conflictSet.Contains(spenderID) {
				continue
			}
			conflictSet.Add(spenderID)
			conflicts = append(conflicts, c.txs[spenderID])
		}
	}

	// Add to the conflict set transactions that require the transition performed
	// by [tx] to be in at least their epoch, but which are in an epoch after [tx]'s
	epoch := tx.Epoch()
	transitionID := tx.TransitionID()
	restrictors := c.restrictions[transitionID]
	for restrictorID := range restrictors {
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
	// requires to occur in [epoch] or later, but which are in an earlier epoch
	for _, restrictedTransitionID := range tx.Restrictions() {
		restrictionIDs := c.transitions[restrictedTransitionID]
		for restrictionID := range restrictionIDs {
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
	return conflicts, nil
}

// Accept notifies this conflict manager that a tx has been conditionally
// accepted. This means that, assuming all the txs this tx depends on are
// accepted, then this tx should be accepted as well.
func (c *Conflicts) Accept(txID ids.ID) {
	tx, exists := c.txs[txID]
	if !exists {
		return
	}

	acceptable := true
	for _, dependency := range tx.Dependencies() {
		if !c.accepted.Contains(dependency) {
			// [tx] is not acceptable because a transition it requires to have
			// been performed has not yet been
			acceptable = false
		}
	}
	if acceptable {
		transitionID := tx.TransitionID()
		c.accepted.Add(transitionID)
		c.acceptable = append(c.acceptable, tx)
	} else {
		c.conditionallyAccepted.Add(txID)
	}
}

func (c *Conflicts) Updateable() ([]choices.Decidable, []choices.Decidable) {
	accepted := c.accepted
	c.accepted = nil
	acceptable := c.acceptable
	c.acceptable = nil

	// rejected tracks the set of txIDs that have been rejected but not yet
	// removed from the graph.
	rejected := ids.Set{}

	// Accept each acceptable tx
	for _, tx := range acceptable {
		tx := tx.(Tx)
		txID := tx.ID()
		transitionID := tx.TransitionID()
		epoch := tx.Epoch()

		// Remove the accepted transaction from the processing tx set
		delete(c.txs, txID)

		// Remove from the transitions map
		txIDs := c.transitions[transitionID]
		txIDs.Remove(txID)
		if txIDs.Len() == 0 {
			delete(c.transitions, transitionID)
		} else {
			c.transitions[transitionID] = txIDs
		}

		// Remove from the UTXO map
		for _, inputID := range tx.InputIDs() {
			spenders := c.utxos[inputID]
			spenders.Remove(txID)
			if spenders.Len() == 0 {
				delete(c.utxos, inputID)
			} else {
				c.utxos[inputID] = spenders
			}
		}

		// Remove from the restrictions map
		for _, restrictionID := range tx.Restrictions() {
			restrictors := c.restrictions[restrictionID]
			restrictors.Remove(txID)
			if restrictors.Len() == 0 {
				delete(c.restrictions, restrictionID)
			} else {
				c.restrictions[restrictionID] = restrictors
			}
		}

		// Remove the dependency pointers of the transactions that depended on
		// [tx]'s transition. If a transaction has been conditionally accepted and
		// it has no dependencies left, then it should be marked as accepted.
		// Reject dependencies that are no longer valid to be accepted.
		// [dependents] is the set of transactions that require transition
		// [transitionID] to be performed before they are accepted
		dependents := c.dependencies[transitionID]
		delete(c.dependencies, transitionID)
	acceptedDependentLoop:
		for dependentID := range dependents {
			dependent := c.txs[dependentID]
			dependentEpoch := dependent.Epoch()
			if dependentEpoch < epoch && !rejected.Contains(dependentID) {
				// [dependent] requires [tx]'s transition to happen
				// no later than [epoch] but the transition happens after.
				// Therefore, [dependent] may no longer be accepted.
				rejected.Add(dependentID)
				c.rejectable = append(c.rejectable, dependent)
			}

			if !c.conditionallyAccepted.Contains(dependentID) {
				continue
			}

			// [dependent] has been conditionally accepted.
			// Check whether all of its transition dependencies are met
			for _, dependencyID := range dependent.Dependencies() {
				if !accepted.Contains(dependencyID) {
					continue acceptedDependentLoop
				}
			}

			// [dependent] has been conditionally accepted
			// and its transition dependencies have been met.
			// Therefore, it is acceptable.
			dependentTransitionID := dependent.TransitionID()
			c.conditionallyAccepted.Remove(dependentID)
			c.accepted.Add(dependentTransitionID)
			c.acceptable = append(c.acceptable, dependent)
		}

		// Mark that the transactions that conflict with [tx] are rejectable
		// Conflicts should never return an error, as the type has already been
		// asserted.
		conflicts, _ := c.Conflicts(tx)
		for _, conflict := range conflicts {
			conflictID := conflict.ID()
			if rejected.Contains(conflictID) {
				continue
			}
			rejected.Add(conflictID)
			c.rejectable = append(c.rejectable, conflict)
		}
	}

	// Reject each rejectable transaction
	rejectable := c.rejectable
	c.rejectable = nil
	for _, tx := range rejectable {
		tx := tx.(Tx)
		txID := tx.ID()
		transitionID := tx.TransitionID()

		// Remove the rejected transaction from the processing tx set
		delete(c.txs, txID)

		// Remove from the transitions map
		txIDs := c.transitions[transitionID]
		txIDs.Remove(txID)
		if txIDs.Len() == 0 {
			delete(c.transitions, transitionID)
		} else {
			c.transitions[transitionID] = txIDs
		}

		// Remove from the UTXO map
		for _, inputID := range tx.InputIDs() {
			spenders := c.utxos[inputID]
			spenders.Remove(txID)
			if spenders.Len() == 0 {
				delete(c.utxos, inputID)
			} else {
				c.utxos[inputID] = spenders
			}
		}

		// Remove from the restrictions map
		for _, restrictionID := range tx.Restrictions() {
			restrictors := c.restrictions[restrictionID]
			restrictors.Remove(txID)
			if restrictors.Len() == 0 {
				delete(c.restrictions, restrictionID)
			} else {
				c.restrictions[restrictionID] = restrictors
			}
		}

		if txIDs.Len() == 0 {
			// [tx] was the only processing tx that performs transition [transitionID]
			// Remove the dependency pointers of the transactions that depended
			// on this transition.
			dependents := c.dependencies[transitionID]
			for dependentID := range dependents {
				dependent := c.txs[dependentID]
				if !rejected.Contains(dependentID) {
					rejected.Add(dependentID)
					c.rejectable = append(c.rejectable, dependent)
				}
			}
			delete(c.dependencies, transitionID)
		} else {
			// There are processing tx's other than [tx] that
			// perform transition [transitionID].
			// Calculate the earlier possible epoch in which the transition may occur.
			lowestRemainingEpoch := uint32(math.MaxUint32)
			for otherTxID := range txIDs {
				otherTx := c.txs[otherTxID]
				otherEpoch := otherTx.Epoch()
				if otherEpoch < lowestRemainingEpoch {
					lowestRemainingEpoch = otherEpoch
				}
			}

			// Mark as rejectable txs that require transition [transitionID] to
			// occur earlier than it possibly may.
			dependents := c.dependencies[transitionID]
			for dependentID := range dependents {
				dependent := c.txs[dependentID]
				dependentEpoch := dependent.Epoch()
				if dependentEpoch < lowestRemainingEpoch && !rejected.Contains(dependentID) {
					rejected.Add(dependentID)
					c.rejectable = append(c.rejectable, dependent)
				}
			}
		}
	}

	return acceptable, rejectable
}
