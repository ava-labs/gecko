// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package conflicts

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
)

// Transition consumes and potentially produces state.
type Transition interface {
	// ID returns a unique ID for this transition.
	//
	// Typically, this is implemented by using a cryptographic hash of a binary
	// representation of this transition. A transition should return the same ID
	// upon repeated calls.
	ID() ids.ID

	// Status returns this transition's current status.
	//
	// If Accept has been called on a transition with this ID, Accepted should
	// be returned. Similarly, if Reject has been called on a transition with
	// this ID and this transition can never be made by another transaction,
	// Rejected should be returned. If the body of this transition is unknown,
	// then Unknown should be returned. Otherwise, Processing should be returned.
	Status() choices.Status

	// Epoch this transition was accepted in.
	//
	// If the status of this transition isn't Accepted, the behavior of this
	// call is undefined.
	Epoch() uint32

	// Dependencies is a list of unfulfilled transitions upon which this transition depends.
	//
	// Each Dependency will be verified before Verify is called on this
	// transition. Similarly, each element of Dependencies must be accepted
	// before this transition is accepted. If a dependency has already been accepted
	// it should not be returned.
	Dependencies() []ids.ID

	// InputIDs is a set where each element is the ID of a piece of state that
	// will be consumed if this transition is accepted.
	//
	// In the context of a UTXO-based payments system, for example, this would
	// be the IDs of the UTXOs consumed by this transition
	InputIDs() []ids.ID

	// Bytes returns a binary representation of this transition.
	Bytes() []byte

	// Verify this transition as being valid.
	//
	// Verify should return nil if acceptance is allowed assuming:
	// - all dependencies are accepted
	// - no transitions with overlapping inputIDs are accepted
	Verify(epoch uint32) error

	// Accept this transition in the provided epoch.
	//
	// This transition will be accepted by every correct node in the network in
	// the provided epoch.
	Accept(epoch uint32) error

	// Reject this transition from the provided epoch.
	//
	// This transition will not be accepted by any correct node in the network
	// in the provided epoch.
	Reject(epoch uint32) error
}
