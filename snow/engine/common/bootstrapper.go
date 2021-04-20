// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package common

import (
	"fmt"
	"time"

	stdmath "math"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/math"
)

const (
	// MaxContainersPerMultiPut is the maximum number of containers that can be
	// sent in a MultiPut message
	MaxContainersPerMultiPut = 2000

	// StatusUpdateFrequency is how many containers should be processed between
	// logs
	StatusUpdateFrequency = 5000

	// MaxOutstandingRequests is the maximum number of GetAncestors sent but not
	// responded to/failed
	MaxOutstandingRequests = 10

	// MaxTimeFetchingAncestors is the maximum amount of time to spend fetching
	// vertices during a call to GetAncestors
	MaxTimeFetchingAncestors = 50 * time.Millisecond
)

// Bootstrapper implements the Engine interface.
type Bootstrapper struct {
	Config

	RequestID uint32

	// IDs of validators we have requested the accepted frontier from but haven't
	// received a reply from
	pendingAcceptedFrontier ids.ShortSet
	acceptedFrontier        ids.Set
	// True if RestartBootstrap has been called at least once
	Restarted bool

	// holds the beacons that were sampled for the accepted frontier
	sampledBeacons  validators.Set
	pendingAccepted ids.ShortSet
	acceptedVotes   map[ids.ID]uint64

	// current weight
	started bool
	weight  uint64

	// number of times the bootstrap was attempted
	bootstrapAttempts int

	// validators that failed to respond with their frontiers
	failedAcceptedFrontierVdrs ids.ShortSet

	// validators that failed to respond with their frontier votes
	failedAcceptedVdrs ids.ShortSet
}

// Initialize implements the Engine interface.
func (b *Bootstrapper) Initialize(config Config) error {
	b.Config = config
	b.Ctx.Log.Info("Starting bootstrap...")

	b.sampledBeacons = validators.NewSet()

	beacons, err := b.Beacons.Sample(config.SampleK)
	if err != nil {
		return err
	}

	err = b.sampledBeacons.Set(beacons)
	if err != nil {
		return err
	}

	for _, vdr := range beacons {
		vdrID := vdr.ID()
		b.pendingAcceptedFrontier.Add(vdrID)
	}

	for _, vdr := range b.Beacons.List() {
		vdrID := vdr.ID()
		b.pendingAccepted.Add(vdrID)
	}

	b.acceptedVotes = make(map[ids.ID]uint64)
	if b.Config.StartupAlpha > 0 {
		return nil
	}
	return b.Startup()
}

// Startup implements the Engine interface.
func (b *Bootstrapper) Startup() error {
	b.bootstrapAttempts++
	b.started = true
	if b.pendingAcceptedFrontier.Len() == 0 {
		b.Ctx.Log.Info("Bootstrapping skipped due to no provided bootstraps")
		return b.Bootstrapable.ForceAccepted(nil)
	}

	// Ask each of the bootstrap validators to send their accepted frontier
	vdrs := ids.ShortSet{}
	vdrs.Union(b.pendingAcceptedFrontier)

	b.RequestID++
	b.Sender.GetAcceptedFrontier(vdrs, b.RequestID)
	return nil
}

// GetAcceptedFrontier implements the Engine interface.
func (b *Bootstrapper) GetAcceptedFrontier(validatorID ids.ShortID, requestID uint32) error {
	acceptedFrontier, err := b.Bootstrapable.CurrentAcceptedFrontier()
	if err != nil {
		return err
	}
	b.Sender.AcceptedFrontier(validatorID, requestID, acceptedFrontier)
	return nil
}

// GetAcceptedFrontierFailed implements the Engine interface.
func (b *Bootstrapper) GetAcceptedFrontierFailed(validatorID ids.ShortID, requestID uint32) error {
	// ignores any late responses
	if requestID != b.RequestID {
		b.Ctx.Log.Debug("Received an Out-of-Sync GetAcceptedFrontierFailed - validator: %v - expectedRequestID: %v, requestID: %v",
			validatorID,
			b.RequestID,
			requestID)
		return nil
	}

	// If we can't get a response from [validatorID], act as though they said their accepted frontier is empty
	// and we add the validator to the failed list
	b.failedAcceptedFrontierVdrs.Add(validatorID)
	return b.AcceptedFrontier(validatorID, requestID, nil)
}

