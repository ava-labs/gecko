// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package meterdb

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/utils/timer"
)

// Database tracks the amount of time each operation takes and how many bytes
// are read/written to the underlying database instance.
type Database struct {
	metrics
	db    database.Database
	clock timer.Clock
}

// New returns a new database with added metrics
func New(
	namespace string,
	registerer prometheus.Registerer,
	db database.Database,
) (*Database, error) {
	meterDB := &Database{db: db}
	return meterDB, meterDB.metrics.Initialize(namespace, registerer)
}

func (db *Database) Has(key []byte) (bool, error) {
	start := db.clock.Time()
	has, err := db.db.Has(key)
	end := db.clock.Time()
	db.readSize.Observe(float64(len(key)))
	db.has.Observe(float64(end.Sub(start)))
	db.hasSize.Observe(float64(len(key)))
	return has, err
}

func (db *Database) Get(key []byte) ([]byte, error) {
	start := db.clock.Time()
	value, err := db.db.Get(key)
	end := db.clock.Time()
	db.readSize.Observe(float64(len(key) + len(value)))
	db.get.Observe(float64(end.Sub(start)))
	db.getSize.Observe(float64(len(key) + len(value)))
	return value, err
}

func (db *Database) Put(key, value []byte) error {
	start := db.clock.Time()
	err := db.db.Put(key, value)
	end := db.clock.Time()
	db.writeSize.Observe(float64(len(key) + len(value)))
	db.put.Observe(float64(end.Sub(start)))
	db.putSize.Observe(float64(len(key) + len(value)))
	return err
}

func (db *Database) Delete(key []byte) error {
	start := db.clock.Time()
	err := db.db.Delete(key)
	end := db.clock.Time()
	db.writeSize.Observe(float64(len(key)))
	db.delete.Observe(float64(end.Sub(start)))
	db.deleteSize.Observe(float64(len(key)))
	return err
}

func (db *Database) NewBatch() database.Batch {
	start := db.clock.Time()
	b := &batch{
		batch: db.db.NewBatch(),
		db:    db,
	}
	end := db.clock.Time()
	db.newBatch.Observe(float64(end.Sub(start)))
	return b
}

func (db *Database) NewIterator() database.Iterator {
	return db.NewIteratorWithStartAndPrefix(nil, nil)
}

func (db *Database) NewIteratorWithStart(start []byte) database.Iterator {
	return db.NewIteratorWithStartAndPrefix(start, nil)
}

func (db *Database) NewIteratorWithPrefix(prefix []byte) database.Iterator {
	return db.NewIteratorWithStartAndPrefix(nil, prefix)
}

func (db *Database) NewIteratorWithStartAndPrefix(
	start,
	prefix []byte,
) database.Iterator {
	startTime := db.clock.Time()
	it := &iterator{
		iterator: db.db.NewIteratorWithStartAndPrefix(start, prefix),
		db:       db,
	}
	end := db.clock.Time()
	db.newIterator.Observe(float64(end.Sub(startTime)))
	return it
}

func (db *Database) Stat(stat string) (string, error) {
	start := db.clock.Time()
	result, err := db.db.Stat(stat)
	end := db.clock.Time()
	db.stat.Observe(float64(end.Sub(start)))
	return result, err
}

func (db *Database) Compact(start, limit []byte) error {
	startTime := db.clock.Time()
	err := db.db.Compact(start, limit)
	end := db.clock.Time()
	db.compact.Observe(float64(end.Sub(startTime)))
	return err
}

func (db *Database) Close() error {
	start := db.clock.Time()
	err := db.db.Close()
	end := db.clock.Time()
	db.close.Observe(float64(end.Sub(start)))
	return err
}

type batch struct {
	batch database.Batch
	db    *Database
}

func (b *batch) Put(key, value []byte) error {
	start := b.db.clock.Time()
	err := b.batch.Put(key, value)
	end := b.db.clock.Time()
	b.db.bPut.Observe(float64(end.Sub(start)))
	b.db.bPutSize.Observe(float64(len(key) + len(value)))
	return err
}

func (b *batch) Delete(key []byte) error {
	start := b.db.clock.Time()
	err := b.batch.Delete(key)
	end := b.db.clock.Time()
	b.db.bDelete.Observe(float64(end.Sub(start)))
	b.db.bDeleteSize.Observe(float64(len(key)))
	return err
}

func (b *batch) Size() int {
	start := b.db.clock.Time()
	size := b.batch.Size()
	end := b.db.clock.Time()
	b.db.bSize.Observe(float64(end.Sub(start)))
	return size
}

func (b *batch) Write() error {
	start := b.db.clock.Time()
	err := b.batch.Write()
	end := b.db.clock.Time()
	batchSize := float64(b.batch.Size())
	b.db.writeSize.Observe(batchSize)
	b.db.bWrite.Observe(float64(end.Sub(start)))
	b.db.bWriteSize.Observe(batchSize)
	return err
}

func (b *batch) Reset() {
	start := b.db.clock.Time()
	b.batch.Reset()
	end := b.db.clock.Time()
	b.db.bReset.Observe(float64(end.Sub(start)))
}

func (b *batch) Replay(w database.KeyValueWriter) error {
	start := b.db.clock.Time()
	err := b.batch.Replay(w)
	end := b.db.clock.Time()
	b.db.bReplay.Observe(float64(end.Sub(start)))
	return err
}

func (b *batch) Inner() database.Batch {
	start := b.db.clock.Time()
	inner := b.batch.Inner()
	end := b.db.clock.Time()
	b.db.bInner.Observe(float64(end.Sub(start)))
	return inner
}

type iterator struct {
	iterator database.Iterator
	db       *Database
}

func (it *iterator) Next() bool {
	start := it.db.clock.Time()
	next := it.iterator.Next()
	end := it.db.clock.Time()
	it.db.iNext.Observe(float64(end.Sub(start)))
	size := float64(len(it.iterator.Key()) + len(it.iterator.Value()))
	it.db.readSize.Observe(size)
	it.db.iNextSize.Observe(size)
	return next
}

func (it *iterator) Error() error {
	start := it.db.clock.Time()
	err := it.iterator.Error()
	end := it.db.clock.Time()
	it.db.iError.Observe(float64(end.Sub(start)))
	return err
}

func (it *iterator) Key() []byte {
	start := it.db.clock.Time()
	key := it.iterator.Key()
	end := it.db.clock.Time()
	it.db.iKey.Observe(float64(end.Sub(start)))
	return key
}

func (it *iterator) Value() []byte {
	start := it.db.clock.Time()
	value := it.iterator.Value()
	end := it.db.clock.Time()
	it.db.iValue.Observe(float64(end.Sub(start)))
	return value
}

func (it *iterator) Release() {
	start := it.db.clock.Time()
	it.iterator.Release()
	end := it.db.clock.Time()
	it.db.iRelease.Observe(float64(end.Sub(start)))
}
