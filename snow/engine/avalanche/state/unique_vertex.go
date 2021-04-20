// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"fmt"
	"strings"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/vertex"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/hashing"
)

// uniqueVertex acts as a cache for vertices in the database.
//
// If a vertex is loaded, it will have one canonical uniqueVertex. The vertex
// will eventually be evicted from memory, when the uniqueVertex is evicted from
// the cache. If the uniqueVertex has a function called again afther this
// eviction, the vertex will be re-loaded from the database.
type uniqueVertex struct {
	serializer *Serializer

	vtxID ids.ID
	v     *vertexState
}

// newUniqueVertex returns a uniqueVertex instance from [b] by checking the cache
// and then parsing the vertex bytes on a cache miss.
func newUniqueVertex(s *Serializer, b []byte) (*uniqueVertex, error) {
	vtx := &uniqueVertex{
		vtxID:      hashing.ComputeHash256Array(b),
		serializer: s,
	}
	vtx.shallowRefresh()

	// If the vtx exists, then the vertex is already known
	if vtx.v.vtx != nil {
		return vtx, nil
	}

	// If it wasn't in the cache parse the vertex and set it
	innerVertex, err := s.parseVertex(b)
	if err != nil {
		return nil, err
	}
	if err := innerVertex.Verify(); err != nil {
		return nil, err
	}
	vtx.v.vtx = innerVertex

	// If the vertex has already been fetched,
	// skip persisting the vertex.
	if vtx.v.status.Fetched() {
		return vtx, nil
	}

	// The vertex is newly parsed, so set the status
	// and persist it.
	vtx.v.status = choices.Processing
	return vtx, vtx.persist()
}

func (vtx *uniqueVertex) refresh() {
	vtx.shallowRefresh()

	if vtx.v.vtx == nil && vtx.v.status.Fetched() {
		vtx.v.vtx = vtx.serializer.state.Vertex(vtx.ID())
	}
}

// shallowRefresh checks the cache for the uniqueVertex and gets the
// most up to date status for [vtx]
// ensures that the status is up to date for this vertex
// inner vertex may be nil after calling shallowRefresh
func (vtx *uniqueVertex) shallowRefresh() {
	if vtx.v == nil {
		vtx.v = &vertexState{}
	}
	if vtx.v.unique {
		return
	}

	unique := vtx.serializer.state.UniqueVertex(vtx)
	prevVtx := vtx.v.vtx
	if unique == vtx {
		vtx.v.status = vtx.serializer.state.Status(vtx.ID())
		vtx.v.unique = true
	} else {
		// If someone is in the cache, they must be up to date
		*vtx = *unique
	}

	if vtx.v.vtx == nil {
		vtx.v.vtx = prevVtx
	}
}

func (vtx *uniqueVertex) Evict() {
	if vtx.v != nil {
		vtx.v.unique = false
		// make sure the parents can be garbage collected
		vtx.v.parents = nil
	}
}

func (vtx *uniqueVertex) setVertex(innerVtx vertex.StatelessVertex) error {
	vtx.shallowRefresh()
	vtx.v.vtx = innerVtx

	if vtx.v.status.Fetched() {
		return nil
	}
	vtx.v.status = choices.Processing
	return vtx.persist()
}

func (vtx *uniqueVertex) persist() error {
	if err := vtx.serializer.state.SetVertex(vtx.v.vtx); err != nil {
		return err
	}
	if err := vtx.serializer.state.SetStatus(vtx.ID(), vtx.v.status); err != nil {
		return err
	}
	return vtx.serializer.db.Commit()
}

func (vtx *uniqueVertex) setStatus(status choices.Status) error {
	vtx.shallowRefresh()
	if vtx.v.status == status {
		return nil
	}
	vtx.v.status = status
	return vtx.serializer.state.SetStatus(vtx.ID(), status)
}

func (vtx *uniqueVertex) ID() ids.ID       { return vtx.vtxID }
func (vtx *uniqueVertex) Key() interface{} { return vtx.vtxID }

func (vtx *uniqueVertex) Accept() error {
	if err := vtx.setStatus(choices.Accepted); err != nil {
		return err
	}

	vtx.serializer.edge.Add(vtx.vtxID)
	parents, err := vtx.Parents()
	if err != nil {
		return err
	}

	for _, parent := range parents {
		vtx.serializer.edge.Remove(parent.ID())
	}

	if err := vtx.serializer.state.SetEdge(vtx.serializer.edge.List()); err != nil {
		return fmt.Errorf("failed to set edge while accepting vertex %s due to %w", vtx.vtxID, err)
	}

	// Should never traverse into parents of a decided vertex. Allows for the
	// parents to be garbage collected
	vtx.v.parents = nil

	return vtx.serializer.db.Commit()
}

