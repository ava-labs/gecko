// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package conflicts

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
)

// Tx consumes state.
type Tx interface {
	choices.Decidable

	// TransitionID is a unique identifier for the transition this transaction
	// will perform.
	TransitionID() ids.ID

	// Epoch this transaction was issued in.
	Epoch() uint32

	// Restrictions returns a list of transition IDs that need to be performed in
	// this transaction's epoch or a later one if this transaction is to be accepted.
	Restrictions() []ids.ID

	// Dependencies is a list of transition IDs that need to be performed before
	// this transaction is to be accepted, that have not already been performed.
	// These dependencies can be executed in the same epoch as this transaction.
	Dependencies() []ids.ID

	// InputIDs is a list where each element is the ID of a piece of state that
	// will be consumed if this transaction is accepted.
	//
	// In the context of a UTXO-based payments system, for example, this would
	// be the IDs of the UTXOs consumed by this transaction.
	InputIDs() []ids.ID
}
