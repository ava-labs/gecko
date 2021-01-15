// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/ava-labs/avalanchego/api"
	"github.com/ava-labs/avalanchego/api/keystore"
	"github.com/ava-labs/avalanchego/chains/atomic"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/utils/sampler"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testChangeAddr = ids.GenerateTestShortID()
)

// Returns:
// 1) genesis bytes of vm
// 2) the VM
// 3) The service that wraps the VM
// 4) atomic memory to use in tests
func setup(t *testing.T) ([]byte, *VM, *Service, *atomic.Memory) {
	genesisBytes, _, vm, m := GenesisVM(t)
	keystore, err := keystore.CreateTestKeystore()
	if err != nil {
		t.Fatal(err)
	}
	if err := keystore.AddUser(username, password); err != nil {
		t.Fatalf("couldn't add user: %s", err)
	}
	vm.ctx.Keystore = keystore.NewBlockchainKeyStore(chainID)
	s := &Service{vm: vm}
	return genesisBytes, vm, s, m
}

// Returns:
// 1) genesis bytes of vm
// 2) the VM
// 3) The service that wraps the VM
// 4) atomic memory to use in tests
func setupWithKeys(t *testing.T) ([]byte, *VM, *Service, *atomic.Memory) {
	genesisBytes, vm, s, m := setup(t)

	// Import the initially funded private keys
	user := userState{vm: vm}
	db, err := s.vm.ctx.Keystore.GetDatabase(username, password)
	if err != nil {
		t.Fatalf("Failed to get user database: %s", err)
	}

	addrs := []ids.ShortID{}
	for _, sk := range keys {
		if err := user.SetKey(db, sk); err != nil {
			t.Fatalf("Failed to set key for user: %s", err)
		}
		addrs = append(addrs, sk.PublicKey().Address())
	}
	if err := user.SetAddresses(db, addrs); err != nil {
		t.Fatalf("Failed to set user addresses: %s", err)
	}

	return genesisBytes, vm, s, m
}

// Sample from a set of addresses and return them raw and formatted as strings.
// The size of the sample is between 1 and len(addrs)
// If len(addrs) == 0, returns nil
func sampleAddrs(t *testing.T, vm *VM, addrs []ids.ShortID) ([]ids.ShortID, []string) {
	sampledAddrs := []ids.ShortID{}
	sampledAddrsStr := []string{}

	sampler := sampler.NewUniform()
	if err := sampler.Initialize(uint64(len(addrs))); err != nil {
		t.Fatal(err)
	}

	numAddrs := 1 + rand.Intn(len(addrs)) // #nosec G404
	indices, err := sampler.Sample(numAddrs)
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range indices {
		addr := addrs[index]
		addrStr, err := vm.FormatLocalAddress(addr)
		if err != nil {
			t.Fatal(err)
		}

		sampledAddrs = append(sampledAddrs, addr)
		sampledAddrsStr = append(sampledAddrsStr, addrStr)
	}
	return sampledAddrs, sampledAddrsStr
}

// Returns error if [numTxFees] tx fees was not deducted from the addresses in [fromAddrs]
// relative to their starting balance
func verifyTxFeeDeducted(t *testing.T, s *Service, fromAddrs []ids.ShortID, numTxFees int) error {
	totalTxFee := numTxFees * int(s.vm.txFee)
	fromAddrsStartBalance := int(startBalance) * len(fromAddrs)

	// Key: Address
	// Value: AVAX balance
	balances := map[ids.ShortID]int{}

	for _, addr := range addrs { // get balances for all addresses
		addrStr, err := s.vm.FormatLocalAddress(addr)
		if err != nil {
			t.Fatal(err)
		}
		reply := &GetBalanceReply{}
		err = s.GetBalance(nil,
			&GetBalanceArgs{
				Address: addrStr,
				AssetID: s.vm.ctx.AVAXAssetID.String(),
			},
			reply,
		)
		if err != nil {
			return fmt.Errorf("couldn't get balance of %s: %w", addr, err)
		}
		balances[addr] = int(reply.Balance)
	}

	fromAddrsTotalBalance := 0
	for _, addr := range fromAddrs {
		fromAddrsTotalBalance += balances[addr]
	}

	if fromAddrsTotalBalance != fromAddrsStartBalance-totalTxFee {
		return fmt.Errorf("expected fromAddrs to have %d balance but have %d",
			fromAddrsStartBalance-totalTxFee,
			fromAddrsTotalBalance,
		)
	}
	return nil
}

