// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/vertex"
)

func TestTxStatus(t *testing.T) {
	s := newSerializer(t, nil)
	stx, err := vertex.Wrap(0, []byte{0}, nil)
	assert.NoError(t, err)
	tr := &conflicts.TestTransition{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
		EpochV:  1,
	}

	tx := tx{
		serializer: s,
		tx:         stx,
		tr:         tr,
	}

	if status := tx.Status(); status != choices.Rejected {
		t.Fatalf("wrong tx status returned")
	}
}
