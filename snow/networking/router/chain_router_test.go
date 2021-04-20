// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package router

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/networking/benchlist"
	"github.com/ava-labs/avalanchego/snow/networking/timeout"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/timer"
)

func TestShutdown(t *testing.T) {
	vdrs := validators.NewSet()
	benchlist := benchlist.NewNoBenchlist()
	tm := timeout.Manager{}
	err := tm.Initialize(&timer.AdaptiveTimeoutConfig{
		InitialTimeout:     time.Millisecond,
		MinimumTimeout:     time.Millisecond,
		MaximumTimeout:     10 * time.Second,
		TimeoutCoefficient: 1.25,
		TimeoutHalflife:    5 * time.Minute,
		MetricsNamespace:   "",
		Registerer:         prometheus.NewRegistry(),
	}, benchlist)
	if err != nil {
		t.Fatal(err)
	}
	go tm.Dispatch()

	chainRouter := ChainRouter{}
	err = chainRouter.Initialize(ids.ShortEmpty, logging.NoLog{}, &tm, time.Hour, time.Second, ids.Set{}, nil, HealthConfig{}, "", prometheus.NewRegistry())
	assert.NoError(t, err)

	engine := common.EngineTest{T: t}
	engine.Default(false)

	shutdownCalled := make(chan struct{}, 1)

	engine.ContextF = snow.DefaultContextTest
	engine.ShutdownF = func() error { shutdownCalled <- struct{}{}; return nil }

	handler := &Handler{}
	err = handler.Initialize(
		&engine,
		vdrs,
		nil,
		1,
		DefaultMaxNonStakerPendingMsgs,
		DefaultStakerPortion,
		DefaultStakerPortion,
		"",
		prometheus.NewRegistry(),
		&Delay{},
	)
	assert.NoError(t, err)

	go handler.Dispatch()

	chainRouter.AddChain(handler)

	chainRouter.Shutdown()

	ticker := time.NewTicker(20 * time.Millisecond)
	select {
	case <-ticker.C:
		t.Fatalf("Handler shutdown was not called or timed out after 20ms during chainRouter shutdown")
	case <-shutdownCalled:
	}

	select {
	case <-handler.closed:
	default:
		t.Fatal("handler shutdown but never closed its closing channel")
	}
}

func TestShutdownTimesOut(t *testing.T) {
	vdrs := validators.NewSet()
	benchlist := benchlist.NewNoBenchlist()
	tm := timeout.Manager{}
	// Ensure that the MultiPut request does not timeout
	err := tm.Initialize(&timer.AdaptiveTimeoutConfig{
		InitialTimeout:     time.Second,
		MinimumTimeout:     500 * time.Millisecond,
		MaximumTimeout:     10 * time.Second,
		TimeoutCoefficient: 1.25,
		TimeoutHalflife:    5 * time.Minute,
		MetricsNamespace:   "",
		Registerer:         prometheus.NewRegistry(),
	}, benchlist)
	if err != nil {
		t.Fatal(err)
	}
	go tm.Dispatch()

	chainRouter := ChainRouter{}
	err = chainRouter.Initialize(ids.ShortEmpty, logging.NoLog{}, &tm, time.Hour, time.Millisecond, ids.Set{}, nil, HealthConfig{}, "", prometheus.NewRegistry())
	assert.NoError(t, err)

	engine := common.EngineTest{T: t}
	engine.Default(false)

	engineFinished := make(chan struct{}, 1)

	// MultiPut blocks for two seconds
	engine.MultiPutF = func(validatorID ids.ShortID, requestID uint32, containers [][]byte) error {
		time.Sleep(2 * time.Second)
		engineFinished <- struct{}{}
		return nil
	}

	closed := new(int)

	engine.ContextF = snow.DefaultContextTest
	engine.ShutdownF = func() error { *closed++; return nil }

	handler := &Handler{}
	err = handler.Initialize(
		&engine,
		vdrs,
		nil,
		1,
		DefaultMaxNonStakerPendingMsgs,
		DefaultStakerPortion,
		DefaultStakerPortion,
		"",
		prometheus.NewRegistry(),
		&Delay{},
	)
	assert.NoError(t, err)

	chainRouter.AddChain(handler)

	go handler.Dispatch()

	shutdownFinished := make(chan struct{}, 1)

	go func() {
		handler.MultiPut(ids.ShortID{}, 1, nil)
		time.Sleep(50 * time.Millisecond) // Pause to ensure message gets processed

		chainRouter.Shutdown()
		shutdownFinished <- struct{}{}
	}()

	select {
	case <-engineFinished:
		t.Fatalf("Shutdown should have finished in one millisecond before timing out instead of waiting for engine to finish shutting down.")
	case <-shutdownFinished:
	}
}

