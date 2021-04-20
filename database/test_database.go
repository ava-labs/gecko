// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package database

import (
	"bytes"
	"testing"
)

var (
	// Tests is a list of all database tests
	Tests = []func(t *testing.T, db Database){
		TestSimpleKeyValue,
		TestSimpleKeyValueClosed,
		TestBatchPut,
		TestBatchDelete,
		TestBatchReset,
		TestBatchReuse,
		TestBatchRewrite,
		TestBatchReplay,
		TestBatchInner,
		TestIterator,
		TestIteratorStart,
		TestIteratorPrefix,
		TestIteratorStartPrefix,
		TestIteratorMemorySafety,
		TestIteratorClosed,
		TestIteratorError,
		TestIteratorErrorAfterRelease,
		TestStatNoPanic,
		TestCompactNoPanic,
		TestMemorySafetyDatabase,
		TestMemorySafetyBatch,
	}
)

// TestSimpleKeyValue tests to make sure that simple Put + Get + Delete + Has
// calls return the expected values.
func TestSimpleKeyValue(t *testing.T, db Database) {
	key := []byte("hello")
	value := []byte("world")

	if has, err := db.Has(key); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if has {
		t.Fatalf("db.Has unexpectedly returned true on key %s", key)
	} else if v, err := db.Get(key); err != ErrNotFound {
		t.Fatalf("Expected %s on db.Get for missing key %s. Returned 0x%x", ErrNotFound, key, v)
	} else if err := db.Delete(key); err != nil {
		t.Fatalf("Unexpected error on db.Delete: %s", err)
	}

	if err := db.Put(key, value); err != nil {
		t.Fatalf("Unexpected error on db.Put: %s", err)
	}

	if has, err := db.Has(key); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if !has {
		t.Fatalf("db.Has unexpectedly returned false on key %s", key)
	} else if v, err := db.Get(key); err != nil {
		t.Fatalf("Unexpected error on db.Get: %s", err)
	} else if !bytes.Equal(value, v) {
		t.Fatalf("db.Get: Returned: 0x%x ; Expected: 0x%x", v, value)
	}

	if err := db.Delete(key); err != nil {
		t.Fatalf("Unexpected error on db.Delete: %s", err)
	}

	if has, err := db.Has(key); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if has {
		t.Fatalf("db.Has unexpectedly returned true on key %s", key)
	} else if v, err := db.Get(key); err != ErrNotFound {
		t.Fatalf("Expected %s on db.Get for missing key %s. Returned 0x%x", ErrNotFound, key, v)
	} else if err := db.Delete(key); err != nil {
		t.Fatalf("Unexpected error on db.Delete: %s", err)
	}
}

// TestSimpleKeyValueClosed tests to make sure that Put + Get + Delete + Has
// calls return the correct error when the database has been closed.
func TestSimpleKeyValueClosed(t *testing.T, db Database) {
	key := []byte("hello")
	value := []byte("world")

	if has, err := db.Has(key); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if has {
		t.Fatalf("db.Has unexpectedly returned true on key %s", key)
	} else if v, err := db.Get(key); err != ErrNotFound {
		t.Fatalf("Expected %s on db.Get for missing key %s. Returned 0x%x", ErrNotFound, key, v)
	} else if err := db.Delete(key); err != nil {
		t.Fatalf("Unexpected error on db.Delete: %s", err)
	}

	if err := db.Put(key, value); err != nil {
		t.Fatalf("Unexpected error on db.Put: %s", err)
	}

	if has, err := db.Has(key); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if !has {
		t.Fatalf("db.Has unexpectedly returned false on key %s", key)
	} else if v, err := db.Get(key); err != nil {
		t.Fatalf("Unexpected error on db.Get: %s", err)
	} else if !bytes.Equal(value, v) {
		t.Fatalf("db.Get: Returned: 0x%x ; Expected: 0x%x", v, value)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Unexpected error on db.Close: %s", err)
	}

	if _, err := db.Has(key); err != ErrClosed {
		t.Fatalf("Expected %s on db.Has after close", ErrClosed)
	} else if _, err := db.Get(key); err != ErrClosed {
		t.Fatalf("Expected %s on db.Get after close", ErrClosed)
	} else if err := db.Put(key, value); err != ErrClosed {
		t.Fatalf("Expected %s on db.Put after close", ErrClosed)
	} else if err := db.Delete(key); err != ErrClosed {
		t.Fatalf("Expected %s on db.Delete after close", ErrClosed)
	} else if err := db.Close(); err != ErrClosed {
		t.Fatalf("Expected %s on db.Close after close", ErrClosed)
	}
}

