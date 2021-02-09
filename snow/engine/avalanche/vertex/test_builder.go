// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
)

var (
	errBuild = errors.New("unexpectedly called Build")

	_ Builder = &TestBuilder{}
)

type TestBuilder struct {
	T         *testing.T
	CantBuild bool
	BuildF    func(
		epoch uint32,
		parentIDs []ids.ID,
		txs []conflicts.Tx,
		restrictions []ids.ID,
	) (avalanche.Vertex, error)
}

func (b *TestBuilder) Default(cant bool) { b.CantBuild = cant }

func (b *TestBuilder) Build(
	epoch uint32,
	parentIDs []ids.ID,
	txs []conflicts.Tx,
	restrictions []ids.ID,
) (avalanche.Vertex, error) {
	if b.BuildF != nil {
		return b.BuildF(epoch, parentIDs, txs, restrictions)
	}
	if b.CantBuild && b.T != nil {
		b.T.Fatal(errBuild)
	}
	return nil, errBuild
}