// AcceptedFrontier implements the Engine interface.
func (b *Bootstrapper) AcceptedFrontier(validatorID ids.ShortID, requestID uint32, containerIDs []ids.ID) error {
	// ignores any late responses
	if requestID != b.RequestID {
		b.Ctx.Log.Debug("Received an Out-of-Sync AcceptedFrontier - validator: %v - expectedRequestID: %v, requestID: %v",
			validatorID,
			b.RequestID,
			requestID)
		return nil
	}

	if !b.pendingAcceptedFrontier.Contains(validatorID) {
		b.Ctx.Log.Debug("Received an AcceptedFrontier message from %s unexpectedly", validatorID)
		return nil
	}

	// Mark that we received a response from [validatorID]
	b.pendingAcceptedFrontier.Remove(validatorID)

	// Union the reported accepted frontier from [validatorID] with the accepted frontier we got from others
	b.acceptedFrontier.Add(containerIDs...)

	// still waiting on requests
	if b.pendingAcceptedFrontier.Len() != 0 {
		return nil
	}

	// We've received the accepted frontier from every bootstrap validator
	// Ask each bootstrap validator to filter the list of containers that we were
	// told are on the accepted frontier such that the list only contains containers
	// they think are accepted
	var err error

	// Create a newAlpha taking using the sampled beacon
	// Keep the proportion of b.Alpha in the newAlpha
	// newAlpha := totalSampledWeight * b.Alpha / totalWeight

	newAlpha := float64(b.sampledBeacons.Weight()*b.Alpha) / float64(b.Beacons.Weight())

	failedBeaconWeight, err := b.Beacons.SubsetWeight(b.failedAcceptedFrontierVdrs)
	if err != nil {
		return err
	}

	// fail the bootstrap if the weight is not enough to bootstrap
	if float64(b.sampledBeacons.Weight())-newAlpha < float64(failedBeaconWeight) {
		if b.Config.RetryBootstrap {
			b.Ctx.Log.Debug("Not enough frontiers received, restarting bootstrap... - Beacons: %d - Failed Bootstrappers: %d "+
				"- bootstrap attempt: %d", b.Beacons.Len(), b.failedAcceptedFrontierVdrs.Len(), b.bootstrapAttempts)
			return b.RestartBootstrap(false)
		}

		b.Ctx.Log.Debug("Didn't receive enough frontiers - failed validators: %d, "+
			"bootstrap attempt: %d", b.failedAcceptedFrontierVdrs.Len(), b.bootstrapAttempts)
	}

	vdrs := ids.ShortSet{}
	vdrs.Union(b.pendingAccepted)

	b.RequestID++
	b.Sender.GetAccepted(vdrs, b.RequestID, b.acceptedFrontier.List())

	return nil
}

// GetAccepted implements the Engine interface.
func (b *Bootstrapper) GetAccepted(validatorID ids.ShortID, requestID uint32, containerIDs []ids.ID) error {
	b.Sender.Accepted(validatorID, requestID, b.Bootstrapable.FilterAccepted(containerIDs))
	return nil
}

// GetAcceptedFailed implements the Engine interface.
func (b *Bootstrapper) GetAcceptedFailed(validatorID ids.ShortID, requestID uint32) error {
	// ignores any late responses
	if requestID != b.RequestID {
		b.Ctx.Log.Debug("Received an Out-of-Sync GetAcceptedFailed - validator: %v - expectedRequestID: %v, requestID: %v",
			validatorID,
			b.RequestID,
			requestID)
		return nil
	}

	// If we can't get a response from [validatorID], act as though they said
	// that they think none of the containers we sent them in GetAccepted are accepted
	b.failedAcceptedVdrs.Add(validatorID)
	return b.Accepted(validatorID, requestID, nil)
}