func TestServiceIssueTx(t *testing.T) {
	genesisBytes, vm, s, _ := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	txArgs := &api.FormattedTx{}
	txReply := &api.JSONTxID{}
	err := s.IssueTx(nil, txArgs, txReply)
	if err == nil {
		t.Fatal("Expected empty transaction to return an error")
	}

	tx := NewTx(t, genesisBytes, vm)
	txArgs.Tx, err = formatting.Encode(formatting.Hex, tx.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	txArgs.Encoding = formatting.Hex
	txReply = &api.JSONTxID{}
	if err := s.IssueTx(nil, txArgs, txReply); err != nil {
		t.Fatal(err)
	}
	if txReply.TxID != tx.ID() {
		t.Fatalf("Expected %q, got %q", txReply.TxID, tx.ID())
	}
}

func TestServiceGetTxStatus(t *testing.T) {
	genesisBytes, vm, s, _ := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	statusArgs := &api.JSONTxID{}
	statusReply := &GetTxStatusReply{}
	if err := s.GetTxStatus(nil, statusArgs, statusReply); err == nil {
		t.Fatal("Expected empty transaction to return an error")
	}

	tx := NewTx(t, genesisBytes, vm)
	statusArgs.TxID = tx.ID()
	statusReply = &GetTxStatusReply{}
	if err := s.GetTxStatus(nil, statusArgs, statusReply); err != nil {
		t.Fatal(err)
	}
	if expected := choices.Unknown; expected != statusReply.Status {
		t.Fatalf(
			"Expected an unsubmitted tx to have status %q, got %q",
			expected.String(), statusReply.Status.String(),
		)
	}

	txStr, err := formatting.Encode(formatting.Hex, tx.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	txArgs := &api.FormattedTx{
		Tx:       txStr,
		Encoding: formatting.Hex,
	}
	txReply := &api.JSONTxID{}
	if err := s.IssueTx(nil, txArgs, txReply); err != nil {
		t.Fatal(err)
	}
	statusReply = &GetTxStatusReply{}
	if err := s.GetTxStatus(nil, statusArgs, statusReply); err != nil {
		t.Fatal(err)
	}
	if expected := choices.Processing; expected != statusReply.Status {
		t.Fatalf(
			"Expected a submitted tx to have status %q, got %q",
			expected.String(), statusReply.Status.String(),
		)
	}
}

func TestServiceGetBalance(t *testing.T) {
	genesisBytes, vm, s, _ := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)
	assetID := genesisTx.ID()
	addr := keys[0].PublicKey().Address()
	addrStr, err := vm.FormatLocalAddress(addr)
	if err != nil {
		t.Fatal(err)
	}

	balanceArgs := &GetBalanceArgs{
		Address: addrStr,
		AssetID: assetID.String(),
	}
	balanceReply := &GetBalanceReply{}
	err = s.GetBalance(nil, balanceArgs, balanceReply)
	assert.NoError(t, err)
	assert.Equal(t, uint64(balanceReply.Balance), startBalance)
	assert.Len(t, balanceReply.UTXOIDs, 1, "should have only returned 1 utxoID")
}

func TestServiceGetAllBalances(t *testing.T) {
	genesisBytes, vm, s, _ := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)
	assetID := genesisTx.ID()
	addr := keys[0].PublicKey().Address()
	addrStr, err := vm.FormatLocalAddress(addr)
	if err != nil {
		t.Fatal(err)
	}

	balanceArgs := &api.JSONAddress{
		Address: addrStr,
	}
	balanceReply := &GetAllBalancesReply{}
	err = s.GetAllBalances(nil, balanceArgs, balanceReply)
	assert.NoError(t, err)

	assert.Len(t, balanceReply.Balances, 1)

	balance := balanceReply.Balances[0]
	alias, err := vm.PrimaryAlias(assetID)
	if err != nil {
		t.Fatalf("Failed to get primary alias of genesis asset: %s", err)
	}
	assert.Equal(t, balance.AssetID, alias)
	assert.Equal(t, uint64(balance.Balance), startBalance)
}

func TestServiceGetTx(t *testing.T) {
	genesisBytes, vm, s, _ := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)
	genesisTxBytes := genesisTx.Bytes()
	txID := genesisTx.ID()

	reply := api.FormattedTx{}
	err := s.GetTx(nil, &api.GetTxArgs{
		TxID: txID,
	}, &reply)
	assert.NoError(t, err)
	if err != nil {
		t.Fatal(err)
	}
	txBytes, err := formatting.Decode(reply.Encoding, reply.Tx)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, genesisTxBytes, txBytes, "Wrong tx returned from service.GetTx")
}

func TestServiceGetNilTx(t *testing.T) {
	_, vm, s, _ := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	reply := api.FormattedTx{}
	err := s.GetTx(nil, &api.GetTxArgs{}, &reply)
	assert.Error(t, err, "Nil TxID should have returned an error")
}

func TestServiceGetUnknownTx(t *testing.T) {
	_, vm, s, _ := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	reply := api.FormattedTx{}
	err := s.GetTx(nil, &api.GetTxArgs{TxID: ids.Empty}, &reply)
	assert.Error(t, err, "Unknown TxID should have returned an error")
}

