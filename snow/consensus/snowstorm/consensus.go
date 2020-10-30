// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowstorm

import (
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"

	sbcon "github.com/ava-labs/avalanchego/snow/consensus/snowball"
)

// Consensus is a snowball instance deciding between an unbounded number of
// non-transitive conflicts. After performing a network sample of k nodes, you
// should call collect with the responses.
type Consensus interface {
	fmt.Stringer

	// Takes in the context, alpha, betaVirtuous, and betaRogue
	Initialize(*snow.Context, Conflicts, sbcon.Parameters) error

	// Returns the parameters that describe this snowstorm instance
	Parameters() sbcon.Parameters

	// Returns the set of virtuous transactions
	// that have not yet been accepted or rejected
	Virtuous() ids.Set

	// Returns the currently preferred transactions to be finalized
	Preferences() ids.Set

	// Returns true iff all remaining transactions are rogue. Note, it is
	// possible that after returning quiesce, a new decision may be added such
	// that this instance should no longer quiesce.
	Quiesce() bool

	// Returns true iff all added transactions have been finalized. Note, it is
	// possible that after returning finalized, a new decision may be added such
	// that this instance is no longer finalized.
	Finalized() bool

	// Returns true if transaction <Tx> is virtuous.
	// That is, no transaction has been added that conflicts with <Tx>
	IsVirtuous(choices.Decidable) (bool, error)

	// Returns the set of transactions conflicting with <Tx>
	Conflicts(choices.Decidable) (ids.Set, error)

	// Returns true iff transaction <Tx> has been added
	Issued(choices.Decidable) bool

	// Adds a new transaction to vote on. Returns if a critical error has
	// occurred.
	Add(choices.Decidable) error

	// Collects the results of a network poll. Assumes all transactions
	// have been previously added. Returns true is any statuses or preferences
	// changed. Returns if a critical error has occurred.
	RecordPoll(ids.Bag) (bool, error)
}
