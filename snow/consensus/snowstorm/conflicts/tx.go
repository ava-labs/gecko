// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package conflicts

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
)

// Tx manages state transitions in epochs, and adds transition restrictions.
type Tx interface {
	choices.Decidable

	// Transition is the description for the transition this transaction will
	// perform if accepted.
	Transition() Transition

	// Epoch this transaction was issued in.
	Epoch() uint32

	// Restrictions returns the IDs of transitions that cannot be performed
	// in an epoch earlier than this transaction's epoch if this transaction
	// is accepted. If any of these transitions are performed in an epoch
	// earlier than this transaction's epoch, this tx can't be accepted.
	Restrictions() []ids.ID

	// Verify this transaction is currently valid to be performed.
	Verify() error

	// Bytes that fully describe this transaction.
	Bytes() []byte
}
