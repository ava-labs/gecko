// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/engine/common/queue"
	"github.com/ava-labs/avalanchego/snow/engine/snowman/block"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/formatting"
)

// Config ...
type Config struct {
	common.Config

	// Blocked tracks operations that are blocked on blocks
	Blocked *queue.Jobs

	VM block.ChainVM

	Bootstrapped func()
}

// Bootstrapper ...
type Bootstrapper struct {
	common.Bootstrapper
	common.Fetcher
	metrics

	// Blocked tracks operations that are blocked on blocks
	Blocked *queue.Jobs

	VM block.ChainVM

	Bootstrapped func()

	// true if all of the vertices in the original accepted frontier have been processed
	processedStartingAcceptedFrontier bool
}

// Initialize this engine.
func (b *Bootstrapper) Initialize(
	config Config,
	onFinished func() error,
	namespace string,
	registerer prometheus.Registerer,
) error {
	b.Blocked = config.Blocked
	b.VM = config.VM
	b.Bootstrapped = config.Bootstrapped
	b.OnFinished = onFinished

	if err := b.metrics.Initialize(namespace, registerer); err != nil {
		return err
	}

	b.Blocked.SetParser(&parser{
		log:         config.Ctx.Log,
		numAccepted: b.numAccepted,
		numDropped:  b.numDropped,
		vm:          b.VM,
	})

	config.Bootstrapable = b
	return b.Bootstrapper.Initialize(config.Config)
}

// CurrentAcceptedFrontier returns the last accepted block
func (b *Bootstrapper) CurrentAcceptedFrontier() []ids.ID {
	acceptedFrontier := []ids.ID{b.VM.LastAccepted()}
	return acceptedFrontier
}

// FilterAccepted returns the blocks in [containerIDs] that we have accepted
func (b *Bootstrapper) FilterAccepted(containerIDs []ids.ID) []ids.ID {
	acceptedIDs := make([]ids.ID, 0, len(containerIDs))
	for _, blkID := range containerIDs {
		if blk, err := b.VM.GetBlock(blkID); err == nil && blk.Status() == choices.Accepted {
			acceptedIDs = append(acceptedIDs, blkID)
		}
	}
	return acceptedIDs
}

// ForceAccepted ...
func (b *Bootstrapper) ForceAccepted(acceptedContainerIDs []ids.ID) error {
	if err := b.VM.Bootstrapping(); err != nil {
		return fmt.Errorf("failed to notify VM that bootstrapping has started: %w",
			err)
	}

	for _, blkID := range acceptedContainerIDs {
		if blk, err := b.VM.GetBlock(blkID); err == nil {
			if err := b.process(blk); err != nil {
				return err
			}
		} else if err := b.fetch(blkID); err != nil {
			return err
		}
	}

	b.processedStartingAcceptedFrontier = true
	if numPending := b.OutstandingRequests.Len(); numPending == 0 {
		return b.finish()
	}
	return nil
}

// Get block [blkID] and its ancestors from a validator
func (b *Bootstrapper) fetch(blkID ids.ID) error {
	// Make sure we haven't already requested this block
	if b.OutstandingRequests.Contains(blkID) {
		return nil
	}

	// Make sure we don't already have this block
	if _, err := b.VM.GetBlock(blkID); err == nil {
		if numPending := b.OutstandingRequests.Len(); numPending == 0 && b.processedStartingAcceptedFrontier {
			return b.finish()
		}
		return nil
	}

	validators, err := b.Beacons.Sample(1) // validator to send request to
	if err != nil {
		return fmt.Errorf("dropping request for %s as there are no validators", blkID)
	}
	validatorID := validators[0].ID()
	b.RequestID++

	b.OutstandingRequests.Add(validatorID, b.RequestID, blkID)
	b.Sender.GetAncestors(validatorID, b.RequestID, blkID) // request block and ancestors
	return nil
}

// MultiPut handles the receipt of multiple containers. Should be received in response to a GetAncestors message to [vdr]
// with request ID [requestID]
func (b *Bootstrapper) MultiPut(vdr ids.ShortID, requestID uint32, blks [][]byte) error {
	if lenBlks := len(blks); lenBlks > common.MaxContainersPerMultiPut {
		b.Ctx.Log.Debug("MultiPut(%s, %d) contains more than maximum number of blocks",
			vdr, requestID)
		return b.GetAncestorsFailed(vdr, requestID)
	} else if lenBlks == 0 {
		b.Ctx.Log.Debug("MultiPut(%s, %d) contains no blocks", vdr, requestID)
		return b.GetAncestorsFailed(vdr, requestID)
	}

	// Make sure this is in response to a request we made
	wantedBlkID, ok := b.OutstandingRequests.Remove(vdr, requestID)
	if !ok { // this message isn't in response to a request we made
		b.Ctx.Log.Debug("received unexpected MultiPut from %s with ID %d",
			vdr, requestID)
		return nil
	}

	wantedBlk, err := b.VM.ParseBlock(blks[0]) // the block we requested
	if err != nil {
		b.Ctx.Log.Debug("Failed to parse requested block %s: %s", wantedBlkID, err)
		return b.fetch(wantedBlkID)
	} else if actualID := wantedBlk.ID(); actualID != wantedBlkID {
		b.Ctx.Log.Debug("expected the first block to be the requested block, %s, but is %s",
			wantedBlk, actualID)
		return b.fetch(wantedBlkID)
	}

	for _, blkBytes := range blks {
		if _, err := b.VM.ParseBlock(blkBytes); err != nil { // persists the block
			b.Ctx.Log.Debug("Failed to parse block: %s", err)
			b.Ctx.Log.Verbo("block: %s", formatting.DumpBytes{Bytes: blkBytes})
		}
	}

	return b.process(wantedBlk)
}

