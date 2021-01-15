// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"testing"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
)

type testOperable struct {
	avax.TestTransferable `serialize:"true"`

	Outputs []verify.State `serialize:"true"`
}

func (o *testOperable) Outs() []verify.State { return o.Outputs }

func TestOperationVerifyNil(t *testing.T) {
	c := linearcodec.NewDefault()
	m := codec.NewDefaultManager()
	if err := m.RegisterCodec(apricotCodecVersion, c); err != nil {
		t.Fatal(err)
	}

	op := (*Operation)(nil)
	if err := op.Verify(m); err == nil {
		t.Fatalf("Should have errored due to nil operation")
	}
}

func TestOperationVerifyEmpty(t *testing.T) {
	c := linearcodec.NewDefault()
	m := codec.NewDefaultManager()
	if err := m.RegisterCodec(apricotCodecVersion, c); err != nil {
		t.Fatal(err)
	}

	op := &Operation{
		Asset: avax.Asset{ID: ids.Empty},
	}
	if err := op.Verify(m); err == nil {
		t.Fatalf("Should have errored due to empty operation")
	}
}

func TestOperationVerifyUTXOIDsNotSorted(t *testing.T) {
	c := linearcodec.NewDefault()
	m := codec.NewDefaultManager()
	if err := m.RegisterCodec(apricotCodecVersion, c); err != nil {
		t.Fatal(err)
	}

	op := &Operation{
		Asset: avax.Asset{ID: ids.Empty},
		UTXOIDs: []*avax.UTXOID{
			{
				TxID:        ids.Empty,
				OutputIndex: 1,
			},
			{
				TxID:        ids.Empty,
				OutputIndex: 0,
			},
		},
		Op: &testOperable{},
	}
	if err := op.Verify(m); err == nil {
		t.Fatalf("Should have errored due to unsorted utxoIDs")
	}
}

func TestOperationVerify(t *testing.T) {
	c := linearcodec.NewDefault()
	m := codec.NewDefaultManager()
	if err := m.RegisterCodec(apricotCodecVersion, c); err != nil {
		t.Fatal(err)
	}

	assetID := ids.GenerateTestID()
	op := &Operation{
		Asset: avax.Asset{ID: assetID},
		UTXOIDs: []*avax.UTXOID{
			{
				TxID:        assetID,
				OutputIndex: 1,
			},
		},
		Op: &testOperable{},
	}
	if err := op.Verify(m); err != nil {
		t.Fatal(err)
	}
}

func TestOperationSorting(t *testing.T) {
	c := linearcodec.NewDefault()
	if err := c.RegisterType(&testOperable{}); err != nil {
		t.Fatal(err)
	}

	m := codec.NewDefaultManager()
	if err := m.RegisterCodec(apricotCodecVersion, c); err != nil {
		t.Fatal(err)
	}

	ops := []*Operation{
		{
			Asset: avax.Asset{ID: ids.Empty},
			UTXOIDs: []*avax.UTXOID{
				{
					TxID:        ids.Empty,
					OutputIndex: 1,
				},
			},
			Op: &testOperable{},
		},
		{
			Asset: avax.Asset{ID: ids.Empty},
			UTXOIDs: []*avax.UTXOID{
				{
					TxID:        ids.Empty,
					OutputIndex: 0,
				},
			},
			Op: &testOperable{},
		},
	}
	if isSortedAndUniqueOperations(ops, m, apricotCodecVersion) {
		t.Fatalf("Shouldn't be sorted")
	}
	sortOperations(ops, m, apricotCodecVersion)
	if !isSortedAndUniqueOperations(ops, m, apricotCodecVersion) {
		t.Fatalf("Should be sorted")
	}
	ops = append(ops, &Operation{
		Asset: avax.Asset{ID: ids.Empty},
		UTXOIDs: []*avax.UTXOID{
			{
				TxID:        ids.Empty,
				OutputIndex: 1,
			},
		},
		Op: &testOperable{},
	})
	if isSortedAndUniqueOperations(ops, m, apricotCodecVersion) {
		t.Fatalf("Shouldn't be unique")
	}
}

func TestOperationTxNotState(t *testing.T) {
	intf := interface{}(&OperationTx{})
	if _, ok := intf.(verify.State); ok {
		t.Fatalf("shouldn't be marked as state")
	}
}
