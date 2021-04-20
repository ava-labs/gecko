package benchlist

import (
	"container/heap"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/prometheus/client_golang/prometheus"

	safemath "github.com/ava-labs/avalanchego/utils/math"
)

// If a peer consistently does not respond to queries, it will
// increase latencies on the network whenever that peer is polled.
// If we cannot terminate the poll early, then the poll will wait
// the full timeout before finalizing the poll and making progress.
// This can increase network latencies to an undesirable level.

// Therefore, nodes that consistently fail are "benched" such that
// queries to that node fail immediately to avoid waiting up to
// the full network timeout for a response.

// Benchlist ...
type Benchlist interface {
	// RegisterResponse registers the response to a query message
	RegisterResponse(validatorID ids.ShortID)
	// RegisterFailure registers that we didn't receive a response within the timeout
	RegisterFailure(validatorID ids.ShortID)
	// IsBenched returns true if messages to [validatorID]
	// should not be sent over the network and should immediately fail.
	IsBenched(validatorID ids.ShortID) bool
}

// Data about a validator who is benched
type benchData struct {
	benchedUntil time.Time
	validatorID  ids.ShortID
	index        int
}

// Implements heap.Interface. Each element is a benched validator
type benchedQueue []*benchData

func (bq benchedQueue) Len() int           { return len(bq) }
func (bq benchedQueue) Less(i, j int) bool { return bq[i].benchedUntil.Before(bq[j].benchedUntil) }
func (bq benchedQueue) Swap(i, j int) {
	bq[i], bq[j] = bq[j], bq[i]
	bq[i].index = i
	bq[j].index = j
}

// Push adds an item to this  queue. x must have type *benchData
func (bq *benchedQueue) Push(x interface{}) {
	item := x.(*benchData)
	item.index = len(*bq)
	*bq = append(*bq, item)
}

// Pop returns the validator that should leave the bench next
func (bq *benchedQueue) Pop() interface{} {
	n := len(*bq)
	item := (*bq)[n-1]
	(*bq)[n-1] = nil // make sure the item is freed from memory
	*bq = (*bq)[:n-1]
	return item
}

type failureStreak struct {
	// Time of first consecutive timeout
	firstFailure time.Time
	// Number of consecutive message timeouts
	consecutive int
}

type benchlist struct {
	lock    sync.RWMutex
	log     logging.Logger
	metrics metrics

	// Fires when the next validator should leave the bench
	// Calls [update] when it fires
	timer *timer.Timer

	// Tells the time. Can be faked for testing.
	clock timer.Clock

	// Validator set of the network
	vdrs validators.Set

	// Validator ID --> Consecutive failure information
	failureStreaks map[ids.ShortID]failureStreak

	// IDs of validators that are currently benched
	benchlistSet ids.ShortSet

	// Min heap containing benched validators and their endtimes
	// Pop() returns the next validator to leave
	benchedQueue benchedQueue

	// A validator will be benched if [threshold] messages in a row
	// to them time out and the first of those messages was more than
	// [minimumFailingDuration] ago
	threshold              int
	minimumFailingDuration time.Duration

	// A benched validator will be benched for between [duration/2] and [duration]
	duration time.Duration

	// The maximum percentage of total network stake that may be benched
	// Must be in [0,1)
	maxPortion float64
}

// NewBenchlist returns a new Benchlist
func NewBenchlist(
	log logging.Logger,
	validators validators.Set,
	threshold int,
	minimumFailingDuration,
	duration time.Duration,
	maxPortion float64,
	namespace string,
	registerer prometheus.Registerer,
) (Benchlist, error) {
	if maxPortion < 0 || maxPortion >= 1 {
		return nil, fmt.Errorf("max portion of benched stake must be in [0,1) but got %f", maxPortion)
	}
	benchlist := &benchlist{
		log:                    log,
		failureStreaks:         make(map[ids.ShortID]failureStreak),
		benchlistSet:           ids.ShortSet{},
		vdrs:                   validators,
		threshold:              threshold,
		minimumFailingDuration: minimumFailingDuration,
		duration:               duration,
		maxPortion:             maxPortion,
	}
	benchlist.timer = timer.NewTimer(benchlist.update)
	go benchlist.timer.Dispatch()
	return benchlist, benchlist.metrics.Initialize(registerer, namespace)
}

// Update removes benched validators whose time on the bench is over
func (b *benchlist) update() {
	b.lock.Lock()
	defer b.lock.Unlock()

	now := b.clock.Time()
	for {
		// [next] is nil when no more validators should
		// leave the bench at this time
		next := b.nextToLeave(now)
		if next == nil {
			break
		}
		b.remove(next)
	}
	// Set next time update will be called
	b.setNextLeaveTime()
}

