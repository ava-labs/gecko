// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avalanche

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm"
)

// TODO: Implement pruning of accepted decisions.
// To perfectly preserve the protocol, this implementation will need to store
// the hashes of all accepted decisions. It is possible to add a heuristic that
// removes sufficiently old decisions. However, that will need to be analyzed to
// ensure safety. It is doable with a weak syncrony assumption.

// Consensus represents a general avalanche instance that can be used directly
// to process a series of partially ordered elements.
type Consensus interface {
	// Takes in alpha, beta1, beta2, the accepted frontier, the join statuses,
	// the mutation statuses, and the consumer statuses. If accept or reject is
	// called, the status maps should be immediately updated accordingly.
	// Assumes each element in the accepted frontier will return accepted from
	// the join status map.
	Initialize(*snow.Context, Parameters, []Vertex) error

	// Returns the parameters that describe this avalanche instance
	Parameters() Parameters

	// Returns the number of vertices processing
	NumProcessing() int

	// Returns true if the transaction is virtuous.
	// That is, no transaction has been added that conflicts with it
	IsVirtuous(snowstorm.Tx) bool

	// Adds a new decision. Assumes the dependencies have already been added.
	// Assumes that mutations don't conflict with themselves. Returns if a
	// critical error has occurred.
	Add(Vertex) error

	// VertexIssued returns true iff Vertex has been added
	VertexIssued(Vertex) bool

	// TxIssued returns true if a vertex containing this transanction has been added
	TxIssued(snowstorm.Tx) bool

	// Returns the set of transaction IDs that are virtuous but not contained in
	// any preferred vertices.
	Orphans() ids.Set

	// Returns a set of vertex IDs that were virtuous at the last update.
	Virtuous() ids.Set

	// Returns a set of vertex IDs that are preferred
	Preferences() ids.Set

	// RecordPoll collects the results of a network poll. If a result has not
	// been added, the result is dropped. Returns if a critical error has
	// occurred.
	RecordPoll(ids.UniqueBag) error

	// Quiesce returns true iff all vertices that have been added but not been accepted or rejected are rogue.
	// Note, it is possible that after returning quiesce, a new decision may be added such
	// that this instance should no longer quiesce.
	Quiesce() bool

	// Finalized returns true if all transactions that have been added have been
	// finalized. Note, it is possible that after returning finalized, a new
	// decision may be added such that this instance is no longer finalized.
	Finalized() bool

	// HealthCheck returns information about the consensus health.
	HealthCheck() (interface{}, error)
}
