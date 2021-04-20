// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package router

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/validators"
)

func TestHandlerDropsTimedOutMessages(t *testing.T) {
	engine := common.EngineTest{T: t}
	engine.Default(true)
	engine.ContextF = snow.DefaultContextTest
	called := make(chan struct{})

	engine.GetAcceptedFrontierF = func(validatorID ids.ShortID, requestID uint32) error {
		t.Fatalf("GetAcceptedFrontier message should have timed out")
		return nil
	}

	engine.GetAcceptedF = func(validatorID ids.ShortID, requestID uint32, containerIDs []ids.ID) error {
		called <- struct{}{}
		return nil
	}

	handler := &Handler{}
	vdrs := validators.NewSet()
	vdr0 := ids.GenerateTestShortID()
	if err := vdrs.AddWeight(vdr0, 1); err != nil {
		t.Fatal(err)
	}
	err := handler.Initialize(
		&engine,
		vdrs,
		nil,
		16,
		DefaultMaxNonStakerPendingMsgs,
		DefaultStakerPortion,
		DefaultStakerPortion,
		"",
		prometheus.NewRegistry(),
		&Delay{},
	)
	assert.NoError(t, err)

	currentTime := time.Now()
	handler.clock.Set(currentTime)

	handler.GetAcceptedFrontier(ids.ShortID{}, 1, currentTime.Add(-time.Second))
	handler.GetAccepted(ids.ShortID{}, 1, currentTime.Add(time.Second), nil)

	go handler.Dispatch()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	select {
	case <-ticker.C:
		t.Fatalf("Calling engine function timed out")
	case <-called:
	}
}

func TestHandlerDoesntDrop(t *testing.T) {
	engine := common.EngineTest{T: t}
	engine.Default(false)
	engine.ContextF = snow.DefaultContextTest

	called := make(chan struct{}, 1)

	engine.GetAcceptedFrontierF = func(validatorID ids.ShortID, requestID uint32) error {
		called <- struct{}{}
		return nil
	}

	handler := &Handler{}
	validators := validators.NewSet()
	err := handler.Initialize(
		&engine,
		validators,
		nil,
		16,
		DefaultMaxNonStakerPendingMsgs,
		DefaultStakerPortion,
		DefaultStakerPortion,
		"",
		prometheus.NewRegistry(),
		&Delay{},
	)
	assert.NoError(t, err)

	handler.GetAcceptedFrontier(ids.ShortID{}, 1, time.Time{})
	go handler.Dispatch()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	select {
	case <-ticker.C:
		t.Fatalf("Calling engine function timed out")
	case <-called:
	}
}

func TestHandlerClosesOnError(t *testing.T) {
	engine := common.EngineTest{T: t}
	engine.Default(false)

	closed := make(chan struct{}, 1)

	engine.ContextF = snow.DefaultContextTest
	engine.GetAcceptedFrontierF = func(validatorID ids.ShortID, requestID uint32) error {
		return errors.New("Engine error should cause handler to close")
	}

	handler := &Handler{}
	err := handler.Initialize(
		&engine,
		validators.NewSet(),
		nil,
		16,
		DefaultMaxNonStakerPendingMsgs,
		DefaultStakerPortion,
		DefaultStakerPortion,
		"",
		prometheus.NewRegistry(),
		&Delay{},
	)
	assert.NoError(t, err)

	handler.clock.Set(time.Now())

	handler.toClose = func() {
		closed <- struct{}{}
	}
	go handler.Dispatch()

	handler.GetAcceptedFrontier(ids.ShortID{}, 1, time.Now().Add(time.Second))

	ticker := time.NewTicker(20 * time.Millisecond)
	select {
	case <-ticker.C:
		t.Fatalf("Handler shutdown timed out before calling toClose")
	case <-closed:
	}
}

func TestHandlerDropsGossipDuringBootstrapping(t *testing.T) {
	engine := common.EngineTest{T: t}
	engine.Default(false)

	engine.CantGossip = true

	closed := make(chan struct{}, 1)

	engine.ContextF = snow.DefaultContextTest
	engine.GetFailedF = func(validatorID ids.ShortID, requestID uint32) error {
		closed <- struct{}{}
		return nil
	}

	handler := &Handler{}
	err := handler.Initialize(
		&engine,
		validators.NewSet(),
		nil,
		16,
		DefaultMaxNonStakerPendingMsgs,
		DefaultStakerPortion,
		DefaultStakerPortion,
		"",
		prometheus.NewRegistry(),
		&Delay{},
	)
	assert.NoError(t, err)

	handler.clock.Set(time.Now())

	go handler.Dispatch()

	handler.Gossip()
	handler.GetFailed(ids.ShortEmpty, 1)

	ticker := time.NewTicker(20 * time.Millisecond)
	select {
	case <-ticker.C:
		t.Fatalf("Handler shutdown timed out before calling toClose")
	case <-closed:
	}
}
