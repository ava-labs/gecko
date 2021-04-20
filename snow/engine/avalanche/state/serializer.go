// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package state manages the meta-data required by consensus for an avalanche
// dag.
package state

import (
	"errors"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/vertex"
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

var _ vertex.Manager = &Serializer{}

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

// Parse implements the avalanche.State interface
func (s *Serializer) ParseVtx(b []byte) (avalanche.Vertex, error) {
	return newUniqueVertex(s, b)
}

// Build implements the avalanche.State interface
func (s *Serializer) BuildVtx(
	epoch uint32,
	parentIDs []ids.ID,
	txs []snowstorm.Tx,
	restrictions []ids.ID,
) (avalanche.Vertex, error) {
	height := uint64(0)
	for _, parentID := range parentIDs {
		parent, err := s.getVertex(parentID)
		if err != nil {
			return nil, err
		}
		height = math.Max64(height, parent.v.vtx.Height())
	}

	txBytes := make([][]byte, len(txs))
	for i, tx := range txs {
		txBytes[i] = tx.Bytes()
	}

	vtx, err := vertex.Build(
		s.ctx.ChainID,
		height,
		epoch,
		parentIDs,
		txBytes,
		restrictions,
	)
	if err != nil {
		return nil, err
	}

	uVtx := &uniqueVertex{
		serializer: s,
		vtxID:      vtx.ID(),
	}
	// setVertex handles the case where this vertex already exists even
	// though we just made it
	return uVtx, uVtx.setVertex(vtx)
}

// Get implements the avalanche.State interface
func (s *Serializer) GetVtx(vtxID ids.ID) (avalanche.Vertex, error) { return s.getVertex(vtxID) }

// Edge implements the avalanche.State interface
func (s *Serializer) Edge() []ids.ID { return s.edge.List() }

func (s *Serializer) parseVertex(b []byte) (vertex.StatelessVertex, error) {
	vtx, err := vertex.Parse(b)
	if err != nil {
		return nil, err
	}
	if vtx.ChainID() != s.ctx.ChainID {
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