// TestMemorySafetyDatabase ensures it is safe to modify a key after passing it
// to Database.Put and Database.Get.
func TestMemorySafetyDatabase(t *testing.T, db Database) {
	key := []byte("key")
	value := []byte("value")
	key2 := []byte("key2")
	value2 := []byte("value2")

	// Put both K/V pairs in the database
	if err := db.Put(key, value); err != nil {
		t.Fatal(err)
	} else if err := db.Put(key2, value2); err != nil {
		t.Fatal(err)
	}
	// Get the value for [key]
	gotVal, err := db.Get(key)
	if err != nil {
		t.Fatalf("should have been able to get value but got %s", err)
	} else if !bytes.Equal(gotVal, value) {
		t.Fatal("got the wrong value")
	}
	// Modify [key]; make sure the value we got before hasn't changed
	key = key2
	gotVal2, err := db.Get(key)
	switch {
	case err != nil:
		t.Fatal(err)
	case !bytes.Equal(gotVal2, value2):
		t.Fatal("got wrong value")
	case !bytes.Equal(gotVal, value):
		t.Fatal("value changed")
	}
	// Reset [key] to its original value and make sure it's correct
	key = []byte("key")
	gotVal, err = db.Get(key)
	if err != nil {
		t.Fatalf("should have been able to get value but got %s", err)
	} else if !bytes.Equal(gotVal, value) {
		t.Fatal("got the wrong value")
	}
}

// TestBatchPut tests to make sure that batched writes work as expected.
func TestBatchPut(t *testing.T, db Database) {
	key := []byte("hello")
	value := []byte("world")

	batch := db.NewBatch()
	if batch == nil {
		t.Fatalf("db.NewBatch returned nil")
	}

	if err := batch.Put(key, value); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if size := batch.Size(); size <= 0 {
		t.Fatalf("batch.Size: Returned: %d ; Expected: > 0", size)
	}

	if err := batch.Write(); err != nil {
		t.Fatalf("Unexpected error on batch.Write: %s", err)
	}

	if has, err := db.Has(key); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if !has {
		t.Fatalf("db.Has unexpectedly returned false on key %s", key)
	} else if v, err := db.Get(key); err != nil {
		t.Fatalf("Unexpected error on db.Get: %s", err)
	} else if !bytes.Equal(value, v) {
		t.Fatalf("db.Get: Returned: 0x%x ; Expected: 0x%x", v, value)
	} else if err := db.Delete(key); err != nil {
		t.Fatalf("Unexpected error on db.Delete: %s", err)
	}

	if batch = db.NewBatch(); batch == nil {
		t.Fatalf("db.NewBatch returned nil")
	} else if err := batch.Put(key, value); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Close(); err != nil {
		t.Fatalf("Error while closing the database: %s", err)
	} else if err := batch.Write(); err != ErrClosed {
		t.Fatalf("Expected %s on batch.Write", ErrClosed)
	}
}

// TestBatchDelete tests to make sure that batched deletes work as expected.
func TestBatchDelete(t *testing.T, db Database) {
	key := []byte("hello")
	value := []byte("world")

	if err := db.Put(key, value); err != nil {
		t.Fatalf("Unexpected error on db.Put: %s", err)
	}

	batch := db.NewBatch()
	if batch == nil {
		t.Fatalf("db.NewBatch returned nil")
	}

	if err := batch.Delete(key); err != nil {
		t.Fatalf("Unexpected error on batch.Delete: %s", err)
	}

	if err := batch.Write(); err != nil {
		t.Fatalf("Unexpected error on batch.Write: %s", err)
	}

	if has, err := db.Has(key); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if has {
		t.Fatalf("db.Has unexpectedly returned true on key %s", key)
	} else if v, err := db.Get(key); err != ErrNotFound {
		t.Fatalf("Expected %s on db.Get for missing key %s. Returned 0x%x", ErrNotFound, key, v)
	} else if err := db.Delete(key); err != nil {
		t.Fatalf("Unexpected error on db.Delete: %s", err)
	}
}

