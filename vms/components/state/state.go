// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"errors"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

const cacheSize = 1000

var errWrongType = errors.New("value in the database was the wrong type")

// Marshaller can marshal itself to bytes
type Marshaller interface {
	Bytes() []byte
}

// State is a key-value store where every value is associated with a "type ID".
// Every different type of value must have its own type ID.
//
// For example, if you're storing blocks, accounts and addresses, each of those types
// must have their own type ID.
//
// Each type ID is associated with a function that specifies how to unmarshal bytes
// to a struct/value of a given type.
//
// State has built-in support for putting and getting choices.Status and ids.ID
// To put/get any other type, you must first register that type using RegisterType
type State interface {
	// In [db], add a key-value pair.
	// [value] will be converted to bytes by calling Bytes() on it.
	// [typeID] must have already been registered using RegisterType.
	// If [value] is nil, the value associated with [key] and [typeID] is deleted (if it exists).
	Put(db database.Database, typeID uint64, key ids.ID, value interface{}) error

	// From [db], get the value of type [typeID] whose key is [key]
	// Returns database.ErrNotFound if the entry doesn't exist
	Get(db database.Database, typeID uint64, key ids.ID) (interface{}, error)

	// Return whether [key] exists in [db] for type [typeID]
	Has(db database.Database, typeID uint64, key ids.ID) (bool, error)

	// PutStatus associates [key] with [status] in [db]
	PutStatus(db database.Database, key ids.ID, status choices.Status) error

	// GetStatus gets the status associated with [key] in [db]
	GetStatus(db database.Database, key ids.ID) choices.Status

	// PutID associates [key] with [ID] in [db]
	PutID(db database.Database, key ids.ID, ID ids.ID) error

	// GetID gets the ID associated with [key] in [db]
	GetID(db database.Database, key ids.ID) (ids.ID, error)

	// PutTime associates [key] with [time] in [db]
	PutTime(db database.Database, key ids.ID, time time.Time) error

	// GetTime gets the time associated with [key] in [db]
	GetTime(db database.Database, key ids.ID) (time.Time, error)

	// Register a new type.
	// When values that were Put with [typeID] are retrieved from the database,
	// they will be unmarshaled from bytes using [unmarshal].
	// Returns an error if there is already a type with ID [typeID]
	RegisterType(typeID uint64,
		marshal func(interface{}) ([]byte, error),
		unmarshal func([]byte) (interface{}, error),
	) error
}

type state struct {
	// Keys:   Type ID
	// Values: Function that unmarshals values
	//         that were Put with that type ID
	unmarshallers map[uint64]func([]byte) (interface{}, error)

	// Keys:   Type ID
	// Values: Function that marshals values
	//         that were Put with that type ID
	marshallers map[uint64]func(interface{}) ([]byte, error)

	// Keys:   Type ID
	// Values: Cache that stores uniqueIDs for values that were put with that type ID
	//         (Saves us from having to re-compute uniqueIDs)
	uniqueIDCaches map[uint64]*cache.LRU
}

// Implements State.RegisterType
func (s *state) RegisterType(
	typeID uint64,
	marshal func(interface{}) ([]byte, error),
	unmarshal func([]byte) (interface{}, error),
) error {

	if _, exists := s.unmarshallers[typeID]; exists {
		return fmt.Errorf("there is already a type with ID %d", typeID)
	}
	s.marshallers[typeID] = marshal
	s.unmarshallers[typeID] = unmarshal
	return nil
}

// Implements State.Put
func (s *state) Put(db database.Database, typeID uint64, key ids.ID, value interface{}) error {
	marshaller, exists := s.marshallers[typeID]
	if !exists {
		return fmt.Errorf("typeID %d has not been registered", typeID)
	}
	// Get the unique ID of thie key/typeID pair
	uID := s.uniqueID(key, typeID)
	if value == nil {
		return db.Delete(uID.Bytes())
	}
	// Put the byte repr. of the value in the database
	valueBytes, err := marshaller(value)
	if err != nil {
		return fmt.Errorf("couldn't marshal value: %w", err)
	}
	return db.Put(uID.Bytes(), valueBytes)
}

