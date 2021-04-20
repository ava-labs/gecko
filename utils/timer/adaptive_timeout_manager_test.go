// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package timer

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/constants"
)

// Test that Initialize works
func TestAdaptiveTimeoutManagerInit(t *testing.T) {
	type test struct {
		config        AdaptiveTimeoutConfig
		shouldErrWith string
	}

	tests := []test{
		{
			config: AdaptiveTimeoutConfig{
				InitialTimeout:     time.Second,
				MinimumTimeout:     2 * time.Second,
				MaximumTimeout:     3 * time.Second,
				TimeoutCoefficient: 2,
				TimeoutHalflife:    5 * time.Minute,
				Registerer:         prometheus.NewRegistry(),
			},
			shouldErrWith: "initial timeout < minimum timeout",
		},
		{
			config: AdaptiveTimeoutConfig{
				InitialTimeout:     5 * time.Second,
				MinimumTimeout:     2 * time.Second,
				MaximumTimeout:     3 * time.Second,
				TimeoutCoefficient: 2,
				TimeoutHalflife:    5 * time.Minute,
				Registerer:         prometheus.NewRegistry(),
			},
			shouldErrWith: "initial timeout > maximum timeout",
		},
		{
			config: AdaptiveTimeoutConfig{
				InitialTimeout:     2 * time.Second,
				MinimumTimeout:     2 * time.Second,
				MaximumTimeout:     3 * time.Second,
				TimeoutCoefficient: 0.9,
				TimeoutHalflife:    5 * time.Minute,
				Registerer:         prometheus.NewRegistry(),
			},
			shouldErrWith: "timeout coefficient < 1",
		},
		{
			config: AdaptiveTimeoutConfig{
				InitialTimeout:     2 * time.Second,
				MinimumTimeout:     2 * time.Second,
				MaximumTimeout:     3 * time.Second,
				TimeoutCoefficient: 1,
				Registerer:         prometheus.NewRegistry(),
			},
			shouldErrWith: "timeout halflife is 0",
		},
		{
			config: AdaptiveTimeoutConfig{
				InitialTimeout:     2 * time.Second,
				MinimumTimeout:     2 * time.Second,
				MaximumTimeout:     3 * time.Second,
				TimeoutCoefficient: 1,
				TimeoutHalflife:    -1 * time.Second,
				Registerer:         prometheus.NewRegistry(),
			},
			shouldErrWith: "timeout halflife is negative",
		},
		{
			config: AdaptiveTimeoutConfig{
				InitialTimeout:     2 * time.Second,
				MinimumTimeout:     2 * time.Second,
				MaximumTimeout:     3 * time.Second,
				TimeoutCoefficient: 1,
				TimeoutHalflife:    5 * time.Minute,
				Registerer:         prometheus.NewRegistry(),
			},
		},
	}

	for _, test := range tests {
		tm := AdaptiveTimeoutManager{}
		err := tm.Initialize(&test.config)
		if err != nil && test.shouldErrWith == "" {
			assert.FailNow(t, "error from valid config", err)
		} else if err == nil && test.shouldErrWith != "" {
			assert.FailNowf(t, "should have errored", test.shouldErrWith)
		}
	}
}

