// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package timer

import (
	"container/heap"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/math"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

var (
	errNonPositiveHalflife = errors.New("timeout halflife must be positive")
)

type adaptiveTimeout struct {
	index    int               // Index in the wait queue
	id       ids.ID            // Unique ID of this timeout
	handler  func()            // Function to execute if timed out
	duration time.Duration     // How long this timeout was set for
	deadline time.Time         // When this timeout should be fired
	msgType  constants.MsgType // Type of this outstanding request
}

// A timeoutQueue implements heap.Interface and holds adaptiveTimeouts.
type timeoutQueue []*adaptiveTimeout

func (tq timeoutQueue) Len() int           { return len(tq) }
func (tq timeoutQueue) Less(i, j int) bool { return tq[i].deadline.Before(tq[j].deadline) }
func (tq timeoutQueue) Swap(i, j int) {
	tq[i], tq[j] = tq[j], tq[i]
	tq[i].index = i
	tq[j].index = j
}

// Push adds an item to this priority queue. x must have type *adaptiveTimeout
func (tq *timeoutQueue) Push(x interface{}) {
	item := x.(*adaptiveTimeout)
	item.index = len(*tq)
	*tq = append(*tq, item)
}

// Pop returns the next item in this queue
func (tq *timeoutQueue) Pop() interface{} {
	n := len(*tq)
	item := (*tq)[n-1]
	(*tq)[n-1] = nil // make sure the item is freed from memory
	*tq = (*tq)[:n-1]
	return item
}

// AdaptiveTimeoutConfig contains the parameters provided to the
// adaptive timeout manager.
type AdaptiveTimeoutConfig struct {
	InitialTimeout time.Duration
	MinimumTimeout time.Duration
	MaximumTimeout time.Duration
	// Timeout is [timeoutCoefficient] * average response time
	// [timeoutCoefficient] must be > 1
	TimeoutCoefficient float64
	// Larger halflife --> less volatile timeout
	// [timeoutHalfLife] must be positive
	TimeoutHalflife  time.Duration
	MetricsNamespace string
	Registerer       prometheus.Registerer
}

// AdaptiveTimeoutManager is a manager for timeouts.
type AdaptiveTimeoutManager struct {
	lock sync.Mutex
	// Tells the time. Can be faked for testing.
	clock                            Clock
	networkTimeoutMetric, avgLatency prometheus.Gauge
	numTimeouts                      prometheus.Counter
	// Averages the response time from all peers
	averager math.Averager
	// Timeout is [timeoutCoefficient] * average response time
	// [timeoutCoefficient] must be > 1
	timeoutCoefficient float64
	minimumTimeout     time.Duration
	maximumTimeout     time.Duration
	currentTimeout     time.Duration // Amount of time before a timeout
	timeoutMap         map[ids.ID]*adaptiveTimeout
	timeoutQueue       timeoutQueue
	timer              *Timer // Timer that will fire to clear the timeouts
}

// Initialize this timeout manager with the provided config
func (tm *AdaptiveTimeoutManager) Initialize(config *AdaptiveTimeoutConfig) error {
	tm.networkTimeoutMetric = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: config.MetricsNamespace,
		Name:      "network_timeout",
		Help:      "Duration of current network timeout in nanoseconds",
	})
	tm.avgLatency = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: config.MetricsNamespace,
		Name:      "avg_network_latency",
		Help:      "Average network latency in nanoseconds",
	})
	tm.numTimeouts = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: config.MetricsNamespace,
		Name:      "request_timeouts",
		Help:      "Number of timed out requests",
	})

	switch {
	case config.InitialTimeout > config.MaximumTimeout:
		return fmt.Errorf("initial timeout (%s) > maximum timeout (%s)", config.InitialTimeout, config.MaximumTimeout)
	case config.InitialTimeout < config.MinimumTimeout:
		return fmt.Errorf("initial timeout (%s) < minimum timeout (%s)", config.InitialTimeout, config.MinimumTimeout)
	case config.TimeoutCoefficient < 1:
		return fmt.Errorf("timeout coefficient must be >= 1 but got %f", config.TimeoutCoefficient)
	case config.TimeoutHalflife <= 0:
		return errNonPositiveHalflife
	}

	tm.timeoutCoefficient = config.TimeoutCoefficient
	tm.averager = math.NewAverager(float64(config.InitialTimeout), config.TimeoutHalflife, tm.clock.Time())
	tm.minimumTimeout = config.MinimumTimeout
	tm.maximumTimeout = config.MaximumTimeout
	tm.currentTimeout = config.InitialTimeout
	tm.timeoutMap = make(map[ids.ID]*adaptiveTimeout)
	tm.timer = NewTimer(tm.Timeout)

	errs := &wrappers.Errs{}
	errs.Add(config.Registerer.Register(tm.networkTimeoutMetric))
	errs.Add(config.Registerer.Register(tm.avgLatency))
	errs.Add(config.Registerer.Register(tm.numTimeouts))
	return errs.Err
}

// TimeoutDuration returns the current network timeout duration
func (tm *AdaptiveTimeoutManager) TimeoutDuration() time.Duration {
	tm.lock.Lock()
	defer tm.lock.Unlock()
	return tm.currentTimeout
}

// Dispatch ...
func (tm *AdaptiveTimeoutManager) Dispatch() { tm.timer.Dispatch() }

