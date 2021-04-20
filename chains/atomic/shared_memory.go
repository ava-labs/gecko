// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package atomic

import (
	"bytes"
	"errors"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/hashing"
)

var (
	smallerValuePrefix = []byte{0}
	smallerIndexPrefix = []byte{1}
	largerValuePrefix  = []byte{2}
	largerIndexPrefix  = []byte{3}

	errDuplicatedOperation = errors.New("duplicated operation on provided value")
)

type dbElement struct {
	// Present indicates the value was removed before existing.
	// If set to false, when this element is added to the shared memory, it will
	// be immediately removed.
	// If set to true, then this element will be removed normally when remove is
	// called.
	Present bool `serialize:"true"`

	// Value is the body of this element.
	Value []byte `serialize:"true"`

	// Traits are a collection of features that can be used to lookup this
	// element.
	Traits [][]byte `serialize:"true"`
}

// Element ...
type Element struct {
	Key    []byte
	Value  []byte
	Traits [][]byte
}

// SharedMemory ...
type SharedMemory interface {
	// Adds to the peer chain's side
	Put(peerChainID ids.ID, elems []*Element, batches ...database.Batch) error

	// Fetches from this chain's side
	Get(peerChainID ids.ID, keys [][]byte) (values [][]byte, err error)
	Indexed(
		peerChainID ids.ID,
		traits [][]byte,
		startTrait,
		startKey []byte,
		limit int,
	) (
		values [][]byte,
		lastTrait,
		lastKey []byte,
		err error,
	)
	Remove(peerChainID ids.ID, keys [][]byte, batches ...database.Batch) error
}

// sharedMemory provides the API for a blockchain to interact with shared memory
// of another blockchain
type sharedMemory struct {
	m           *Memory
	thisChainID ids.ID
}

func (sm *sharedMemory) Put(peerChainID ids.ID, elems []*Element, batches ...database.Batch) error {
	sharedID := sm.m.sharedID(peerChainID, sm.thisChainID)
	vdb, db := sm.m.GetDatabase(sharedID)
	defer sm.m.ReleaseDatabase(sharedID)

	s := state{
		c: sm.m.codec,
	}
	if bytes.Compare(sm.thisChainID[:], peerChainID[:]) == -1 {
		s.valueDB = prefixdb.New(largerValuePrefix, db)
		s.indexDB = prefixdb.New(largerIndexPrefix, db)
	} else {
		s.valueDB = prefixdb.New(smallerValuePrefix, db)
		s.indexDB = prefixdb.New(smallerIndexPrefix, db)
	}

	for _, elem := range elems {
		if err := s.SetValue(elem); err != nil {
			return err
		}
	}

	myBatch, err := vdb.CommitBatch()
	if err != nil {
		return err
	}
	return WriteAll(myBatch, batches...)
}

func (sm *sharedMemory) Get(peerChainID ids.ID, keys [][]byte) ([][]byte, error) {
	sharedID := sm.m.sharedID(peerChainID, sm.thisChainID)
	_, db := sm.m.GetDatabase(sharedID)
	defer sm.m.ReleaseDatabase(sharedID)

	s := state{
		c: sm.m.codec,
	}
	if bytes.Compare(sm.thisChainID[:], peerChainID[:]) == -1 {
		s.valueDB = prefixdb.New(smallerValuePrefix, db)
	} else {
		s.valueDB = prefixdb.New(largerValuePrefix, db)
	}

	values := make([][]byte, len(keys))
	for i, key := range keys {
		elem, err := s.Value(key)
		if err != nil {
			return nil, err
		}
		values[i] = elem.Value
	}
	return values, nil
}

func (sm *sharedMemory) Indexed(
	peerChainID ids.ID,
	traits [][]byte,
	startTrait,
	startKey []byte,
	limit int,
) ([][]byte, []byte, []byte, error) {
	sharedID := sm.m.sharedID(peerChainID, sm.thisChainID)
	_, db := sm.m.GetDatabase(sharedID)
	defer sm.m.ReleaseDatabase(sharedID)

	s := state{
		c: sm.m.codec,
	}
	if bytes.Compare(sm.thisChainID[:], peerChainID[:]) == -1 {
		s.valueDB = prefixdb.New(smallerValuePrefix, db)
		s.indexDB = prefixdb.New(smallerIndexPrefix, db)
	} else {
		s.valueDB = prefixdb.New(largerValuePrefix, db)
		s.indexDB = prefixdb.New(largerIndexPrefix, db)
	}

	keys, lastTrait, lastKey, err := s.getKeys(traits, startTrait, startKey, limit)
	if err != nil {
		return nil, nil, nil, err
	}

	values := make([][]byte, len(keys))
	for i, key := range keys {
		elem, err := s.Value(key)
		if err != nil {
			return nil, nil, nil, err
		}
		values[i] = elem.Value
	}
	return values, lastTrait, lastKey, nil
}

