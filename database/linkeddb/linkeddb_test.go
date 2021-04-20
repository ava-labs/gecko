// (c) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package linkeddb

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/memdb"
)

func TestLinkedDB(t *testing.T) {
	assert := assert.New(t)

	db := memdb.New()
	ldb := NewDefault(db)

	key := []byte("hello")
	value := []byte("world")

	has, err := ldb.Has(key)
	assert.NoError(err)
	assert.False(has, "db unexpectedly had key %s", key)

	_, err = ldb.Get(key)
	assert.Equal(database.ErrNotFound, err, "Expected db.Get to return a Not Found error.")

	err = ldb.Delete(key)
	assert.NoError(err)

	err = ldb.Put(key, value)
	assert.NoError(err)

	has, err = ldb.Has(key)
	assert.NoError(err)
	assert.True(has, "db should have had key %s", key)

	v, err := ldb.Get(key)
	assert.NoError(err)
	assert.Equal(value, v)

	err = ldb.Delete(key)
	assert.NoError(err)

	has, err = ldb.Has(key)
	assert.NoError(err)
	assert.False(has, "db unexpectedly had key %s", key)

	_, err = ldb.Get(key)
	assert.Equal(database.ErrNotFound, err, "Expected db.Get to return a Not Found error.")

	iterator := db.NewIterator()
	next := iterator.Next()
	assert.False(next, "database should be empty")
	iterator.Release()
}

func TestLinkedDBDuplicatedPut(t *testing.T) {
	assert := assert.New(t)

	db := memdb.New()
	ldb := NewDefault(db)

	key := []byte("hello")
	value1 := []byte("world1")
	value2 := []byte("world2")

	err := ldb.Put(key, value1)
	assert.NoError(err)

	err = ldb.Put(key, value2)
	assert.NoError(err)

	v, err := ldb.Get(key)
	assert.NoError(err)
	assert.Equal(value2, v)

	err = ldb.Delete(key)
	assert.NoError(err)

	iterator := db.NewIterator()
	next := iterator.Next()
	assert.False(next, "database should be empty")
	iterator.Release()
}

func TestLinkedDBMultiplePuts(t *testing.T) {
	assert := assert.New(t)

	db := memdb.New()
	ldb := NewDefault(db)

	key1 := []byte("hello1")
	key2 := []byte("hello2")
	key3 := []byte("hello3")
	value1 := []byte("world1")
	value2 := []byte("world2")
	value3 := []byte("world3")

	err := ldb.Put(key1, value1)
	assert.NoError(err)

	err = ldb.Put(key2, value2)
	assert.NoError(err)

	v, err := ldb.Get(key1)
	assert.NoError(err)
	assert.Equal(value1, v)

	v, err = ldb.Get(key2)
	assert.NoError(err)
	assert.Equal(value2, v)

	err = ldb.Delete(key2)
	assert.NoError(err)

	err = ldb.Put(key2, value2)
	assert.NoError(err)

	err = ldb.Put(key3, value3)
	assert.NoError(err)

	err = ldb.Delete(key2)
	assert.NoError(err)

	err = ldb.Delete(key1)
	assert.NoError(err)

	err = ldb.Delete(key3)
	assert.NoError(err)

	iterator := db.NewIterator()
	next := iterator.Next()
	assert.False(next, "database should be empty")
	iterator.Release()
}

func TestEmptyLinkedDBIterator(t *testing.T) {
	assert := assert.New(t)

	db := memdb.New()
	ldb := NewDefault(db)

	iterator := ldb.NewIterator()
	next := iterator.Next()
	assert.False(next, "The iterator should now be exhausted")

	k := iterator.Key()
	assert.Nil(k, "The iterator returned the wrong key")

	v := iterator.Value()
	assert.Nil(v, "The iterator returned the wrong value")

	err := iterator.Error()
	assert.NoError(err)

	iterator.Release()
}

func TestLinkedDBLoadHeadKey(t *testing.T) {
	assert := assert.New(t)

	db := memdb.New()
	ldb := NewDefault(db)

	key := []byte("hello")
	value := []byte("world")

	err := ldb.Put(key, value)
	assert.NoError(err)

	ldb = NewDefault(db)

	iterator := ldb.NewIterator()
	next := iterator.Next()
	assert.True(next, "The iterator shouldn't be exhausted yet")

	k := iterator.Key()
	assert.Equal(key, k, "The iterator returned the wrong key")

	v := iterator.Value()
	assert.Equal(value, v, "The iterator returned the wrong value")

	next = iterator.Next()
	assert.False(next, "The iterator should now be exhausted")

	k = iterator.Key()
	assert.Nil(k, "The iterator returned the wrong key")

	v = iterator.Value()
	assert.Nil(v, "The iterator returned the wrong value")

	err = iterator.Error()
	assert.NoError(err)

	iterator.Release()
}

