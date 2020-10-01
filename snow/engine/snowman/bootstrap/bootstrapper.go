// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/engine/common/queue"
	"github.com/ava-labs/avalanchego/snow/engine/snowman/block"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/vms/components/missing"
)

const (
	// Size of block cache
	// Must be larger than common.MaxContainersPerMultiput
	blockCacheSize = 10000
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

	// Cache of blocks.
	// Key: Block ID
	// Value: The block
	blockCache cache.LRU

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
	b.blockCache = cache.LRU{Size: blockCacheSize}

	if err := b.metrics.Initialize(namespace, registerer); err != nil {
		return err
	}

	b.Blocked.SetParser(&parser{
		log:         config.Ctx.Log,
		numAccepted: b.numAccepted,
		numDropped:  b.numDropped,
		vm:          b.VM,
		bs:          b,
	})

	config.Bootstrapable = b
	return b.Bootstrapper.Initialize(config.Config)
}

// CurrentAcceptedFrontier returns the last accepted block
func (b *Bootstrapper) CurrentAcceptedFrontier() ids.Set {
	acceptedFrontier := ids.Set{}
	acceptedFrontier.Add(b.VM.LastAccepted())
	return acceptedFrontier
}

// FilterAccepted returns the blocks in [containerIDs] that we have accepted
func (b *Bootstrapper) FilterAccepted(containerIDs ids.Set) ids.Set {
	acceptedIDs := ids.Set{}
	for _, blkID := range containerIDs.List() {
		if blk, err := b.GetBlock(blkID); err == nil && blk.Status() == choices.Accepted {
			acceptedIDs.Add(blkID)
		}
	}
	return acceptedIDs
}

// ForceAccepted ...
func (b *Bootstrapper) ForceAccepted(acceptedContainerIDs ids.Set) error {
	if err := b.VM.Bootstrapping(); err != nil {
		return fmt.Errorf("failed to notify VM that bootstrapping has started: %w",
			err)
	}

	for _, blkID := range acceptedContainerIDs.List() {
		if blk, err := b.GetBlock(blkID); err == nil {
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
	if _, err := b.GetBlock(blkID); err == nil {
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
	var blk, tail snowman.Block
	var err error
	for i, blkBytes := range blks {
		if blk, err = b.VM.ParseBlock(blkBytes); err != nil { // Parse the block from bytes
			b.Ctx.Log.Debug("Failed to parse block: %s", err)
			b.Ctx.Log.Verbo("block: %s", formatting.DumpBytes{Bytes: blkBytes})
			if err := b.fetch(wantedBlkID); err != nil {
				return fmt.Errorf("couldn't fetch %s", wantedBlkID)
			}
			break
		}
		if i == 0 {
			tail = blk
		}
		blkID := blk.ID()
		if !blkID.Equals(wantedBlkID) {
			b.Ctx.Log.Debug("expected the next block to be %s but is %s", wantedBlkID, blkID)
			if err := b.fetch(wantedBlkID); err != nil {
				return fmt.Errorf("couldn't fetch %s", wantedBlkID)
			}
			break
		}
		wantedBlkID = blk.Parent()
		if err := b.VM.SaveBlock(blk); err != nil {
			return fmt.Errorf("couldn't save block %s: %w", blk.ID(), err)
		}
		b.blockCache.Put(blkID, blk) // Put block in cache
	}

	return b.process(tail)
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
		err := b.Blocked.Push(&blockJob{
			numAccepted: b.numAccepted,
			numDropped:  b.numDropped,
			blk:         blk,
			bs:          b,
		})
		if err == nil {
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
		blkID = blk.Parent()
		blk, err = b.GetBlock(blkID)
		if err != nil {
			blk = &missing.Block{BlkID: blkID}
		}
		status = blk.Status()
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

		b.Ctx.ConsensusDispatcher.Accept(b.Ctx, job.ID(), job.Bytes())
		b.Ctx.DecisionDispatcher.Accept(b.Ctx, job.ID(), job.Bytes())
	}
	b.Ctx.Log.Info("executed %d blocks", numExecuted)
	return nil
}

// GetBlock gets the block with ID [id].
// Return an error if the block can't be found.
func (b *Bootstrapper) GetBlock(id ids.ID) (snowman.Block, error) {
	if block, ok := b.blockCache.Get(id); ok {
		return block.(snowman.Block), nil
	}
	block, err := b.VM.GetBlock(id)
	if err != nil {
		return nil, err
	}
	b.blockCache.Put(id, block)
	return block, err
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
