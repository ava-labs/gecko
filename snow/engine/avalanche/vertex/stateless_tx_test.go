// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
)

func TestTxVerify(t *testing.T) {
	tooManyRestrictions := make([]ids.ID, maxTransitionsPerVtx+1)
	for i := range tooManyRestrictions {
		tooManyRestrictions[i][0] = byte(i)
	}

	tests := []struct {
		name      string
		tx        StatelessTx
		shouldErr bool
	}{
		{
			name:      "zero tx",
			tx:        statelessTx{innerStatelessTx: innerStatelessTx{}},
			shouldErr: false,
		},
		{
			name: "valid tx",
			tx: statelessTx{innerStatelessTx: innerStatelessTx{
				Version:      0,
				Epoch:        0,
				Transition:   []byte{},
				Restrictions: []ids.ID{},
			}},
			shouldErr: false,
		},
		{
			name: "unsorted tx restrictions",
			tx: statelessTx{innerStatelessTx: innerStatelessTx{
				Version:      0,
				Epoch:        0,
				Transition:   []byte{},
				Restrictions: []ids.ID{{1}, {0}},
			}},
			shouldErr: true,
		},
		{
			name: "duplicate tx restrictions",
			tx: statelessTx{innerStatelessTx: innerStatelessTx{
				Version:      0,
				Epoch:        0,
				Transition:   []byte{},
				Restrictions: []ids.ID{{0}, {0}},
			}},
			shouldErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.tx.Verify()
			if test.shouldErr && err == nil {
				t.Fatal("expected verify to return an error but it didn't")
			} else if !test.shouldErr && err != nil {
				t.Fatalf("expected verify to pass but it returned: %s", err)
			}
		})
	}
}
