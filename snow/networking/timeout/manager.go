// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package timeout

import (
	"fmt"
	"sync"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/networking/benchlist"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/timer"
)

// Manager registers and fires timeouts for the snow API.
type Manager struct {
	lock         sync.Mutex
	tm           timer.AdaptiveTimeoutManager
	benchlistMgr benchlist.Manager
	metrics      metrics
}

// Initialize this timeout manager.
func (m *Manager) Initialize(timeoutConfig *timer.AdaptiveTimeoutConfig, benchlistMgr benchlist.Manager) error {
	m.benchlistMgr = benchlistMgr
	return m.tm.Initialize(timeoutConfig)
}

// Dispatch ...
func (m *Manager) Dispatch() {
	m.tm.Dispatch()
}

// TimeoutDuration returns the current network timeout duration
func (m *Manager) TimeoutDuration() time.Duration {
	return m.tm.TimeoutDuration()
}

// IsBenched returns true if messages to [validatorID] regarding [chainID]
// should not be sent over the network and should immediately fail.
func (m *Manager) IsBenched(validatorID ids.ShortID, chainID ids.ID) bool {
	return m.benchlistMgr.IsBenched(validatorID, chainID)
}

// RegisterChain ...
func (m *Manager) RegisterChain(ctx *snow.Context, namespace string) error {
	if err := m.metrics.RegisterChain(ctx, namespace); err != nil {
		return fmt.Errorf("couldn't register timeout metrics for chain %s: %w", ctx.ChainID, err)
	}
	if err := m.benchlistMgr.RegisterChain(ctx, namespace); err != nil {
		return fmt.Errorf("couldn't register chain %s with benchlist manager: %w", ctx.ChainID, err)
	}
	return nil
}

// RegisterRequests notes that we sent a request of type [msgType] to [validatorID]
// regarding chain [chainID]. If we don't receive a response in time, [timeoutHandler]
// is executed.
func (m *Manager) RegisterRequest(
	validatorID ids.ShortID,
	chainID ids.ID,
	msgType constants.MsgType,
	uniqueRequestID ids.ID,
	timeoutHandler func(),
) (time.Time, bool) {
	newTimeoutHandler := func() {
		// If this request timed out, tell the benchlist manager
		m.benchlistMgr.RegisterFailure(chainID, validatorID)
		timeoutHandler()
	}
	return m.tm.Put(uniqueRequestID, msgType, newTimeoutHandler), true
}

// RegisterResponse registers that we received a response from [validatorID]
// regarding the given request ID and chain.
func (m *Manager) RegisterResponse(
	validatorID ids.ShortID,
	chainID ids.ID,
	uniqueRequestID ids.ID,
	msgType constants.MsgType,
	latency time.Duration,
) {
	m.lock.Lock()
	m.metrics.observe(chainID, msgType, latency)
	m.lock.Unlock()
	m.benchlistMgr.RegisterResponse(chainID, validatorID)
	m.tm.Remove(uniqueRequestID)
}

// RegisterRequestToUnreachableValidator registers that we would have sent
// a query to a validator but they are unreachable because they are bench
// or because of network conditions (e.g. we're not connected), so we didn't
// send the query. For the sake// of calculating the average latency and
// network timeout, we act as though we sent the validator a request and it timed out.
func (m *Manager) RegisterRequestToUnreachableValidator() {
	m.tm.ObserveLatency(m.TimeoutDuration())
}