// TestMemorySafetyDatabase ensures it is safe to modify a key after passing it
// to Batch.Put.
func TestMemorySafetyBatch(t *testing.T, db Database) {
	key := []byte("hello")
	value := []byte("world")
	valueCopy := []byte("world")

	batch := db.NewBatch()
	if batch == nil {
		t.Fatalf("db.NewBatch returned nil")
	}

	// Put a key in the batch
	if err := batch.Put(key, value); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if size := batch.Size(); size <= 0 {
		t.Fatalf("batch.Size: Returned: %d ; Expected: > 0", size)
	}

	// Modify the key
	keyCopy := key
	key = []byte("jello")
	if err := batch.Write(); err != nil {
		t.Fatalf("Unexpected error on batch.Write: %s", err)
	}

	// Make sure the original key was written to the database
	if has, err := db.Has(keyCopy); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if !has {
		t.Fatalf("db.Has unexpectedly returned false on key %s", key)
	} else if v, err := db.Get(keyCopy); err != nil {
		t.Fatalf("Unexpected error on db.Get: %s", err)
	} else if !bytes.Equal(valueCopy, v) {
		t.Fatalf("db.Get: Returned: 0x%x ; Expected: 0x%x", v, value)
	}

	// Make sure the new key wasn't written to the database
	if has, err := db.Has(key); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if has {
		t.Fatal("database shouldn't have the new key")
	}
}

// TestBatchReset tests to make sure that a batch drops un-written operations
// when it is reset.
func TestBatchReset(t *testing.T, db Database) {
	key := []byte("hello")
	value := []byte("world")

	if err := db.Put(key, value); err != nil {
		t.Fatalf("Unexpected error on db.Put: %s", err)
	}

	batch := db.NewBatch()
	if batch == nil {
		t.Fatalf("db.NewBatch returned nil")
	}

	if err := batch.Delete(key); err != nil {
		t.Fatalf("Unexpected error on batch.Delete: %s", err)
	}

	batch.Reset()

	if err := batch.Write(); err != nil {
		t.Fatalf("Unexpected error on batch.Write: %s", err)
	}

	if has, err := db.Has(key); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if !has {
		t.Fatalf("db.Has unexpectedly returned false on key %s", key)
	} else if v, err := db.Get(key); err != nil {
		t.Fatalf("Unexpected error on db.Get: %s", err)
	} else if !bytes.Equal(value, v) {
		t.Fatalf("db.Get: Returned: 0x%x ; Expected: 0x%x", v, value)
	}
}

// TestBatchReuse tests to make sure that a batch can be reused once it is
// reset.
func TestBatchReuse(t *testing.T, db Database) {
	key1 := []byte("hello1")
	value1 := []byte("world1")

	key2 := []byte("hello2")
	value2 := []byte("world2")

	batch := db.NewBatch()
	if batch == nil {
		t.Fatalf("db.NewBatch returned nil")
	}

	if err := batch.Put(key1, value1); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	if err := batch.Write(); err != nil {
		t.Fatalf("Unexpected error on batch.Write: %s", err)
	}

	if err := db.Delete(key1); err != nil {
		t.Fatalf("Unexpected error on database.Delete: %s", err)
	}

	if has, err := db.Has(key1); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if has {
		t.Fatalf("db.Has unexpectedly returned true on key %s", key1)
	}

	batch.Reset()

	if err := batch.Put(key2, value2); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	if err := batch.Write(); err != nil {
		t.Fatalf("Unexpected error on batch.Write: %s", err)
	}

	if has, err := db.Has(key1); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if has {
		t.Fatalf("db.Has unexpectedly returned true on key %s", key1)
	} else if has, err := db.Has(key2); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if !has {
		t.Fatalf("db.Has unexpectedly returned false on key %s", key2)
	} else if v, err := db.Get(key2); err != nil {
		t.Fatalf("Unexpected error on db.Get: %s", err)
	} else if !bytes.Equal(value2, v) {
		t.Fatalf("db.Get: Returned: 0x%x ; Expected: 0x%x", v, value2)
	}
}

