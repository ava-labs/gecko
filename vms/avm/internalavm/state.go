// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package internalavm

import (
	"errors"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/components/avax"
)

var (
	errCacheTypeMismatch = errors.New("type returned from cache doesn't match the expected type")
)

func uniqueID(id ids.ID, prefix uint64, cacher cache.Cacher) ids.ID {
	if cachedIDIntf, found := cacher.Get(id); found {
		return cachedIDIntf.(ids.ID)
	}
	uID := id.Prefix(prefix)
	cacher.Put(id, uID)
	return uID
}

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
	if _, err := s.GenesisCodec.Unmarshal(bytes, tx); err != nil {
		return nil, err
	}
	unsignedBytes, err := s.GenesisCodec.Marshal(codecVersion, &tx.UnsignedTx)
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
