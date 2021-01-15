// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/components/verify"
)

type StatelessTx interface {
	verify.Verifiable
	ID() ids.ID
	Bytes() []byte

	Epoch() uint32
	Transition() []byte
	Restrictions() []ids.ID
}

type statelessTx struct {
	// This wrapper exists so that the function calls aren't ambiguous
	innerStatelessTx

	// cache the ID of this transaction
	id ids.ID

	// cache the binary format of this transaction
	bytes []byte
}

func (t statelessTx) ID() ids.ID             { return t.id }
func (t statelessTx) Bytes() []byte          { return t.bytes }
func (t statelessTx) Epoch() uint32          { return t.innerStatelessTx.Epoch }
func (t statelessTx) Transition() []byte     { return t.innerStatelessTx.Transition }
func (t statelessTx) Restrictions() []ids.ID { return t.innerStatelessTx.Restrictions }

type innerStatelessTx struct {
	Version      uint16   `json:"version"`
	Epoch        uint32   `serializeV0:"true" serializeV1:"true" json:"epoch"`
	Transition   []byte   `serializeV0:"true" serializeV1:"true" json:"transition"`
	Restrictions []ids.ID `serializeV0:"true" serializeV1:"true" len:"128" json:"restrictions"`
}

func (t innerStatelessTx) Verify() error {
	switch {
	case !ids.IsSortedAndUniqueIDs(t.Restrictions):
		return errInvalidRestrictions
	default:
		return nil
	}
}
