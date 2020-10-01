// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package timestampvm

import (
	"errors"
	"time"

	"github.com/ava-labs/avalanchego/vms/components/core"
)

var (
	errTimestampTooEarly = errors.New("block's timestamp is earlier than its parent's timestamp")
	errDatabaseGet       = errors.New("error while retrieving data from database")
	errDatabaseSave      = errors.New("error while saving block to the database")
	errTimestampTooLate  = errors.New("block's timestamp is more than 1 hour ahead of local time")
)

// Block is a block on the chain.
// Each block contains:
// 1) A piece of data (a string)
// 2) A timestamp
type Block struct {
	*core.Block `serialize:"true"`
	Data        [dataLen]byte `serialize:"true"`
	Timestamp   int64         `serialize:"true"`
}

// Verify returns nil iff this block is valid.
// To be valid, it must be that:
// b.parent.Timestamp < b.Timestamp <= [local time] + 1 hour
func (b *Block) Verify() error {
	if accepted, err := b.Block.Verify(); err != nil || accepted {
		return err
	}

	// Get [b]'s parent
	parent, ok := b.ParentBlock().(*Block)
	if !ok {
		return errDatabaseGet
	}

	if b.Timestamp < time.Unix(parent.Timestamp, 0).Unix() {
		return errTimestampTooEarly
	}

	if b.Timestamp >= time.Now().Add(time.Hour).Unix() {
		return errTimestampTooLate
	}

	return nil
}
