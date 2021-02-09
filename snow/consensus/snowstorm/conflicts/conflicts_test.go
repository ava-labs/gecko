// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package conflicts

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
)

func TestInvalidTx(t *testing.T) {
	c := New()

	tx := &choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}

	{
		err := c.Add(tx)
		assert.Error(t, err)
	}
	{
		_, err := c.IsVirtuous(tx)
		assert.Error(t, err)
	}
	{
		_, err := c.Conflicts(tx)
		assert.Error(t, err)
	}
	assert.Empty(t, c.txs)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.pendingAccept)
	assert.Empty(t, c.pendingReject)
}

func TestNoConflicts(t *testing.T) {
	c := New()

	tx := &TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}

	conflicts, err := c.Conflicts(tx)
	assert.NoError(t, err)
	assert.Empty(t, conflicts)
}

func TestIsVirtuousConflicts(t *testing.T) {
	c := New()

	tx := &TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}

	virtuous, err := c.IsVirtuous(tx)
	assert.NoError(t, err)
	assert.True(t, virtuous)
}

func TestAcceptConflicts(t *testing.T) {
	c := New()

	tx := &TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}

	err := c.Add(tx)
	assert.NoError(t, err)

	toAccepts, toRejects := c.Updateable()
	assert.Empty(t, toAccepts)
	assert.Empty(t, toRejects)

	c.Accept(tx.ID())

	toAccepts, toRejects = c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Empty(t, toRejects)
	assert.Empty(t, c.txs)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.pendingAccept)
	assert.Empty(t, c.pendingReject)

	toAccept := toAccepts[0]
	assert.Equal(t, tx.ID(), toAccept.ID())
}