func TestServiceGetUTXOs(t *testing.T) {
	_, vm, s, m := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	rawAddr := ids.GenerateTestShortID()
	rawEmptyAddr := ids.GenerateTestShortID()

	numUTXOs := 10
	// Put a bunch of UTXOs
	for i := 0; i < numUTXOs; i++ {
		if err := vm.state.FundUTXO(&avax.UTXO{
			UTXOID: avax.UTXOID{
				TxID: ids.GenerateTestID(),
			},
			Asset: avax.Asset{ID: vm.ctx.AVAXAssetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 1,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{rawAddr},
				},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	sm := m.NewSharedMemory(platformChainID)

	elems := make([]*atomic.Element, numUTXOs)
	for i := range elems {
		utxo := &avax.UTXO{
			UTXOID: avax.UTXOID{
				TxID: ids.GenerateTestID(),
			},
			Asset: avax.Asset{ID: vm.ctx.AVAXAssetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 1,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{rawAddr},
				},
			},
		}

		utxoBytes, err := vm.codec.Marshal(apricotCodecVersion, utxo)
		if err != nil {
			t.Fatal(err)
		}
		utxoID := utxo.InputID()
		elems[i] = &atomic.Element{
			Key:   utxoID[:],
			Value: utxoBytes,
			Traits: [][]byte{
				rawAddr.Bytes(),
			},
		}
	}

	if err := sm.Put(vm.ctx.ChainID, elems); err != nil {
		t.Fatal(err)
	}

	hrp := constants.GetHRP(vm.ctx.NetworkID)
	xAddr, err := vm.FormatLocalAddress(rawAddr)
	if err != nil {
		t.Fatal(err)
	}
	pAddr, err := vm.FormatAddress(platformChainID, rawAddr)
	if err != nil {
		t.Fatal(err)
	}
	unknownChainAddr, err := formatting.FormatAddress("R", hrp, rawAddr.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	xEmptyAddr, err := vm.FormatLocalAddress(rawEmptyAddr)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		label     string
		count     int
		shouldErr bool
		args      *api.GetUTXOsArgs
	}{
		{
			label:     "invalid address: ''",
			shouldErr: true,
			args: &api.GetUTXOsArgs{
				Addresses: []string{""},
			},
		},
		{
			label:     "invalid address: '-'",
			shouldErr: true,
			args: &api.GetUTXOsArgs{
				Addresses: []string{"-"},
			},
		},
		{
			label:     "invalid address: 'foo'",
			shouldErr: true,
			args: &api.GetUTXOsArgs{
				Addresses: []string{"foo"},
			},
		},
		{
			label:     "invalid address: 'foo-bar'",
			shouldErr: true,
			args: &api.GetUTXOsArgs{
				Addresses: []string{"foo-bar"},
			},
		},
		{
			label:     "invalid address: '<ChainID>'",
			shouldErr: true,
			args: &api.GetUTXOsArgs{
				Addresses: []string{vm.ctx.ChainID.String()},
			},
		},
		{
			label:     "invalid address: '<ChainID>-'",
			shouldErr: true,
			args: &api.GetUTXOsArgs{
				Addresses: []string{fmt.Sprintf("%s-", vm.ctx.ChainID.String())},
			},
		},
		{
			label:     "invalid address: '<Unknown ID>-<addr>'",
			shouldErr: true,
			args: &api.GetUTXOsArgs{
				Addresses: []string{unknownChainAddr},
			},
		},
		{
			label:     "no addresses",
			shouldErr: true,
			args:      &api.GetUTXOsArgs{},
		},
		{
			label: "get all X-chain UTXOs",
			count: numUTXOs,
			args: &api.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
				},
			},
		},
		{
			label: "get one X-chain UTXO",
			count: 1,
			args: &api.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
				},
				Limit: 1,
			},
		},
		{
			label: "limit greater than number of UTXOs",
			count: numUTXOs,
			args: &api.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
				},
				Limit: json.Uint32(numUTXOs + 1),
			},
		},
		{
			label: "no utxos to return",
			count: 0,
			args: &api.GetUTXOsArgs{
				Addresses: []string{
					xEmptyAddr,
				},
			},
		},
		{
			label: "multiple address with utxos",
			count: numUTXOs,
			args: &api.GetUTXOsArgs{
				Addresses: []string{
					xEmptyAddr,
					xAddr,
				},
			},
		},
		{
			label: "get all P-chain UTXOs",
			count: numUTXOs,
			args: &api.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
				},
				SourceChain: "P",
			},
		},
		{
			label:     "invalid source chain ID",
			shouldErr: true,
			count:     numUTXOs,
			args: &api.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
				},
				SourceChain: "HomeRunDerby",
			},
		},
		{
			label: "get all P-chain UTXOs",
			count: numUTXOs,
			args: &api.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
				},
				SourceChain: "P",
			},
		},
		{
			label:     "get UTXOs from multiple chains",
			shouldErr: true,
			args: &api.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
					pAddr,
				},
			},
		},
		{
			label:     "get UTXOs for an address on a different chain",
			shouldErr: true,
			args: &api.GetUTXOsArgs{
				Addresses: []string{
					pAddr,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			reply := &api.GetUTXOsReply{}
			err := s.GetUTXOs(nil, test.args, reply)
			if err != nil {
				if !test.shouldErr {
					t.Fatal(err)
				}
				return
			}
			if test.shouldErr {
				t.Fatal("should have errored")
			}
			if test.count != len(reply.UTXOs) {
				t.Fatalf("Expected %d utxos, got %d", test.count, len(reply.UTXOs))
			}
		})
	}
}