// GetAncestorsFailed is called when a GetAncestors message we sent fails
func (b *Bootstrapper) GetAncestorsFailed(vdr ids.ShortID, requestID uint32) error {
	blkID, ok := b.OutstandingRequests.Remove(vdr, requestID)
	if !ok {
		b.Ctx.Log.Debug("GetAncestorsFailed(%s, %d) called but there was no outstanding request to this validator with this ID",
			vdr, requestID)
		return nil
	}
	// Send another request for this
	return b.fetch(blkID)
}

// process a block
func (b *Bootstrapper) process(blk snowman.Block) error {
	status := blk.Status()
	blkID := blk.ID()
	for status == choices.Processing {
		if err := b.Blocked.Push(&blockJob{
			numAccepted: b.numAccepted,
			numDropped:  b.numDropped,
			blk:         blk,
		}); err == nil {
			b.numFetched.Inc()
			b.NumFetched++                                      // Progress tracker
			if b.NumFetched%common.StatusUpdateFrequency == 0 { // Periodically print progress
				b.Ctx.Log.Info("fetched %d blocks", b.NumFetched)
			}
		}

		if err := b.Blocked.Commit(); err != nil {
			return err
		}

		// Process this block's parent
		blk = blk.Parent()
		status = blk.Status()
		blkID = blk.ID()
	}

	switch status := blk.Status(); status {
	case choices.Unknown:
		if err := b.fetch(blkID); err != nil {
			return err
		}
	case choices.Rejected: // Should never happen
		return fmt.Errorf("bootstrapping wants to accept %s, however it was previously rejected", blkID)
	}

	if numPending := b.OutstandingRequests.Len(); numPending == 0 && b.processedStartingAcceptedFrontier {
		return b.finish()
	}
	return nil
}

func (b *Bootstrapper) finish() error {
	if b.IsBootstrapped() {
		return nil
	}
	b.Ctx.Log.Info("bootstrapping fetched %d blocks. executing state transitions...",
		b.NumFetched)

	if err := b.executeAll(b.Blocked); err != nil {
		return err
	}

	if err := b.VM.Bootstrapped(); err != nil {
		return fmt.Errorf("failed to notify VM that bootstrapping has finished: %w",
			err)
	}

	// Start consensus
	if err := b.OnFinished(); err != nil {
		return err
	}
	b.Ctx.Bootstrapped()

	if b.Bootstrapped != nil {
		b.Bootstrapped()
	}
	return nil
}

func (b *Bootstrapper) executeAll(jobs *queue.Jobs) error {
	numExecuted := 0
	for job, err := jobs.Pop(); err == nil; job, err = jobs.Pop() {
		if err := jobs.Execute(job); err != nil {
			return err
		}
		if err := jobs.Commit(); err != nil {
			return err
		}
		numExecuted++
		if numExecuted%common.StatusUpdateFrequency == 0 { // Periodically print progress
			b.Ctx.Log.Info("executed %d blocks", numExecuted)
		}

		jobID := job.DispatchID()
		jobBytes := job.DispatchBytes()
		b.Ctx.ConsensusDispatcher.Accept(b.Ctx, jobID, jobBytes)
		b.Ctx.DecisionDispatcher.Accept(b.Ctx, jobID, jobBytes)
	}
	b.Ctx.Log.Info("executed %d blocks", numExecuted)
	return nil
}

// Connected implements the Engine interface.
func (b *Bootstrapper) Connected(validatorID ids.ShortID) error {
	if connector, ok := b.VM.(validators.Connector); ok {
		connector.Connected(validatorID)
	}
	return b.Bootstrapper.Connected(validatorID)
}

// Disconnected implements the Engine interface.
func (b *Bootstrapper) Disconnected(validatorID ids.ShortID) error {
	if connector, ok := b.VM.(validators.Connector); ok {
		connector.Disconnected(validatorID)
	}
	return b.Bootstrapper.Disconnected(validatorID)
}
