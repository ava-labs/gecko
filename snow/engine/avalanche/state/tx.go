// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/vertex"
)

var (
	_ conflicts.Tx = &tx{}
)

type tx struct {
	serializer *Serializer
	tx         vertex.StatelessTx
	tr         conflicts.Transition
}

func (t *tx) ID() ids.ID { return t.tx.ID() }

func (t *tx) Accept() error {
	epoch := t.tx.Epoch()
	if err := t.tr.Accept(epoch); err != nil {
		return err
	}
	if err := t.serializer.state.SetTxStatus(t.ID(), choices.Accepted); err != nil {
		return err
	}
	for _, restriction := range t.tx.Restrictions() {
		previousEpoch := t.serializer.state.TrRestriction(restriction)
		if previousEpoch >= epoch {
			continue
		}
		if err := t.serializer.state.SetTrRestriction(restriction, epoch); err != nil {
			return err
		}
	}
	return t.serializer.db.Commit()
}

func (t *tx) Reject() error {
	if err := t.tr.Reject(t.tx.Epoch()); err != nil {
		return err
	}
	if err := t.serializer.state.SetTxStatus(t.ID(), choices.Rejected); err != nil {
		return err
	}
	return t.serializer.db.Commit()
}

func (t *tx) Status() choices.Status {
	txStatus := t.serializer.state.TxStatus(t.ID())
	if txStatus != choices.Unknown {
		return txStatus
	}
	trStatus := t.tr.Status()
	if trStatus == choices.Accepted && t.tx.Epoch() != t.tr.Epoch() {
		return choices.Rejected
	}
	return trStatus
}

func (t *tx) Transition() conflicts.Transition { return t.tr }

func (t *tx) Epoch() uint32 { return t.tx.Epoch() }

func (t *tx) Restrictions() []ids.ID { return t.tx.Restrictions() }

func (t *tx) Verify() error {
	trID := t.tr.ID()
	epoch := t.tx.Epoch()
	restriction := t.serializer.state.TrRestriction(trID)
	if restriction > epoch {
		return fmt.Errorf("transition %s was restriction to epoch %d", trID, epoch)
	}
	return t.tr.Verify(t.tx.Epoch())
}

func (t *tx) Bytes() []byte { return t.tx.Bytes() }
