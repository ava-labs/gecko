// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"errors"
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/components/verify"
)

const (
	// maxNumParents is the max number of parents a vertex may have
	maxNumParents = 128

	// maxTransitionsPerVtx is the max number of transitions a vertex may have
	maxTransitionsPerVtx = 128

	// maxRestrictionsPerVtx is the max number of restrictions a vertex may have
	maxRestrictionsPerVtx = 128
)

var (
	errFutureField         = errors.New("field specified in a previous version")
	errTooManyparentIDs    = fmt.Errorf("vertex contains more than %d parentIDs", maxNumParents)
	errNoOperations        = errors.New("vertex contains no operations")
	errTooManyTransitions  = fmt.Errorf("vertex contains more than %d transitions", maxTransitionsPerVtx)
	errTooManyRestrictions = fmt.Errorf("vertex contains more than %d restrictions", maxRestrictionsPerVtx)
	errInvalidParents      = errors.New("vertex contains non-sorted or duplicated parentIDs")
	errInvalidRestrictions = errors.New("vertex contains non-sorted or duplicated restrictions")
	errInvalidTransitions  = errors.New("vertex contains non-sorted or duplicated transitions")

	_ StatelessVertex = statelessVertex{}
)

type StatelessVertex interface {
	verify.Verifiable
	ID() ids.ID
	Bytes() []byte

	Version() uint16
	ChainID() ids.ID
	Height() uint64
	Epoch() uint32
	ParentIDs() []ids.ID
	Transitions() [][]byte
	Restrictions() []ids.ID
}

type statelessVertex struct {
	// This wrapper exists so that the function calls aren't ambiguous
	innerStatelessVertex

	// cache the ID of this vertex
	id ids.ID

	// cache the binary format of this vertex
	bytes []byte
}

func (v statelessVertex) ID() ids.ID             { return v.id }
func (v statelessVertex) Bytes() []byte          { return v.bytes }
func (v statelessVertex) Version() uint16        { return v.innerStatelessVertex.Version }
func (v statelessVertex) ChainID() ids.ID        { return v.innerStatelessVertex.ChainID }
func (v statelessVertex) Height() uint64         { return v.innerStatelessVertex.Height }
func (v statelessVertex) Epoch() uint32          { return v.innerStatelessVertex.Epoch }
func (v statelessVertex) ParentIDs() []ids.ID    { return v.innerStatelessVertex.ParentIDs }
func (v statelessVertex) Transitions() [][]byte  { return v.innerStatelessVertex.Transitions }
func (v statelessVertex) Restrictions() []ids.ID { return v.innerStatelessVertex.Restrictions }

type innerStatelessVertex struct {
	Version      uint16   `json:"version"`
	ChainID      ids.ID   `serializeV0:"true" serializeV1:"true" json:"chainID"`
	Height       uint64   `serializeV0:"true" serializeV1:"true" json:"height"`
	Epoch        uint32   `serializeV0:"true" serializeV1:"true" json:"epoch"`
	ParentIDs    []ids.ID `serializeV0:"true" serializeV1:"true" len:"128" json:"parentIDs"`
	Transitions  [][]byte `serializeV0:"true" serializeV1:"true" len:"128" json:"transitions"`
	Restrictions []ids.ID `serializeV1:"true" len:"128" json:"restrictions"`
}

func (v innerStatelessVertex) Verify() error {
	// Version specific verification
	switch v.Version {
	case 0:
		switch {
		case v.Epoch != 0:
			return fmt.Errorf("invalid epoch %d for v0 codec", v.Epoch)
		case len(v.Restrictions) != 0:
			return errFutureField
		}
	case 1:
		// Cannot allow a vertex using v1 codec to be issued into 0th
		// epoch since nodes that have not updated will not be able to
		// parse the vertex.
		if v.Epoch == 0 {
			return fmt.Errorf("invalid epoch 0 for v1 codec")
		}
	default:
		return fmt.Errorf("invalid version %d", v.Version)
	}

	// General verification
	switch {
	case len(v.ParentIDs) > maxNumParents:
		return errTooManyparentIDs
	case len(v.Transitions)+len(v.Restrictions) == 0:
		return errNoOperations
	case len(v.Transitions) > maxTransitionsPerVtx:
		return errTooManyTransitions
	case len(v.Restrictions) > maxRestrictionsPerVtx:
		return errTooManyRestrictions
	case !ids.IsSortedAndUniqueIDs(v.ParentIDs):
		return errInvalidParents
	case !ids.IsSortedAndUniqueIDs(v.Restrictions):
		return errInvalidRestrictions
	case !IsSortedAndUniqueHashOf(v.Transitions):
		return errInvalidTransitions
	default:
		return nil
	}
}