// TestBatchRewrite tests to make sure that write can be called multiple times
// on a batch and the values will be updated correctly.
func TestBatchRewrite(t *testing.T, db Database) {
	key := []byte("hello1")
	value := []byte("world1")

	batch := db.NewBatch()
	if batch == nil {
		t.Fatalf("db.NewBatch returned nil")
	}

	if err := batch.Put(key, value); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	if err := batch.Write(); err != nil {
		t.Fatalf("Unexpected error on batch.Write: %s", err)
	}

	if err := db.Delete(key); err != nil {
		t.Fatalf("Unexpected error on database.Delete: %s", err)
	}

	if has, err := db.Has(key); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if has {
		t.Fatalf("db.Has unexpectedly returned true on key %s", key)
	}

	if err := batch.Write(); err != nil {
		t.Fatalf("Unexpected error on batch.Write: %s", err)
	}

	if has, err := db.Has(key); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if !has {
		t.Fatalf("db.Has unexpectedly returned false on key %s", key)
	} else if v, err := db.Get(key); err != nil {
		t.Fatalf("Unexpected error on db.Get: %s", err)
	} else if !bytes.Equal(value, v) {
		t.Fatalf("db.Get: Returned: 0x%x ; Expected: 0x%x", v, value)
	}
}

// TestBatchReplay tests to make sure that batches will correctly replay their
// contents.
func TestBatchReplay(t *testing.T, db Database) {
	key1 := []byte("hello1")
	value1 := []byte("world1")

	key2 := []byte("hello2")
	value2 := []byte("world2")

	batch := db.NewBatch()
	if batch == nil {
		t.Fatalf("db.NewBatch returned nil")
	}

	if err := batch.Put(key1, value1); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := batch.Put(key2, value2); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	secondBatch := db.NewBatch()
	if secondBatch == nil {
		t.Fatalf("db.NewBatch returned nil")
	}

	if err := batch.Replay(secondBatch); err != nil {
		t.Fatalf("Unexpected error on batch.Replay: %s", err)
	}

	if err := secondBatch.Write(); err != nil {
		t.Fatalf("Unexpected error on batch.Write: %s", err)
	}

	if has, err := db.Has(key1); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if !has {
		t.Fatalf("db.Has unexpectedly returned false on key %s", key1)
	} else if v, err := db.Get(key1); err != nil {
		t.Fatalf("Unexpected error on db.Get: %s", err)
	} else if !bytes.Equal(value1, v) {
		t.Fatalf("db.Get: Returned: 0x%x ; Expected: 0x%x", v, value1)
	}

	thirdBatch := db.NewBatch()
	if thirdBatch == nil {
		t.Fatalf("db.NewBatch returned nil")
	}

	if err := thirdBatch.Delete(key1); err != nil {
		t.Fatalf("Unexpected error on batch.Delete: %s", err)
	} else if err := thirdBatch.Delete(key2); err != nil {
		t.Fatalf("Unexpected error on batch.Delete: %s", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Unexpected error on db.Close: %s", err)
	}

	if err := batch.Replay(db); err != ErrClosed {
		t.Fatalf("Expected %s on batch.Replay", ErrClosed)
	} else if err := thirdBatch.Replay(db); err != ErrClosed {
		t.Fatalf("Expected %s on batch.Replay", ErrClosed)
	}
}

// TestBatchInner tests to make sure that inner can be used to write to the
// database.
func TestBatchInner(t *testing.T, db Database) {
	key1 := []byte("hello1")
	value1 := []byte("world1")

	key2 := []byte("hello2")
	value2 := []byte("world2")

	firstBatch := db.NewBatch()
	if firstBatch == nil {
		t.Fatalf("db.NewBatch returned nil")
	}

	if err := firstBatch.Put(key1, value1); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	secondBatch := db.NewBatch()
	if secondBatch == nil {
		t.Fatalf("db.NewBatch returned nil")
	}

	if err := secondBatch.Put(key2, value2); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	innerFirstBatch := firstBatch.Inner()
	innerSecondBatch := secondBatch.Inner()

	if err := innerFirstBatch.Replay(innerSecondBatch); err != nil {
		t.Fatalf("Unexpected error on batch.Replay: %s", err)
	}

	if err := innerSecondBatch.Write(); err != nil {
		t.Fatalf("Unexpected error on batch.Write: %s", err)
	}

	if has, err := db.Has(key1); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if !has {
		t.Fatalf("db.Has unexpectedly returned false on key %s", key1)
	} else if v, err := db.Get(key1); err != nil {
		t.Fatalf("Unexpected error on db.Get: %s", err)
	} else if !bytes.Equal(value1, v) {
		t.Fatalf("db.Get: Returned: 0x%x ; Expected: 0x%x", v, value1)
	} else if has, err := db.Has(key2); err != nil {
		t.Fatalf("Unexpected error on db.Has: %s", err)
	} else if !has {
		t.Fatalf("db.Has unexpectedly returned false on key %s", key2)
	} else if v, err := db.Get(key2); err != nil {
		t.Fatalf("Unexpected error on db.Get: %s", err)
	} else if !bytes.Equal(value2, v) {
		t.Fatalf("db.Get: Returned: 0x%x ; Expected: 0x%x", v, value2)
	}
}

// TestIterator tests to make sure the database iterates over the database
// contents lexicographically.
func TestIterator(t *testing.T, db Database) {
	key1 := []byte("hello1")
	value1 := []byte("world1")

	key2 := []byte("hello2")
	value2 := []byte("world2")

	if err := db.Put(key1, value1); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Put(key2, value2); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	iterator := db.NewIterator()
	if iterator == nil {
		t.Fatalf("db.NewIterator returned nil")
	}
	defer iterator.Release()

	if !iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", false, true)
	} else if key := iterator.Key(); !bytes.Equal(key, key1) {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: 0x%x", key, key1)
	} else if value := iterator.Value(); !bytes.Equal(value, value1) {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: 0x%x", value, value1)
	} else if !iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", false, true)
	} else if key := iterator.Key(); !bytes.Equal(key, key2) {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: 0x%x", key, key2)
	} else if value := iterator.Value(); !bytes.Equal(value, value2) {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: 0x%x", value, value2)
	} else if iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", true, false)
	} else if key := iterator.Key(); key != nil {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: nil", key)
	} else if value := iterator.Value(); value != nil {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: nil", value)
	} else if err := iterator.Error(); err != nil {
		t.Fatalf("iterator.Error Returned: %s ; Expected: nil", err)
	}
}

