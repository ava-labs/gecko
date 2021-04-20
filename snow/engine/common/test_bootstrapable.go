// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package common

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
)

// BootstrapableTest is a test engine that supports bootstrapping
type BootstrapableTest struct {
	T *testing.T

	CantCurrentAcceptedFrontier,
	CantFilterAccepted,
	CantForceAccepted bool

	CurrentAcceptedFrontierF func() (acceptedContainerIDs []ids.ID, err error)
	FilterAcceptedF          func(containerIDs []ids.ID) (acceptedContainerIDs []ids.ID)
	ForceAcceptedF           func(acceptedContainerIDs []ids.ID) error
}

// Default sets the default on call handling
func (b *BootstrapableTest) Default(cant bool) {
	b.CantCurrentAcceptedFrontier = cant
	b.CantFilterAccepted = cant
	b.CantForceAccepted = cant
}

// CurrentAcceptedFrontier implements the Bootstrapable interface
func (b *BootstrapableTest) CurrentAcceptedFrontier() ([]ids.ID, error) {
	if b.CurrentAcceptedFrontierF != nil {
		return b.CurrentAcceptedFrontierF()
	}
	if b.CantCurrentAcceptedFrontier && b.T != nil {
		b.T.Fatalf("Unexpectedly called CurrentAcceptedFrontier")
	}
	return nil, nil
}

// FilterAccepted implements the Bootstrapable interface
func (b *BootstrapableTest) FilterAccepted(containerIDs []ids.ID) []ids.ID {
	if b.FilterAcceptedF != nil {
		return b.FilterAcceptedF(containerIDs)
	}
	if b.CantFilterAccepted && b.T != nil {
		b.T.Fatalf("Unexpectedly called FilterAccepted")
	}
	return nil
}

// ForceAccepted implements the Bootstrapable interface
func (b *BootstrapableTest) ForceAccepted(containerIDs []ids.ID) error {
	if b.ForceAcceptedF != nil {
		return b.ForceAcceptedF(containerIDs)
	} else if b.CantForceAccepted {
		if b.T != nil {
			b.T.Fatalf("Unexpectedly called ForceAccepted")
		}
		return errors.New("unexpectedly called ForceAccepted")
	}
	return nil
}
