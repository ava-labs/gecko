// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package conflicts

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
)

type Test struct {
	Name  string
	Steps []Step
}

type Step struct {
	Add           []Tx
	Processing    []ids.ID
	NotProcessing []ids.ID
	IsVirtuous    []Tx
	IsNotVirtuous []Tx
	Conflicts     []TxConflicts
	Accept        []ids.ID
	Acceptable    []ids.ID
	Rejectable    []ids.ID
	ShouldBeEmpty bool
}

type TxConflicts struct {
	Tx        Tx
	Conflicts []ids.ID
}

func TestVectors(t *testing.T) {
	inputID0 := ids.GenerateTestID()
	inputID1 := ids.GenerateTestID()
	inputID2 := ids.GenerateTestID()

	trAID := ids.GenerateTestID()
	trA := &TestTransition{IDV: trAID}
	trBID := ids.GenerateTestID()
	trB := &TestTransition{
		IDV:       trBID,
		InputIDsV: []ids.ID{inputID0},
	}
	trCID := ids.GenerateTestID()
	trC := &TestTransition{
		IDV:       trCID,
		InputIDsV: []ids.ID{inputID0},
	}
	trDID := ids.GenerateTestID()
	trD := &TestTransition{
		IDV:           trDID,
		DependenciesV: []Transition{trA},
	}
	trEID := ids.GenerateTestID()
	trE := &TestTransition{
		IDV:           trEID,
		DependenciesV: []Transition{trD},
	}
	trFID := ids.GenerateTestID()
	trF := &TestTransition{
		IDV:           trFID,
		DependenciesV: []Transition{trB},
	}
	trGID := ids.GenerateTestID()
	trG := &TestTransition{
		IDV:           trGID,
		DependenciesV: []Transition{trC},
		InputIDsV:     []ids.ID{inputID1},
	}
	trHID := ids.GenerateTestID()
	trH := &TestTransition{
		IDV:       trHID,
		InputIDsV: []ids.ID{inputID1},
	}
	trIID := ids.GenerateTestID()
	trI := &TestTransition{
		IDV:           trIID,
		DependenciesV: []Transition{trG},
		InputIDsV:     []ids.ID{inputID2},
	}

	//          | Dependencies | Inputs |
	trs := []*TestTransition{
		trA, // |              |        |
		trB, // |              |      0 |
		trC, // |              |      0 |
		trD, // |            A |        |
		trE, // |            D |        |
		trF, // |            B |        |
		trG, // |            C |      1 |
		trH, // |              |      1 |
		trI, // |            G |      2 |
	}

	txAEpoch0ID := ids.GenerateTestID()
	txAEpoch0 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txAEpoch0ID},
		TransitionV:   trA,
	}
	txBEpoch0ID := ids.GenerateTestID()
	txBEpoch0 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txBEpoch0ID},
		TransitionV:   trB,
	}
	txBEpoch1ID := ids.GenerateTestID()
	txBEpoch1 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txBEpoch1ID},
		TransitionV:   trB,
		EpochV:        1,
	}
	txCEpoch0ID := ids.GenerateTestID()
	txCEpoch0 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txCEpoch0ID},
		TransitionV:   trC,
	}
	txCEpoch1ID := ids.GenerateTestID()
	txCEpoch1 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txCEpoch1ID},
		TransitionV:   trC,
		EpochV:        1,
	}
	txCEpoch2ID := ids.GenerateTestID()
	txCEpoch2 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txCEpoch2ID},
		TransitionV:   trC,
		EpochV:        2,
	}
	txDEpoch0ID := ids.GenerateTestID()
	txDEpoch0 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txDEpoch0ID},
		TransitionV:   trD,
	}
	txEEpoch0ID := ids.GenerateTestID()
	txEEpoch0 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txEEpoch0ID},
		TransitionV:   trE,
	}
	txFEpoch0ID := ids.GenerateTestID()
	txFEpoch0 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txFEpoch0ID},
		TransitionV:   trF,
	}
	txGEpoch0ID := ids.GenerateTestID()
	txGEpoch0 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txGEpoch0ID},
		TransitionV:   trG,
	}
	txGEpoch1ID := ids.GenerateTestID()
	txGEpoch1 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txGEpoch1ID},
		TransitionV:   trG,
		EpochV:        1,
	}
	txHEpoch0ID := ids.GenerateTestID()
	txHEpoch0 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txHEpoch0ID},
		TransitionV:   trH,
	}
	txIEpoch0ID := ids.GenerateTestID()
	txIEpoch0 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txIEpoch0ID},
		TransitionV:   trI,
	}
	txARestCEpoch1ID := ids.GenerateTestID()
	txARestCEpoch1 := &TestTx{
		TestDecidable: choices.TestDecidable{IDV: txARestCEpoch1ID},
		TransitionV:   trA,
		EpochV:        1,
		RestrictionsV: []ids.ID{trCID},
	}

	txs := []*TestTx{ //   | Epoch | Restrictions | Transitions | Dependencies | Inputs |
		txAEpoch0,      // |     0 |              |           A |              |        |
		txBEpoch0,      // |     0 |              |           B |              |      0 |
		txBEpoch1,      // |     1 |              |           B |              |      0 |
		txCEpoch0,      // |     0 |              |           C |              |      0 |
		txCEpoch1,      // |     1 |              |           C |              |      0 |
		txCEpoch2,      // |     2 |              |           C |              |      0 |
		txDEpoch0,      // |     0 |              |           D |            A |        |
		txEEpoch0,      // |     0 |              |           E |            D |        |
		txFEpoch0,      // |     0 |              |           F |            B |        |
		txGEpoch0,      // |     0 |              |           G |            C |      1 |
		txGEpoch1,      // |     1 |              |           G |            C |      1 |
		txHEpoch0,      // |     0 |              |           H |              |      1 |
		txIEpoch0,      // |     0 |              |           I |            G |      2 |
		txARestCEpoch1, // |     1 |            C |           A |              |        |
	}

	tests := []struct {
		Name  string
		Steps []Step
	}{
		{
			Name: "adding changes processing",
			Steps: []Step{
				{
					NotProcessing: []ids.ID{
						trAID,
					},
				},
				{
					Add: []Tx{
						txAEpoch0,
					},
					Processing: []ids.ID{
						trAID,
					},
				},
			},
		},
		{
			Name: "correctly marked as virtuous",
			Steps: []Step{
				{
					IsVirtuous: []Tx{
						txAEpoch0,
					},
				},
				{
					Add: []Tx{
						txAEpoch0,
					},
					IsVirtuous: []Tx{
						txAEpoch0,
					},
				},
			},
		},
		{
			Name: "correctly marked as rogue",
			Steps: []Step{
				{
					IsVirtuous: []Tx{
						txBEpoch0,
						txCEpoch0,
					},
					Conflicts: []TxConflicts{
						{
							Tx: txBEpoch0,
						},
						{
							Tx: txCEpoch0,
						},
					},
				},
				{
					Add: []Tx{
						txBEpoch0,
					},
					IsVirtuous: []Tx{
						txBEpoch0,
					},
					IsNotVirtuous: []Tx{
						txCEpoch0,
					},
					Conflicts: []TxConflicts{
						{
							Tx: txBEpoch0,
						},
						{
							Tx: txCEpoch0,
							Conflicts: []ids.ID{
								txBEpoch0ID,
							},
						},
					},
				},
			},
		},
		{
			Name: "restriction would be conflicting",
			Steps: []Step{
				{
					IsVirtuous: []Tx{
						txCEpoch0,
						txARestCEpoch1,
					},
					Conflicts: []TxConflicts{
						{
							Tx: txCEpoch0,
						},
						{
							Tx: txARestCEpoch1,
						},
					},
				},
				{
					Add: []Tx{
						txCEpoch0,
					},
					IsVirtuous: []Tx{
						txCEpoch0,
					},
					IsNotVirtuous: []Tx{
						txARestCEpoch1,
					},
					Conflicts: []TxConflicts{
						{
							Tx: txCEpoch0,
						},
						{
							Tx: txARestCEpoch1,
							Conflicts: []ids.ID{
								txCEpoch0ID,
							},
						},
					},
				},
			},
		},
		{
			Name: "restriction is conflicting",
			Steps: []Step{
				{
					IsVirtuous: []Tx{
						txARestCEpoch1,
						txCEpoch0,
					},
					Conflicts: []TxConflicts{
						{
							Tx: txARestCEpoch1,
						},
						{
							Tx: txCEpoch0,
						},
					},
				},
				{
					Add: []Tx{
						txARestCEpoch1,
					},
					IsVirtuous: []Tx{
						txARestCEpoch1,
					},
					IsNotVirtuous: []Tx{
						txCEpoch0,
					},
					Conflicts: []TxConflicts{
						{
							Tx: txARestCEpoch1,
						},
						{
							Tx: txCEpoch0,
							Conflicts: []ids.ID{
								txARestCEpoch1ID,
							},
						},
					},
				},
			},
		},
		{
			Name: "accept no conflicts",
			Steps: []Step{
				{
					Add: []Tx{
						txAEpoch0,
					},
					Accept: []ids.ID{
						txAEpoch0ID,
					},
					Acceptable: []ids.ID{
						txAEpoch0ID,
					},
					Rejectable:    []ids.ID{},
					ShouldBeEmpty: true,
				},
			},
		},
		{
			Name: "accept no conflicts with dependency",
			Steps: []Step{
				{
					Add: []Tx{
						txAEpoch0,
						txDEpoch0,
					},
					Accept: []ids.ID{
						txAEpoch0ID,
					},
					Acceptable: []ids.ID{
						txAEpoch0ID,
					},
					Rejectable: []ids.ID{},
				},
				{
					Accept: []ids.ID{
						txDEpoch0ID,
					},
					Acceptable: []ids.ID{
						txDEpoch0ID,
					},
					Rejectable:    []ids.ID{},
					ShouldBeEmpty: true,
				},
			},
		},
		{
			Name: "accept no conflicts with pending dependency",
			Steps: []Step{
				{
					Add: []Tx{
						txAEpoch0,
						txDEpoch0,
					},
					Accept: []ids.ID{
						txDEpoch0ID,
					},
					Acceptable: []ids.ID{},
					Rejectable: []ids.ID{},
				},
				{
					Accept: []ids.ID{
						txAEpoch0ID,
					},
					Acceptable: []ids.ID{
						txAEpoch0ID,
						txDEpoch0ID,
					},
					Rejectable:    []ids.ID{},
					ShouldBeEmpty: true,
				},
			},
		},
		{
			Name: "accept no conflicts with pending transitive dependency",
			Steps: []Step{
				{
					Add: []Tx{
						txAEpoch0,
						txDEpoch0,
						txEEpoch0,
					},
					Accept: []ids.ID{
						txEEpoch0ID,
					},
					Acceptable: []ids.ID{},
					Rejectable: []ids.ID{},
				},
				{
					Accept: []ids.ID{
						txDEpoch0ID,
					},
					Acceptable: []ids.ID{},
					Rejectable: []ids.ID{},
				},
				{
					Accept: []ids.ID{
						txAEpoch0ID,
					},
					Acceptable: []ids.ID{
						txAEpoch0ID,
						txDEpoch0ID,
						txEEpoch0ID,
					},
					Rejectable:    []ids.ID{},
					ShouldBeEmpty: true,
				},
			},
		},
		{
			Name: "transitively reject accepted dependency",
			Steps: []Step{
				{
					Add: []Tx{
						txBEpoch0,
						txCEpoch0,
						txFEpoch0,
					},
					Accept: []ids.ID{
						txFEpoch0ID,
					},
					Acceptable: []ids.ID{},
					Rejectable: []ids.ID{},
				},
				{
					Accept: []ids.ID{
						txCEpoch0ID,
					},
					Acceptable: []ids.ID{
						txCEpoch0ID,
					},
					Rejectable: []ids.ID{
						txBEpoch0ID,
						txFEpoch0ID,
					},
					ShouldBeEmpty: true,
				},
			},
		},
		{
			Name: "transitively reject accepted dependency due to epoch",
			Steps: []Step{
				{
					Add: []Tx{
						txBEpoch0,
						txBEpoch1,
						txFEpoch0,
					},
					Accept: []ids.ID{
						txFEpoch0ID,
					},
					Acceptable: []ids.ID{},
					Rejectable: []ids.ID{},
				},
				{
					Accept: []ids.ID{
						txBEpoch1ID,
					},
					Acceptable: []ids.ID{
						txBEpoch1ID,
					},
					Rejectable: []ids.ID{
						txBEpoch0ID,
						txFEpoch0ID,
					},
					ShouldBeEmpty: true,
				},
			},
		},
		{
			Name: "accept restricted dependency",
			Steps: []Step{
				{
					Add: []Tx{
						txCEpoch0,
						txCEpoch1,
						txGEpoch0,
						txGEpoch1,
						txARestCEpoch1,
					},
					Accept: []ids.ID{
						txARestCEpoch1ID,
					},
					Acceptable: []ids.ID{
						txARestCEpoch1ID,
					},
					Rejectable: []ids.ID{
						txCEpoch0ID,
						txGEpoch0ID,
					},
				},
				{
					Accept: []ids.ID{
						txCEpoch1ID,
					},
					Acceptable: []ids.ID{
						txCEpoch1ID,
					},
					Rejectable: []ids.ID{},
				},
				{
					Accept: []ids.ID{
						txGEpoch1ID,
					},
					Acceptable: []ids.ID{
						txGEpoch1ID,
					},
					Rejectable:    []ids.ID{},
					ShouldBeEmpty: true,
				},
			},
		},
		{
			Name: "accept with rejected dependency",
			Steps: []Step{
				{
					Add: []Tx{
						txBEpoch0,
						txCEpoch0,
						txGEpoch0,
						txHEpoch0,
					},
					Accept: []ids.ID{
						txHEpoch0ID,
					},
					Acceptable: []ids.ID{
						txHEpoch0ID,
					},
					Rejectable: []ids.ID{
						txGEpoch0ID,
					},
				},
				{
					Accept: []ids.ID{
						txCEpoch0ID,
					},
					Acceptable: []ids.ID{
						txCEpoch0ID,
					},
					Rejectable: []ids.ID{
						txBEpoch0ID,
					},
					ShouldBeEmpty: true,
				},
			},
		},
		{
			Name: "transitively reject dependency then accept now virtuous tx",
			Steps: []Step{
				{
					Add: []Tx{
						txBEpoch0,
						txCEpoch0,
						txGEpoch0,
						txHEpoch0,
					},
					Accept: []ids.ID{
						txBEpoch0ID,
					},
					Acceptable: []ids.ID{
						txBEpoch0ID,
					},
					Rejectable: []ids.ID{
						txCEpoch0ID,
						txGEpoch0ID,
					},
				},
				{
					Accept: []ids.ID{
						txHEpoch0ID,
					},
					Acceptable: []ids.ID{
						txHEpoch0ID,
					},
					Rejectable:    []ids.ID{},
					ShouldBeEmpty: true,
				},
			},
		},
		{
			Name: "reject multiple from multiple epoch conflicts",
			Steps: []Step{
				{
					Add: []Tx{
						txCEpoch0,
						txCEpoch1,
						txCEpoch2,
						txGEpoch1,
					},
					Accept: []ids.ID{
						txCEpoch2ID,
					},
					Acceptable: []ids.ID{
						txCEpoch2ID,
					},
					Rejectable: []ids.ID{
						txCEpoch0ID,
						txCEpoch1ID,
						txGEpoch1ID,
					},
					ShouldBeEmpty: true,
				},
			},
		},
		{
			Name: "reject twice across epoch rounds",
			Steps: []Step{
				{
					Add: []Tx{
						txCEpoch0,
						txCEpoch1,
						txGEpoch0,
						txGEpoch1,
						txIEpoch0,
					},
					Accept: []ids.ID{
						txGEpoch1ID,
					},
					Acceptable: []ids.ID{},
					Rejectable: []ids.ID{},
				},
				{
					Accept: []ids.ID{
						txCEpoch1ID,
					},
					Acceptable: []ids.ID{
						txCEpoch1ID,
						txGEpoch1ID,
					},
					Rejectable: []ids.ID{
						txCEpoch0ID,
						txGEpoch0ID,
						txIEpoch0ID,
					},
					ShouldBeEmpty: true,
				},
			},
		},
		{
			Name: "reject twice across epoch rounds",
			Steps: []Step{
				{
					Add: []Tx{
						txCEpoch0,
						txCEpoch1,
						txGEpoch0,
						txGEpoch1,
						txIEpoch0,
					},
					Accept: []ids.ID{
						txGEpoch1ID,
					},
					Acceptable: []ids.ID{},
					Rejectable: []ids.ID{},
				},
				{
					Accept: []ids.ID{
						txCEpoch1ID,
					},
					Acceptable: []ids.ID{
						txCEpoch1ID,
						txGEpoch1ID,
					},
					Rejectable: []ids.ID{
						txCEpoch0ID,
						txGEpoch0ID,
						txIEpoch0ID,
					},
					ShouldBeEmpty: true,
				},
				{
					Add:           []Tx(nil),
					Processing:    []ids.ID(nil),
					NotProcessing: []ids.ID(nil),
					IsVirtuous:    []Tx(nil),
					IsNotVirtuous: []Tx(nil),
					Conflicts:     []TxConflicts(nil),
					Accept:        []ids.ID(nil),
					Acceptable:    []ids.ID(nil),
					Rejectable:    []ids.ID(nil),
					ShouldBeEmpty: false,
				},
			},
		},
	}
	for _, test := range tests {
		for _, tr := range trs {
			tr.StatusV = choices.Processing
			tr.EpochV = 0
		}
		for _, tx := range txs {
			tx.StatusV = choices.Processing
		}

		t.Run(test.Name, func(t *testing.T) {
			c := New()

			for _, step := range test.Steps {
				for _, tx := range step.Add {
					c.Add(tx)
				}
				for _, trID := range step.Processing {
					processing := c.Processing(trID)
					assert.True(t, processing, "transition %s should have been processing", trID)
				}
				for _, trID := range step.NotProcessing {
					processing := c.Processing(trID)
					assert.False(t, processing, "transition %s shouldn't have been processing", trID)
				}
				for _, tx := range step.IsVirtuous {
					isVirtuous := c.IsVirtuous(tx)
					assert.True(t, isVirtuous, "tx %s should have been virtuous", tx.ID())
				}
				for _, tx := range step.IsNotVirtuous {
					isVirtuous := c.IsVirtuous(tx)
					assert.False(t, isVirtuous, "tx %s shouldn't have been virtuous", tx.ID())
				}
				for _, conf := range step.Conflicts {
					conflicts := c.Conflicts(conf.Tx)
					conflictsTxIDs := make([]ids.ID, len(conflicts))
					for i, tx := range conflicts {
						conflictsTxIDs[i] = tx.ID()
					}
					assert.ElementsMatch(t, conf.Conflicts, conflictsTxIDs, "wrong transactions marked as conflicting")
				}
				for _, txID := range step.Accept {
					c.Accept(txID)
				}
				acceptable, rejectable := c.Updateable()
				acceptableTxIDs := make([]ids.ID, len(acceptable))
				for i, tx := range acceptable {
					err := tx.Accept()
					assert.NoError(t, err)

					acceptableTxIDs[i] = tx.ID()
				}
				rejectableTxIDs := make([]ids.ID, len(rejectable))
				for i, tx := range rejectable {
					err := tx.Reject()
					assert.NoError(t, err)

					rejectableTxIDs[i] = tx.ID()
				}

				if step.Acceptable != nil {
					assert.Equal(t, step.Acceptable, acceptableTxIDs, "wrong transactions or order marked as acceptable")
				}
				if step.Rejectable != nil {
					assert.ElementsMatch(t, step.Rejectable, rejectableTxIDs, "wrong transactions marked as rejectable")
				}

				if step.ShouldBeEmpty {
					assert.Empty(t, c.txs)
					assert.Empty(t, c.transitionNodes)
					assert.Empty(t, c.utxos)
					assert.Empty(t, c.conditionallyAccepted)
					assert.Empty(t, c.acceptableIDs)
					assert.Empty(t, c.acceptable)
					assert.Empty(t, c.rejectableIDs)
					assert.Empty(t, c.rejectable)
				}
			}
		})
	}
}

