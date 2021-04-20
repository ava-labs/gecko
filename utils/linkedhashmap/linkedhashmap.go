// (c) 2019-2021, Ava Labs, Inte. All rights reserved.
// See the file LICENSE for licensing terms.

package linkedhashmap

import (
	"container/list"
	"sync"

	"github.com/ava-labs/avalanchego/ids"
)

// Hashmap provides an O(1) mapping from an [ids.ID] to any value.
type Hashmap interface {
	Put(key ids.ID, val interface{})
	Get(key ids.ID) (val interface{}, exists bool)
	Delete(key ids.ID)
	Len() int
}

// LinkedHashmap is a hashmap that keeps track of the oldest pairing an the
// newest pairing.
type LinkedHashmap interface {
	Hashmap

	Oldest() (val interface{}, exists bool)
	Newest() (val interface{}, exists bool)
}

type linkedHashmap struct {
	lock      sync.Mutex
	entryMap  map[ids.ID]*list.Element
	entryList *list.List
}

func New() LinkedHashmap {
	return &linkedHashmap{
		entryMap:  make(map[ids.ID]*list.Element),
		entryList: list.New(),
	}
}

func (lh *linkedHashmap) Put(key ids.ID, val interface{}) {
	lh.lock.Lock()
	defer lh.lock.Unlock()

	lh.put(key, val)
}

func (lh *linkedHashmap) Get(key ids.ID) (interface{}, bool) {
	lh.lock.Lock()
	defer lh.lock.Unlock()

	return lh.get(key)
}

func (lh *linkedHashmap) Delete(key ids.ID) {
	lh.lock.Lock()
	defer lh.lock.Unlock()

	lh.delete(key)
}

func (lh *linkedHashmap) Len() int {
	lh.lock.Lock()
	defer lh.lock.Unlock()

	return lh.len()
}

func (lh *linkedHashmap) Oldest() (interface{}, bool) {
	lh.lock.Lock()
	defer lh.lock.Unlock()

	return lh.oldest()
}

func (lh *linkedHashmap) Newest() (interface{}, bool) {
	lh.lock.Lock()
	defer lh.lock.Unlock()

	return lh.newest()
}

func (lh *linkedHashmap) put(key ids.ID, value interface{}) {
	if e, ok := lh.entryMap[key]; ok {
		lh.entryList.MoveToBack(e)
		e.Value = value
	} else {
		lh.entryMap[key] = lh.entryList.PushBack(value)
	}
}

func (lh *linkedHashmap) get(key ids.ID) (interface{}, bool) {
	if e, ok := lh.entryMap[key]; ok {
		return e.Value, true
	}
	return nil, false
}

func (lh *linkedHashmap) delete(key ids.ID) {
	if e, ok := lh.entryMap[key]; ok {
		lh.entryList.Remove(e)
		delete(lh.entryMap, key)
	}
}

func (lh *linkedHashmap) len() int { return len(lh.entryMap) }

func (lh *linkedHashmap) oldest() (interface{}, bool) {
	if val := lh.entryList.Front(); val != nil {
		return val.Value, true
	}
	return nil, false
}

func (lh *linkedHashmap) newest() (interface{}, bool) {
	if val := lh.entryList.Back(); val != nil {
		return val.Value, true
	}
	return nil, false
}
