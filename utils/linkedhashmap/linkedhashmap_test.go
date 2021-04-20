// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package linkedhashmap

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/stretchr/testify/assert"
)

func TestLinkedHashmap(t *testing.T) {
	assert := assert.New(t)

	lh := New()
	assert.Equal(0, lh.Len(), "a new hashmap should be empty")

	key0 := ids.GenerateTestID()
	_, exists := lh.Get(key0)
	assert.False(exists, "shouldn't have found the value")

	_, exists = lh.Oldest()
	assert.False(exists, "shouldn't have found a value")

	_, exists = lh.Newest()
	assert.False(exists, "shouldn't have found a value")

	lh.Put(key0, 0)
	assert.Equal(1, lh.Len(), "wrong hashmap length")

	val0, exists := lh.Get(key0)
	assert.True(exists, "should have found the value")
	assert.Equal(0, val0, "wrong value")

	val0, exists = lh.Oldest()
	assert.True(exists, "should have found the value")
	assert.Equal(0, val0, "wrong value")

	val0, exists = lh.Newest()
	assert.True(exists, "should have found the value")
	assert.Equal(0, val0, "wrong value")

	key1 := ids.GenerateTestID()
	lh.Put(key1, 1)
	assert.Equal(2, lh.Len(), "wrong hashmap length")

	val1, exists := lh.Get(key1)
	assert.True(exists, "should have found the value")
	assert.Equal(1, val1, "wrong value")

	val0, exists = lh.Oldest()
	assert.True(exists, "should have found the value")
	assert.Equal(0, val0, "wrong value")

	val1, exists = lh.Newest()
	assert.True(exists, "should have found the value")
	assert.Equal(1, val1, "wrong value")

	lh.Delete(key0)
	assert.Equal(1, lh.Len(), "wrong hashmap length")

	_, exists = lh.Get(key0)
	assert.False(exists, "shouldn't have found the value")

	val1, exists = lh.Oldest()
	assert.True(exists, "should have found the value")
	assert.Equal(1, val1, "wrong value")

	val1, exists = lh.Newest()
	assert.True(exists, "should have found the value")
	assert.Equal(1, val1, "wrong value")

	lh.Put(key0, 0)
	assert.Equal(2, lh.Len(), "wrong hashmap length")

	lh.Put(key1, 1)
	assert.Equal(2, lh.Len(), "wrong hashmap length")

	val0, exists = lh.Oldest()
	assert.True(exists, "should have found the value")
	assert.Equal(0, val0, "wrong value")

	val1, exists = lh.Newest()
	assert.True(exists, "should have found the value")
	assert.Equal(1, val1, "wrong value")
}
