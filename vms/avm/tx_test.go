// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/hierarchycodec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

func setupCodec() (codec.GeneralCodec, codec.Manager) {
	c := hierarchycodec.NewDefault()
	preApricotCodec := linearcodec.NewDefault()
	m := codec.NewDefaultManager()
	errs := wrappers.Errs{}

	errs.Add(
		preApricotCodec.RegisterType(&BaseTx{}),
		preApricotCodec.RegisterType(&CreateAssetTx{}),
		preApricotCodec.RegisterType(&OperationTx{}),
		preApricotCodec.RegisterType(&ImportTx{}),
		preApricotCodec.RegisterType(&ExportTx{}),
		preApricotCodec.RegisterType(&secp256k1fx.TransferInput{}),
		preApricotCodec.RegisterType(&secp256k1fx.MintOutput{}),
		preApricotCodec.RegisterType(&secp256k1fx.TransferOutput{}),
		preApricotCodec.RegisterType(&secp256k1fx.MintOperation{}),
		preApricotCodec.RegisterType(&secp256k1fx.Credential{}),
		m.RegisterCodec(preApricotCodecVersion, preApricotCodec),

		c.RegisterType(&BaseTx{}),
		c.RegisterType(&CreateAssetTx{}),
		c.RegisterType(&OperationTx{}),
		c.RegisterType(&ImportTx{}),
		c.RegisterType(&ExportTx{}),
	)
	c.NextGroup()
	errs.Add(
		c.RegisterType(&secp256k1fx.TransferInput{}),
		c.RegisterType(&secp256k1fx.MintOutput{}),
		c.RegisterType(&secp256k1fx.TransferOutput{}),
		c.RegisterType(&secp256k1fx.MintOperation{}),
		c.RegisterType(&secp256k1fx.Credential{}),
		c.RegisterType(&secp256k1fx.ManagedAssetStatusOutput{}),
		c.RegisterType(&secp256k1fx.UpdateManagedAssetOperation{}),
		m.RegisterCodec(apricotCodecVersion, c),
	)
	if errs.Errored() {
		panic(errs.Err)
	}
	return c, m
}

func TestTxNil(t *testing.T) {
	ctx := NewContext(t)
	c := linearcodec.NewDefault()
	m := codec.NewDefaultManager()
	if err := m.RegisterCodec(apricotCodecVersion, c); err != nil {
		t.Fatal(err)
	}

	tx := (*Tx)(nil)
	if err := tx.SyntacticVerify(ctx, m, ids.Empty, 0, 0, 1, 0); err == nil {
		t.Fatalf("Should have errored due to nil tx")
	}
	if err := tx.SemanticVerify(nil, nil, 0); err == nil {
		t.Fatalf("Should have errored due to nil tx")
	}
}

func TestTxEmpty(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()
	tx := &Tx{}
	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1, 0); err == nil {
		t.Fatalf("Should have errored due to nil tx")
	}
}

func TestTxInvalidCredential(t *testing.T) {
	ctx := NewContext(t)
	c, m := setupCodec()
	if err := c.RegisterType(&avax.TestVerifiable{}); err != nil {
		t.Fatal(err)
	}

	tx := &Tx{
		Version: apricotCodecVersion,
		UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
			Ins: []*avax.TransferableInput{{
				UTXOID: avax.UTXOID{
					TxID:        ids.Empty,
					OutputIndex: 0,
				},
				Asset: avax.Asset{ID: assetID},
				In: &secp256k1fx.TransferInput{
					Amt: 20 * units.KiloAvax,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{
							0,
						},
					},
				},
			}},
		}},
		Creds: []verify.Verifiable{&avax.TestVerifiable{Err: errors.New("")}},
	}
	if err := tx.SignSECP256K1Fx(m, nil); err != nil {
		t.Fatal(err)
	}

	if err := tx.SyntacticVerify(ctx, m, ids.Empty, 0, 0, 1, 0); err == nil {
		t.Fatalf("Tx should have failed due to an invalid credential")
	}
}

func TestTxInvalidUnsignedTx(t *testing.T) {
	ctx := NewContext(t)
	c, m := setupCodec()
	if err := c.RegisterType(&avax.TestVerifiable{}); err != nil {
		t.Fatal(err)
	}

	tx := &Tx{
		Version: apricotCodecVersion,
		UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
			Ins: []*avax.TransferableInput{
				{
					UTXOID: avax.UTXOID{
						TxID:        ids.Empty,
						OutputIndex: 0,
					},
					Asset: avax.Asset{ID: assetID},
					In: &secp256k1fx.TransferInput{
						Amt: 20 * units.KiloAvax,
						Input: secp256k1fx.Input{
							SigIndices: []uint32{
								0,
							},
						},
					},
				},
				{
					UTXOID: avax.UTXOID{
						TxID:        ids.Empty,
						OutputIndex: 0,
					},
					Asset: avax.Asset{ID: assetID},
					In: &secp256k1fx.TransferInput{
						Amt: 20 * units.KiloAvax,
						Input: secp256k1fx.Input{
							SigIndices: []uint32{
								0,
							},
						},
					},
				},
			},
		}},
		Creds: []verify.Verifiable{
			&avax.TestVerifiable{},
			&avax.TestVerifiable{},
		},
	}
	if err := tx.SignSECP256K1Fx(m, nil); err != nil {
		t.Fatal(err)
	}

	if err := tx.SyntacticVerify(ctx, m, ids.Empty, 0, 0, 1, 0); err == nil {
		t.Fatalf("Tx should have failed due to an invalid unsigned tx")
	}
}

func TestTxInvalidNumberOfCredentials(t *testing.T) {
	ctx := NewContext(t)
	c, m := setupCodec()
	if err := c.RegisterType(&avax.TestVerifiable{}); err != nil {
		t.Fatal(err)
	}

	tx := &Tx{
		Version: apricotCodecVersion,
		UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
			Ins: []*avax.TransferableInput{
				{
					UTXOID: avax.UTXOID{TxID: ids.Empty, OutputIndex: 0},
					Asset:  avax.Asset{ID: assetID},
					In: &secp256k1fx.TransferInput{
						Amt: 20 * units.KiloAvax,
						Input: secp256k1fx.Input{
							SigIndices: []uint32{
								0,
							},
						},
					},
				},
				{
					UTXOID: avax.UTXOID{TxID: ids.Empty, OutputIndex: 1},
					Asset:  avax.Asset{ID: assetID},
					In: &secp256k1fx.TransferInput{
						Amt: 20 * units.KiloAvax,
						Input: secp256k1fx.Input{
							SigIndices: []uint32{
								0,
							},
						},
					},
				},
			},
		}},
		Creds: []verify.Verifiable{&avax.TestVerifiable{}},
	}
	if err := tx.SignSECP256K1Fx(m, nil); err != nil {
		t.Fatal(err)
	}

	if err := tx.SyntacticVerify(ctx, m, ids.Empty, 0, 0, 1, 0); err == nil {
		t.Fatalf("Tx should have failed due to an invalid unsigned tx")
	}
}
