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
}

func TestNoConflicts(t *testing.T) {
	c := New()

	tx := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: ids.GenerateTestID(),
	}

	virtuous, err := c.IsVirtuous(tx)
	assert.NoError(t, err)
	assert.True(t, virtuous)

	conflicts, err := c.Conflicts(tx)
	assert.NoError(t, err)
	assert.Empty(t, conflicts)
}

func TestInputConflicts(t *testing.T) {
	c := New()

	inputIDs := []ids.ID{ids.GenerateTestID()}
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: ids.GenerateTestID(),
		InputIDsV:     inputIDs,
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: ids.GenerateTestID(),
		InputIDsV:     inputIDs,
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	virtuous, err := c.IsVirtuous(tx1)
	assert.NoError(t, err)
	assert.False(t, virtuous)

	conflicts, err := c.Conflicts(tx1)
	assert.NoError(t, err)
	assert.NotEmpty(t, conflicts)
}

func TestOuterRestrictionConflicts(t *testing.T) {
	c := New()

	transitionID := ids.GenerateTestID()
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: transitionID,
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: ids.GenerateTestID(),
		EpochV:        1,
		RestrictionsV: []ids.ID{transitionID},
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	virtuous, err := c.IsVirtuous(tx1)
	assert.NoError(t, err)
	assert.False(t, virtuous)

	conflicts, err := c.Conflicts(tx1)
	assert.NoError(t, err)
	assert.Len(t, conflicts, 1)
}

func TestInnerRestrictionConflicts(t *testing.T) {
	c := New()

	transitionID := ids.GenerateTestID()
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: transitionID,
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: ids.GenerateTestID(),
		EpochV:        1,
		RestrictionsV: []ids.ID{transitionID},
	}

	err := c.Add(tx1)
	assert.NoError(t, err)

	virtuous, err := c.IsVirtuous(tx0)
	assert.NoError(t, err)
	assert.False(t, virtuous)

	conflicts, err := c.Conflicts(tx0)
	assert.NoError(t, err)
	assert.Len(t, conflicts, 1)
}

func TestAcceptNoConflicts(t *testing.T) {
	c := New()

	tx := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: ids.GenerateTestID(),
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
	assert.Empty(t, c.transitions)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.restrictions)
	assert.Empty(t, c.dependencies)

	toAccept := toAccepts[0]
	assert.Equal(t, tx.ID(), toAccept.ID())
}

func TestAcceptNoConflictsWithDependency(t *testing.T) {
	c := New()

	transitionID := ids.GenerateTestID()
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: transitionID,
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: ids.GenerateTestID(),
		DependenciesV: []ids.ID{transitionID},
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	err = c.Add(tx1)
	assert.NoError(t, err)

	toAccepts, toRejects := c.Updateable()
	assert.Empty(t, toAccepts)
	assert.Empty(t, toRejects)

	c.Accept(tx1.ID())

	toAccepts, toRejects = c.Updateable()
	assert.Empty(t, toAccepts)
	assert.Empty(t, toRejects)

	c.Accept(tx0.ID())

	toAccepts, toRejects = c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Empty(t, toRejects)

	toAccept := toAccepts[0]
	assert.Equal(t, tx0.ID(), toAccept.ID())

	toAccepts, toRejects = c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Empty(t, toRejects)
	assert.Empty(t, c.txs)
	assert.Empty(t, c.transitions)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.restrictions)
	assert.Empty(t, c.dependencies)

	toAccept = toAccepts[0]
	assert.Equal(t, tx1.ID(), toAccept.ID())
}

func TestAcceptRejectedDependency(t *testing.T) {
	c := New()

	transitionID := ids.GenerateTestID()
	inputIDs := []ids.ID{ids.GenerateTestID()}
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: transitionID,
		InputIDsV:     inputIDs,
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: ids.GenerateTestID(),
		InputIDsV:     inputIDs,
	}
	tx2 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: ids.GenerateTestID(),
		DependenciesV: []ids.ID{transitionID},
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	err = c.Add(tx1)
	assert.NoError(t, err)

	err = c.Add(tx2)
	assert.NoError(t, err)

	toAccepts, toRejects := c.Updateable()
	assert.Empty(t, toAccepts)
	assert.Empty(t, toRejects)

	c.Accept(tx1.ID())

	toAccepts, toRejects = c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Len(t, toRejects, 1)

	toAccept := toAccepts[0]
	assert.Equal(t, tx1.ID(), toAccept.ID())

	toReject := toRejects[0]
	assert.Equal(t, tx0.ID(), toReject.ID())

	toAccepts, toRejects = c.Updateable()
	assert.Empty(t, toAccepts)
	assert.Len(t, toRejects, 1)
	assert.Empty(t, c.txs)
	assert.Empty(t, c.transitions)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.restrictions)
	assert.Empty(t, c.dependencies)

	toReject = toRejects[0]
	assert.Equal(t, tx2.ID(), toReject.ID())
}

func TestAcceptRejectedEpochDependency(t *testing.T) {
	c := New()

	transitionID := ids.GenerateTestID()
	inputIDs := []ids.ID{ids.GenerateTestID()}
	tx0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: transitionID,
		InputIDsV:     inputIDs,
	}
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: transitionID,
		InputIDsV:     inputIDs,
	}
	tx2 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: transitionID,
		EpochV:        1,
		InputIDsV:     inputIDs,
	}
	tx3 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionIDV: ids.GenerateTestID(),
		DependenciesV: []ids.ID{transitionID},
	}

	err := c.Add(tx0)
	assert.NoError(t, err)

	err = c.Add(tx1)
	assert.NoError(t, err)

	err = c.Add(tx2)
	assert.NoError(t, err)

	err = c.Add(tx3)
	assert.NoError(t, err)

	toAccepts, toRejects := c.Updateable()
	assert.Empty(t, toAccepts)
	assert.Empty(t, toRejects)

	c.Accept(tx2.ID())

	toAccepts, toRejects = c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Len(t, toRejects, 3)
	assert.Empty(t, c.txs)
	assert.Empty(t, c.transitions)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.restrictions)
	assert.Empty(t, c.dependencies)
}
