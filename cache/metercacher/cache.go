// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package metercacher

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/utils/timer"
)

type Cache struct {
	metrics
	cache cache.Cacher
	clock timer.Clock
}

func New(
	namespace string,
	registerer prometheus.Registerer,
	cache cache.Cacher,
) (cache.Cacher, error) {
	meterCache := &Cache{cache: cache}
	return meterCache, meterCache.metrics.Initialize(namespace, registerer)
}

func (c *Cache) Put(key, value interface{}) {
	start := c.clock.Time()
	c.cache.Put(key, value)
	end := c.clock.Time()
	c.put.Observe(float64(end.Sub(start)))
}

func (c *Cache) Get(key interface{}) (interface{}, bool) {
	start := c.clock.Time()
	value, has := c.cache.Get(key)
	end := c.clock.Time()
	c.get.Observe(float64(end.Sub(start)))
	if has {
		c.hit.Inc()
	} else {
		c.miss.Inc()
	}

	return value, has
}

func (c *Cache) Evict(key interface{}) {
	start := c.clock.Time()
	c.cache.Evict(key)
	end := c.clock.Time()
	c.evict.Observe(float64(end.Sub(start)))
}

func (c *Cache) Flush() {
	start := c.clock.Time()
	c.cache.Flush()
	end := c.clock.Time()
	c.flush.Observe(float64(end.Sub(start)))
}