func (s *state) Has(db database.Database, typeID uint64, key ids.ID) (bool, error) {
	return db.Has(s.uniqueID(key, typeID).Bytes())
}

// Implements State.Get
func (s *state) Get(db database.Database, typeID uint64, key ids.ID) (interface{}, error) {
	unmarshal, exists := s.unmarshallers[typeID]
	if !exists {
		return nil, fmt.Errorf("typeID %d has not been registered", typeID)
	}

	// The unique ID of this key/typeID pair
	uID := s.uniqueID(key, typeID)

	// Get the value from the database
	valueBytes, err := db.Get(uID.Bytes())
	if err != nil {
		return nil, err
	}

	// Unmarshal the value from bytes and return it
	return unmarshal(valueBytes)
}

// PutStatus associates [key] with [status] in [db]
func (s *state) PutStatus(db database.Database, key ids.ID, status choices.Status) error {
	return s.Put(db, StatusTypeID, key, status)
}

// GetStatus gets the status associated with [key] in [db]
// Return choices.Processing if can't get the status from database
func (s *state) GetStatus(db database.Database, key ids.ID) choices.Status {
	statusInterface, err := s.Get(db, StatusTypeID, key)
	if err != nil {
		return choices.Processing
	}

	status, ok := statusInterface.(choices.Status)
	if !ok || status.Valid() != nil {
		return choices.Processing
	}

	return status
}

// PutID associates [key] with [ID] in [db]
func (s *state) PutID(db database.Database, key ids.ID, ID ids.ID) error {
	return s.Put(db, IDTypeID, key, ID)
}

// GetID gets the ID associated with [key] in [db]
func (s *state) GetID(db database.Database, key ids.ID) (ids.ID, error) {
	IDInterface, err := s.Get(db, IDTypeID, key)
	if err != nil {
		return ids.ID{}, err
	}

	if ID, ok := IDInterface.(ids.ID); ok {
		return ID, nil
	}

	return ids.ID{}, errWrongType
}

// PutTime associates [key] with [time] in [db]
func (s *state) PutTime(db database.Database, key ids.ID, time time.Time) error {
	return s.Put(db, TimeTypeID, key, time)
}

// GetTime gets the time associated with [key] in [db]
func (s *state) GetTime(db database.Database, key ids.ID) (time.Time, error) {
	timeInterface, err := s.Get(db, TimeTypeID, key)
	if err != nil {
		return time.Time{}, err
	}

	if time, ok := timeInterface.(time.Time); ok {
		return time, nil
	}

	return time.Time{}, errWrongType
}

// Prefix [ID] with [typeID] to prevent key collisions in the database
func (s *state) uniqueID(ID ids.ID, typeID uint64) ids.ID {
	uIDCache, cacheExists := s.uniqueIDCaches[typeID]
	if cacheExists {
		if uID, uIDExists := uIDCache.Get(ID); uIDExists { // Get the uniqueID associated with [typeID] and [ID]
			return uID.(ids.ID)
		}
	} else {
		s.uniqueIDCaches[typeID] = &cache.LRU{Size: cacheSize}
	}
	uID := ID.Prefix(typeID)
	s.uniqueIDCaches[typeID].Put(ID, uID)
	return uID
}

// NewState returns a new State
func NewState() (State, error) {
	state := &state{
		marshallers:    make(map[uint64]func(interface{}) ([]byte, error)),
		unmarshallers:  make(map[uint64]func([]byte) (interface{}, error)),
		uniqueIDCaches: make(map[uint64]*cache.LRU),
	}

	// Register ID, Status and time.Time so they can be put/get without client code
	// having to register them

	errs := wrappers.Errs{}
	errs.Add(
		state.RegisterType(IDTypeID, marshalID, unmarshalID),
		state.RegisterType(StatusTypeID, marshalStatus, unmarshalStatus),
		state.RegisterType(TimeTypeID, marshalTime, unmarshalTime),
	)
	return state, errs.Err
}
