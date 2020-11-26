// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package internalavm

import (
	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/vms/components/avax"
)

const (
	txID uint64 = iota
	utxoID
	txStatusID
	dbInitializedID
)

var (
	dbInitialized = ids.Empty.Prefix(dbInitializedID)
)

// prefixedState wraps a state object. By prefixing the state, there will be no
// collisions between different types of objects that have the same hash.
type prefixedState struct {
	state *state

	tx, utxo, txStatus cache.Cacher
	uniqueTx           cache.Deduplicator
}

// UniqueTx de-duplicates the transaction.
func (s *prefixedState) UniqueTx(tx *UniqueTx) *UniqueTx {
	return s.uniqueTx.Deduplicate(tx).(*UniqueTx)
}

// Tx attempts to load a transaction from storage.
func (s *prefixedState) Tx(id ids.ID) (*Tx, error) { return s.state.Tx(uniqueID(id, txID, s.tx)) }

// SetTx saves the provided transaction to storage.
func (s *prefixedState) SetTx(id ids.ID, tx *Tx) error {
	return s.state.SetTx(uniqueID(id, txID, s.tx), tx)
}

// UTXO attempts to load a utxo from storage.
func (s *prefixedState) UTXO(id ids.ID) (*avax.UTXO, error) {
	return s.state.UTXO(uniqueID(id, utxoID, s.utxo))
}

// SetUTXO saves the provided utxo to storage.
func (s *prefixedState) SetUTXO(id ids.ID, utxo *avax.UTXO) error {
	return s.state.SetUTXO(uniqueID(id, utxoID, s.utxo), utxo)
}

// Status returns the status of the provided transaction id from storage.
func (s *prefixedState) Status(id ids.ID) (choices.Status, error) {
	return s.state.Status(uniqueID(id, txStatusID, s.txStatus))
}

// SetStatus saves the provided status to storage.
func (s *prefixedState) SetStatus(id ids.ID, status choices.Status) error {
	return s.state.SetStatus(uniqueID(id, txStatusID, s.txStatus), status)
}

// DBInitialized returns the status of this database. If the database is
// uninitialized, the status will be unknown.
func (s *prefixedState) DBInitialized() (choices.Status, error) { return s.state.Status(dbInitialized) }

// SetDBInitialized saves the provided status of the database.
func (s *prefixedState) SetDBInitialized(status choices.Status) error {
	return s.state.SetStatus(dbInitialized, status)
}

// Funds returns a list of UTXO IDs such that each UTXO references [addr].
// All returned UTXO IDs have IDs greater than [start], where ids.Empty is the "least" ID.
// Returns at most [limit] UTXO IDs.
func (s *prefixedState) Funds(addr []byte, start ids.ID, limit int) ([]ids.ID, error) {
	return s.state.IDs(addr, start[:], limit)
}

// SpendUTXO consumes the provided utxo.
func (s *prefixedState) SpendUTXO(utxoID ids.ID) error {
	utxo, err := s.UTXO(utxoID)
	if err != nil {
		return err
	}
	if err := s.SetUTXO(utxoID, nil); err != nil {
		return err
	}

	addressable, ok := utxo.Out.(avax.Addressable)
	if !ok {
		return nil
	}

	return s.removeUTXO(addressable.Addresses(), utxoID)
}

func (s *prefixedState) removeUTXO(addrs [][]byte, utxoID ids.ID) error {
	for _, addr := range addrs {
		if err := s.state.RemoveID(addr, utxoID); err != nil {
			return err
		}
	}
	return nil
}

// FundUTXO adds the provided utxo to the database
func (s *prefixedState) FundUTXO(utxo *avax.UTXO) error {
	utxoID := utxo.InputID()
	if err := s.SetUTXO(utxoID, utxo); err != nil {
		return err
	}

	addressable, ok := utxo.Out.(avax.Addressable)
	if !ok {
		return nil
	}

	return s.addUTXO(addressable.Addresses(), utxoID)
}

func (s *prefixedState) addUTXO(addrs [][]byte, utxoID ids.ID) error {
	for _, addr := range addrs {
		if err := s.state.AddID(addr, utxoID); err != nil {
			return err
		}
	}
	return nil
}
