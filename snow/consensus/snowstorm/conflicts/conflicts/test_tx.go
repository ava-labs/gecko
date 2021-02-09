// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package conflicts

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
)

var (
	_ Tx = &TestTx{}
)

// TestTx is a useful test tx
type TestTx struct {
	choices.TestDecidable

	TransitionIDV ids.ID
	EpochV        uint32
	RestrictionsV []ids.ID
	DependenciesV []ids.ID
	InputIDsV     []ids.ID
}

// TransitionID implements the Tx interface
func (t *TestTx) TransitionID() ids.ID { return t.TransitionIDV }

// Epoch implements the Tx interface
func (t *TestTx) Epoch() uint32 { return t.EpochV }

// Restrictions implements the Tx interface
func (t *TestTx) Restrictions() []ids.ID { return t.RestrictionsV }

// Dependencies implements the Tx interface
func (t *TestTx) Dependencies() []ids.ID { return t.DependenciesV }

// InputIDs implements the Tx interface
func (t *TestTx) InputIDs() []ids.ID { return t.InputIDsV }
