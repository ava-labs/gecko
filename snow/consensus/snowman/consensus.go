// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowman

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/consensus/snowball"
)

// Consensus represents a general snowman instance that can be used directly to
// process a series of dependent operations.
type Consensus interface {
	// Takes in the context, snowball parameters, and the last accepted block.
	Initialize(
		ctx *snow.Context,
		params snowball.Parameters,
		lastAcceptedID ids.ID,
		lastAcceptedHeight uint64,
	) error

	// Returns the parameters that describe this snowman instance
	Parameters() snowball.Parameters

	// Returns the number of blocks processing
	NumProcessing() int

	// Adds a new decision. Assumes the dependency has already been added.
	// Returns if a critical error has occurred.
	Add(Block) error

	// AcceptedOrProcessing returns true if the block has been accepted or is
	// currently processing
	AcceptedOrProcessing(Block) bool

	// DecidedOrProcessing returns true if the block has been decided or is
	// currently processing.
	DecidedOrProcessing(Block) bool

	// IsPreferred returns true if the block is currently on the preferred
	// chain.
	IsPreferred(Block) bool

	// Returns the ID of the tail of the strongly preferred sequence of
	// decisions.
	Preference() ids.ID

	// RecordPoll collects the results of a network poll. Assumes all decisions
	// have been previously added. Returns if a critical error has occurred.
	RecordPoll(ids.Bag) error

	// Finalized returns true if all decisions that have been added have been
	// finalized. Note, it is possible that after returning finalized, a new
	// decision may be added such that this instance is no longer finalized.
	Finalized() bool

	// HealthCheck returns information about the consensus health.
	HealthCheck() (interface{}, error)
}
