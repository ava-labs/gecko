// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
)

var (
	_ conflicts.Transition = &emptyTransition{}
)

type emptyTransition struct {
	id ids.ID
}

func (t emptyTransition) ID() ids.ID { return t.id }

func (emptyTransition) Accept(uint32) error { return nil }

func (emptyTransition) Reject(uint32) error { return nil }

func (emptyTransition) Verify(uint32) error { return nil }

func (emptyTransition) Status() choices.Status { return choices.Processing }

func (emptyTransition) Epoch() uint32 { return 0 }

func (emptyTransition) Dependencies() []ids.ID { return nil }

func (emptyTransition) InputIDs() []ids.ID { return nil }

func (emptyTransition) Bytes() []byte { return nil }
