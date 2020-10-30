// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package state manages the meta-data required by consensus for an avalanche
// dag.
package state

import (
	"errors"
	"fmt"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/vertex"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/math"
)

const (
	dbCacheSize = 10000
	idCacheSize = 1000
)

var (
	errUnknownVertex = errors.New("unknown vertex")
	errWrongChainID  = errors.New("wrong ChainID in vertex")
)

// Serializer manages the state of multiple vertices
type Serializer struct {
	ctx   *snow.Context
	vm    vertex.DAGVM
	state *prefixedState
	db    *versiondb.Database
	edge  ids.Set
}

// Initialize implements the avalanche.State interface
func (s *Serializer) Initialize(ctx *snow.Context, vm vertex.DAGVM, db database.Database) {
	s.ctx = ctx
	s.vm = vm

	vdb := versiondb.New(db)
	dbCache := &cache.LRU{Size: dbCacheSize}
	rawState := &state{
		serializer: s,
		dbCache:    dbCache,
		db:         vdb,
	}
	s.state = newPrefixedState(rawState, idCacheSize)
	s.db = vdb

	s.edge.Add(s.state.Edge()...)
}

// ParseVertex implements the avalanche.State interface
func (s *Serializer) ParseVertex(b []byte) (avalanche.Vertex, error) {
	return newUniqueVertex(s, b)
}

// BuildVertex implements the avalanche.State interface
func (s *Serializer) BuildVertex(parentIDs []ids.ID, txs []conflicts.Tx) (avalanche.Vertex, error) {
	if len(txs) == 0 {
		return nil, errNoTxs
	} else if l := len(txs); l > maxTxsPerVtx {
		return nil, fmt.Errorf("number of txs (%d) exceeds max (%d)", l, maxTxsPerVtx)
	} else if l := len(parentIDs); l > maxNumParents {
		return nil, fmt.Errorf("number of parents (%d) exceeds max (%d)", l, maxNumParents)
	}

	ids.SortIDs(parentIDs)
	sortTxs(txs)

	height := uint64(0)
	for _, parentID := range parentIDs {
		parent, err := s.getVertex(parentID)
		if err != nil {
			return nil, err
		}
		height = math.Max64(height, parent.v.vtx.height)
	}

	vtx := &innerVertex{
		chainID:   s.ctx.ChainID,
		height:    height + 1,
		parentIDs: parentIDs,
		txs:       txs,
	}

	bytes, err := vtx.Marshal()
	if err != nil {
		return nil, err
	}
	vtx.bytes = bytes
	vtx.id = ids.NewID(hashing.ComputeHash256Array(vtx.bytes))

	uVtx := &uniqueVertex{
		serializer: s,
		vtxID:      vtx.ID(),
	}
	// setVertex handles the case where this vertex already exists even
	// though we just made it
	return uVtx, uVtx.setVertex(vtx)
}

// GetVertex implements the avalanche.State interface
func (s *Serializer) GetVertex(vtxID ids.ID) (avalanche.Vertex, error) { return s.getVertex(vtxID) }

// Edge implements the avalanche.State interface
func (s *Serializer) Edge() []ids.ID { return s.edge.List() }

func (s *Serializer) parseVertex(b []byte) (*innerVertex, error) {
	vtx := &innerVertex{}
	if err := vtx.Unmarshal(b, s.vm); err != nil {
		return nil, err
	} else if !vtx.chainID.Equals(s.ctx.ChainID) {
		return nil, errWrongChainID
	}
	return vtx, nil
}

func (s *Serializer) getVertex(vtxID ids.ID) (*uniqueVertex, error) {
	vtx := &uniqueVertex{
		serializer: s,
		vtxID:      vtxID,
	}
	if vtx.Status() == choices.Unknown {
		return nil, errUnknownVertex
	}
	return vtx, nil
}