func TestRejectDependencyTwice(t *testing.T) {
	c := New()

	trA := &TestTransition{
		IDV:       ids.GenerateTestID(),
		StatusV:   choices.Processing,
		InputIDsV: []ids.ID{ids.GenerateTestID()},
	}
	trB := &TestTransition{
		IDV:           ids.GenerateTestID(),
		StatusV:       choices.Processing,
		InputIDsV:     []ids.ID{ids.GenerateTestID()},
		DependenciesV: []Transition{trA},
	}
	trC := &TestTransition{
		IDV:           ids.GenerateTestID(),
		StatusV:       choices.Processing,
		InputIDsV:     []ids.ID{ids.GenerateTestID()},
		DependenciesV: []Transition{trB},
	}

	txA0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trA,
	}
	txA1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trA,
		EpochV:      1,
	}
	txA2 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trA,
		EpochV:      2,
	}
	txB0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trB,
	}
	txB1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trB,
		EpochV:      1,
	}
	txB2 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trB,
		EpochV:      2,
	}
	txC0 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trC,
	}
	txC1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trC,
		EpochV:      1,
	}
	txC2 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trC,
		EpochV:      2,
	}

	c.Add(txA0)
	c.Add(txA1)
	c.Add(txA2)
	c.Add(txB0)
	c.Add(txB1)
	c.Add(txB2)
	c.Add(txC0)
	c.Add(txC1)
	c.Add(txC2)

	c.Accept(txA2.ID())

	toAccepts := c.updateAccepted()
	toRejects := c.updateRejected()
	assert.Len(t, toAccepts, 1)
	assert.Len(t, toRejects, 4)

	toAccept := toAccepts[0]
	assert.Equal(t, txA2.ID(), toAccept.ID())
	err := toAccept.Accept()
	assert.NoError(t, err)

	for i, toReject := range toRejects {
		err = toReject.Reject()
		assert.NoError(t, err, "Error rejecting the %d rejectable transaction %s", i, toReject.ID())
	}

	toAccepts = c.updateAccepted()
	toRejects = c.updateRejected()
	assert.Empty(t, toAccepts)
	assert.Len(t, toRejects, 2)

	for i, toReject := range toRejects {
		err = toReject.Reject()
		assert.NoError(t, err, "Error rejecting the %d rejectable transaction %s", i, toReject.ID())
	}

	toAccepts = c.updateAccepted()
	toRejects = c.updateRejected()
	assert.Empty(t, toAccepts)
	assert.Empty(t, toRejects)

	c.Accept(txB2.ID())

	toAccepts = c.updateAccepted()
	toRejects = c.updateRejected()
	assert.Len(t, toAccepts, 1)
	assert.Empty(t, toRejects)

	toAccept = toAccepts[0]
	assert.Equal(t, txB2.ID(), toAccept.ID())
	err = toAccept.Accept()
	assert.NoError(t, err)

	c.Accept(txC2.ID())

	toAccepts = c.updateAccepted()
	toRejects = c.updateRejected()
	assert.Len(t, toAccepts, 1)
	assert.Empty(t, toRejects)

	toAccept = toAccepts[0]
	assert.Equal(t, txC2.ID(), toAccept.ID())
	err = toAccept.Accept()
	assert.NoError(t, err)

	toAccepts = c.updateAccepted()
	toRejects = c.updateRejected()
	assert.Empty(t, toAccepts)
	assert.Empty(t, toRejects)
	assert.Empty(t, c.txs)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.transitionNodes)
}

