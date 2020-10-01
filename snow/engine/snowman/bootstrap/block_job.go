// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/engine/common/queue"
	"github.com/ava-labs/avalanchego/snow/engine/snowman/block"
	"github.com/ava-labs/avalanchego/utils/logging"
)

type parser struct {
	log                     logging.Logger
	numAccepted, numDropped prometheus.Counter
	vm                      block.ChainVM
	bs                      *Bootstrapper
}

func (p *parser) Parse(blkBytes []byte) (queue.Job, error) {
	blk, err := p.vm.ParseBlock(blkBytes)
	if err != nil {
		return nil, err
	}
	p.bs.blockCache.Put(blk.ID(), blk)
	return &blockJob{
		log:         p.log,
		numAccepted: p.numAccepted,
		numDropped:  p.numDropped,
		blk:         blk,
		bs:          p.bs,
	}, nil
}

type blockJob struct {
	log                     logging.Logger
	numAccepted, numDropped prometheus.Counter
	blk                     snowman.Block
	bs                      *Bootstrapper
}

func (b *blockJob) ID() ids.ID { return b.blk.ID() }
func (b *blockJob) MissingDependencies() (ids.Set, error) {
	missing := ids.Set{}
	parentID := b.blk.Parent()
	if parent, err := b.bs.GetBlock(parentID); err != nil || parent.Status() != choices.Accepted { // Can't get parent block --> parent is not accepted
		missing.Add(parentID)
	}
	return missing, nil
}
func (b *blockJob) Execute() error {
	deps, err := b.MissingDependencies()
	if err != nil {
		return err
	}
	if deps.Len() != 0 {
		b.numDropped.Inc()
		return errors.New("attempting to accept a block with missing dependencies")
	}
	status := b.blk.Status()
	switch status {
	case choices.Unknown, choices.Rejected:
		b.numDropped.Inc()
		return fmt.Errorf("attempting to execute block with status %s", status)
	case choices.Processing:
		if err := b.blk.Verify(); err != nil {
			return fmt.Errorf("block %s failed verification during bootstrapping due to: %w",
				b.blk.ID(), err)
		}

		b.numAccepted.Inc()
		if err := b.blk.Accept(); err != nil {
			b.log.Debug("block %s failed to accept during bootstrapping due to %s",
				b.blk.ID(), err)
			return fmt.Errorf("failed to accept block in bootstrapping: %w", err)
		}
		if err := b.bs.VM.SaveBlock(b.blk); err != nil {
			return fmt.Errorf("couldn't save block %s: %w", b.blk.ID(), err)
		}

	}
	return nil
}
func (b *blockJob) Bytes() []byte { return b.blk.Bytes() }
