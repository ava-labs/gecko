// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cache

import (
	"container/list"
	"sync"
)

// EvictableLRU is an LRU cache that notifies the objects when they are evicted.
type EvictableLRU struct {
	lock      sync.Mutex
	entryMap  map[interface{}]*list.Element
	entryList *list.List
	Size      int
}

// Deduplicate implements the Deduplicator interface
func (c *EvictableLRU) Deduplicate(value Evictable) Evictable {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.deduplicate(value)
}

// Flush implements the Deduplicator interface
func (c *EvictableLRU) Flush() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.flush()
}

func (c *EvictableLRU) init() {
	if c.entryMap == nil {
		c.entryMap = make(map[interface{}]*list.Element)
	}
	if c.entryList == nil {
		c.entryList = list.New()
	}
	if c.Size <= 0 {
		c.Size = 1
	}
}

func (c *EvictableLRU) resize() {
	for c.entryList.Len() > c.Size {
		e := c.entryList.Front()
		c.entryList.Remove(e)

		val := e.Value.(Evictable)
		delete(c.entryMap, val.Key())
		val.Evict()
	}
}

func (c *EvictableLRU) deduplicate(value Evictable) Evictable {
	c.init()
	c.resize()

	key := value.Key()
	if e, ok := c.entryMap[key]; !ok {
		if c.entryList.Len() >= c.Size {
			e = c.entryList.Front()
			c.entryList.MoveToBack(e)

			val := e.Value.(Evictable)
			delete(c.entryMap, val.Key())
			val.Evict()

			e.Value = value
		} else {
			e = c.entryList.PushBack(value)
		}
		c.entryMap[key] = e
	} else {
		c.entryList.MoveToBack(e)

		val := e.Value.(Evictable)
		value = val
	}
	return value
}

func (c *EvictableLRU) flush() {
	c.init()

	size := c.Size
	c.Size = 0
	c.resize()
	c.Size = size
}