func TestGetAssetDescription(t *testing.T) {
	genesisBytes, vm, s, _ := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	avaxAssetID := genesisTx.ID()

	reply := GetAssetDescriptionReply{}
	err := s.GetAssetDescription(nil, &GetAssetDescriptionArgs{
		AssetID: avaxAssetID.String(),
	}, &reply)
	if err != nil {
		t.Fatal(err)
	}

	if reply.Name != "AVAX" {
		t.Fatalf("Wrong name returned from GetAssetDescription %s", reply.Name)
	}
	if reply.Symbol != "SYMB" {
		t.Fatalf("Wrong name returned from GetAssetDescription %s", reply.Symbol)
	}
}

func TestGetBalance(t *testing.T) {
	genesisBytes, vm, s, _ := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	avaxAssetID := genesisTx.ID()

	reply := GetBalanceReply{}
	addrStr, err := vm.FormatLocalAddress(keys[0].PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}
	err = s.GetBalance(nil, &GetBalanceArgs{
		Address: addrStr,
		AssetID: avaxAssetID.String(),
	}, &reply)
	if err != nil {
		t.Fatal(err)
	}

	if uint64(reply.Balance) != startBalance {
		t.Fatalf("Wrong balance returned from GetBalance %d", reply.Balance)
	}
}

func TestCreateFixedCapAsset(t *testing.T) {
	_, vm, s, _ := setupWithKeys(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	reply := AssetIDChangeAddr{}
	addrStr, err := vm.FormatLocalAddress(keys[0].PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}

	changeAddrStr, err := vm.FormatLocalAddress(testChangeAddr)
	if err != nil {
		t.Fatal(err)
	}
	_, fromAddrsStr := sampleAddrs(t, vm, addrs)

	err = s.CreateFixedCapAsset(nil, &CreateAssetArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass: api.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  api.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		Name:         "testAsset",
		Symbol:       "TEST",
		Denomination: 1,
		InitialHolders: []*Holder{{
			Amount:  123456789,
			Address: addrStr,
		}},
	}, &reply)
	if err != nil {
		t.Fatal(err)
	} else if reply.ChangeAddr != changeAddrStr {
		t.Fatalf("expected change address %s but got %s", changeAddrStr, reply.ChangeAddr)
	}
}

func TestCreateVariableCapAsset(t *testing.T) {
	_, vm, s, _ := setupWithKeys(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	reply := AssetIDChangeAddr{}
	minterAddrStr, err := vm.FormatLocalAddress(keys[0].PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}
	_, fromAddrsStr := sampleAddrs(t, vm, addrs)
	changeAddrStr := fromAddrsStr[0]
	if err != nil {
		t.Fatal(err)
	}

	err = s.CreateVariableCapAsset(nil, &CreateAssetArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass: api.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  api.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		Name:   "test asset",
		Symbol: "TEST",
		MinterSets: []Minters{
			{
				Threshold: 1,
				Minters: []string{
					minterAddrStr,
				},
			},
		},
	}, &reply)
	if err != nil {
		t.Fatal(err)
	} else if reply.ChangeAddr != changeAddrStr {
		t.Fatalf("expected change address %s but got %s", changeAddrStr, reply.ChangeAddr)
	}

	createAssetTx := newUniqueTx(vm, reply.AssetID, nil)
	if status := createAssetTx.Status(); status != choices.Processing {
		t.Fatalf("CreateVariableCapAssetTx status should have been Processing, but was %s", status)
	}
	if err := createAssetTx.Accept(0); err != nil {
		t.Fatalf("Failed to accept CreateVariableCapAssetTx due to: %s", err)
	}

	createdAssetID := reply.AssetID.String()
	// Test minting of the created variable cap asset
	mintArgs := &MintArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass: api.UserPass{
				Username: username,
				Password: password,
			},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		Amount:  200,
		AssetID: createdAssetID,
		To:      minterAddrStr, // Send newly minted tokens to this address
	}
	mintReply := &api.JSONTxIDChangeAddr{}
	if err := s.Mint(nil, mintArgs, mintReply); err != nil {
		t.Fatalf("Failed to mint variable cap asset due to: %s", err)
	} else if mintReply.ChangeAddr != changeAddrStr {
		t.Fatalf("expected change address %s but got %s", changeAddrStr, mintReply.ChangeAddr)
	}

	mintTx := newUniqueTx(vm, mintReply.TxID, nil)
	if status := mintTx.Status(); status != choices.Processing {
		t.Fatalf("MintTx status should have been Processing, but was %s", status)
	}
	if err := mintTx.Accept(0); err != nil {
		t.Fatalf("Failed to accept MintTx due to: %s", err)
	}

	sendArgs := &SendArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass: api.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  api.JSONFromAddrs{From: []string{minterAddrStr}},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		SendOutput: SendOutput{
			Amount:  200,
			AssetID: createdAssetID,
			To:      fromAddrsStr[0],
		},
	}
	sendReply := &api.JSONTxIDChangeAddr{}
	if err := s.Send(nil, sendArgs, sendReply); err != nil {
		t.Fatalf("Failed to send newly minted variable cap asset due to: %s", err)
	} else if sendReply.ChangeAddr != changeAddrStr {
		t.Fatalf("expected change address to be %s but got %s", changeAddrStr, sendReply.ChangeAddr)
	}
}

