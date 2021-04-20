// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package router

import (
	"time"

	"github.com/ava-labs/avalanchego/health"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/networking/timeout"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/prometheus/client_golang/prometheus"
)

// Router routes consensus messages to the Handler of the consensus
// engine that the messages are intended for
type Router interface {
	ExternalRouter
	InternalRouter

	Initialize(
		nodeID ids.ShortID,
		log logging.Logger,
		timeouts *timeout.Manager,
		gossipFrequency,
		shutdownTimeout time.Duration,
		criticalChains ids.Set,
		onFatal func(),
		healthConfig HealthConfig,
		metricsNamespace string,
		metricsRegisterer prometheus.Registerer,
	) error
	Shutdown()
	AddChain(chain *Handler)
	RemoveChain(chainID ids.ID)
	health.Checkable
}

// ExternalRouter routes messages from the network to the
// Handler of the consensus engine that the message is intended for
type ExternalRouter interface {
	RegisterRequest(validatorID ids.ShortID, chainID ids.ID, requestID uint32, msgType constants.MsgType)
	GetAcceptedFrontier(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Time)
	AcceptedFrontier(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID)
	GetAccepted(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Time, containerIDs []ids.ID)
	Accepted(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID)
	GetAncestors(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Time, containerID ids.ID)
	MultiPut(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containers [][]byte)
	Get(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Time, containerID ids.ID)
	Put(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerID ids.ID, container []byte)
	PushQuery(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Time, containerID ids.ID, container []byte)
	PullQuery(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Time, containerID ids.ID)
	Chits(validatorID ids.ShortID, chainID ids.ID, requestID uint32, votes []ids.ID)
}

// InternalRouter deals with messages internal to this node
type InternalRouter interface {
	GetAcceptedFrontierFailed(validatorID ids.ShortID, chainID ids.ID, requestID uint32)
	GetAcceptedFailed(validatorID ids.ShortID, chainID ids.ID, requestID uint32)
	GetFailed(validatorID ids.ShortID, chainID ids.ID, requestID uint32)
	GetAncestorsFailed(validatorID ids.ShortID, chainID ids.ID, requestID uint32)
	QueryFailed(validatorID ids.ShortID, chainID ids.ID, requestID uint32)
	Connected(validatorID ids.ShortID)
	Disconnected(validatorID ids.ShortID)
}
