// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"fmt"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/components/avax"
)

const (
	txID uint64 = iota
	utxoID
	txStatusID
	dbInitializedID
	txEpochID
)

var (
	dbInitialized = ids.Empty.Prefix(dbInitializedID)
)

func uniqueID(id ids.ID, prefix uint64, cacher cache.Cacher) ids.ID {
	if cachedIDIntf, found := cacher.Get(id); found {
		return cachedIDIntf.(ids.ID)
	}
	uID := id.Prefix(prefix)
	cacher.Put(id, uID)
	return uID
}

// prefixedState wraps a state object. By prefixing the state, there will be no
// collisions between different types of objects that have the same hash.
type prefixedState struct {
	state *state

	tx, utxo, txStatus, txEpoch cache.Cacher
	uniqueTx                    cache.Deduplicator
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

// Epoch returns the epoch from storage.
func (s *prefixedState) Epoch(id ids.ID) (uint32, error) {
	return s.state.Int(uniqueID(id, txEpochID, s.txEpoch))
}

// SetEpoch saves the epoch to storage.
func (s *prefixedState) SetEpoch(id ids.ID, val uint32) error {
	return s.state.SetInt(uniqueID(id, txEpochID, s.txEpoch), val)
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

// PutManagedAssetStatus records that the asset's status changed in [epoch]
// Status changes don't go into effect until epoch [epoch + 2].
// For example, if you update a managed asset from unfrozen to frozen
// with a tx in epoch n then the asset can be spent in epoch n and n+1.
// As such, we store on disk both the most recent status and the status
// before that.
func (s *prefixedState) PutManagedAssetStatus(
	assetID ids.ID,
	epoch uint32,
	status ManagedAssetStatus,
) error {
	// Get the most recent status
	_, oldStatus, _, err := s.ManagedAssetStatus(assetID)
	if err != nil && err != database.ErrNotFound {
		return fmt.Errorf("couldn't get old asset status: %w", err)
	} else if err == database.ErrNotFound {
		// There is no most recent status (i.e. this is for an asset creation)
		// so put the same status again.
		oldStatus = status
	}

	// Serialize the old status and new status
	bothStatuses := [2]ManagedAssetStatus{status, oldStatus}
	bothStatusesBytes, err := s.state.Codec.Marshal(s.state.CodecVersionF(), bothStatuses)
	if err != nil {
		return fmt.Errorf("couldn't serialize asset status: %w", err)
	}

	// Pack into a byte array
	p := wrappers.Packer{MaxSize: 2*wrappers.IntLen + len(bothStatusesBytes)}
	p.PackInt(epoch)
	p.PackBytes(bothStatusesBytes)
	if p.Errored() {
		return fmt.Errorf("couldn't pack statuses to byte array: %w", err)
	}

	// Put into database
	key := make([]byte, len(freezeAssetPrefix)+hashing.HashLen)
	copy(key, freezeAssetPrefix)
	copy(key[len(freezeAssetPrefix):], assetID[:])
	return s.state.DB.Put(key, p.Bytes)
}

// ManagedAssetStatus returns:
// 1) The epoch in which the last status update occurred (or was created, if never updated)
// 2) The most recent status update
// 3) The status update before that. If none exists, also returns the most recent status update.
// Should return nil for a managed asset that has already been created.
// Return database.ErrNotFound if the asset ID doesn't correspond to a managed asset.
func (s *prefixedState) ManagedAssetStatus(assetID ids.ID) (
	uint32,
	ManagedAssetStatus,
	ManagedAssetStatus,
	error,
) {
	key := make([]byte, len(freezeAssetPrefix)+hashing.HashLen)
	copy(key, freezeAssetPrefix)
	copy(key[len(freezeAssetPrefix):], assetID[:])

	bytes, err := s.state.DB.Get(key)
	if err != nil {
		return 0, nil, nil, err
	}

	p := wrappers.Packer{MaxSize: len(bytes)}
	p.Bytes = bytes
	epoch := p.UnpackInt()
	if p.Errored() {
		return 0, nil, nil, fmt.Errorf("couldn't unpack epoch: %w", err)
	}
	statusesBytes := p.UnpackBytes()
	if p.Errored() {
		return 0, nil, nil, fmt.Errorf("couldn't unpack statuses bytes: %w", err)
	}

	var statuses [2]ManagedAssetStatus
	if _, err := s.state.Codec.Unmarshal(statusesBytes, &statuses); err != nil {
		return 0, nil, nil, fmt.Errorf("couldn't deserialize asset statuses: %w", err)
	}
	return epoch, statuses[0], statuses[1], nil
}
