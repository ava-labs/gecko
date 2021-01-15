// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"errors"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/components/avax"
)

var (
	errCacheTypeMismatch = errors.New("type returned from cache doesn't match the expected type")
	freezeAssetPrefix    = []byte("freezeAsset")
)

// state is a thin wrapper around a database to provide, caching, serialization,
// and de-serialization.
type state struct{ avax.State }

// Tx attempts to load a transaction from storage.
func (s *state) Tx(id ids.ID) (*Tx, error) {
	if txIntf, found := s.Cache.Get(id); found {
		if tx, ok := txIntf.(*Tx); ok {
			return tx, nil
		}
		return nil, errCacheTypeMismatch
	}

	bytes, err := s.DB.Get(id[:])
	if err != nil {
		return nil, err
	}

	// The key was in the database
	tx := &Tx{}
	version, err := s.GenesisCodec.Unmarshal(bytes, tx)
	if err != nil {
		return nil, err
	}
	tx.Version = version

	// The byte representation of this transaction, and the ID,
	// which is derived from it, are created by serializing the
	// transaction using the codec version it was created with
	unsignedBytes, err := s.GenesisCodec.Marshal(version, &tx.UnsignedTx)
	if err != nil {
		return nil, err
	}
	tx.Initialize(unsignedBytes, bytes)

	s.Cache.Put(id, tx)
	return tx, nil
}

// SetTx saves the provided transaction to storage.
func (s *state) SetTx(id ids.ID, tx *Tx) error {
	if tx == nil {
		s.Cache.Evict(id)
		return s.DB.Delete(id[:])
	}

	s.Cache.Put(id, tx)
	return s.DB.Put(id[:], tx.Bytes())
}