func (vtx *uniqueVertex) Reject() error {
	if err := vtx.setStatus(choices.Rejected); err != nil {
		return err
	}

	// Should never traverse into parents of a decided vertex. Allows for the
	// parents to be garbage collected
	vtx.v.parents = nil

	return vtx.serializer.db.Commit()
}

// TODO: run performance test to see if shallow refreshing
// (which will mean that refresh must be called in Bytes and Verify)
// improves performance
func (vtx *uniqueVertex) Status() choices.Status { vtx.refresh(); return vtx.v.status }

func (vtx *uniqueVertex) Parents() ([]avalanche.Vertex, error) {
	vtx.refresh()

	if vtx.v.vtx == nil {
		return nil, fmt.Errorf("failed to get parents for vertex with status: %s", vtx.v.status)
	}

	parentIDs := vtx.v.vtx.ParentIDs()
	if len(vtx.v.parents) != len(parentIDs) {
		vtx.v.parents = make([]avalanche.Vertex, len(parentIDs))
		for i, parentID := range parentIDs {
			vtx.v.parents[i] = &uniqueVertex{
				serializer: vtx.serializer,
				vtxID:      parentID,
			}
		}
	}

	return vtx.v.parents, nil
}

func (vtx *uniqueVertex) Height() (uint64, error) {
	vtx.refresh()

	if vtx.v.vtx == nil {
		return 0, fmt.Errorf("failed to get height for vertex with status: %s", vtx.v.status)
	}

	return vtx.v.vtx.Height(), nil
}

func (vtx *uniqueVertex) Epoch() (uint32, error) {
	vtx.refresh()

	if vtx.v.vtx == nil {
		return 0, fmt.Errorf("failed to get epoch for vertex with status: %s", vtx.v.status)
	}

	return vtx.v.vtx.Epoch(), nil
}

func (vtx *uniqueVertex) Txs() ([]snowstorm.Tx, error) {
	vtx.refresh()

	if vtx.v.vtx == nil {
		return nil, fmt.Errorf("failed to get txs for vertex with status: %s", vtx.v.status)
	}

	txs := vtx.v.vtx.Txs()
	if len(txs) != len(vtx.v.txs) {
		vtx.v.txs = make([]snowstorm.Tx, len(txs))
		for i, txBytes := range txs {
			tx, err := vtx.serializer.vm.ParseTx(txBytes)
			if err != nil {
				return nil, err
			}
			vtx.v.txs[i] = tx
		}
	}

	return vtx.v.txs, nil
}

func (vtx *uniqueVertex) Bytes() []byte { return vtx.v.vtx.Bytes() }

func (vtx *uniqueVertex) Verify() error { return vtx.v.vtx.Verify() }

func (vtx *uniqueVertex) String() string {
	sb := strings.Builder{}

	parents, err := vtx.Parents()
	if err != nil {
		sb.WriteString(fmt.Sprintf("Vertex(ID = %s, Error=error while retrieving vertex parents: %s)", vtx.ID(), err))
		return sb.String()
	}
	txs, err := vtx.Txs()
	if err != nil {
		sb.WriteString(fmt.Sprintf("Vertex(ID = %s, Error=error while retrieving vertex txs: %s)", vtx.ID(), err))
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf(
		"Vertex(ID = %s, Status = %s, Number of Dependencies = %d, Number of Transactions = %d)",
		vtx.ID(),
		vtx.Status(),
		len(parents),
		len(txs),
	))

	parentFormat := fmt.Sprintf("\n    Parent[%s]: ID = %%s, Status = %%s",
		formatting.IntFormat(len(parents)-1))
	for i, parent := range parents {
		sb.WriteString(fmt.Sprintf(parentFormat, i, parent.ID(), parent.Status()))
	}

	txFormat := fmt.Sprintf("\n    Transaction[%s]: ID = %%s, Status = %%s",
		formatting.IntFormat(len(txs)-1))
	for i, tx := range txs {
		sb.WriteString(fmt.Sprintf(txFormat, i, tx.ID(), tx.Status()))
	}

	return sb.String()
}

type vertexState struct {
	unique bool

	vtx    vertex.StatelessVertex
	status choices.Status

	parents []avalanche.Vertex
	txs     []snowstorm.Tx
}
