// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package common

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
)

// SubnetTest is a test subnet
type SubnetTest struct {
	T *testing.T

	CantIsBootstrapped, CantBootstrapped bool

	IsBootstrappedF func() bool
	BootstrappedF   func(ids.ID)
}

// Default set the default callable value to [cant]
func (s *SubnetTest) Default(cant bool) {
	s.CantIsBootstrapped = cant
	s.CantBootstrapped = cant
}

// IsBootstrapped calls IsBootstrappedF if it was initialized. If it wasn't
// initialized and this function shouldn't be called and testing was
// initialized, then testing will fail. Defaults to returning false.
func (s *SubnetTest) IsBootstrapped() bool {
	if s.IsBootstrappedF != nil {
		return s.IsBootstrappedF()
	}
	if s.CantIsBootstrapped && s.T != nil {
		s.T.Fatalf("Unexpectedly called IsBootstrapped")
	}
	return false
}

// Bootstrapped calls BootstrappedF if it was initialized. If it wasn't
// initialized and this function shouldn't be called and testing was
// initialized, then testing will fail.
func (s *SubnetTest) Bootstrapped(chainID ids.ID) {
	if s.BootstrappedF != nil {
		s.BootstrappedF(chainID)
	} else if s.CantBootstrapped && s.T != nil {
		s.T.Fatalf("Unexpectedly called Bootstrapped")
	}
}