func TestSingleLinkedDBIterator(t *testing.T) {
	assert := assert.New(t)

	db := memdb.New()
	ldb := NewDefault(db)

	key := []byte("hello")
	value := []byte("world")

	err := ldb.Put(key, value)
	assert.NoError(err)

	iterator := ldb.NewIterator()
	next := iterator.Next()
	assert.True(next, "The iterator shouldn't be exhausted yet")

	k := iterator.Key()
	assert.Equal(key, k, "The iterator returned the wrong key")

	v := iterator.Value()
	assert.Equal(value, v, "The iterator returned the wrong value")

	next = iterator.Next()
	assert.False(next, "The iterator should now be exhausted")

	k = iterator.Key()
	assert.Nil(k, "The iterator returned the wrong key")

	v = iterator.Value()
	assert.Nil(v, "The iterator returned the wrong value")

	err = iterator.Error()
	assert.NoError(err)

	iterator.Release()
}

func TestMultipleLinkedDBIterator(t *testing.T) {
	assert := assert.New(t)

	db := memdb.New()
	ldb := NewDefault(db)

	key0 := []byte("hello0")
	key1 := []byte("hello1")
	value0 := []byte("world0")
	value1 := []byte("world1")

	err := ldb.Put(key0, value0)
	assert.NoError(err)

	err = ldb.Put(key1, value1)
	assert.NoError(err)

	iterator := ldb.NewIterator()
	next := iterator.Next()
	assert.True(next, "The iterator shouldn't be exhausted yet")

	k := iterator.Key()
	assert.Equal(key1, k, "The iterator returned the wrong key")

	v := iterator.Value()
	assert.Equal(value1, v, "The iterator returned the wrong value")

	next = iterator.Next()
	assert.True(next, "The iterator shouldn't be exhausted yet")

	k = iterator.Key()
	assert.Equal(key0, k, "The iterator returned the wrong key")

	v = iterator.Value()
	assert.Equal(value0, v, "The iterator returned the wrong value")

	next = iterator.Next()
	assert.False(next, "The iterator should now be exhausted")

	err = iterator.Error()
	assert.NoError(err)

	iterator.Release()
}

func TestMultipleLinkedDBIteratorStart(t *testing.T) {
	assert := assert.New(t)

	db := memdb.New()
	ldb := NewDefault(db)

	key0 := []byte("hello0")
	key1 := []byte("hello1")
	value0 := []byte("world0")
	value1 := []byte("world1")

	err := ldb.Put(key0, value0)
	assert.NoError(err)

	err = ldb.Put(key1, value1)
	assert.NoError(err)

	iterator := ldb.NewIteratorWithStart(key1)
	next := iterator.Next()
	assert.True(next, "The iterator shouldn't be exhausted yet")

	k := iterator.Key()
	assert.Equal(key1, k, "The iterator returned the wrong key")

	v := iterator.Value()
	assert.Equal(value1, v, "The iterator returned the wrong value")

	next = iterator.Next()
	assert.True(next, "The iterator shouldn't be exhausted yet")

	k = iterator.Key()
	assert.Equal(key0, k, "The iterator returned the wrong key")

	v = iterator.Value()
	assert.Equal(value0, v, "The iterator returned the wrong value")

	next = iterator.Next()
	assert.False(next, "The iterator should now be exhausted")

	err = iterator.Error()
	assert.NoError(err)

	iterator.Release()
}

func TestSingleLinkedDBIteratorStart(t *testing.T) {
	assert := assert.New(t)

	db := memdb.New()
	ldb := NewDefault(db)

	key0 := []byte("hello0")
	key1 := []byte("hello1")
	value0 := []byte("world0")
	value1 := []byte("world1")

	err := ldb.Put(key0, value0)
	assert.NoError(err)

	err = ldb.Put(key1, value1)
	assert.NoError(err)

	iterator := ldb.NewIteratorWithStart(key0)

	next := iterator.Next()
	assert.True(next, "The iterator shouldn't be exhausted yet")

	k := iterator.Key()
	assert.Equal(key0, k, "The iterator returned the wrong key")

	v := iterator.Value()
	assert.Equal(value0, v, "The iterator returned the wrong value")

	next = iterator.Next()
	assert.False(next, "The iterator should now be exhausted")

	err = iterator.Error()
	assert.NoError(err)

	iterator.Release()
}

func TestEmptyLinkedDBIteratorStart(t *testing.T) {
	assert := assert.New(t)

	db := memdb.New()
	ldb := NewDefault(db)

	key0 := []byte("hello0")
	key1 := []byte("hello1")
	key2 := []byte("hello2")
	value0 := []byte("world0")
	value1 := []byte("world1")

	err := ldb.Put(key0, value0)
	assert.NoError(err)

	err = ldb.Put(key1, value1)
	assert.NoError(err)

	iterator := ldb.NewIteratorWithStart(key2)

	next := iterator.Next()
	assert.False(next, "The iterator should now be exhausted")

	err = iterator.Error()
	assert.NoError(err)

	iterator.Release()
}