func (sm *sharedMemory) Remove(peerChainID ids.ID, keys [][]byte, batches ...database.Batch) error {
	sharedID := sm.m.sharedID(peerChainID, sm.thisChainID)
	vdb, db := sm.m.GetDatabase(sharedID)
	defer sm.m.ReleaseDatabase(sharedID)

	s := state{
		c: sm.m.codec,
	}
	if bytes.Compare(sm.thisChainID[:], peerChainID[:]) == -1 {
		s.valueDB = prefixdb.New(smallerValuePrefix, db)
		s.indexDB = prefixdb.New(smallerIndexPrefix, db)
	} else {
		s.valueDB = prefixdb.New(largerValuePrefix, db)
		s.indexDB = prefixdb.New(largerIndexPrefix, db)
	}

	for _, key := range keys {
		if err := s.RemoveValue(key); err != nil {
			return err
		}
	}

	myBatch, err := vdb.CommitBatch()
	if err != nil {
		return err
	}
	return WriteAll(myBatch, batches...)
}

type state struct {
	c       codec.Manager
	valueDB database.Database
	indexDB database.Database
}

func (s *state) Value(key []byte) (*Element, error) {
	value, err := s.loadValue(key)
	if err != nil {
		return nil, err
	}

	if !value.Present {
		return nil, database.ErrNotFound
	}

	return &Element{
		Key:    key,
		Value:  value.Value,
		Traits: value.Traits,
	}, nil
}

func (s *state) SetValue(e *Element) error {
	value, err := s.loadValue(e.Key)
	if err == nil {
		// The key was already registered with the state.

		if !value.Present {
			// This was previously optimistically deleted from the database, so
			// it should be immediately removed.
			return s.valueDB.Delete(e.Key)
		}

		// This key was written twice, which is invalid
		return errDuplicatedOperation
	}
	if err != database.ErrNotFound {
		// An unexpected error occurred, so we should propagate that error
		return err
	}

	for _, trait := range e.Traits {
		traitDB := prefixdb.New(trait, s.indexDB)
		if err := traitDB.Put(e.Key, nil); err != nil {
			return err
		}
	}

	dbElem := dbElement{
		Present: true,
		Value:   e.Value,
		Traits:  e.Traits,
	}

	valueBytes, err := s.c.Marshal(codecVersion, &dbElem)
	if err != nil {
		return err
	}
	return s.valueDB.Put(e.Key, valueBytes)
}

func (s *state) RemoveValue(key []byte) error {
	value, err := s.loadValue(key)
	if err != nil {
		if err != database.ErrNotFound {
			// An unexpected error occurred, so we should propagate that error
			return err
		}

		// The value doesn't exist, so we should optimistically deleted it
		dbElem := dbElement{Present: false}
		valueBytes, err := s.c.Marshal(codecVersion, &dbElem)
		if err != nil {
			return err
		}
		return s.valueDB.Put(key, valueBytes)
	}

	// Don't allow the removal of something that was already removed.
	if !value.Present {
		return errDuplicatedOperation
	}

	for _, trait := range value.Traits {
		traitDB := prefixdb.New(trait, s.indexDB)
		if err := traitDB.Delete(key); err != nil {
			return err
		}
	}
	return s.valueDB.Delete(key)
}

func (s *state) loadValue(key []byte) (*dbElement, error) {
	valueBytes, err := s.valueDB.Get(key)
	if err != nil {
		return nil, err
	}

	// The key was in the database
	value := &dbElement{}
	_, err = s.c.Unmarshal(valueBytes, value)
	return value, err
}

func (s *state) getKeys(traits [][]byte, startTrait, startKey []byte, limit int) ([][]byte, []byte, []byte, error) {
	tracked := ids.Set{}
	keys := [][]byte(nil)
	lastTrait := startTrait
	lastKey := startKey
	utils.Sort2DBytes(traits)
	for _, trait := range traits {
		switch bytes.Compare(trait, startTrait) {
		case -1:
			continue
		case 1:
			startKey = nil
		}

		lastTrait = trait
		var err error
		lastKey, err = s.appendTraitKeys(&keys, &tracked, &limit, trait, startKey)
		if err != nil {
			return nil, nil, nil, err
		}

		if limit == 0 {
			break
		}
	}
	return keys, lastTrait, lastKey, nil
}

func (s *state) appendTraitKeys(keys *[][]byte, tracked *ids.Set, limit *int, trait, startKey []byte) ([]byte, error) {
	lastKey := startKey

	traitDB := prefixdb.New(trait, s.indexDB)
	iter := traitDB.NewIteratorWithStart(startKey)
	defer iter.Release()
	for iter.Next() && *limit > 0 {
		key := iter.Key()
		lastKey = key

		id := hashing.ComputeHash256Array(key)
		if tracked.Contains(id) {
			continue
		}

		tracked.Add(id)
		*keys = append(*keys, key)
		*limit--
	}
	return lastKey, iter.Error()
}
