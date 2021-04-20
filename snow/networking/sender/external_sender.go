// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package sender

import (
	"time"

	"github.com/ava-labs/avalanchego/ids"
)

// ExternalSender sends consensus messages to other validators
// Right now this is implemented in the networking package
type ExternalSender interface {
	// Send a GetAcceptedFrontier message for chain [chainID] to validators in [validatorIDs].
	// The validator should reply by [deadline].
	// Returns the IDs of validators that may receive the message.
	// If we're not connected to a validator in [validatorIDs], for example,
	// it will not be included in the return value.
	GetAcceptedFrontier(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration) []ids.ShortID
	AcceptedFrontier(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID)

	GetAccepted(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, containerIDs []ids.ID) []ids.ShortID
	Accepted(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID)

	// Request ancestors of container [containerID] in chain [chainID] from validator [validatorID].
	// The validator should reply by [deadline].
	// Returns true if the validator may receive the message.
	// If we're not connected to [validatorID], for example, returns false.
	GetAncestors(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID) bool
	MultiPut(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containers [][]byte)

	Get(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID) bool
	Put(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerID ids.ID, container []byte)

	PushQuery(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID, container []byte) []ids.ShortID
	PullQuery(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID) []ids.ShortID
	Chits(validatorID ids.ShortID, chainID ids.ID, requestID uint32, votes []ids.ID)

	Gossip(chainID ids.ID, containerID ids.ID, container []byte)
}