// TestIteratorStart tests to make sure the the iterator can be configured to
// start mid way through the database.
func TestIteratorStart(t *testing.T, db Database) {
	key1 := []byte("hello1")
	value1 := []byte("world1")

	key2 := []byte("hello2")
	value2 := []byte("world2")

	if err := db.Put(key1, value1); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Put(key2, value2); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	iterator := db.NewIteratorWithStart(key2)
	if iterator == nil {
		t.Fatalf("db.NewIteratorWithStart returned nil")
	}
	defer iterator.Release()

	if !iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", false, true)
	} else if key := iterator.Key(); !bytes.Equal(key, key2) {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: 0x%x", key, key2)
	} else if value := iterator.Value(); !bytes.Equal(value, value2) {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: 0x%x", value, value2)
	} else if iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", true, false)
	} else if key := iterator.Key(); key != nil {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: nil", key)
	} else if value := iterator.Value(); value != nil {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: nil", value)
	} else if err := iterator.Error(); err != nil {
		t.Fatalf("iterator.Error Returned: %s ; Expected: nil", err)
	}
}

// TestIteratorPrefix tests to make sure the iterator can be configured to skip
// keys missing the provided prefix.
func TestIteratorPrefix(t *testing.T, db Database) {
	key1 := []byte("hello")
	value1 := []byte("world1")

	key2 := []byte("goodbye")
	value2 := []byte("world2")

	if err := db.Put(key1, value1); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Put(key2, value2); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	iterator := db.NewIteratorWithPrefix([]byte("h"))
	if iterator == nil {
		t.Fatalf("db.NewIteratorWithPrefix returned nil")
	}
	defer iterator.Release()

	if !iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", false, true)
	} else if key := iterator.Key(); !bytes.Equal(key, key1) {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: 0x%x", key, key1)
	} else if value := iterator.Value(); !bytes.Equal(value, value1) {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: 0x%x", value, value1)
	} else if iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", true, false)
	} else if key := iterator.Key(); key != nil {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: nil", key)
	} else if value := iterator.Value(); value != nil {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: nil", value)
	} else if err := iterator.Error(); err != nil {
		t.Fatalf("iterator.Error Returned: %s ; Expected: nil", err)
	}
}

