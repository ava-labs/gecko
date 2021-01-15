// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/utils/hashing"
)

// Builder builds a vertex given a set of parentIDs and transactions.
type Builder interface {
	// Build a new vertex from the contents of a vertex
	Build(
		epoch uint32,
		parentIDs []ids.ID,
		trs []conflicts.Transition,
		restrictions []ids.ID,
	) (avalanche.Vertex, error)
}

// Build a new stateless vertex from the contents of a vertex
func Build(
	chainID ids.ID,
	height uint64,
	epoch uint32,
	parentIDs []ids.ID,
	trs [][]byte,
	restrictions []ids.ID,
) (StatelessVertex, error) {
	ids.SortIDs(parentIDs)
	SortHashOf(trs)
	ids.SortIDs(restrictions)

	var version uint16
	switch epoch {
	case 0:
		version = noEpochTransitionsCodecVersion
	default:
		version = apricotCodecVersion
	}
	innerVtx := innerStatelessVertex{
		Version:      version,
		ChainID:      chainID,
		Height:       height,
		Epoch:        epoch,
		ParentIDs:    parentIDs,
		Transitions:  trs,
		Restrictions: restrictions,
	}
	if err := innerVtx.Verify(); err != nil {
		return nil, err
	}

	vtxBytes, err := Codec.Marshal(innerVtx.Version, innerVtx)
	vtx := statelessVertex{
		innerStatelessVertex: innerVtx,
		id:                   hashing.ComputeHash256Array(vtxBytes),
		bytes:                vtxBytes,
	}
	return vtx, err
}