func TestRejectTwiceAcrossRounds(t *testing.T) {
	c := New()

	conflictInput := ids.GenerateTestID()

	// Regression test to ensure that a transaction cannot be rejected
	// twice across multiple iterations of the worklist. This is prevented
	// by maintaining the set of rejected txIDs across multiple iterations
	// of the worklist.
	trA := &TestTransition{
		IDV:       ids.GenerateTestID(),
		StatusV:   choices.Processing,
		InputIDsV: []ids.ID{conflictInput},
	}
	trB := &TestTransition{
		IDV:       ids.GenerateTestID(),
		StatusV:   choices.Processing,
		InputIDsV: []ids.ID{conflictInput},
	}
	trC := &TestTransition{
		IDV:           ids.GenerateTestID(),
		StatusV:       choices.Processing,
		InputIDsV:     []ids.ID{ids.GenerateTestID()},
		DependenciesV: []Transition{trA},
	}
	trD := &TestTransition{
		IDV:           ids.GenerateTestID(),
		StatusV:       choices.Processing,
		InputIDsV:     []ids.ID{ids.GenerateTestID()},
		DependenciesV: []Transition{trC},
	}

	txA := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trA,
	}
	txB := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trB,
	}
	txC := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trC,
	}
	txD := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trD,
	}

	c.Add(txA)
	c.Add(txB)
	c.Add(txC)
	c.Add(txD)

	// Accept txB first, such that it is marked as accepted.
	c.Accept(txB.ID())

	// Accepting txB should cause txA to be rejected due to a conflict.
	// Since txA is rejected, txC will be rejected due to a missing dependency.
	// Since trD depends on trC, this will transtiively cause txD to be
	// rejected as well.
	toAccepts, toRejects := c.Updateable()
	assert.Len(t, toAccepts, 1)
	assert.Len(t, toRejects, 3)

	expectedAccepts := ids.Set{}
	expectedAccepts.Add(txB.ID())
	for i, toAccept := range toAccepts {
		err := toAccept.Accept()
		assert.NoError(t, err)
		assert.True(t, expectedAccepts.Contains(toAccept.ID()), "Unexpected accepted txID: %s index %d", toAccept.ID(), i)
	}

	expectedRejects := ids.Set{}
	expectedRejects.Add(txA.ID(), txC.ID(), txD.ID())
	for i, toReject := range toRejects {
		assert.True(t, expectedRejects.Contains(toReject.ID()), "Unexpected rejected txID: %s index %d", toReject.ID(), i)
		err := toReject.Reject()
		assert.NoError(t, err)
	}

	toAccepts, toRejects = c.Updateable()
	assert.Empty(t, toAccepts)
	assert.Empty(t, toRejects)
	assert.Empty(t, c.txs)
	assert.Empty(t, c.utxos)
	assert.Empty(t, c.transitionNodes)
	assert.Equal(t, 0, c.acceptableIDs.Len())
	assert.Equal(t, 0, c.rejectableIDs.Len())
}
