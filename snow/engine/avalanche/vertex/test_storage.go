// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
)

var (
	errGet  = errors.New("unexpectedly called Get")
	errEdge = errors.New("unexpectedly called Edge")

	_ Storage = &TestStorage{}
)

type TestStorage struct {
	T                    *testing.T
	CantGetVtx, CantEdge bool
	GetVtxF              func(ids.ID) (avalanche.Vertex, error)
	EdgeF                func() []ids.ID
}

func (s *TestStorage) Default(cant bool) {
	s.CantGetVtx = cant
	s.CantEdge = cant
}

func (s *TestStorage) GetVtx(id ids.ID) (avalanche.Vertex, error) {
	if s.GetVtxF != nil {
		return s.GetVtxF(id)
	}
	if s.CantGetVtx && s.T != nil {
		s.T.Fatal(errGet)
	}
	return nil, errGet
}

func (s *TestStorage) Edge() []ids.ID {
	if s.EdgeF != nil {
		return s.EdgeF()
	}
	if s.CantEdge && s.T != nil {
		s.T.Fatal(errEdge)
	}
	return nil
}
