// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package math

import (
	"sync"
	"time"
)

type syncAverager struct {
	lock     sync.RWMutex
	averager Averager
}

func NewSyncAverager(averager Averager) Averager {
	return &syncAverager{
		averager: averager,
	}
}

func (a *syncAverager) Observe(value float64, currentTime time.Time) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.averager.Observe(value, currentTime)
}

func (a *syncAverager) Read() float64 {
	a.lock.RLock()
	defer a.lock.RUnlock()

	return a.averager.Read()
}