// Ensure that a timeout fires if we don't get a response to a request
func TestRouterTimeout(t *testing.T) {
	// Create a timeout manager
	maxTimeout := 25 * time.Millisecond
	tm := timeout.Manager{}
	err := tm.Initialize(&timer.AdaptiveTimeoutConfig{
		InitialTimeout:     10 * time.Millisecond,
		MinimumTimeout:     10 * time.Millisecond,
		MaximumTimeout:     maxTimeout,
		TimeoutCoefficient: 1,
		TimeoutHalflife:    5 * time.Minute,
		MetricsNamespace:   "",
		Registerer:         prometheus.NewRegistry(),
	}, benchlist.NewNoBenchlist())
	if err != nil {
		t.Fatal(err)
	}
	go tm.Dispatch()

	// Create a router
	chainRouter := ChainRouter{}
	err = chainRouter.Initialize(ids.ShortEmpty, logging.NoLog{}, &tm, time.Hour, time.Millisecond, ids.Set{}, nil, HealthConfig{}, "", prometheus.NewRegistry())
	assert.NoError(t, err)

	// Create an engine and handler
	engine := common.EngineTest{T: t}
	engine.Default(false)

	var (
		calledGetFailed, calledGetAncestorsFailed,
		calledQueryFailed, calledQueryFailed2,
		calledGetAcceptedFailed, calledGetAcceptedFrontierFailed bool

		wg = sync.WaitGroup{}
	)

	engine.GetFailedF = func(validatorID ids.ShortID, requestID uint32) error { wg.Done(); calledGetFailed = true; return nil }
	engine.GetAncestorsFailedF = func(validatorID ids.ShortID, requestID uint32) error {
		defer wg.Done()
		calledGetAncestorsFailed = true
		return nil
	}
	engine.QueryFailedF = func(validatorID ids.ShortID, requestID uint32) error {
		defer wg.Done()
		if !calledQueryFailed {
			calledQueryFailed = true
			return nil
		}
		calledQueryFailed2 = true
		return nil
	}
	engine.GetAcceptedFailedF = func(validatorID ids.ShortID, requestID uint32) error {
		defer wg.Done()
		calledGetAcceptedFailed = true
		return nil
	}
	engine.GetAcceptedFrontierFailedF = func(validatorID ids.ShortID, requestID uint32) error {
		defer wg.Done()
		calledGetAcceptedFrontierFailed = true
		return nil
	}

	engine.ContextF = snow.DefaultContextTest

	handler := &Handler{}
	err = handler.Initialize(
		&engine,
		validators.NewSet(),
		nil,
		DefaultMaxNonStakerPendingMsgs,
		DefaultMaxNonStakerPendingMsgs,
		DefaultStakerPortion,
		DefaultStakerPortion,
		"",
		prometheus.NewRegistry(),
		&Delay{},
	)
	assert.NoError(t, err)

	chainRouter.AddChain(handler)
	go handler.Dispatch()

	// Register requests for each request type
	msgs := []constants.MsgType{
		constants.GetMsg,
		constants.GetAncestorsMsg,
		constants.PullQueryMsg,
		constants.PushQueryMsg,
		constants.GetAcceptedMsg,
		constants.GetAcceptedFrontierMsg,
	}

	wg.Add(len(msgs))

	for i, msg := range msgs {
		chainRouter.RegisterRequest(ids.GenerateTestShortID(), handler.ctx.ChainID, uint32(i), msg)
	}

	wg.Wait()
	chainRouter.lock.Lock()
	defer chainRouter.lock.Unlock()
	assert.True(t, calledGetFailed && calledGetAncestorsFailed && calledQueryFailed2 && calledGetAcceptedFailed && calledGetAcceptedFrontierFailed)
}

func TestRouterClearTimeouts(t *testing.T) {
	// Create a timeout manager
	tm := timeout.Manager{}
	err := tm.Initialize(&timer.AdaptiveTimeoutConfig{
		InitialTimeout:     3 * time.Second,
		MinimumTimeout:     3 * time.Second,
		MaximumTimeout:     5 * time.Minute,
		TimeoutCoefficient: 1,
		TimeoutHalflife:    5 * time.Minute,
		MetricsNamespace:   "",
		Registerer:         prometheus.NewRegistry(),
	}, benchlist.NewNoBenchlist())
	if err != nil {
		t.Fatal(err)
	}
	go tm.Dispatch()

	// Create a router
	chainRouter := ChainRouter{}
	err = chainRouter.Initialize(ids.ShortEmpty, logging.NoLog{}, &tm, time.Hour, time.Millisecond, ids.Set{}, nil, HealthConfig{}, "", prometheus.NewRegistry())
	assert.NoError(t, err)

	// Create an engine and handler
	engine := common.EngineTest{T: t}
	engine.Default(false)

	engine.ContextF = snow.DefaultContextTest

	handler := &Handler{}
	err = handler.Initialize(
		&engine,
		validators.NewSet(),
		nil,
		DefaultMaxNonStakerPendingMsgs,
		DefaultMaxNonStakerPendingMsgs,
		DefaultStakerPortion,
		DefaultStakerPortion,
		"",
		prometheus.NewRegistry(),
		&Delay{},
	)
	assert.NoError(t, err)

	chainRouter.AddChain(handler)
	go handler.Dispatch()

	// Register requests for each request type
	msgs := []constants.MsgType{
		constants.GetMsg,
		constants.GetAncestorsMsg,
		constants.PullQueryMsg,
		constants.PushQueryMsg,
		constants.GetAcceptedMsg,
		constants.GetAcceptedFrontierMsg,
	}

	vID := ids.GenerateTestShortID()
	for i, msg := range msgs {
		chainRouter.RegisterRequest(vID, handler.ctx.ChainID, uint32(i), msg)
	}

	// Clear each timeout by simulating responses to the queries
	// Note: Depends on the ordering of [msgs]
	chainRouter.Put(vID, handler.ctx.ChainID, 0, ids.GenerateTestID(), nil)
	chainRouter.MultiPut(vID, handler.ctx.ChainID, 1, nil)
	chainRouter.Chits(vID, handler.ctx.ChainID, 2, nil)
	chainRouter.Chits(vID, handler.ctx.ChainID, 3, nil)
	chainRouter.Accepted(vID, handler.ctx.ChainID, 4, nil)
	chainRouter.AcceptedFrontier(vID, handler.ctx.ChainID, 5, nil)

	assert.Equal(t, chainRouter.timedRequests.Len(), 0)
}