// Accepted implements the Engine interface.
func (b *Bootstrapper) Accepted(validatorID ids.ShortID, requestID uint32, containerIDs []ids.ID) error {
	// ignores any late responses
	if requestID != b.RequestID {
		b.Ctx.Log.Debug("Received an Out-of-Sync Accepted - validator: %v - expectedRequestID: %v, requestID: %v",
			validatorID,
			b.RequestID,
			requestID)
		return nil
	}

	if !b.pendingAccepted.Contains(validatorID) {
		b.Ctx.Log.Debug("Received an Accepted message from %s unexpectedly", validatorID)
		return nil
	}
	// Mark that we received a response from [validatorID]
	b.pendingAccepted.Remove(validatorID)

	weight := uint64(0)
	if w, ok := b.Beacons.GetWeight(validatorID); ok {
		weight = w
	}

	for _, containerID := range containerIDs {
		previousWeight := b.acceptedVotes[containerID]
		newWeight, err := math.Add64(weight, previousWeight)
		if err != nil {
			b.Ctx.Log.Error("Error calculating the Accepted votes - weight: %v, previousWeight: %v", weight, previousWeight)
			newWeight = stdmath.MaxUint64
		}
		b.acceptedVotes[containerID] = newWeight
	}

	// wait on pending responses
	if b.pendingAccepted.Len() != 0 {
		return nil
	}

	// We've received the filtered accepted frontier from every bootstrap validator
	// Accept all containers that have a sufficient weight behind them
	accepted := make([]ids.ID, 0, len(b.acceptedVotes))
	for containerID, weight := range b.acceptedVotes {
		if weight >= b.Alpha {
			accepted = append(accepted, containerID)
		}
	}

	// if we don't have enough weight for the bootstrap to be accepted then retry or fail the bootstrap
	size := len(accepted)
	if size == 0 && b.Beacons.Len() > 0 {
		// retry the bootstrap if the weight is not enough to bootstrap
		failedBeaconWeight, err := b.Beacons.SubsetWeight(b.failedAcceptedVdrs)
		if err != nil {
			return err
		}

		// in a zero network there will be no accepted votes but the voting weight will be greater than the failed weight
		if b.Config.RetryBootstrap && b.Beacons.Weight()-b.Alpha < failedBeaconWeight {
			b.Ctx.Log.Debug("Not enough votes received, restarting bootstrap... - Beacons: %d - Failed Bootstrappers: %d "+
				"- bootstrap attempt: %d", b.Beacons.Len(), b.failedAcceptedVdrs.Len(), b.bootstrapAttempts)
			return b.RestartBootstrap(false)
		}
	}

	if !b.Restarted {
		b.Ctx.Log.Info("Bootstrapping started syncing with %d vertices in the accepted frontier", size)
	} else {
		b.Ctx.Log.Debug("Bootstrapping started syncing with %d vertices in the accepted frontier", size)
	}

	return b.Bootstrapable.ForceAccepted(accepted)
}

// Connected implements the Engine interface.
func (b *Bootstrapper) Connected(validatorID ids.ShortID) error {
	if b.started {
		return nil
	}
	weight, ok := b.Beacons.GetWeight(validatorID)
	if !ok {
		return nil
	}
	weight, err := math.Add64(weight, b.weight)
	if err != nil {
		return err
	}
	b.weight = weight
	if b.weight < b.StartupAlpha {
		return nil
	}
	return b.Startup()
}

// Disconnected implements the Engine interface.
func (b *Bootstrapper) Disconnected(validatorID ids.ShortID) error {
	if weight, ok := b.Beacons.GetWeight(validatorID); ok {
		// TODO: Account for weight changes in a more robust manner.

		// Sub64 should rarely error since only validators that have added their
		// weight can become disconnected. Because it is possible that there are
		// changes to the validators set, we utilize that Sub64 returns 0 on
		// error.
		b.weight, _ = math.Sub64(b.weight, weight)
	}
	return nil
}

func (b *Bootstrapper) RestartBootstrap(reset bool) error {
	b.Restarted = true

	// resets the attempts when we're pulling blocks/vertices
	// we don't want to fail the bootstrap at that stage
	if reset {
		b.Ctx.Log.Debug("Checking for new frontiers")
		b.bootstrapAttempts = 0
	}

	if b.bootstrapAttempts >= b.RetryBootstrapMaxAttempts {
		return fmt.Errorf("failed to boostrap the chain after %d attempts", b.bootstrapAttempts)
	}

	// reset the failed responses
	b.failedAcceptedFrontierVdrs = ids.ShortSet{}
	b.failedAcceptedVdrs = ids.ShortSet{}
	b.sampledBeacons = validators.NewSet()

	b.acceptedFrontier.Clear()

	beacons, err := b.Beacons.Sample(b.Config.SampleK)
	if err != nil {
		return err
	}

	err = b.sampledBeacons.Set(beacons)
	if err != nil {
		return err
	}

	for _, vdr := range beacons {
		vdrID := vdr.ID()
		b.pendingAcceptedFrontier.Add(vdrID) // necessarily emptied out
	}

	for _, vdr := range b.Beacons.List() {
		vdrID := vdr.ID()
		b.pendingAccepted.Add(vdrID)
	}

	b.acceptedVotes = make(map[ids.ID]uint64)
	return b.Startup()
}
