// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package conflicts

import (
	"errors"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/events"
)

var (
	errInvalidTxType = errors.New("invalid tx type")
)

type Conflicts struct {
	// track the currently processing txs
	txs map[[32]byte]Tx

	// track which txs are currently consuming which utxos
	utxos map[[32]byte]ids.Set

	// keeps track of whether dependencies have been accepted
	pendingAccept events.Blocker

	// keeps track of whether dependencies have been rejected
	pendingReject events.Blocker

	// track txs that have been marked as ready to accept
	acceptable []choices.Decidable

	// track txs that have been marked as ready to reject
	rejectable []choices.Decidable
}

func New() *Conflicts {
	return &Conflicts{
		txs:   make(map[[32]byte]Tx),
		utxos: make(map[[32]byte]ids.Set),
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
	c.txs[txID.Key()] = tx
	for _, inputID := range tx.InputIDs() {
		inputKey := inputID.Key()
		spenders := c.utxos[inputKey]
		spenders.Add(txID)
		c.utxos[inputKey] = spenders
	}

	toReject := &rejector{
		c:  c,
		tx: tx,
	}

	toReject.deps.Add(txID)
	for _, dependency := range tx.Dependencies() {
		if dependency.Status() != choices.Accepted {
			// If the dependency isn't accepted, then it must be processing.
			// This tx should be accepted after this tx is accepted. Note that
			// the dependencies can't already be rejected, because it is assumed
			// that this tx is currently considered valid.
			toReject.deps.Add(dependency.ID())
		}
	}
	c.pendingReject.Register(toReject)
	return nil
}

// IsVirtuous checks the currently processing txs for conflicts. It is assumed
// any tx passed into this function is not yet processing.
func (c *Conflicts) IsVirtuous(txIntf choices.Decidable) (bool, error) {
	tx, ok := txIntf.(Tx)
	if !ok {
		return false, errInvalidTxType
	}

	for _, inputID := range tx.InputIDs() {
		if _, exists := c.utxos[inputID.Key()]; exists {
			return false, nil
		}
	}
	return true, nil
}

// Conflicts returns the collection of txs that are currently processing that
// conflict with the provided tx. It is assumed any tx passed into this function
// is not yet processing.
func (c *Conflicts) Conflicts(txIntf choices.Decidable) (ids.Set, error) {
	tx, ok := txIntf.(Tx)
	if !ok {
		return nil, errInvalidTxType
	}
	return c.conflicts(tx), nil
}

func (c *Conflicts) conflicts(tx Tx) ids.Set {
	var conflicts ids.Set
	for _, inputID := range tx.InputIDs() {
		conflicts.Union(c.utxos[inputID.Key()])
	}
	return conflicts
}

// Accept notifies this conflict manager that a tx has been conditionally
// accepted. This means that assuming all the txs this tx depends on are
// accepted, then this tx should be accepted as well. This assumes that the tx
// has been issued, hasn't previously been marked as conditionally accepted, and
// hasn't been returned as being rejectable.
func (c *Conflicts) Accept(txID ids.ID) {
	tx := c.txs[txID.Key()]
	toAccept := &acceptor{
		c:  c,
		tx: tx,
	}
	for _, dependency := range tx.Dependencies() {
		if dependency.Status() != choices.Accepted {
			// If the dependency isn't accepted, then it must be processing.
			// This tx should be accepted after this tx is accepted. Note that
			// the dependencies can't already be rejected, because it is assumed
			// that this tx is currently considered valid.
			toAccept.deps.Add(dependency.ID())
		}
	}
	c.pendingAccept.Register(toAccept)
}

func (c *Conflicts) Updateable() ([]choices.Decidable, []choices.Decidable) {
	acceptable := c.acceptable
	c.acceptable = nil
	for _, tx := range acceptable {
		txID := tx.ID()
		txKey := txID.Key()

		tx := tx.(Tx)
		for _, inputID := range tx.InputIDs() {
			inputKey := inputID.Key()
			spenders := c.utxos[inputKey]
			delete(spenders, txKey)
			if spenders.Len() == 0 {
				delete(c.utxos, inputKey)
			} else {
				c.utxos[inputKey] = spenders
			}
		}

		delete(c.txs, txKey)
		c.pendingAccept.Fulfill(txID)
		c.pendingReject.Abandon(txID)

		for conflictKey := range c.conflicts(tx) {
			c.pendingReject.Fulfill(ids.NewID(conflictKey))
		}
	}

	rejectable := c.rejectable
	c.rejectable = nil
	for _, tx := range rejectable {
		txID := tx.ID()
		txKey := txID.Key()

		tx := tx.(Tx)
		for _, inputID := range tx.InputIDs() {
			inputKey := inputID.Key()
			spenders := c.utxos[inputKey]
			delete(spenders, txKey)
			if spenders.Len() == 0 {
				delete(c.utxos, inputKey)
			} else {
				c.utxos[inputKey] = spenders
			}
		}

		delete(c.txs, txKey)
		c.pendingAccept.Abandon(txID)
		c.pendingReject.Fulfill(txID)
	}

	return acceptable, rejectable
}
