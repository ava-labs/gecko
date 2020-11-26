// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package internalavm

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/ava-labs/avalanchego/vms/avm/vmargs"

	"github.com/ava-labs/avalanchego/api/apiargs"

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
	balances := map[[20]byte]int{}

	for _, addr := range addrs { // get balances for all addresses
		addrStr, err := s.vm.FormatLocalAddress(addr)
		if err != nil {
			t.Fatal(err)
		}
		reply := &vmargs.GetBalanceReply{}
		err = s.GetBalance(nil,
			&vmargs.GetBalanceArgs{
				Address: addrStr,
				AssetID: s.vm.ctx.AVAXAssetID.String(),
			},
			reply,
		)
		if err != nil {
			return fmt.Errorf("couldn't get balance of %s: %w", addr, err)
		}
		balances[addr.Key()] = int(reply.Balance)
	}

	fromAddrsTotalBalance := 0
	for _, addr := range fromAddrs {
		fromAddrsTotalBalance += balances[addr.Key()]
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

	txArgs := &apiargs.FormattedTx{}
	txReply := &apiargs.JSONTxID{}
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
	txReply = &apiargs.JSONTxID{}
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

	statusArgs := &apiargs.JSONTxID{}
	statusReply := &vmargs.GetTxStatusReply{}
	if err := s.GetTxStatus(nil, statusArgs, statusReply); err == nil {
		t.Fatal("Expected empty transaction to return an error")
	}

	tx := NewTx(t, genesisBytes, vm)
	statusArgs.TxID = tx.ID()
	statusReply = &vmargs.GetTxStatusReply{}
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
	txArgs := &apiargs.FormattedTx{
		Tx:       txStr,
		Encoding: formatting.Hex,
	}
	txReply := &apiargs.JSONTxID{}
	if err := s.IssueTx(nil, txArgs, txReply); err != nil {
		t.Fatal(err)
	}
	statusReply = &vmargs.GetTxStatusReply{}
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

	balanceArgs := &vmargs.GetBalanceArgs{
		Address: addrStr,
		AssetID: assetID.String(),
	}
	balanceReply := &vmargs.GetBalanceReply{}
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

	balanceArgs := &apiargs.JSONAddress{
		Address: addrStr,
	}
	balanceReply := &vmargs.GetAllBalancesReply{}
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

	reply := apiargs.FormattedTx{}
	err := s.GetTx(nil, &apiargs.GetTxArgs{
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

	reply := apiargs.FormattedTx{}
	err := s.GetTx(nil, &apiargs.GetTxArgs{}, &reply)
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

	reply := apiargs.FormattedTx{}
	err := s.GetTx(nil, &apiargs.GetTxArgs{TxID: ids.Empty}, &reply)
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

		utxoBytes, err := vm.codec.Marshal(codecVersion, utxo)
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
		args      *vmargs.GetUTXOsArgs
	}{
		{
			label:     "invalid address: ''",
			shouldErr: true,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{""},
			},
		},
		{
			label:     "invalid address: '-'",
			shouldErr: true,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{"-"},
			},
		},
		{
			label:     "invalid address: 'foo'",
			shouldErr: true,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{"foo"},
			},
		},
		{
			label:     "invalid address: 'foo-bar'",
			shouldErr: true,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{"foo-bar"},
			},
		},
		{
			label:     "invalid address: '<ChainID>'",
			shouldErr: true,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{vm.ctx.ChainID.String()},
			},
		},
		{
			label:     "invalid address: '<ChainID>-'",
			shouldErr: true,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{fmt.Sprintf("%s-", vm.ctx.ChainID.String())},
			},
		},
		{
			label:     "invalid address: '<Unknown ID>-<addr>'",
			shouldErr: true,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{unknownChainAddr},
			},
		},
		{
			label:     "no addresses",
			shouldErr: true,
			args:      &vmargs.GetUTXOsArgs{},
		},
		{
			label: "get all X-chain UTXOs",
			count: numUTXOs,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
				},
			},
		},
		{
			label: "get one X-chain UTXO",
			count: 1,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
				},
				Limit: 1,
			},
		},
		{
			label: "limit greater than number of UTXOs",
			count: numUTXOs,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
				},
				Limit: json.Uint32(numUTXOs + 1),
			},
		},
		{
			label: "no utxos to return",
			count: 0,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{
					xEmptyAddr,
				},
			},
		},
		{
			label: "multiple address with utxos",
			count: numUTXOs,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{
					xEmptyAddr,
					xAddr,
				},
			},
		},
		{
			label: "get all P-chain UTXOs",
			count: numUTXOs,
			args: &vmargs.GetUTXOsArgs{
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
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
				},
				SourceChain: "HomeRunDerby",
			},
		},
		{
			label: "get all P-chain UTXOs",
			count: numUTXOs,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
				},
				SourceChain: "P",
			},
		},
		{
			label:     "get UTXOs from multiple chains",
			shouldErr: true,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{
					xAddr,
					pAddr,
				},
			},
		},
		{
			label:     "get UTXOs for an address on a different chain",
			shouldErr: true,
			args: &vmargs.GetUTXOsArgs{
				Addresses: []string{
					pAddr,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			reply := &vmargs.GetUTXOsReply{}
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

	reply := vmargs.GetAssetDescriptionReply{}
	err := s.GetAssetDescription(nil, &vmargs.GetAssetDescriptionArgs{
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

	reply := vmargs.GetBalanceReply{}
	addrStr, err := vm.FormatLocalAddress(keys[0].PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}
	err = s.GetBalance(nil, &vmargs.GetBalanceArgs{
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

	reply := vmargs.AssetIDChangeAddr{}
	addrStr, err := vm.FormatLocalAddress(keys[0].PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}

	changeAddrStr, err := vm.FormatLocalAddress(testChangeAddr)
	if err != nil {
		t.Fatal(err)
	}
	_, fromAddrsStr := sampleAddrs(t, vm, addrs)

	err = s.CreateFixedCapAsset(nil, &vmargs.CreateAssetArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass: apiargs.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		Name:         "testAsset",
		Symbol:       "TEST",
		Denomination: 1,
		InitialHolders: []*vmargs.Holder{{
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

	reply := vmargs.AssetIDChangeAddr{}
	addrStr, err := vm.FormatLocalAddress(keys[0].PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}
	changeAddrStr, err := vm.FormatLocalAddress(testChangeAddr)
	if err != nil {
		t.Fatal(err)
	}
	_, fromAddrsStr := sampleAddrs(t, vm, addrs)

	err = s.CreateVariableCapAsset(nil, &vmargs.CreateAssetArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass: apiargs.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		Name:   "test asset",
		Symbol: "TEST",
		MinterSets: []vmargs.Owners{
			{
				Threshold: 1,
				Minters: []string{
					addrStr,
				},
			},
		},
	}, &reply)
	if err != nil {
		t.Fatal(err)
	} else if reply.ChangeAddr != changeAddrStr {
		t.Fatalf("expected change address %s but got %s", changeAddrStr, reply.ChangeAddr)
	}

	createAssetTx := UniqueTx{
		vm:   vm,
		txID: reply.AssetID,
	}
	if status := createAssetTx.Status(); status != choices.Processing {
		t.Fatalf("CreateVariableCapAssetTx status should have been Processing, but was %s", status)
	}
	if err := createAssetTx.Accept(); err != nil {
		t.Fatalf("Failed to accept CreateVariableCapAssetTx due to: %s", err)
	}

	createdAssetID := reply.AssetID.String()
	// Test minting of the created variable cap asset
	mintArgs := &vmargs.MintArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass: apiargs.UserPass{
				Username: username,
				Password: password,
			},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		Amount:  200,
		AssetID: createdAssetID,
		To:      addrStr,
	}
	mintReply := &apiargs.JSONTxIDChangeAddr{}
	if err := s.Mint(nil, mintArgs, mintReply); err != nil {
		t.Fatalf("Failed to mint variable cap asset due to: %s", err)
	} else if mintReply.ChangeAddr != changeAddrStr {
		t.Fatalf("expected change address %s but got %s", changeAddrStr, mintReply.ChangeAddr)
	}

	mintTx := UniqueTx{
		vm:   vm,
		txID: mintReply.TxID,
	}

	if status := mintTx.Status(); status != choices.Processing {
		t.Fatalf("MintTx status should have been Processing, but was %s", status)
	}
	if err := mintTx.Accept(); err != nil {
		t.Fatalf("Failed to accept MintTx due to: %s", err)
	}

	sendArgs := &vmargs.SendArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass: apiargs.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		SendOutput: vmargs.SendOutput{
			Amount:  200,
			AssetID: createdAssetID,
			To:      addrStr,
		},
	}
	sendReply := &apiargs.JSONTxIDChangeAddr{}
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

	createArgs := &vmargs.CreateNFTAssetArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass: apiargs.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: fromAddrsStr[0]},
		},
		Name:   "BIG COIN",
		Symbol: "COIN",
		MinterSets: []vmargs.Owners{
			{
				Threshold: 1,
				Minters: []string{
					addrStr,
				},
			},
		},
	}
	createReply := &vmargs.AssetIDChangeAddr{}
	if err := s.CreateNFTAsset(nil, createArgs, createReply); err != nil {
		t.Fatalf("Failed to mint variable cap asset due to: %s", err)
	} else if createReply.ChangeAddr != fromAddrsStr[0] {
		t.Fatalf("expected change address to be %s but got %s", fromAddrsStr[0], createReply.ChangeAddr)
	}

	assetID := createReply.AssetID
	createNFTTx := UniqueTx{
		vm:   vm,
		txID: createReply.AssetID,
	}
	// Accept the transaction so that we can Mint NFTs for the test
	if createNFTTx.Status() != choices.Processing {
		t.Fatalf("CreateNFTTx should have been processing after creating the NFT")
	}
	if err := createNFTTx.Accept(); err != nil {
		t.Fatalf("Failed to accept CreateNFT transaction: %s", err)
	} else if err := verifyTxFeeDeducted(t, s, fromAddrs, 1); err != nil {
		t.Fatal(err)
	}

	payload, err := formatting.Encode(formatting.Hex, []byte{1, 2, 3, 4, 5})
	if err != nil {
		t.Fatal(err)
	}
	mintArgs := &vmargs.MintNFTArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass: apiargs.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  apiargs.JSONFromAddrs{},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: fromAddrsStr[0]},
		},
		AssetID:  assetID.String(),
		Payload:  payload,
		To:       addrStr,
		Encoding: formatting.Hex,
	}
	mintReply := &apiargs.JSONTxIDChangeAddr{}

	if err := s.MintNFT(nil, mintArgs, mintReply); err != nil {
		t.Fatalf("MintNFT returned an error: %s", err)
	} else if createReply.ChangeAddr != fromAddrsStr[0] {
		t.Fatalf("expected change address to be %s but got %s", fromAddrsStr[0], mintReply.ChangeAddr)
	}

	mintNFTTx := UniqueTx{
		vm:   vm,
		txID: mintReply.TxID,
	}
	if mintNFTTx.Status() != choices.Processing {
		t.Fatal("MintNFTTx should have been processing after minting the NFT")
	}

	// Accept the transaction so that we can send the newly minted NFT
	if err := mintNFTTx.Accept(); err != nil {
		t.Fatalf("Failed to accept MintNFTTx: %s", err)
	}

	sendArgs := &vmargs.SendNFTArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass: apiargs.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  apiargs.JSONFromAddrs{},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: fromAddrsStr[0]},
		},
		AssetID: assetID.String(),
		GroupID: 0,
		To:      addrStr,
	}
	sendReply := &apiargs.JSONTxIDChangeAddr{}
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
	importArgs := &vmargs.ImportKeyArgs{
		UserPass: apiargs.UserPass{
			Username: username,
			Password: password,
		},
		PrivateKey: constants.SecretKeyPrefix + privKeyStr,
	}
	importReply := &apiargs.JSONAddress{}
	if err = s.ImportKey(nil, importArgs, importReply); err != nil {
		t.Fatal(err)
	}

	addrStr, err := vm.FormatLocalAddress(sk.PublicKey().Address())
	if err != nil {
		t.Fatal(err)
	}
	exportArgs := &vmargs.ExportKeyArgs{
		UserPass: apiargs.UserPass{
			Username: username,
			Password: password,
		},
		Address: addrStr,
	}
	exportReply := &vmargs.ExportKeyReply{}
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
	args := vmargs.ImportKeyArgs{
		UserPass: apiargs.UserPass{
			Username: username,
			Password: password,
		},
		PrivateKey: constants.SecretKeyPrefix + privKeyStr,
	}
	reply := apiargs.JSONAddress{}
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

	reply2 := apiargs.JSONAddress{}
	if err = s.ImportKey(nil, &args, &reply2); err != nil {
		t.Fatal(err)
	}

	if reply2.Address != expectedAddress {
		t.Fatalf("Reply address: %s did not match expected address: %s", reply2.Address, expectedAddress)
	}

	addrsArgs := apiargs.UserPass{
		Username: username,
		Password: password,
	}
	addrsReply := apiargs.JSONAddresses{}
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

	args := &vmargs.SendArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass: apiargs.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		SendOutput: vmargs.SendOutput{
			Amount:  500,
			AssetID: assetID.String(),
			To:      addrStr,
		},
	}
	reply := &apiargs.JSONTxIDChangeAddr{}
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

	args := &vmargs.SendMultipleArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass: apiargs.UserPass{
				Username: username,
				Password: password,
			},
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: fromAddrsStr},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddrStr},
		},
		Outputs: []vmargs.SendOutput{
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
	reply := &apiargs.JSONTxIDChangeAddr{}
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

	if _, err = vm.GetTx(reply.TxID); err != nil {
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

	createArgs := &apiargs.UserPass{
		Username: username,
		Password: password,
	}
	createReply := &apiargs.JSONAddress{}

	if err := s.CreateAddress(nil, createArgs, createReply); err != nil {
		t.Fatalf("Failed to create address: %s", err)
	}

	newAddr := createReply.Address

	listArgs := &apiargs.UserPass{
		Username: username,
		Password: password,
	}
	listReply := &apiargs.JSONAddresses{}

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
	utxoBytes, err := vm.codec.Marshal(codecVersion, utxo)
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
	args := &vmargs.ImportArgs{
		UserPass: apiargs.UserPass{
			Username: username,
			Password: password,
		},
		SourceChain: "P",
		To:          addrStr,
	}
	reply := &apiargs.JSONTxID{}
	if err := s.ImportAVAX(nil, args, reply); err != nil {
		t.Fatalf("Failed to import AVAX due to %s", err)
	}
}
