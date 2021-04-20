// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowman

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/engine/common"
)

// Engine describes the events that can occur to a Snowman instance.
//
// The engine is used to fetch, order, and decide on the fate of blocks. This
// engine runs the leaderless version of the Snowman consensus protocol.
// Therefore, the liveness of this protocol tolerant to O(sqrt(n)) Byzantine
// Nodes where n is the number of nodes in the network. Therefore, this protocol
// should only be run in a Crash Fault Tolerant environment, or in an
// environment where lose of liveness and manual intervention is tolerable.
type Engine interface {
	common.Engine

	// Initialize this engine.
	Initialize(Config) error

	// GetBlock returns a block by its ID.
	// Returns an error if unknown.
	GetBlock(blkID ids.ID) (snowman.Block, error)
}
