// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
)

func TestVertexVerify(t *testing.T) {
	tooManyParents := make([]ids.ID, maxNumParents+1)
	for i := range tooManyParents {
		tooManyParents[i][0] = byte(i)
	}
	tooManyTxs := make([][]byte, maxTransitionsPerVtx+1)
	for i := range tooManyTxs {
		tooManyTxs[i] = []byte{byte(i)}
	}
	tooManyRestrictions := make([]ids.ID, maxTransitionsPerVtx+1)
	for i := range tooManyRestrictions {
		tooManyRestrictions[i][0] = byte(i)
	}

	tests := []struct {
		name      string
		vertex    StatelessVertex
		shouldErr bool
	}{
		{
			name:      "zero vertex",
			vertex:    statelessVertex{innerStatelessVertex: innerStatelessVertex{}},
			shouldErr: true,
		},
		{
			name: "valid vertex",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        0,
				ParentIDs:    []ids.ID{},
				Transitions:  [][]byte{{}},
				Restrictions: []ids.ID{},
			}},
			shouldErr: false,
		},
		{
			name: "invalid vertex epoch",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        1,
				ParentIDs:    []ids.ID{},
				Transitions:  [][]byte{{}},
				Restrictions: []ids.ID{},
			}},
			shouldErr: true,
		},
		{
			name: "too many vertex parents",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        0,
				ParentIDs:    tooManyParents,
				Transitions:  [][]byte{{}},
				Restrictions: []ids.ID{},
			}},
			shouldErr: true,
		},
		{
			name: "no vertex transitions",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        0,
				ParentIDs:    []ids.ID{},
				Transitions:  [][]byte{},
				Restrictions: []ids.ID{},
			}},
			shouldErr: true,
		},
		{
			name: "too many vertex transitions",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        0,
				ParentIDs:    []ids.ID{},
				Transitions:  tooManyTxs,
				Restrictions: []ids.ID{},
			}},
			shouldErr: true,
		},
		{
			name: "too many vertex restrictions",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        0,
				ParentIDs:    []ids.ID{},
				Transitions:  [][]byte{{}},
				Restrictions: tooManyRestrictions,
			}},
			shouldErr: true,
		},
		{
			name: "unsorted vertex parents",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        0,
				ParentIDs:    []ids.ID{{1}, {0}},
				Transitions:  [][]byte{{}},
				Restrictions: []ids.ID{},
			}},
			shouldErr: true,
		},
		{
			name: "unsorted vertex restrictions",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        0,
				ParentIDs:    []ids.ID{},
				Transitions:  [][]byte{{}},
				Restrictions: []ids.ID{{1}, {0}},
			}},
			shouldErr: true,
		},
		{
			name: "unsorted vertex transitions",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        0,
				ParentIDs:    []ids.ID{},
				Transitions:  [][]byte{{0}, {1}}, // note that transitions are sorted by their hashes
				Restrictions: []ids.ID{},
			}},
			shouldErr: true,
		},
		{
			name: "duplicate vertex parents",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        0,
				ParentIDs:    []ids.ID{{0}, {0}},
				Transitions:  [][]byte{{}},
				Restrictions: []ids.ID{},
			}},
			shouldErr: true,
		},
		{
			name: "duplicate vertex restrictions",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        0,
				ParentIDs:    []ids.ID{},
				Transitions:  [][]byte{{}},
				Restrictions: []ids.ID{{0}, {0}},
			}},
			shouldErr: true,
		},
		{
			name: "duplicate vertex transitions",
			vertex: statelessVertex{innerStatelessVertex: innerStatelessVertex{
				Version:      0,
				ChainID:      ids.ID{},
				Height:       0,
				Epoch:        0,
				ParentIDs:    []ids.ID{},
				Transitions:  [][]byte{{0}, {0}},
				Restrictions: []ids.ID{},
			}},
			shouldErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.vertex.Verify()
			if test.shouldErr && err == nil {
				t.Fatal("expected verify to return an error but it didn't")
			} else if !test.shouldErr && err != nil {
				t.Fatalf("expected verify to pass but it returned: %s", err)
			}
		})
	}
}
