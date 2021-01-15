// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"math"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/stretchr/testify/require"
)

func TestPrefixedSetsAndGets(t *testing.T) {
	_, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	state := vm.state
	if err := vm.CodecRegistry().RegisterType(&avax.TestVerifiable{}); err != nil {
		t.Fatal(err)
	}

	utxo := &avax.UTXO{
		UTXOID: avax.UTXOID{
			TxID:        ids.Empty,
			OutputIndex: 1,
		},
		Asset: avax.Asset{ID: ids.Empty},
		Out:   &avax.TestVerifiable{},
	}

	tx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
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
	}}}
	if err := tx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	if err := state.SetUTXO(ids.Empty, utxo); err != nil {
		t.Fatal(err)
	}
	if err := state.SetTx(ids.Empty, tx); err != nil {
		t.Fatal(err)
	}
	if err := state.SetStatus(ids.Empty, choices.Accepted); err != nil {
		t.Fatal(err)
	}

	resultUTXO, err := state.UTXO(ids.Empty)
	if err != nil {
		t.Fatal(err)
	}
	resultTx, err := state.Tx(ids.Empty)
	if err != nil {
		t.Fatal(err)
	}
	resultStatus, err := state.Status(ids.Empty)
	if err != nil {
		t.Fatal(err)
	}

	if resultUTXO.OutputIndex != 1 {
		t.Fatalf("Wrong UTXO returned")
	}
	if resultTx.ID() != tx.ID() {
		t.Fatalf("Wrong Tx returned")
	}
	if resultStatus != choices.Accepted {
		t.Fatalf("Wrong Status returned")
	}
}

func TestPrefixedFundingNoAddresses(t *testing.T) {
	_, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	state := vm.state
	if err := vm.CodecRegistry().RegisterType(&avax.TestVerifiable{}); err != nil {
		t.Fatal(err)
	}

	utxo := &avax.UTXO{
		UTXOID: avax.UTXOID{
			TxID:        ids.Empty,
			OutputIndex: 1,
		},
		Asset: avax.Asset{ID: ids.Empty},
		Out:   &avax.TestVerifiable{},
	}

	if err := state.FundUTXO(utxo); err != nil {
		t.Fatal(err)
	}
	if err := state.SpendUTXO(utxo.InputID()); err != nil {
		t.Fatal(err)
	}
}

func TestPrefixedFundingAddresses(t *testing.T) {
	_, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	state := vm.state
	if err := vm.CodecRegistry().RegisterType(&avax.TestAddressable{}); err != nil {
		t.Fatal(err)
	}

	utxo := &avax.UTXO{
		UTXOID: avax.UTXOID{
			TxID:        ids.Empty,
			OutputIndex: 1,
		},
		Asset: avax.Asset{ID: ids.Empty},
		Out: &avax.TestAddressable{
			Addrs: [][]byte{{0}},
		},
	}

	if err := state.FundUTXO(utxo); err != nil {
		t.Fatal(err)
	}
	funds, err := state.Funds([]byte{0}, ids.Empty, math.MaxInt32)
	if err != nil {
		t.Fatal(err)
	}
	if len(funds) != 1 {
		t.Fatalf("Should have returned 1 utxoIDs")
	}
	if utxoID := funds[0]; utxoID != utxo.InputID() {
		t.Fatalf("Returned wrong utxoID")
	}
	if err := state.SpendUTXO(utxo.InputID()); err != nil {
		t.Fatal(err)
	}
	funds, err = state.Funds([]byte{0}, ids.Empty, math.MaxInt32)
	if err != nil {
		t.Fatal(err)
	}
	if len(funds) != 0 {
		t.Fatalf("Should have returned 0 utxoIDs")
	}
}

func TestPrefixedStateManagedAssetStatus(t *testing.T) {
	// Setup
	_, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()
	state := vm.state

	// Ensure non-existent asset has no status
	_, _, _, err := state.ManagedAssetStatus(ids.GenerateTestID())
	require.Error(t, err)

	// Put a status
	testAssetID := ids.GenerateTestID()
	testEpoch := uint32(1)
	testStatus := &secp256k1fx.ManagedAssetStatusOutput{
		IsFrozen: true,
		Mgr: secp256k1fx.OutputOwners{
			Threshold: 1,
			Locktime:  2,
			Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
		},
	}
	err = state.PutManagedAssetStatus(testAssetID, testEpoch, testStatus)
	require.NoError(t, err)

	// Get the status
	epoch, status, oldStatus, err := state.ManagedAssetStatus(testAssetID)
	require.NoError(t, err)
	require.Equal(t, testEpoch, epoch)
	require.Equal(t, testStatus, status)
	require.Equal(t, testStatus, oldStatus)

	// Put a new status
	testEpoch2 := uint32(2)
	testStatus2 := &secp256k1fx.ManagedAssetStatusOutput{
		IsFrozen: true,
		Mgr: secp256k1fx.OutputOwners{
			Threshold: 2,
			Locktime:  3,
			Addrs:     []ids.ShortID{ids.GenerateTestShortID(), ids.GenerateTestShortID()},
		},
	}
	err = state.PutManagedAssetStatus(testAssetID, testEpoch2, testStatus2)
	require.NoError(t, err)

	// Get the statuses
	epoch, status, oldStatus, err = state.ManagedAssetStatus(testAssetID)
	require.NoError(t, err)
	require.Equal(t, testEpoch2, epoch)
	require.Equal(t, testStatus2, status)
	require.Equal(t, testStatus, oldStatus)

	// Put a new status
	testEpoch3 := uint32(3)
	testStatus3 := &secp256k1fx.ManagedAssetStatusOutput{
		IsFrozen: true,
		Mgr: secp256k1fx.OutputOwners{
			Threshold: 3,
			Locktime:  4,
			Addrs:     []ids.ShortID{ids.GenerateTestShortID(), ids.GenerateTestShortID(), ids.GenerateTestShortID()},
		},
	}
	err = state.PutManagedAssetStatus(testAssetID, testEpoch3, testStatus3)
	require.NoError(t, err)

	// Get the statuses
	epoch, status, oldStatus, err = state.ManagedAssetStatus(testAssetID)
	require.NoError(t, err)
	require.Equal(t, testEpoch3, epoch)
	require.Equal(t, testStatus3, status)
	require.Equal(t, testStatus2, oldStatus)
}
