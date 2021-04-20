// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package math

import (
	"testing"
	"time"
)

func TestAverager(t *testing.T) {
	halflife := time.Second
	currentTime := time.Now()

	a := NewSyncAverager(NewAverager(0, halflife, currentTime))
	if value := a.Read(); value != 0 {
		t.Fatalf("wrong value returned. Expected %f ; Returned %f", 0.0, value)
	}

	currentTime = currentTime.Add(halflife)
	a.Observe(1, currentTime)
	if value := a.Read(); value != 1.0/1.5 {
		t.Fatalf("wrong value returned. Expected %f ; Returned %f", 1.0/1.5, value)
	}
}

func TestAveragerTimeTravel(t *testing.T) {
	halflife := time.Second
	currentTime := time.Now()

	a := NewSyncAverager(NewAverager(1, halflife, currentTime))
	if value := a.Read(); value != 1 {
		t.Fatalf("wrong value returned. Expected %f ; Returned %f", 1.0, value)
	}

	currentTime = currentTime.Add(-halflife)
	a.Observe(0, currentTime)
	if value := a.Read(); value != 1.0/1.5 {
		t.Fatalf("wrong value returned. Expected %f ; Returned %f", 1.0/1.5, value)
	}
}