// TestIteratorStartPrefix tests to make sure that the iterator can start mid
// way through the database while skipping a prefix.
func TestIteratorStartPrefix(t *testing.T, db Database) {
	key1 := []byte("hello1")
	value1 := []byte("world1")

	key2 := []byte("z")
	value2 := []byte("world2")

	key3 := []byte("hello3")
	value3 := []byte("world3")

	if err := db.Put(key1, value1); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Put(key2, value2); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Put(key3, value3); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	iterator := db.NewIteratorWithStartAndPrefix(key1, []byte("h"))
	if iterator == nil {
		t.Fatalf("db.NewIteratorWithStartAndPrefix returned nil")
	}
	defer iterator.Release()

	if !iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", false, true)
	} else if key := iterator.Key(); !bytes.Equal(key, key1) {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: 0x%x", key, key1)
	} else if value := iterator.Value(); !bytes.Equal(value, value1) {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: 0x%x", value, value1)
	} else if !iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", false, true)
	} else if key := iterator.Key(); !bytes.Equal(key, key3) {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: 0x%x", key, key3)
	} else if value := iterator.Value(); !bytes.Equal(value, value3) {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: 0x%x", value, value3)
	} else if iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", true, false)
	} else if key := iterator.Key(); key != nil {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: nil", key)
	} else if value := iterator.Value(); value != nil {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: nil", value)
	} else if err := iterator.Error(); err != nil {
		t.Fatalf("iterator.Error Returned: %s ; Expected: nil", err)
	}
}

// TestIteratorMemorySafety tests to make sure that keys can values are able to
// be modified from the returned iterator.
func TestIteratorMemorySafety(t *testing.T, db Database) {
	key1 := []byte("hello1")
	value1 := []byte("world1")

	key2 := []byte("z")
	value2 := []byte("world2")

	key3 := []byte("hello3")
	value3 := []byte("world3")

	if err := db.Put(key1, value1); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Put(key2, value2); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Put(key3, value3); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	iterator := db.NewIterator()
	if iterator == nil {
		t.Fatalf("db.NewIterator returned nil")
	}
	defer iterator.Release()

	keys := [][]byte{}
	values := [][]byte{}
	for iterator.Next() {
		keys = append(keys, iterator.Key())
		values = append(values, iterator.Value())
	}

	expectedKeys := [][]byte{
		key1,
		key3,
		key2,
	}
	expectedValues := [][]byte{
		value1,
		value3,
		value2,
	}

	for i, key := range keys {
		value := values[i]
		expectedKey := expectedKeys[i]
		expectedValue := expectedValues[i]

		if !bytes.Equal(key, expectedKey) {
			t.Fatalf("Wrong key")
		}
		if !bytes.Equal(value, expectedValue) {
			t.Fatalf("Wrong key")
		}
	}
}

