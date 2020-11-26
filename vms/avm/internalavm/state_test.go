// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package internalavm

import (
	"math"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

// Test function IDs when argument start is empty
func TestStateIDsNoStart(t *testing.T) {
	_, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	state := vm.state.state

	id0 := ids.ID{0x01, 0}
	id1 := ids.ID{0x02, 0}
	id2 := ids.ID{0x03, 0}

	if _, err := state.IDs(ids.Empty[:], []byte{}, math.MaxInt32); err != nil {
		t.Fatal(err)
	}

	expected := []ids.ID{id0, id1}
	for _, id := range expected {
		if err := state.AddID(ids.Empty[:], id); err != nil {
			t.Fatal(err)
		}
	}

	result, err := state.IDs(ids.Empty[:], []byte{}, 0)
	if err != nil {
		t.Fatal(err)
	} else if len(result) != 0 {
		t.Fatal("result should have length 0 because limit is 0")
	}

	result, err = state.IDs(ids.Empty[:], []byte{}, 1)
	if err != nil {
		t.Fatal(err)
	} else if len(result) != 1 {
		t.Fatal("result should have length 0 because limit is 1")
	}

	result, err = state.IDs(ids.Empty[:], []byte{}, math.MaxInt32)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != len(expected) {
		t.Fatalf("Returned the wrong number of ids")
	}
	ids.SortIDs(result)
	for i, resultID := range result {
		expectedID := expected[i]
		if expectedID != resultID {
			t.Fatalf("Wrong ID returned")
		}
	}

	for _, id := range expected {
		if err := state.RemoveID(ids.Empty[:], id); err != nil {
			t.Fatal(err)
		}
	}

	result, err = state.IDs(ids.Empty[:], []byte{}, math.MaxInt32)
	if err != nil {
		t.Fatal(err)
	} else if len(result) != 0 {
		t.Fatalf("Should have returned 0 IDs")
	}

	expected = []ids.ID{id1, id2}
	for _, id := range expected {
		if err := state.AddID(ids.Empty[:], id); err != nil {
			t.Fatal(err)
		}
	}

	result, err = state.IDs(ids.Empty[:], []byte{}, math.MaxInt32)
	if err != nil {
		t.Fatal(err)
	} else if len(result) != len(expected) {
		t.Fatalf("Returned the wrong number of ids")
	}

	ids.SortIDs(result)
	for i, resultID := range result {
		expectedID := expected[i]
		if expectedID != resultID {
			t.Fatalf("Wrong ID returned")
		}
	}

	state.Cache.Flush()

	result, err = state.IDs(ids.Empty[:], []byte{}, math.MaxInt32)
	if err != nil {
		t.Fatal(err)
	} else if len(result) != len(expected) {
		t.Fatalf("Returned the wrong number of ids")
	}

	ids.SortIDs(result)
	for i, resultID := range result {
		expectedID := expected[i]
		if expectedID != resultID {
			t.Fatalf("Wrong ID returned")
		}
	}

	if err := state.SetStatus(ids.Empty, choices.Accepted); err != nil {
		t.Fatal(err)
	}

	statusResult, err := state.Status(ids.Empty)
	if err != nil {
		t.Fatal(err)
	} else if statusResult != choices.Accepted {
		t.Fatalf("Should have returned the %s status", choices.Accepted)
	}

	for _, id := range expected {
		if err := state.RemoveID(ids.Empty[:], id); err != nil {
			t.Fatal(err)
		}
	}

	result, err = state.IDs(ids.Empty[:], []byte{}, math.MaxInt32)
	if err != nil {
		t.Fatal(err)
	} else if len(result) != 0 {
		t.Fatalf("Should have returned 0 IDs")
	}
}

func TestStateIDsWithStart(t *testing.T) {
	_, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	state := vm.state.state
	id0 := ids.ID{0x01, 0}
	id1 := ids.ID{0x02, 0}
	id2 := ids.ID{0x03, 0}

	// State should be empty to start
	if _, err := state.IDs(ids.Empty[:], []byte{}, math.MaxInt32); err != nil {
		t.Fatal(err)
	}

	// Put all three IDs
	if err := state.AddID(ids.Empty[:], id0); err != nil {
		t.Fatal(err)
	} else if err := state.AddID(ids.Empty[:], id1); err != nil {
		t.Fatal(err)
	} else if err := state.AddID(ids.Empty[:], id2); err != nil {
		t.Fatal(err)
	}

	if result, err := state.IDs(ids.Empty[:], []byte{}, math.MaxInt32); err != nil { // start at beginning
		t.Fatal(err)
	} else if len(result) != 3 {
		t.Fatalf("result should have all 3 IDs but has %d", len(result))
	}

	result, err := state.IDs(ids.Empty[:], id0[:], math.MaxInt32)
	switch {
	case err != nil: // start after id0
		t.Fatal(err)
	case len(result) != 2:
		t.Fatalf("result should have 2 IDs but has %d", len(result))
	case result[0] != id1 && result[1] != id1:
		t.Fatal("result should have id1")
	case result[0] != id2 && result[1] != id2:
		t.Fatal("result should have id2")
	}

	result, err = state.IDs(ids.Empty[:], id1[:], math.MaxInt32)
	switch {
	case err != nil: // start after id1
		t.Fatal(err)
	case len(result) != 1:
		t.Fatalf("result should have 1 IDs but has %d", len(result))
	case result[0] != id2:
		t.Fatal("result should be id2")
	}
}

func TestStateStatuses(t *testing.T) {
	_, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	state := vm.state.state

	id := ids.GenerateTestID()
	if _, err := state.Status(id); err == nil {
		t.Fatalf("Should have errored when reading ids")
	}

	if err := state.SetStatus(id, choices.Accepted); err != nil {
		t.Fatal(err)
	}

	status, err := state.Status(id)
	if err != nil {
		t.Fatal(err)
	}
	if status != choices.Accepted {
		t.Fatalf("Should have returned the %s status", choices.Accepted)
	}

	if err := state.AddID(id[:], id); err != nil {
		t.Fatal(err)
	}

	status, err = state.Status(id)
	if err != nil {
		t.Fatal(err)
	}
	if status != choices.Accepted {
		t.Fatalf("Should have returned the %s status", choices.Accepted)
	}

	if err := state.SetStatus(id, choices.Unknown); err != nil {
		t.Fatal(err)
	}

	if _, err := state.Status(id); err == nil {
		t.Fatalf("Should have errored when reading ids")
	}
}

func TestStateUTXOs(t *testing.T) {
	_, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	state := vm.state.state

	if err := vm.CodecRegistry().RegisterType(&avax.TestVerifiable{}); err != nil {
		t.Fatal(err)
	}

	if _, err := state.UTXO(ids.Empty); err == nil {
		t.Fatalf("Should have errored when reading utxo")
	}

	utxo := &avax.UTXO{
		UTXOID: avax.UTXOID{
			TxID:        ids.Empty,
			OutputIndex: 1,
		},
		Asset: avax.Asset{ID: ids.Empty},
		Out:   &avax.TestVerifiable{},
	}

	if err := state.SetUTXO(ids.Empty, utxo); err != nil {
		t.Fatal(err)
	}

	result, err := state.UTXO(ids.Empty)
	if err != nil {
		t.Fatal(err)
	}

	if result.OutputIndex != 1 {
		t.Fatalf("Wrong UTXO returned")
	}

	state.Cache.Flush()

	result, err = state.UTXO(ids.Empty)
	if err != nil {
		t.Fatal(err)
	}

	if result.OutputIndex != 1 {
		t.Fatalf("Wrong UTXO returned")
	}

	if err := state.SetUTXO(ids.Empty, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := state.UTXO(ids.Empty); err == nil {
		t.Fatalf("Should have errored when reading utxo")
	}

	if err := state.SetUTXO(ids.Empty, &avax.UTXO{}); err == nil {
		t.Fatalf("Should have errored packing the utxo")
	}

	if err := state.SetStatus(ids.Empty, choices.Accepted); err != nil {
		t.Fatal(err)
	}

	if _, err := state.UTXO(ids.Empty); err == nil {
		t.Fatalf("Should have errored when reading utxo")
	}

	state.Cache.Flush()

	if _, err := state.UTXO(ids.Empty); err == nil {
		t.Fatalf("Should have errored when reading utxo")
	}
}

func TestStateTXs(t *testing.T) {
	_, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	state := vm.state.state

	if err := vm.CodecRegistry().RegisterType(&avax.TestTransferable{}); err != nil {
		t.Fatal(err)
	}

	if _, err := state.Tx(ids.Empty); err == nil {
		t.Fatalf("Should have errored when reading tx")
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

	if err := state.SetTx(ids.Empty, tx); err != nil {
		t.Fatal(err)
	}

	result, err := state.Tx(ids.Empty)
	if err != nil {
		t.Fatal(err)
	}

	if result.ID() != tx.ID() {
		t.Fatalf("Wrong Tx returned")
	}

	state.Cache.Flush()

	result, err = state.Tx(ids.Empty)
	if err != nil {
		t.Fatal(err)
	}

	if result.ID() != tx.ID() {
		t.Fatalf("Wrong Tx returned")
	}

	if err := state.SetTx(ids.Empty, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := state.Tx(ids.Empty); err == nil {
		t.Fatalf("Should have errored when reading tx")
	}

	if err := state.SetStatus(ids.Empty, choices.Accepted); err != nil {
		t.Fatal(err)
	}

	if _, err := state.Tx(ids.Empty); err == nil {
		t.Fatalf("Should have errored when reading tx")
	}

	state.Cache.Flush()

	if _, err := state.Tx(ids.Empty); err == nil {
		t.Fatalf("Should have errored when reading tx")
	}
}
