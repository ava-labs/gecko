// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/snow/engine/common"
)

// DAGVM defines the minimum functionality that an avalanche VM must
// implement
type DAGVM interface {
	common.VM

	// Return any transitions that have not been sent to consensus yet
	Pending() []conflicts.Transition

	// Convert a stream of bytes to a transition or return an error
	Parse(tr []byte) (conflicts.Transition, error)

	// Retrieve a transition that was submitted previously
	Get(ids.ID) (conflicts.Transition, error)
}
