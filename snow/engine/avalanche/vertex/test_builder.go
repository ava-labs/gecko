// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm"
)

var (
	errBuild = errors.New("unexpectedly called Build")

	_ Builder = &TestBuilder{}
)

type TestBuilder struct {
	T            *testing.T
	CantBuildVtx bool
	BuildVtxF    func(
		epoch uint32,
		parentIDs []ids.ID,
		txs []snowstorm.Tx,
		restrictions []ids.ID,
	) (avalanche.Vertex, error)
}

func (b *TestBuilder) Default(cant bool) { b.CantBuildVtx = cant }

func (b *TestBuilder) BuildVtx(
	epoch uint32,
	parentIDs []ids.ID,
	txs []snowstorm.Tx,
	restrictions []ids.ID,
) (avalanche.Vertex, error) {
	if b.BuildVtxF != nil {
		return b.BuildVtxF(epoch, parentIDs, txs, restrictions)
	}
	if b.CantBuildVtx && b.T != nil {
		b.T.Fatal(errBuild)
	}
	return nil, errBuild
}
