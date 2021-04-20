package benchlist

import (
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

var (
	minimumFailingDuration = 5 * time.Minute
)

// Test that validators are properly added to the bench
func TestBenchlistAdd(t *testing.T) {
	vdrs := validators.NewSet()
	vdr0 := validators.GenerateRandomValidator(50)
	vdr1 := validators.GenerateRandomValidator(50)
	vdr2 := validators.GenerateRandomValidator(50)
	vdr3 := validators.GenerateRandomValidator(50)
	vdr4 := validators.GenerateRandomValidator(50)

	errs := wrappers.Errs{}
	errs.Add(
		vdrs.AddWeight(vdr0.ID(), vdr0.Weight()),
		vdrs.AddWeight(vdr1.ID(), vdr1.Weight()),
		vdrs.AddWeight(vdr2.ID(), vdr2.Weight()),
		vdrs.AddWeight(vdr3.ID(), vdr3.Weight()),
		vdrs.AddWeight(vdr4.ID(), vdr4.Weight()),
	)
	if errs.Errored() {
		t.Fatal(errs.Err)
	}

	threshold := 3
	duration := time.Minute
	maxPortion := 0.5
	benchIntf, err := NewBenchlist(
		logging.NoLog{},
		vdrs,
		threshold,
		minimumFailingDuration,
		duration,
		maxPortion,
		"",
		prometheus.NewRegistry(),
	)
	if err != nil {
		t.Fatal(err)
	}
	b := benchIntf.(*benchlist)
	defer b.timer.Stop()
	now := time.Now()
	b.clock.Set(now)

	// Nobody should be benched at the start
	b.lock.Lock()
	assert.False(t, b.isBenched(vdr0.ID()))
	assert.False(t, b.isBenched(vdr1.ID()))
	assert.False(t, b.isBenched(vdr2.ID()))
	assert.False(t, b.isBenched(vdr3.ID()))
	assert.False(t, b.isBenched(vdr4.ID()))
	assert.Len(t, b.failureStreaks, 0)
	assert.Equal(t, b.benchedQueue.Len(), 0)
	assert.Equal(t, b.benchlistSet.Len(), 0)
	b.lock.Unlock()

	// Register [threshold - 1] failures in a row for vdr0
	for i := 0; i < threshold-1; i++ {
		b.RegisterFailure(vdr0.ID())
	}

	// Still shouldn't be benched due to not enough consecutive failure
	assert.False(t, b.isBenched(vdr0.ID()))
	assert.Equal(t, b.benchedQueue.Len(), 0)
	assert.Equal(t, b.benchlistSet.Len(), 0)
	assert.Len(t, b.failureStreaks, 1)
	fs := b.failureStreaks[vdr0.ID()]
	assert.Equal(t, threshold-1, fs.consecutive)
	assert.True(t, fs.firstFailure.Equal(now))

	// Register another failure
	b.RegisterFailure(vdr0.ID())

	// Still shouldn't be benched because not enough time (any in this case)
	// has passed since the first failure
	b.lock.Lock()
	assert.False(t, b.isBenched(vdr0.ID()))
	assert.Equal(t, b.benchedQueue.Len(), 0)
	assert.Equal(t, b.benchlistSet.Len(), 0)
	b.lock.Unlock()

	// Move the time up
	now = now.Add(minimumFailingDuration).Add(time.Second)
	b.lock.Lock()
	b.clock.Set(now)
	b.lock.Unlock()

	// Register another failure
	b.RegisterFailure(vdr0.ID())

	// Now this validator should be benched
	b.lock.Lock()
	assert.True(t, b.isBenched(vdr0.ID()))
	assert.Equal(t, b.benchedQueue.Len(), 1)
	assert.Equal(t, b.benchlistSet.Len(), 1)

	next := b.benchedQueue[0]
	assert.Equal(t, vdr0.ID(), next.validatorID)
	assert.True(t, !next.benchedUntil.After(now.Add(duration)))
	assert.True(t, !next.benchedUntil.Before(now.Add(duration/2)))
	assert.Len(t, b.failureStreaks, 0)
	b.lock.Unlock()

	// Give another validator [threshold-1] failures
	for i := 0; i < threshold-1; i++ {
		b.RegisterFailure(vdr1.ID())
	}

	// Advance the time
	b.lock.Lock()
	now = now.Add(minimumFailingDuration)
	b.lock.Unlock()

	// Register another failure
	b.RegisterResponse(vdr1.ID())

	// vdr1 shouldn't be benched
	// The response should have cleared its consecutive failures
	b.lock.Lock()
	assert.True(t, b.isBenched(vdr0.ID()))
	assert.False(t, b.isBenched(vdr1.ID()))
	assert.Equal(t, b.benchedQueue.Len(), 1)
	assert.Equal(t, b.benchlistSet.Len(), 1)
	assert.Len(t, b.failureStreaks, 0)
	b.lock.Unlock()

	// Register another failure for vdr0, who is benched
	b.RegisterFailure(vdr0.ID())

	// A failure for an already benched validator should not count against it
	b.lock.Lock()
	assert.Len(t, b.failureStreaks, 0)
	b.lock.Unlock()
}

// Test that the benchlist won't bench more than the maximum portion of stake
func TestBenchlistMaxStake(t *testing.T) {
	vdrs := validators.NewSet()
	vdr0 := validators.GenerateRandomValidator(1000)
	vdr1 := validators.GenerateRandomValidator(1000)
	vdr2 := validators.GenerateRandomValidator(1000)
	vdr3 := validators.GenerateRandomValidator(2000)
	vdr4 := validators.GenerateRandomValidator(100)
	// Total weight is 5100

	errs := wrappers.Errs{}
	errs.Add(
		vdrs.AddWeight(vdr0.ID(), vdr0.Weight()),
		vdrs.AddWeight(vdr1.ID(), vdr1.Weight()),
		vdrs.AddWeight(vdr2.ID(), vdr2.Weight()),
		vdrs.AddWeight(vdr3.ID(), vdr3.Weight()),
		vdrs.AddWeight(vdr4.ID(), vdr4.Weight()),
	)
	if errs.Errored() {
		t.Fatal(errs.Err)
	}

	threshold := 3
	duration := 1 * time.Hour
	// Shouldn't bench more than 2550 (5100/2)
	maxPortion := 0.5
	benchIntf, err := NewBenchlist(
		logging.NoLog{},
		vdrs,
		threshold,
		minimumFailingDuration,
		duration,
		maxPortion,
		"",
		prometheus.NewRegistry(),
	)
	if err != nil {
		t.Fatal(err)
	}
	b := benchIntf.(*benchlist)
	defer b.timer.Stop()
	now := time.Now()
	b.clock.Set(now)

	// Register [threshold-1] failures for 3 validators
	for _, vdr := range []validators.Validator{vdr0, vdr1, vdr2} {
		for i := 0; i < threshold-1; i++ {
			b.RegisterFailure(vdr.ID())
		}
	}

	// Advance the time to past the minimum failing duration
	newTime := now.Add(minimumFailingDuration).Add(time.Second)
	b.lock.Lock()
	b.clock.Set(newTime)
	b.lock.Unlock()

	// Register another failure for all three
	for _, vdr := range []validators.Validator{vdr0, vdr1, vdr2} {
		b.RegisterFailure(vdr.ID())
	}

	// Only vdr0 and vdr1 should be benched (total weight 2000)
	// Benching vdr2 (weight 1000) would cause the amount benched
	// to exceed the maximum
	b.lock.Lock()
	assert.True(t, b.isBenched(vdr0.ID()))
	assert.True(t, b.isBenched(vdr1.ID()))
	assert.False(t, b.isBenched(vdr2.ID()))
	assert.Equal(t, b.benchedQueue.Len(), 2)
	assert.Equal(t, b.benchlistSet.Len(), 2)
	assert.Len(t, b.failureStreaks, 1)
	fs := b.failureStreaks[vdr2.ID()]
	fs.consecutive = threshold
	fs.firstFailure = now
	b.lock.Unlock()

	// Register threshold - 1 failures for vdr4
	for i := 0; i < threshold-1; i++ {
		b.RegisterFailure(vdr4.ID())
	}

	// Advance the time past min failing duration
	newTime2 := newTime.Add(minimumFailingDuration).Add(time.Second)
	b.lock.Lock()
	b.clock.Set(newTime2)
	b.lock.Unlock()

	// Register another failure for vdr4
	b.RegisterFailure(vdr4.ID())

	// vdr4 should be benched now
	b.lock.Lock()
	assert.True(t, b.isBenched(vdr0.ID()))
	assert.True(t, b.isBenched(vdr1.ID()))
	assert.True(t, b.isBenched(vdr4.ID()))
	assert.Equal(t, 3, b.benchedQueue.Len())
	assert.Equal(t, 3, b.benchlistSet.Len())
	assert.Contains(t, b.benchlistSet, vdr0.ID())
	assert.Contains(t, b.benchlistSet, vdr1.ID())
	assert.Contains(t, b.benchlistSet, vdr4.ID())
	assert.Len(t, b.failureStreaks, 1) // for vdr2
	b.lock.Unlock()

	// More failures for vdr2 shouldn't add it to the bench
	// because the max bench amount would be exceeded
	for i := 0; i < threshold-1; i++ {
		b.RegisterFailure(vdr2.ID())
	}

	b.lock.Lock()
	assert.True(t, b.isBenched(vdr0.ID()))
	assert.True(t, b.isBenched(vdr1.ID()))
	assert.True(t, b.isBenched(vdr4.ID()))
	assert.False(t, b.isBenched(vdr2.ID()))
	assert.Equal(t, 3, b.benchedQueue.Len())
	assert.Equal(t, 3, b.benchlistSet.Len())
	assert.Len(t, b.failureStreaks, 1)
	assert.Contains(t, b.failureStreaks, vdr2.ID())

	// Ensure the benched queue root has the min end time
	minEndTime := b.benchedQueue[0].benchedUntil
	benchedIDs := []ids.ShortID{vdr0.ID(), vdr1.ID(), vdr4.ID()}
	for _, benchedVdr := range b.benchedQueue {
		assert.Contains(t, benchedIDs, benchedVdr.validatorID)
		assert.True(t, !benchedVdr.benchedUntil.Before(minEndTime))
	}

	b.lock.Unlock()
}

// Test validators are removed from the bench correctly
func TestBenchlistRemove(t *testing.T) {
	vdrs := validators.NewSet()
	vdr0 := validators.GenerateRandomValidator(1000)
	vdr1 := validators.GenerateRandomValidator(1000)
	vdr2 := validators.GenerateRandomValidator(1000)
	vdr3 := validators.GenerateRandomValidator(1000)
	vdr4 := validators.GenerateRandomValidator(1000)
	// Total weight is 5100

	errs := wrappers.Errs{}
	errs.Add(
		vdrs.AddWeight(vdr0.ID(), vdr0.Weight()),
		vdrs.AddWeight(vdr1.ID(), vdr1.Weight()),
		vdrs.AddWeight(vdr2.ID(), vdr2.Weight()),
		vdrs.AddWeight(vdr3.ID(), vdr3.Weight()),
		vdrs.AddWeight(vdr4.ID(), vdr4.Weight()),
	)
	if errs.Errored() {
		t.Fatal(errs.Err)
	}

	threshold := 3
	duration := 2 * time.Second
	maxPortion := 0.76 // can bench 3 of the 5 validators
	benchIntf, err := NewBenchlist(
		logging.NoLog{},
		vdrs,
		threshold,
		minimumFailingDuration,
		duration,
		maxPortion,
		"",
		prometheus.NewRegistry(),
	)
	if err != nil {
		t.Fatal(err)
	}
	b := benchIntf.(*benchlist)
	defer b.timer.Stop()
	now := time.Now()
	b.lock.Lock()
	b.clock.Set(now)
	b.lock.Unlock()

	// Register [threshold-1] failures for 3 validators
	for _, vdr := range []validators.Validator{vdr0, vdr1, vdr2} {
		for i := 0; i < threshold-1; i++ {
			b.RegisterFailure(vdr.ID())
		}
	}

	// Advance the time past the min failing duration and register another failure
	// for each
	now = now.Add(minimumFailingDuration).Add(time.Second)
	b.lock.Lock()
	b.clock.Set(now)
	b.lock.Unlock()
	for _, vdr := range []validators.Validator{vdr0, vdr1, vdr2} {
		b.RegisterFailure(vdr.ID())
	}

	// All 3 should be benched
	b.lock.Lock()
	assert.True(t, b.isBenched(vdr0.ID()))
	assert.True(t, b.isBenched(vdr1.ID()))
	assert.True(t, b.isBenched(vdr2.ID()))
	assert.Equal(t, 3, b.benchedQueue.Len())
	assert.Equal(t, 3, b.benchlistSet.Len())
	assert.Len(t, b.failureStreaks, 0)

	// Ensure the benched queue root has the min end time
	minEndTime := b.benchedQueue[0].benchedUntil
	benchedIDs := []ids.ShortID{vdr0.ID(), vdr1.ID(), vdr2.ID()}
	for _, benchedVdr := range b.benchedQueue {
		assert.Contains(t, benchedIDs, benchedVdr.validatorID)
		assert.True(t, !benchedVdr.benchedUntil.Before(minEndTime))
	}

	// Set the benchlist's clock past when all validators should be unbenched
	// so that when its timer fires, it can remove them
	b.clock.Set(b.clock.Time().Add(duration))
	b.lock.Unlock()

	// Make sure each validator is eventually removed
	assert.Eventually(
		t,
		func() bool {
			return !b.IsBenched(vdr0.ID())
		},
		duration+time.Second, // extra time.Second as grace period
		100*time.Millisecond,
	)

	assert.Eventually(
		t,
		func() bool {
			return !b.IsBenched(vdr1.ID())
		},
		duration+time.Second,
		100*time.Millisecond,
	)

	assert.Eventually(
		t,
		func() bool {
			return !b.IsBenched(vdr2.ID())
		},
		duration+time.Second,
		100*time.Millisecond,
	)

}