func TestNFTWorkflow(t *testing.T) {
	_, vm, s, _ := setupWithKeys(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	fromAddrs, fromAddrsStr := sampleAddrs(t, vm, addrs)

	// Test minting of the created variable cap asset
	addrStr, err := vm.FormatLocalAddress(keys[0].PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}

	createArgs := &CreateNFTAssetArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass: api.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  api.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: fromAddrsStr[0]},
		},
		Name:   "BIG COIN",
		Symbol: "COIN",
		MinterSets: []Minters{
			{
				Threshold: 1,
				Minters: []string{
					addrStr,
				},
			},
		},
	}
	createReply := &AssetIDChangeAddr{}
	if err := s.CreateNFTAsset(nil, createArgs, createReply); err != nil {
		t.Fatalf("Failed to mint variable cap asset due to: %s", err)
	} else if createReply.ChangeAddr != fromAddrsStr[0] {
		t.Fatalf("expected change address to be %s but got %s", fromAddrsStr[0], createReply.ChangeAddr)
	}

	assetID := createReply.AssetID
	createNFTTx := newUniqueTx(vm, assetID, nil)
	// Accept the transaction so that we can Mint NFTs for the test
	if createNFTTx.Status() != choices.Processing {
		t.Fatalf("CreateNFTTx should have been processing after creating the NFT")
	}
	if err := createNFTTx.Accept(0); err != nil {
		t.Fatalf("Failed to accept CreateNFT transaction: %s", err)
	} else if err := verifyTxFeeDeducted(t, s, fromAddrs, 1); err != nil {
		t.Fatal(err)
	}

	payload, err := formatting.Encode(formatting.Hex, []byte{1, 2, 3, 4, 5})
	if err != nil {
		t.Fatal(err)
	}
	mintArgs := &MintNFTArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass: api.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  api.JSONFromAddrs{},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: fromAddrsStr[0]},
		},
		AssetID:  assetID.String(),
		Payload:  payload,
		To:       addrStr,
		Encoding: formatting.Hex,
	}
	mintReply := &api.JSONTxIDChangeAddr{}

	if err := s.MintNFT(nil, mintArgs, mintReply); err != nil {
		t.Fatalf("MintNFT returned an error: %s", err)
	} else if createReply.ChangeAddr != fromAddrsStr[0] {
		t.Fatalf("expected change address to be %s but got %s", fromAddrsStr[0], mintReply.ChangeAddr)
	}

	mintNFTTx := newUniqueTx(vm, mintReply.TxID, nil)
	if mintNFTTx.Status() != choices.Processing {
		t.Fatal("MintNFTTx should have been processing after minting the NFT")
	}

	// Accept the transaction so that we can send the newly minted NFT
	if err := mintNFTTx.Accept(0); err != nil {
		t.Fatalf("Failed to accept MintNFTTx: %s", err)
	}

	sendArgs := &SendNFTArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass: api.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  api.JSONFromAddrs{},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: fromAddrsStr[0]},
		},
		AssetID: assetID.String(),
		GroupID: 0,
		To:      addrStr,
	}
	sendReply := &api.JSONTxIDChangeAddr{}
	if err := s.SendNFT(nil, sendArgs, sendReply); err != nil {
		t.Fatalf("Failed to send NFT due to: %s", err)
	} else if sendReply.ChangeAddr != fromAddrsStr[0] {
		t.Fatalf("expected change address to be %s but got %s", fromAddrsStr[0], sendReply.ChangeAddr)
	}
}