// Stop executing timeouts
func (tm *AdaptiveTimeoutManager) Stop() { tm.timer.Stop() }

// Put registers a timeout for [id]. If the timeout occurs, [timeoutHandler] is called.
// Returns the time at which the timeout will fire if it is not first
// removed by calling [tm.Remove].
func (tm *AdaptiveTimeoutManager) Put(id ids.ID, msgType constants.MsgType, timeoutHandler func()) time.Time {
	tm.lock.Lock()
	defer tm.lock.Unlock()
	return tm.put(id, msgType, timeoutHandler)
}

// Assumes [tm.lock] is held
func (tm *AdaptiveTimeoutManager) put(id ids.ID, msgType constants.MsgType, handler func()) time.Time {
	currentTime := tm.clock.Time()
	tm.remove(id, currentTime)

	timeout := &adaptiveTimeout{
		id:       id,
		handler:  handler,
		duration: tm.currentTimeout,
		deadline: currentTime.Add(tm.currentTimeout),
		msgType:  msgType,
	}
	tm.timeoutMap[id] = timeout
	heap.Push(&tm.timeoutQueue, timeout)

	tm.setNextTimeoutTime()
	return timeout.deadline
}

// Remove the timeout associated with [id].
// Its timeout handler will not be called.
func (tm *AdaptiveTimeoutManager) Remove(id ids.ID) {
	tm.lock.Lock()
	defer tm.lock.Unlock()
	tm.remove(id, tm.clock.Time())
}

// Assumes [tm.lock] is held
func (tm *AdaptiveTimeoutManager) remove(id ids.ID, now time.Time) {
	timeout, exists := tm.timeoutMap[id]
	if !exists {
		return
	}

	// Observe the response time to update average network response time
	// Don't include Get requests in calculation, since an adversary
	// can cause you to issue a Get request and then cause it to timeout,
	// increasing your timeout.
	if timeout.msgType != constants.GetMsg {
		timeoutRegisteredAt := timeout.deadline.Add(-1 * timeout.duration)
		latency := now.Sub(timeoutRegisteredAt)
		tm.observeLatencyAndUpdateTimeout(latency, now)
	}

	// Remove the timeout from the map
	delete(tm.timeoutMap, id)

	// Remove the timeout from the queue
	heap.Remove(&tm.timeoutQueue, timeout.index)
}

// Timeout registers a timeout
func (tm *AdaptiveTimeoutManager) Timeout() {
	tm.lock.Lock()
	defer tm.lock.Unlock()
	tm.timeout()
}

// Assumes [tm.lock] is held when called
// and released after this method returns.
func (tm *AdaptiveTimeoutManager) timeout() {
	currentTime := tm.clock.Time()
	for {
		// getNextTimeoutHandler returns nil once there is nothing left to remove
		timeoutHandler := tm.getNextTimeoutHandler(currentTime)
		if timeoutHandler == nil {
			break
		}
		tm.numTimeouts.Inc()

		// Don't execute a callback with a lock held
		tm.lock.Unlock()
		timeoutHandler()
		tm.lock.Lock()
	}
	tm.setNextTimeoutTime()
}

// ObserveLatency allows the caller to manually register a response latency.
// We use this to pretend that it a query to a benched validator
// timed out when actually, we never even sent them a request.
func (tm *AdaptiveTimeoutManager) ObserveLatency(latency time.Duration) {
	tm.lock.Lock()
	defer tm.lock.Unlock()
	tm.observeLatencyAndUpdateTimeout(latency, tm.clock.Time())
}

// Add a latency observation to the averager and update the timeout
// Assumes [tm.lock] is held
func (tm *AdaptiveTimeoutManager) observeLatencyAndUpdateTimeout(latency time.Duration, now time.Time) {
	tm.averager.Observe(float64(latency), now)
	avgLatency := tm.averager.Read()
	tm.currentTimeout = time.Duration(tm.timeoutCoefficient * avgLatency)
	if tm.currentTimeout > tm.maximumTimeout {
		tm.currentTimeout = tm.maximumTimeout
	} else if tm.currentTimeout < tm.minimumTimeout {
		tm.currentTimeout = tm.minimumTimeout
	}
	// Update the metrics
	tm.networkTimeoutMetric.Set(float64(tm.currentTimeout))
	tm.avgLatency.Set(avgLatency)
}

// Returns the handler function associated with the next timeout.
// If there are no timeouts, or if the next timeout is after [currentTime],
// returns nil.
// Assumes [tm.lock] is held
func (tm *AdaptiveTimeoutManager) getNextTimeoutHandler(currentTime time.Time) func() {
	if tm.timeoutQueue.Len() == 0 {
		return nil
	}

	nextTimeout := tm.timeoutQueue[0]
	if nextTimeout.deadline.After(currentTime) {
		return nil
	}
	tm.remove(nextTimeout.id, currentTime)
	return nextTimeout.handler
}

// Calculate the time of the next timeout and set
// the timer to fire at that time.
func (tm *AdaptiveTimeoutManager) setNextTimeoutTime() {
	if tm.timeoutQueue.Len() == 0 {
		// There are no pending timeouts
		tm.timer.Cancel()
		return
	}

	currentTime := tm.clock.Time()
	nextTimeout := tm.timeoutQueue[0]
	timeToNextTimeout := nextTimeout.deadline.Sub(currentTime)
	tm.timer.SetTimeoutIn(timeToNextTimeout)
}
