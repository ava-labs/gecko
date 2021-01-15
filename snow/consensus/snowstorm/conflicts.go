// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowstorm

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
)

// Conflicts is an interface used to manage conflict sets
type Conflicts interface {
	// Add [tx] to conflict tracking
	//
	// Assumes Add has not already been called with [tx]
	Add(tx conflicts.Tx)

	// Processing returns if [trID] is currently being processed
	Processing(trID ids.ID) bool

	// IsVirtuous returns true if there are no known conflicts with [tx]
	IsVirtuous(tx conflicts.Tx) bool

	// Conflicts returns the known transactions that conflict with [tx]
	Conflicts(tx conflicts.Tx) []conflicts.Tx

	// Mark [txID] as conditionally accepted
	Accept(txID ids.ID)

	// Updateable returns all transactions that can be accepted or rejected
	//
	// Acceptable transactions must have been identified as having been
	// conditionally accepted
	//
	// If an acceptable transaction was marked as having a conflict, then that
	// conflict should be returned in rejectable in the same call the acceptable
	// transaction was returned or in a prior call
	//
	// Acceptable transactions must be returned in in the order they should be
	// accepted
	Updateable() (acceptable []conflicts.Tx, rejectable []conflicts.Tx)
}