func TestImportExportKey(t *testing.T) {
	_, vm, s, _ := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	factory := crypto.FactorySECP256K1R{}
	skIntf, err := factory.NewPrivateKey()
	if err != nil {
		t.Fatalf("problem generating private key: %s", err)
	}
	sk := skIntf.(*crypto.PrivateKeySECP256K1R)

	privKeyStr, err := formatting.Encode(formatting.CB58, sk.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	importArgs := &ImportKeyArgs{
		UserPass: api.UserPass{
			Username: username,
			Password: password,
		},
		PrivateKey: constants.SecretKeyPrefix + privKeyStr,
	}
	importReply := &api.JSONAddress{}
	if err = s.ImportKey(nil, importArgs, importReply); err != nil {
		t.Fatal(err)
	}

	addrStr, err := vm.FormatLocalAddress(sk.PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}
	exportArgs := &ExportKeyArgs{
		UserPass: api.UserPass{
			Username: username,
			Password: password,
		},
		Address: addrStr,
	}
	exportReply := &ExportKeyReply{}
	if err = s.ExportKey(nil, exportArgs, exportReply); err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(exportReply.PrivateKey, constants.SecretKeyPrefix) {
		t.Fatalf("ExportKeyReply private key: %s mssing secret key prefix: %s", exportReply.PrivateKey, constants.SecretKeyPrefix)
	}

	parsedKeyBytes, err := formatting.Decode(formatting.CB58, strings.TrimPrefix(exportReply.PrivateKey, constants.SecretKeyPrefix))
	if err != nil {
		t.Fatal("Failed to parse exported private key")
	}
	if !bytes.Equal(sk.Bytes(), parsedKeyBytes) {
		t.Fatal("Unexpected key was found in ExportKeyReply")
	}
}

func TestImportAVMKeyNoDuplicates(t *testing.T) {
	_, vm, s, _ := setup(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	factory := crypto.FactorySECP256K1R{}
	skIntf, err := factory.NewPrivateKey()
	if err != nil {
		t.Fatalf("problem generating private key: %s", err)
	}
	sk := skIntf.(*crypto.PrivateKeySECP256K1R)
	privKeyStr, err := formatting.Encode(formatting.CB58, sk.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	args := ImportKeyArgs{
		UserPass: api.UserPass{
			Username: username,
			Password: password,
		},
		PrivateKey: constants.SecretKeyPrefix + privKeyStr,
	}
	reply := api.JSONAddress{}
	if err = s.ImportKey(nil, &args, &reply); err != nil {
		t.Fatal(err)
	}

	expectedAddress, err := vm.FormatLocalAddress(sk.PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}

	if reply.Address != expectedAddress {
		t.Fatalf("Reply address: %s did not match expected address: %s", reply.Address, expectedAddress)
	}

	reply2 := api.JSONAddress{}
	if err = s.ImportKey(nil, &args, &reply2); err != nil {
		t.Fatal(err)
	}

	if reply2.Address != expectedAddress {
		t.Fatalf("Reply address: %s did not match expected address: %s", reply2.Address, expectedAddress)
	}

	addrsArgs := api.UserPass{
		Username: username,
		Password: password,
	}
	addrsReply := api.JSONAddresses{}
	if err := s.ListAddresses(nil, &addrsArgs, &addrsReply); err != nil {
		t.Fatal(err)
	}

	if len(addrsReply.Addresses) != 1 {
		t.Fatal("Importing the same key twice created duplicate addresses")
	}

	if addrsReply.Addresses[0] != expectedAddress {
		t.Fatal("List addresses returned an incorrect address")
	}
}

func TestSend(t *testing.T) {
	genesisBytes, vm, s, _ := setupWithKeys(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)
	assetID := genesisTx.ID()
	addr := keys[0].PublicKey().Address()

	addrStr, err := vm.FormatLocalAddress(addr)
	if err != nil {
		t.Fatal(err)
	}
	changeAddrStr, err := vm.FormatLocalAddress(testChangeAddr)
	if err != nil {
		t.Fatal(err)
	}
	_, fromAddrsStr := sampleAddrs(t, vm, addrs)

	args := &SendArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass: api.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  api.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		SendOutput: SendOutput{
			Amount:  500,
			AssetID: assetID.String(),
			To:      addrStr,
		},
	}
	reply := &api.JSONTxIDChangeAddr{}
	vm.timer.Cancel()
	if err := s.Send(nil, args, reply); err != nil {
		t.Fatalf("Failed to send transaction: %s", err)
	} else if reply.ChangeAddr != changeAddrStr {
		t.Fatalf("expected change address to be %s but got %s", changeAddrStr, reply.ChangeAddr)
	}

	pendingTxs := vm.txs
	if len(pendingTxs) != 1 {
		t.Fatalf("Expected to find 1 pending tx after send, but found %d", len(pendingTxs))
	}

	if reply.TxID != pendingTxs[0].ID() {
		t.Fatal("Transaction ID returned by Send does not match the transaction found in vm's pending transactions")
	}
}

func TestSendMultiple(t *testing.T) {
	genesisBytes, vm, s, _ := setupWithKeys(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)
	assetID := genesisTx.ID()
	addr := keys[0].PublicKey().Address()

	addrStr, err := vm.FormatLocalAddress(addr)
	if err != nil {
		t.Fatal(err)
	}
	changeAddrStr, err := vm.FormatLocalAddress(testChangeAddr)
	if err != nil {
		t.Fatal(err)
	}
	_, fromAddrsStr := sampleAddrs(t, vm, addrs)

	args := &SendMultipleArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass: api.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  api.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		Outputs: []SendOutput{
			{
				Amount:  500,
				AssetID: assetID.String(),
				To:      addrStr,
			},
			{
				Amount:  1000,
				AssetID: assetID.String(),
				To:      addrStr,
			},
		},
	}
	reply := &api.JSONTxIDChangeAddr{}
	vm.timer.Cancel()
	if err := s.SendMultiple(nil, args, reply); err != nil {
		t.Fatalf("Failed to send transaction: %s", err)
	} else if reply.ChangeAddr != changeAddrStr {
		t.Fatalf("expected change address to be %s but got %s", changeAddrStr, reply.ChangeAddr)
	}

	pendingTxs := vm.txs
	if len(pendingTxs) != 1 {
		t.Fatalf("Expected to find 1 pending tx after send, but found %d", len(pendingTxs))
	}

	if reply.TxID != pendingTxs[0].ID() {
		t.Fatal("Transaction ID returned by SendMultiple does not match the transaction found in vm's pending transactions")
	}

	if _, err = vm.Get(reply.TxID); err != nil {
		t.Fatalf("Failed to retrieve created transaction: %s", err)
	}
}

func TestCreateAndListAddresses(t *testing.T) {
	_, vm, s, _ := setup(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	createArgs := &api.UserPass{
		Username: username,
		Password: password,
	}
	createReply := &api.JSONAddress{}

	if err := s.CreateAddress(nil, createArgs, createReply); err != nil {
		t.Fatalf("Failed to create address: %s", err)
	}

	newAddr := createReply.Address

	listArgs := &api.UserPass{
		Username: username,
		Password: password,
	}
	listReply := &api.JSONAddresses{}

	if err := s.ListAddresses(nil, listArgs, listReply); err != nil {
		t.Fatalf("Failed to list addresses: %s", err)
	}

	for _, addr := range listReply.Addresses {
		if addr == newAddr {
			return
		}
	}
	t.Fatalf("Failed to find newly created address among %d addresses", len(listReply.Addresses))
}

func TestImportAVAX(t *testing.T) {
	genesisBytes, vm, s, m := setupWithKeys(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()
	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)
	assetID := genesisTx.ID()
	addr0 := keys[0].PublicKey().Address()

	// Must set AVAX assetID to be the correct asset since only AVAX can be imported
	utxo := &avax.UTXO{
		UTXOID: avax.UTXOID{TxID: ids.Empty},
		Asset:  avax.Asset{ID: assetID},
		Out: &secp256k1fx.TransferOutput{
			Amt: 7,
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{addr0},
			},
		},
	}
	utxoBytes, err := vm.codec.Marshal(apricotCodecVersion, utxo)
	if err != nil {
		t.Fatal(err)
	}

	peerSharedMemory := m.NewSharedMemory(platformChainID)
	utxoID := utxo.InputID()
	if err := peerSharedMemory.Put(vm.ctx.ChainID, []*atomic.Element{{
		Key:   utxoID[:],
		Value: utxoBytes,
		Traits: [][]byte{
			addr0.Bytes(),
		},
	}}); err != nil {
		t.Fatal(err)
	}

	addrStr, err := vm.FormatLocalAddress(keys[0].PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}
	args := &ImportArgs{
		UserPass: api.UserPass{
			Username: username,
			Password: password,
		},
		SourceChain: "P",
		To:          addrStr,
	}
	reply := &api.JSONTxID{}
	if err := s.ImportAVAX(nil, args, reply); err != nil {
		t.Fatalf("Failed to import AVAX due to %s", err)
	}
}

func TestManagedAssetAPI(t *testing.T) {
	_, vm, s, _ := setupWithKeys(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	reply := AssetIDChangeAddr{}
	addrStr, err := vm.FormatLocalAddress(keys[0].PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}
	minterAddrStr, err := vm.FormatLocalAddress(keys[1].PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}

	_, fromAddrsStr := sampleAddrs(t, vm, addrs)

	// Create a fixed cap managed asset
	err = s.CreateAsset(nil, &CreateAssetArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass: api.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  api.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: addrStr},
		},
		Name:         "testAsset",
		Symbol:       "TEST",
		Denomination: 1,
		InitialHolders: []*Holder{{
			Amount:  123456789,
			Address: addrStr,
		}},
		MinterSets: []Minters{
			{
				Threshold: 1,
				Minters:   []string{minterAddrStr},
			},
		},
		Manager: Manager{
			Threshold: 1,
			Addrs:     []string{addrStr},
		},
	}, &reply)
	require.NoError(t, err)

	// Hacky way of ensuring the tx was issued and then accepting it
	createAssetTx := UniqueTx{
		vm:   vm,
		txID: reply.AssetID,
	}
	require.Equal(t, choices.Processing, createAssetTx.Status())
	err = createAssetTx.Accept(0) // Accept the tx, creating the asset
	require.NoError(t, err)

	// Ensure the initial holder got the asset
	getBalanceReply := GetBalanceReply{}
	err = s.GetBalance(
		nil,
		&GetBalanceArgs{
			Address: addrStr,
			AssetID: reply.AssetID.String(),
		},
		&getBalanceReply,
	)
	require.NoError(t, err)
	require.Equal(t, uint64(123456789), uint64(getBalanceReply.Balance))

	// Send some of the asset normally (not as manager)
	sendReply := &api.JSONTxIDChangeAddr{}
	formattedToAddr, _ := s.vm.FormatLocalAddress(ids.GenerateTestShortID())
	err = s.Send(
		nil,
		&SendArgs{
			JSONSpendHeader: api.JSONSpendHeader{
				UserPass: api.UserPass{
					Username: username,
					Password: password,
				},
				JSONFromAddrs:  api.JSONFromAddrs{From: []string{addrStr}},
				JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: addrStr},
			},
			SendOutput: SendOutput{
				Amount:  1,
				AssetID: createAssetTx.ID().String(),
				To:      formattedToAddr,
			},
		},
		sendReply,
	)
	require.NoError(t, err)

	// Hacky way of ensuring the tx was issued and then accepting it
	sendTx := UniqueTx{
		vm:   vm,
		txID: sendReply.TxID,
	}
	require.Equal(t, choices.Processing, sendTx.Status())
	err = sendTx.Accept(0) // Accept the tx
	require.NoError(t, err)

	// Ensure the send worked
	getBalanceReply = GetBalanceReply{}
	err = s.GetBalance(
		nil,
		&GetBalanceArgs{
			Address: formattedToAddr,
			AssetID: reply.AssetID.String(),
		},
		&getBalanceReply,
	)
	require.NoError(t, err)
	require.Equal(t, uint64(1), uint64(getBalanceReply.Balance))

	// Send the asset as manager from the address we just sent to
	// back to the original address we minted to
	sendAsManagerResponse := &SendAsManagerResponse{}
	err = s.SendAsManager(
		nil,
		&SendAsManagerArgs{
			SendArgs: SendArgs{
				JSONSpendHeader: api.JSONSpendHeader{
					UserPass: api.UserPass{
						Username: username,
						Password: password,
					},
					JSONFromAddrs:  api.JSONFromAddrs{From: []string{formattedToAddr}},
					JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: formattedToAddr},
				},
				SendOutput: SendOutput{
					Amount:  1,
					AssetID: createAssetTx.ID().String(),
					To:      addrStr,
				},
			},
		},
		sendAsManagerResponse,
	)
	require.NoError(t, err)

	// Accept the tx
	sendAsManagerTx := UniqueTx{
		vm:   vm,
		txID: sendAsManagerResponse.TxID,
	}
	require.Equal(t, choices.Processing, sendAsManagerTx.Status())
	err = sendAsManagerTx.Accept(0) // Accept the tx
	require.NoError(t, err)

	// Ensure the balance was updated correctly
	// Ensure the initial holder got the asset
	getBalanceReply = GetBalanceReply{}
	err = s.GetBalance(
		nil,
		&GetBalanceArgs{
			Address: addrStr,
			AssetID: createAssetTx.ID().String(),
		},
		&getBalanceReply,
	)
	require.NoError(t, err)
	require.Equal(t, uint64(123456789), uint64(getBalanceReply.Balance))

	// Mint some units of the asset
	mintResponse := &api.JSONTxIDChangeAddr{}
	err = s.Mint(
		nil,
		&MintArgs{
			JSONSpendHeader: api.JSONSpendHeader{
				UserPass: api.UserPass{
					Username: username,
					Password: password,
				},
				JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: formattedToAddr},
			},
			Amount:  100,
			AssetID: createAssetTx.ID().String(),
			To:      minterAddrStr,
		},
		mintResponse,
	)
	require.NoError(t, err)

	// Accept the tx
	mintTx := UniqueTx{
		vm:   vm,
		txID: mintResponse.TxID,
	}
	require.Equal(t, choices.Processing, mintTx.Status())
	err = mintTx.Accept(0) // Accept the tx
	require.NoError(t, err)

	/*
		// Freeze the asset and change the manager to minterAddrStr
		updateResponse := &api.JSONTxIDChangeAddr{}
		err = s.UpdateManagedAsset(
			nil,
			&UpdateManagedAssetArgs{
				JSONSpendHeader: api.JSONSpendHeader{
					UserPass: api.UserPass{
						Username: username,
						Password: password,
					},
					JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: formattedToAddr},
				},
				AssetID: createAssetTx.ID().String(),
				Frozen:  true,
				Manager: Manager{
					Threshold: 1,
					Addrs:     []string{minterAddrStr},
				},
			},
			updateResponse,
		)
		require.NoError(t, err)

		// Accept the tx
		updateTx := UniqueTx{
			vm:   vm,
			txID: updateResponse.TxID,
		}
		require.Equal(t, choices.Processing, updateTx.Status())
		err = updateTx.Accept(0) // Accept the tx
		require.NoError(t, err)
	*/

}
