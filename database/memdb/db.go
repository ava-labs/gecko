// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package memdb

import (
	"sort"
	"strings"
	"sync"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/nodb"
	"github.com/ava-labs/avalanchego/utils"
)

const (
	// DefaultSize is the default initial size of the memory database
	DefaultSize = 1 << 10
)

// Database is an ephemeral key-value store that implements the Database
// interface.
type Database struct {
	lock sync.RWMutex
	db   map[string][]byte
}

// New returns a map with the Database interface methods implemented.
func New() *Database { return NewWithSize(DefaultSize) }

// NewWithSize returns a map pre-allocated to the provided size with the
// Database interface methods implemented.
func NewWithSize(size int) *Database { return &Database{db: make(map[string][]byte, size)} }

// Close implements the Database interface
func (db *Database) Close() error {
	db.lock.Lock()
	defer db.lock.Unlock()

	if db.db == nil {
		return database.ErrClosed
	}
	db.db = nil
	return nil
}

// Has implements the Database interface
func (db *Database) Has(key []byte) (bool, error) {
	db.lock.RLock()
	defer db.lock.RUnlock()

	if db.db == nil {
		return false, database.ErrClosed
	}
	_, ok := db.db[string(key)]
	return ok, nil
}

// Get implements the Database interface
func (db *Database) Get(key []byte) ([]byte, error) {
	db.lock.RLock()
	defer db.lock.RUnlock()

	if db.db == nil {
		return nil, database.ErrClosed
	}
	if entry, ok := db.db[string(key)]; ok {
		return utils.CopyBytes(entry), nil
	}
	return nil, database.ErrNotFound
}

// Put implements the Database interface
func (db *Database) Put(key []byte, value []byte) error {
	db.lock.Lock()
	defer db.lock.Unlock()

	if db.db == nil {
		return database.ErrClosed
	}
	db.db[string(key)] = utils.CopyBytes(value)
	return nil
}

// Delete implements the Database interface
func (db *Database) Delete(key []byte) error {
	db.lock.Lock()
	defer db.lock.Unlock()

	if db.db == nil {
		return database.ErrClosed
	}
	delete(db.db, string(key))
	return nil
}

// NewBatch implements the Database interface
func (db *Database) NewBatch() database.Batch { return &batch{db: db} }

// NewIterator implements the Database interface
func (db *Database) NewIterator() database.Iterator {
	return db.NewIteratorWithStartAndPrefix(nil, nil)
}

// NewIteratorWithStart implements the Database interface
func (db *Database) NewIteratorWithStart(start []byte) database.Iterator {
	return db.NewIteratorWithStartAndPrefix(start, nil)
}

// NewIteratorWithPrefix implements the Database interface
func (db *Database) NewIteratorWithPrefix(prefix []byte) database.Iterator {
	return db.NewIteratorWithStartAndPrefix(nil, prefix)
}

// NewIteratorWithStartAndPrefix implements the Database interface
func (db *Database) NewIteratorWithStartAndPrefix(start, prefix []byte) database.Iterator {
	db.lock.RLock()
	defer db.lock.RUnlock()

	if db.db == nil {
		return &nodb.Iterator{Err: database.ErrClosed}
	}

	startString := string(start)
	prefixString := string(prefix)
	keys := make([]string, 0, len(db.db))
	for key := range db.db {
		if strings.HasPrefix(key, prefixString) && key >= startString {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys) // Keys need to be in sorted order
	values := make([][]byte, 0, len(keys))
	for _, key := range keys {
		values = append(values, db.db[key])
	}
	return &iterator{
		keys:   keys,
		values: values,
	}
}

// Stat implements the Database interface
func (db *Database) Stat(property string) (string, error) { return "", database.ErrNotFound }

// Compact implements the Database interface
func (db *Database) Compact(start []byte, limit []byte) error {
	db.lock.RLock()
	defer db.lock.RUnlock()

	if db.db == nil {
		return database.ErrClosed
	}
	return nil
}

type keyValue struct {
	key    []byte
	value  []byte
	delete bool
}

type batch struct {
	db     *Database
	writes []keyValue
	size   int
}

func (b *batch) Put(key, value []byte) error {
	b.writes = append(b.writes, keyValue{utils.CopyBytes(key), utils.CopyBytes(value), false})
	b.size += len(key) + len(value)
	return nil
}

func (b *batch) Delete(key []byte) error {
	b.writes = append(b.writes, keyValue{utils.CopyBytes(key), nil, true})
	b.size += len(key)
	return nil
}

// Size implements the Batch interface
func (b *batch) Size() int { return b.size }

// Write implements the Batch interface
func (b *batch) Write() error {
	b.db.lock.Lock()
	defer b.db.lock.Unlock()

	if b.db.db == nil {
		return database.ErrClosed
	}

	for _, kv := range b.writes {
		key := string(kv.key)
		if kv.delete {
			delete(b.db.db, key)
		} else {
			b.db.db[key] = kv.value
		}
	}
	return nil
}

// Reset implements the Batch interface
func (b *batch) Reset() {
	if cap(b.writes) > len(b.writes)*database.MaxExcessCapacityFactor {
		b.writes = make([]keyValue, 0, cap(b.writes)/database.CapacityReductionFactor)
	} else {
		b.writes = b.writes[:0]
	}
	b.size = 0
}

// Replay implements the Batch interface
func (b *batch) Replay(w database.KeyValueWriter) error {
	for _, keyvalue := range b.writes {
		if keyvalue.delete {
			if err := w.Delete(keyvalue.key); err != nil {
				return err
			}
		} else if err := w.Put(keyvalue.key, keyvalue.value); err != nil {
			return err
		}
	}
	return nil
}

// Inner returns itself
func (b *batch) Inner() database.Batch { return b }

type iterator struct {
	initialized bool
	keys        []string
	values      [][]byte
}

// Next implements the Iterator interface
func (it *iterator) Next() bool {
	// If the iterator was not yet initialized, do it now
	if !it.initialized {
		it.initialized = true
		return len(it.keys) > 0
	}
	// Iterator already initialize, advance it
	if len(it.keys) > 0 {
		it.keys = it.keys[1:]
		it.values = it.values[1:]
	}
	return len(it.keys) > 0
}

// Error implements the Iterator interface
func (it *iterator) Error() error { return nil }

// Key implements the Iterator interface
func (it *iterator) Key() []byte {
	if len(it.keys) > 0 {
		return []byte(it.keys[0])
	}
	return nil
}

// Value implements the Iterator interface
func (it *iterator) Value() []byte {
	if len(it.values) > 0 {
		return it.values[0]
	}
	return nil
}

// Release implements the Iterator interface
func (it *iterator) Release() { it.keys = nil; it.values = nil }
