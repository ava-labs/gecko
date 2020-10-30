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

	err := c.Add(tx)
	assert.Error(t, err)

	_, err = c.IsVirtuous(tx)
	assert.Error(t, err)

	_, err = c.Conflicts(tx)
	assert.Error(t, err)
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

func TestIsVirtuousNoConflicts(t *testing.T) {
	c := New()

	tx := &TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}

	virtuous, err := c.IsVirtuous(tx)
	assert.NoError(t, err)
	assert.True(t, virtuous)
}

func TestIsVirtuousConflicts(t *testing.T) {
	c := New()

	inputID := ids.GenerateTestID()
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID},
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID},
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	virtuous, err := c.IsVirtuous(tx1)
	assert.NoError(t, err)
	assert.False(t, virtuous)
}

func TestAccept(t *testing.T) {
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
	assert.True(t, toAccept.ID().Equals(tx.ID()))
}

func TestAcceptLastSpender(t *testing.T) {
	c := New()

	tx := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{ids.GenerateTestID()},
	}

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
	assert.True(t, toAccept.ID().Equals(tx.ID()))
}

func TestAcceptDependency(t *testing.T) {
	c := New()

	tx0 := &TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		DependenciesV: []Tx{tx0},
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	err = c.Add(tx1)
	assert.NoError(t, err)

	c.Accept(tx1.ID())

	toAccepts, toRejects := c.Updateable()
	assert.Empty(t, toAccepts)
	assert.Empty(t, toRejects)
	c.Accept(tx0.ID())

	toAccepts, toRejects = c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Empty(t, toRejects)

	toAccept := toAccepts[0]
	assert.True(t, toAccept.ID().Equals(tx0.ID()))

	err = toAccept.Accept()
	assert.NoError(t, err)

	toAccepts, toRejects = c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Empty(t, toRejects)

	toAccept = toAccepts[0]
	assert.True(t, toAccept.ID().Equals(tx1.ID()))
	assert.Empty(t, c.txs)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.pendingAccept)
	assert.Empty(t, c.pendingReject)
}

func TestReject(t *testing.T) {
	c := New()

	inputID := ids.GenerateTestID()
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID},
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID},
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	err = c.Add(tx1)
	assert.NoError(t, err)

	c.Accept(tx0.ID())

	toAccepts, toRejects := c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Len(t, toRejects, 1)
	assert.Empty(t, c.txs)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.pendingAccept)
	assert.Empty(t, c.pendingReject)

	toAccept := toAccepts[0]
	assert.True(t, toAccept.ID().Equals(tx0.ID()))

	toReject := toRejects[0]
	assert.True(t, toReject.ID().Equals(tx1.ID()))
}

func TestRejectPartialSpenders(t *testing.T) {
	c := New()

	inputID0 := ids.GenerateTestID()
	inputID1 := ids.GenerateTestID()
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID0},
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID0, inputID1},
	}
	tx2 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID1},
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	err = c.Add(tx1)
	assert.NoError(t, err)

	err = c.Add(tx2)
	assert.NoError(t, err)

	c.Accept(tx0.ID())

	toAccepts, toRejects := c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Len(t, toRejects, 1)

	toAccept := toAccepts[0]
	assert.True(t, toAccept.ID().Equals(tx0.ID()))

	toReject := toRejects[0]
	assert.True(t, toReject.ID().Equals(tx1.ID()))

	c.Accept(tx2.ID())

	toAccepts, toRejects = c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Empty(t, toRejects)

	toAccept = toAccepts[0]
	assert.True(t, toAccept.ID().Equals(tx2.ID()))
	assert.Empty(t, c.txs)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.pendingAccept)
	assert.Empty(t, c.pendingReject)
}

func TestRejectDependency(t *testing.T) {
	c := New()

	inputID := ids.GenerateTestID()
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID},
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID},
	}
	tx2 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		DependenciesV: []Tx{tx1},
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	err = c.Add(tx1)
	assert.NoError(t, err)

	err = c.Add(tx2)
	assert.NoError(t, err)

	c.Accept(tx0.ID())

	toAccepts, toRejects := c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Len(t, toRejects, 2)
	assert.Empty(t, c.txs)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.pendingAccept)
	assert.Empty(t, c.pendingReject)

	toAccept := toAccepts[0]
	assert.True(t, toAccept.ID().Equals(tx0.ID()))

	toReject := toRejects[0]
	assert.True(t, toReject.ID().Equals(tx1.ID()))

	toReject = toRejects[1]
	assert.True(t, toReject.ID().Equals(tx2.ID()))
}

func TestRejectConditionallyAcceptedDependency(t *testing.T) {
	c := New()

	inputID := ids.GenerateTestID()
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID},
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID},
	}
	tx2 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		DependenciesV: []Tx{tx1},
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	err = c.Add(tx1)
	assert.NoError(t, err)

	err = c.Add(tx2)
	assert.NoError(t, err)

	c.Accept(tx2.ID())

	toAccepts, toRejects := c.Updateable()
	assert.Empty(t, toAccepts)
	assert.Empty(t, toRejects)

	c.Accept(tx0.ID())

	toAccepts, toRejects = c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Len(t, toRejects, 2)
	assert.Empty(t, c.txs)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.pendingAccept)
	assert.Empty(t, c.pendingReject)

	toAccept := toAccepts[0]
	assert.True(t, toAccept.ID().Equals(tx0.ID()))

	toReject := toRejects[0]
	assert.True(t, toReject.ID().Equals(tx1.ID()))

	toReject = toRejects[1]
	assert.True(t, toReject.ID().Equals(tx2.ID()))
}

func TestRejectThenConditionallyAcceptDependency(t *testing.T) {
	c := New()

	inputID := ids.GenerateTestID()
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID},
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		InputIDsV: []ids.ID{inputID},
	}
	tx2 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		DependenciesV: []Tx{tx1},
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	err = c.Add(tx1)
	assert.NoError(t, err)

	err = c.Add(tx2)
	assert.NoError(t, err)

	c.Accept(tx0.ID())
	c.Accept(tx2.ID())

	toAccepts, toRejects := c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Len(t, toRejects, 2)
	assert.Empty(t, c.txs)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.pendingAccept)
	assert.Empty(t, c.pendingReject)

	toAccept := toAccepts[0]
	assert.True(t, toAccept.ID().Equals(tx0.ID()))

	toReject := toRejects[0]
	assert.True(t, toReject.ID().Equals(tx1.ID()))

	toReject = toRejects[1]
	assert.True(t, toReject.ID().Equals(tx2.ID()))
}

func TestTestTx(t *testing.T) {
	tx := &TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}

	err := tx.Verify()
	assert.NoError(t, err)

	bytes := tx.Bytes()
	assert.Empty(t, bytes)
}
