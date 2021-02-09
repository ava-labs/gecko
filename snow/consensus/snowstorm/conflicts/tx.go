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

	// Dependencies is a list of transactions upon which this transaction
	// depends. Each element of Dependencies must be verified before Verify is
	// called on this transaction.
	//
	// Similarly, each element of Dependencies must be accepted before this
	// transaction is accepted.
	Dependencies() []Tx

	// InputIDs is a set where each element is the ID of a piece of state that
	// will be consumed if this transaction is accepted.
	//
	// In the context of a UTXO-based payments system, for example, this would
	// be the IDs of the UTXOs consumed by this transaction
	InputIDs() []ids.ID

	Verify() error
	Bytes() []byte
}
