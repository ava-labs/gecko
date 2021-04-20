// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nodb

import (
	"github.com/ava-labs/avalanchego/database"
)

// Database is a lightning fast key value store with probabilistic operations.
type Database struct{}

// Has returns false, nil
func (*Database) Has([]byte) (bool, error) { return false, database.ErrClosed }

// Get returns nil, error
func (*Database) Get([]byte) ([]byte, error) { return nil, database.ErrClosed }

// Put returns nil
func (*Database) Put(_, _ []byte) error { return database.ErrClosed }

// Delete returns nil
func (*Database) Delete([]byte) error { return database.ErrClosed }

// NewBatch returns a new batch
func (*Database) NewBatch() database.Batch { return &Batch{} }

// NewIterator returns a new empty iterator
func (*Database) NewIterator() database.Iterator { return &Iterator{} }

// NewIteratorWithStart returns a new empty iterator
func (*Database) NewIteratorWithStart([]byte) database.Iterator { return &Iterator{} }

// NewIteratorWithPrefix returns a new empty iterator
func (*Database) NewIteratorWithPrefix([]byte) database.Iterator { return &Iterator{} }

// NewIteratorWithStartAndPrefix returns a new empty iterator
func (db *Database) NewIteratorWithStartAndPrefix(start, prefix []byte) database.Iterator {
	return &Iterator{}
}

// Stat returns an error
func (*Database) Stat(string) (string, error) { return "", database.ErrClosed }

// Compact returns nil
func (*Database) Compact(_, _ []byte) error { return database.ErrClosed }

// Close returns nil
func (*Database) Close() error { return database.ErrClosed }

// Batch does nothing
type Batch struct{}

// Put returns nil
func (*Batch) Put(_, _ []byte) error { return database.ErrClosed }

// Delete returns nil
func (*Batch) Delete([]byte) error { return database.ErrClosed }

// Size returns 0
func (*Batch) Size() int { return 0 }

// Write returns nil
func (*Batch) Write() error { return database.ErrClosed }

// Reset does nothing
func (*Batch) Reset() {}

// Replay does nothing
func (*Batch) Replay(database.KeyValueWriter) error { return database.ErrClosed }

// Inner returns itself
func (b *Batch) Inner() database.Batch { return b }

// Iterator does nothing
type Iterator struct{ Err error }

// Next returns false
func (*Iterator) Next() bool { return false }

// Error returns any errors
func (it *Iterator) Error() error { return it.Err }

// Key returns nil
func (*Iterator) Key() []byte { return nil }

// Value returns nil
func (*Iterator) Value() []byte { return nil }

// Release does nothing
func (*Iterator) Release() {}