func TestAdaptiveTimeoutManager(t *testing.T) {
	// Initialize
	assert := assert.New(t)
	tm := AdaptiveTimeoutManager{}
	config := &AdaptiveTimeoutConfig{
		InitialTimeout:     250 * time.Millisecond,
		MinimumTimeout:     250 * time.Millisecond,
		MaximumTimeout:     10 * time.Second,
		TimeoutHalflife:    5 * time.Minute,
		TimeoutCoefficient: 1.25,
		MetricsNamespace:   constants.PlatformName,
		Registerer:         prometheus.NewRegistry(),
	}
	err := tm.Initialize(config)
	assert.NoError(err)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go tm.Dispatch()
	defer tm.Stop()

	// Assert initial state is correct
	tm.lock.Lock()
	assert.Len(tm.timeoutMap, 0)
	assert.Len(tm.timeoutQueue, 0)
	assert.False(tm.timer.shouldExecute)
	assert.Equal(config.InitialTimeout, tm.currentTimeout)
	tm.lock.Unlock()

	// Register a timeout
	id0 := ids.GenerateTestID()
	timeoutZeroCalled := utils.AtomicBool{}
	tm.Put(
		id0,
		constants.PullQueryMsg,
		func() { timeoutZeroCalled.SetValue(true) },
	)

	tm.lock.Lock()
	// id0 should be in the timeout map
	assert.Contains(tm.timeoutMap, id0)
	// The timeout for id0 should be be in the timeout queue
	assert.Len(tm.timeoutQueue, 1)
	// The timeout should be in the future
	assert.True(tm.timeoutQueue[0].deadline.After(tm.clock.Time()))
	// But not too far in the future
	assert.True(tm.timeoutQueue[0].deadline.Before(tm.clock.Time().Add(tm.maximumTimeout)))
	// Timeout should be set to fire
	assert.True(tm.timer.shouldExecute)
	tm.lock.Unlock()

	// Try to remove some non-existent timeout
	tm.Remove(ids.GenerateTestID())

	// State should be the same
	tm.lock.Lock()
	// id0 should be in the timeout map
	assert.Contains(tm.timeoutMap, id0)
	// The timeout for id0 should be be in the timeout queue
	assert.Len(tm.timeoutQueue, 1)
	// The timeout should be in the future
	assert.True(tm.timeoutQueue[0].deadline.After(tm.clock.Time()))
	// But not too far in the future
	assert.True(tm.timeoutQueue[0].deadline.Before(tm.clock.Time().Add(tm.maximumTimeout)))
	// Timeout should be set to fire
	assert.True(tm.timer.shouldExecute)
	tm.lock.Unlock()

	// This should overwrite the first Put for id0
	tm.Put(
		id0,
		constants.PullQueryMsg,
		func() { wg.Done() },
	)

	tm.lock.Lock()
	// id0 should be in the timeout map
	assert.Contains(tm.timeoutMap, id0)
	// The timeout for id0 should be be in the timeout queue
	assert.Len(tm.timeoutQueue, 1)
	// The timeout should be in the future
	assert.True(tm.timeoutQueue[0].deadline.After(tm.clock.Time()))
	// But not too far in the future
	assert.True(!tm.timeoutQueue[0].deadline.After(tm.clock.Time().Add(tm.maximumTimeout)))
	// Timeout should be set to fire
	assert.True(tm.timer.shouldExecute)
	tm.lock.Unlock()

	// Wait until timeout fires
	wg.Wait()
	// Give [tm.timer] a moment to set [tm.timer.shouldExecute] to false
	// If test is being flaky, try increasing this
	time.Sleep(100 * time.Millisecond)

	// Make sure first timeout we registered then overwrote never fires
	assert.False(timeoutZeroCalled.GetValue())

	tm.lock.Lock()
	assert.Len(tm.timeoutMap, 0)
	assert.Len(tm.timeoutQueue, 0)
	assert.False(tm.timer.shouldExecute)
	tm.lock.Unlock()

	// Register two more timeouts
	id1 := ids.GenerateTestID()
	id2 := ids.GenerateTestID()
	wg.Add(2)
	tm.Put(
		id1,
		constants.PullQueryMsg,
		func() { wg.Done() },
	)
	tm.Put(
		id2,
		constants.PullQueryMsg,
		func() { wg.Done() },
	)

	tm.lock.Lock()
	// id1 should be in the timeout map
	assert.Contains(tm.timeoutMap, id1)
	// id2 should be in the timeout map
	assert.Contains(tm.timeoutMap, id2)
	assert.Len(tm.timeoutMap, 2)
	assert.Len(tm.timeoutQueue, 2)
	assert.True(!tm.timeoutQueue[0].deadline.After(tm.timeoutQueue[1].deadline))
	// Timeout should be set to fire
	assert.True(tm.timer.shouldExecute)
	tm.lock.Unlock()

	// Wait until both timeouts fire
	wg.Wait()
	// Give [tm.timer] a moment to set [tm.timer.shouldExecute] to false
	// If test is being flaky, try increasing this
	time.Sleep(100 * time.Millisecond)

	tm.lock.Lock()
	assert.Len(tm.timeoutMap, 0)
	assert.Len(tm.timeoutQueue, 0)
	assert.False(tm.timer.shouldExecute)
	tm.lock.Unlock()
}

func TestAdaptiveTimeoutManager2(t *testing.T) {
	tm := AdaptiveTimeoutManager{}
	err := tm.Initialize(&AdaptiveTimeoutConfig{
		InitialTimeout:     time.Millisecond,
		MinimumTimeout:     time.Millisecond,
		MaximumTimeout:     time.Hour,
		TimeoutHalflife:    5 * time.Minute,
		TimeoutCoefficient: 1.25,
		MetricsNamespace:   constants.PlatformName,
		Registerer:         prometheus.NewRegistry(),
	})
	if err != nil {
		t.Fatal(err)
	}
	go tm.Dispatch()

	var lock sync.Mutex

	numSuccessful := 5

	wg := sync.WaitGroup{}
	wg.Add(numSuccessful)

	callback := new(func())
	*callback = func() {
		lock.Lock()
		defer lock.Unlock()

		numSuccessful--
		if numSuccessful > 0 {
			tm.Put(ids.ID{byte(numSuccessful)}, constants.PullQueryMsg, *callback)
		}
		if numSuccessful >= 0 {
			wg.Done()
		}
		if numSuccessful%2 == 0 {
			tm.Remove(ids.ID{byte(numSuccessful)})
			tm.Put(ids.ID{byte(numSuccessful)}, constants.PullQueryMsg, *callback)
		}
	}
	(*callback)()
	(*callback)()

	wg.Wait()
}