// TestIteratorClosed tests to make sure that an iterator that was created with
// a closed database will report a closed error correctly.
func TestIteratorClosed(t *testing.T, db Database) {
	key1 := []byte("hello1")
	value1 := []byte("world1")

	if err := db.Put(key1, value1); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Unexpected error on db.Close: %s", err)
	}

	iterator := db.NewIterator()
	if iterator == nil {
		t.Fatalf("db.NewIterator returned nil")
	}
	defer iterator.Release()

	if iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", true, false)
	} else if key := iterator.Key(); key != nil {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: nil", key)
	} else if value := iterator.Value(); value != nil {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: nil", value)
	} else if err := iterator.Error(); err != ErrClosed {
		t.Fatalf("Expected %s on iterator.Error", ErrClosed)
	}
}

// TestIteratorError tests to make sure that an iterator still works after the
// database is closed.
func TestIteratorError(t *testing.T, db Database) {
	key := []byte("hello1")
	value := []byte("world1")

	if err := db.Put(key, value); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	iterator := db.NewIterator()
	if iterator == nil {
		t.Fatalf("db.NewIterator returned nil")
	}
	defer iterator.Release()

	if err := db.Close(); err != nil {
		t.Fatalf("Unexpected error on db.Close: %s", err)
	}

	if !iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", false, true)
	}
	if itKey := iterator.Key(); !bytes.Equal(itKey, key) {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: 0x%x", itKey, key)
	}
	if itValue := iterator.Value(); !bytes.Equal(itValue, value) {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: 0x%x", itValue, value)
	}
	if err := iterator.Error(); err != nil {
		t.Fatalf("Expected no error on iterator.Error but got %s", err)
	}
}

// TestIteratorErrorAfterRelease tests to make sure that an iterator that was
// released still reports the error correctly.
func TestIteratorErrorAfterRelease(t *testing.T, db Database) {
	key := []byte("hello1")
	value := []byte("world1")

	if err := db.Put(key, value); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Unexpected error on db.Close: %s", err)
	}

	iterator := db.NewIterator()
	if iterator == nil {
		t.Fatalf("db.NewIterator returned nil")
	}

	iterator.Release()

	if iterator.Next() {
		t.Fatalf("iterator.Next Returned: %v ; Expected: %v", false, true)
	}
	if key := iterator.Key(); key != nil {
		t.Fatalf("iterator.Key Returned: 0x%x ; Expected: nil", key)
	}
	if value := iterator.Value(); value != nil {
		t.Fatalf("iterator.Value Returned: 0x%x ; Expected: nil", value)
	}
	if err := iterator.Error(); err != ErrClosed {
		t.Fatalf("Expected %s on iterator.Error", ErrClosed)
	}
}

// TestStatNoPanic tests to make sure that Stat never panics.
func TestStatNoPanic(t *testing.T, db Database) {
	key1 := []byte("hello1")
	value1 := []byte("world1")

	key2 := []byte("z")
	value2 := []byte("world2")

	key3 := []byte("hello3")
	value3 := []byte("world3")

	if err := db.Put(key1, value1); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Put(key2, value2); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Put(key3, value3); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	// Stat could error or not redpending on the implementation, but it
	// shouldn't panic
	_, _ = db.Stat("")

	if err := db.Close(); err != nil {
		t.Fatalf("Unexpected error on db.Close: %s", err)
	}

	// Stat could error or not redpending on the implementation, but it
	// shouldn't panic
	_, _ = db.Stat("")
}

// TestCompactNoPanic tests to make sure compact never panics.
func TestCompactNoPanic(t *testing.T, db Database) {
	key1 := []byte("hello1")
	value1 := []byte("world1")

	key2 := []byte("z")
	value2 := []byte("world2")

	key3 := []byte("hello3")
	value3 := []byte("world3")

	if err := db.Put(key1, value1); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Put(key2, value2); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	} else if err := db.Put(key3, value3); err != nil {
		t.Fatalf("Unexpected error on batch.Put: %s", err)
	}

	if err := db.Compact(nil, nil); err != nil {
		t.Fatalf("Unexpected error on db.Compact")
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Unexpected error on db.Close: %s", err)
	}

	if err := db.Compact(nil, nil); err != ErrClosed {
		t.Fatalf("Expected error %s on db.Close but got %s", ErrClosed, err)
	}
}