// Remove [validator] from the benchlist
// Assumes [b.lock] is held
func (b *benchlist) remove(validator *benchData) {
	// Update state
	id := validator.validatorID
	b.log.Debug("removing validator %s from benchlist", id)
	heap.Remove(&b.benchedQueue, validator.index)
	b.benchlistSet.Remove(id)

	// Update metrics
	b.metrics.numBenched.Set(float64(b.benchedQueue.Len()))
	benchedStake, err := b.vdrs.SubsetWeight(b.benchlistSet)
	if err != nil {
		// This should never happen
		b.log.Error("couldn't get benched stake: %w", err)
		return
	}
	b.metrics.weightBenched.Set(float64(benchedStake))
}

// Returns the next validator that should leave
// the bench at time [now]. nil if no validator should.
// Assumes [b.lock] is held
func (b *benchlist) nextToLeave(now time.Time) *benchData {
	if b.benchedQueue.Len() == 0 {
		return nil
	}
	next := b.benchedQueue[0]
	if now.Before(next.benchedUntil) {
		return nil
	}
	return next
}

// Set [b.timer] to fire when the next validator should leave the bench
// Assumes [b.lock] is held
func (b *benchlist) setNextLeaveTime() {
	if b.benchedQueue.Len() == 0 {
		b.timer.Cancel()
		return
	}
	now := b.clock.Time()
	next := b.benchedQueue[0]
	nextLeave := next.benchedUntil.Sub(now)
	b.timer.SetTimeoutIn(nextLeave)
}

// IsBenched returns true if messages to [validatorID]
// should not be sent over the network and should immediately fail.
func (b *benchlist) IsBenched(validatorID ids.ShortID) bool {
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.isBenched(validatorID)
}

// isBenched checks if [validatorID] is currently benched
// and calls cleanup if its benching period has elapsed
// Assumes [b.lock] is held.
func (b *benchlist) isBenched(validatorID ids.ShortID) bool {
	if _, ok := b.benchlistSet[validatorID]; ok {
		return true
	}
	return false
}

// RegisterResponse notes that we received a response from validator [validatorID]
func (b *benchlist) RegisterResponse(validatorID ids.ShortID) {
	b.lock.Lock()
	defer b.lock.Unlock()
	delete(b.failureStreaks, validatorID)
}

// RegisterResponse notes that a request to validator [validatorID] timed out
func (b *benchlist) RegisterFailure(validatorID ids.ShortID) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.benchlistSet.Contains(validatorID) {
		// This validator is benched. Ignore failures until they're not.
		return
	}

	failureStreak := b.failureStreaks[validatorID]
	// Increment consecutive failures
	failureStreak.consecutive++
	now := b.clock.Time()
	// Update first failure time
	if failureStreak.firstFailure.IsZero() {
		// This is the first consecutive failure
		failureStreak.firstFailure = now
	}
	b.failureStreaks[validatorID] = failureStreak

	if failureStreak.consecutive >= b.threshold && now.After(failureStreak.firstFailure.Add(b.minimumFailingDuration)) {
		b.bench(validatorID)
	}

}

// Assumes [b.lock] is held
// Assumes [validatorID] is not already benched
func (b *benchlist) bench(validatorID ids.ShortID) {
	benchedStake, err := b.vdrs.SubsetWeight(b.benchlistSet)
	if err != nil {
		// This should never happen
		b.log.Error("couldn't get benched stake: %w. Resetting benchlist", err)
		return
	}

	validatorStake, isVdr := b.vdrs.GetWeight(validatorID)
	if !isVdr {
		// We might want to bench a non-validator because they don't respond to
		// my Get requests, but we choose to only bench validators.
		return
	}

	newBenchedStake, err := safemath.Add64(benchedStake, validatorStake)
	if err != nil {
		// This should never happen
		b.log.Error("overflow calculating new benched stake with validator %s", validatorID)
		return
	}

	totalStake := b.vdrs.Weight()
	maxBenchedStake := float64(totalStake) * b.maxPortion

	if float64(newBenchedStake) > maxBenchedStake {
		b.log.Debug(
			"not benching %s because benched stake (%f) would exceed max (%f)",
			validatorID,
			float64(newBenchedStake),
			maxBenchedStake,
		)
		return
	}

	// Validator is benched for between [b.duration]/2 and [b.duration]
	now := b.clock.Time()
	minBenchDuration := b.duration / 2
	minBenchedUntil := now.Add(minBenchDuration)
	maxBenchedUntil := now.Add(b.duration)
	diff := maxBenchedUntil.Sub(minBenchedUntil)
	benchedUntil := minBenchedUntil.Add(time.Duration(rand.Float64() * float64(diff))) // #nosec G404

	// Add to benchlist times with randomized delay
	b.benchlistSet.Add(validatorID)
	delete(b.failureStreaks, validatorID)
	heap.Push(
		&b.benchedQueue,
		&benchData{validatorID: validatorID, benchedUntil: benchedUntil},
	)
	b.log.Debug(
		"benching validator %s for %s after %d consecutive failed queries.",
		validatorID,
		benchedUntil.Sub(now),
		b.threshold,
	)

	// Set [b.timer] to fire when next validator should leave bench
	b.setNextLeaveTime()

	// Update metrics
	b.metrics.numBenched.Set(float64(b.benchedQueue.Len()))
	b.metrics.weightBenched.Set(float64(newBenchedStake))
}
