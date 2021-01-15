// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package conflicts

import (
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
)

var (
	_ Transition = &TestTransition{}
)

// TestTransition is a useful test tx
type TestTransition struct {
	IDV           ids.ID
	StatusV       choices.Status
	EpochV        uint32
	DependenciesV []Transition
	InputIDsV     []ids.ID
	BytesV        []byte
	VerifyV       map[uint32]error
	AcceptV       map[uint32]error
	RejectV       map[uint32]error
}

// ID implements the Transition interface
func (t *TestTransition) ID() ids.ID { return t.IDV }

// Status implements the Transition interface
func (t *TestTransition) Status() choices.Status { return t.StatusV }

// Epoch implements the Transition interface
func (t *TestTransition) Epoch() uint32 { return t.EpochV }

// Dependencies implements the Transition interface
func (t *TestTransition) Dependencies() []ids.ID {
	dependencies := make([]ids.ID, 0, len(t.DependenciesV))
	for _, dep := range t.DependenciesV {
		if dep.Status() != choices.Accepted {
			dependencies = append(dependencies, dep.ID())
		}
	}
	return dependencies
}

// InputIDs implements the Transition interface
func (t *TestTransition) InputIDs() []ids.ID { return t.InputIDsV }

// Bytes implements the Transition interface
func (t *TestTransition) Bytes() []byte { return t.BytesV }

// Verify implements the Transition interface
func (t *TestTransition) Verify(epoch uint32) error {
	switch t.StatusV {
	case choices.Processing:
		return t.VerifyV[epoch]
	default:
		return fmt.Errorf("verify unexpectedly called with status %s",
			t.StatusV)
	}
}

// Accept implements the Transition interface
func (t *TestTransition) Accept(epoch uint32) error {
	switch t.StatusV {
	case choices.Processing:
		t.EpochV = epoch
		t.StatusV = choices.Accepted
		return t.AcceptV[epoch]
	default:
		return fmt.Errorf("invalid state transition from %s to %s",
			t.StatusV, choices.Accepted)
	}
}

// Reject implements the Transition interface
func (t *TestTransition) Reject(epoch uint32) error {
	switch t.StatusV {
	case choices.Processing:
		return t.RejectV[epoch]
	case choices.Accepted:
		if t.EpochV == epoch {
			return fmt.Errorf("invalid state transition from Accepted to Rejected in epoch %d", epoch)
		}
		return t.RejectV[epoch]
	default:
		return fmt.Errorf("invalid state transition from %s to %s",
			t.StatusV, choices.Rejected)
	}
}
