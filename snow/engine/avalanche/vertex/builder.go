// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm"
	"github.com/ava-labs/avalanchego/utils/hashing"
)

// Builder builds a vertex given a set of parentIDs and transactions.
type Builder interface {
	// Build a new vertex from the contents of a vertex
	BuildVtx(
		epoch uint32,
		parentIDs []ids.ID,
		txs []snowstorm.Tx,
		restrictions []ids.ID,
	) (avalanche.Vertex, error)
}

// Build a new stateless vertex from the contents of a vertex
func Build(
	chainID ids.ID,
	height uint64,
	epoch uint32,
	parentIDs []ids.ID,
	txs [][]byte,
	restrictions []ids.ID,
) (StatelessVertex, error) {
	ids.SortIDs(parentIDs)
	SortHashOf(txs)
	ids.SortIDs(restrictions)

	innerVtx := innerStatelessVertex{
		Version:      noEpochTransitionsCodecVersion,
		ChainID:      chainID,
		Height:       height,
		Epoch:        epoch,
		ParentIDs:    parentIDs,
		Txs:          txs,
		Restrictions: restrictions,
	}
	if err := innerVtx.Verify(); err != nil {
		return nil, err
	}

	vtxBytes, err := c.Marshal(innerVtx.Version, innerVtx)
	vtx := statelessVertex{
		innerStatelessVertex: innerVtx,
		id:                   hashing.ComputeHash256Array(vtxBytes),
		bytes:                vtxBytes,
	}
	return vtx, err
}
