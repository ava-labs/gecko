// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/api"
	"github.com/ava-labs/avalanchego/api/keystore"
	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/vms/avm"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"

	cjson "github.com/ava-labs/avalanchego/utils/json"
)

var (
	// Test user username
	testUsername string = "ScoobyUser"

	// Test user password, must meet minimum complexity/length requirements
	testPassword string = "ShaggyPassword1Zoinks!"

	// Bytes docoded from CB58 "ewoqjP7PxY4yr3iLTpLisriqt94hdyDFNgchSxGGztUrTXtNN"
	testPrivateKey []byte = []byte{
		0x56, 0x28, 0x9e, 0x99, 0xc9, 0x4b, 0x69, 0x12,
		0xbf, 0xc1, 0x2a, 0xdc, 0x09, 0x3c, 0x9b, 0x51,
		0x12, 0x4f, 0x0d, 0xc5, 0x4a, 0xc7, 0xa7, 0x66,
		0xb2, 0xbc, 0x5c, 0xcf, 0x55, 0x8d, 0x80, 0x27,
	}

	// 3cb7d3842e8cee6a0ebd09f1fe884f6861e1b29c
	// Platform address resulting from the above private key
	testAddress string = "P-testing18jma8ppw3nhx5r4ap8clazz0dps7rv5umpc36y"
)

func defaultService(t *testing.T) *Service {
	vm, _ := defaultVM()
	vm.Ctx.Lock.Lock()
	defer vm.Ctx.Lock.Unlock()
	ks := keystore.New(logging.NoLog{}, memdb.New())
	if err := ks.CreateUser(testUsername, testPassword); err != nil {
		t.Fatal(err)
	}
	vm.SnowmanVM.Ctx.Keystore = ks.NewBlockchainKeyStore(vm.SnowmanVM.Ctx.ChainID)
	return &Service{vm: vm}
}

// Give user [testUsername] control of [testPrivateKey] and keys[0] (which is funded)
func defaultAddress(t *testing.T, service *Service) {
	service.vm.Ctx.Lock.Lock()
	defer service.vm.Ctx.Lock.Unlock()
	userDB, err := service.vm.SnowmanVM.Ctx.Keystore.GetDatabase(testUsername, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	user := user{db: userDB}
	pk, err := service.vm.factory.ToPrivateKey(testPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	privKey := pk.(*crypto.PrivateKeySECP256K1R)
	if err := user.putAddress(privKey); err != nil {
		t.Fatal(err)
	} else if err := user.putAddress(keys[0]); err != nil {
		t.Fatal(err)
	}
}

func TestAddValidator(t *testing.T) {
	expectedJSONString := `{"username":"","password":"","from":null,"changeAddr":"","txID":"11111111111111111111111111111111LpoYY","startTime":"0","endTime":"0","nodeID":"","rewardAddress":"","delegationFeeRate":"0.0000"}`
	args := AddValidatorArgs{}
	bytes, err := json.Marshal(&args)
	if err != nil {
		t.Fatal(err)
	}
	jsonString := string(bytes)
	if jsonString != expectedJSONString {
		t.Fatalf("Expected: %s\nResult: %s", expectedJSONString, jsonString)
	}
}

func TestCreateBlockchainArgsParsing(t *testing.T) {
	jsonString := `{"vmID":"lol","fxIDs":["secp256k1"], "name":"awesome", "username":"bob loblaw", "password":"yeet", "genesisData":"SkB92YpWm4Q2iPnLGCuDPZPgUQMxajqQQuz91oi3xD984f8r"}`
	args := CreateBlockchainArgs{}
	err := json.Unmarshal([]byte(jsonString), &args)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = json.Marshal(args.GenesisData); err != nil {
		t.Fatal(err)
	}
}

func TestExportKey(t *testing.T) {
	jsonString := `{"username":"ScoobyUser","password":"ShaggyPassword1Zoinks!","address":"` + testAddress + `"}`
	args := ExportKeyArgs{}
	err := json.Unmarshal([]byte(jsonString), &args)
	if err != nil {
		t.Fatal(err)
	}

	service := defaultService(t)
	defaultAddress(t, service)
	service.vm.Ctx.Lock.Lock()
	defer func() {
		if err := service.vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		service.vm.Ctx.Lock.Unlock()
	}()

	reply := ExportKeyReply{}
	if err := service.ExportKey(nil, &args, &reply); err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(reply.PrivateKey, constants.SecretKeyPrefix) {
		t.Fatalf("ExportKeyReply is missing secret key prefix: %s", constants.SecretKeyPrefix)
	}
	privateKeyString := strings.TrimPrefix(reply.PrivateKey, constants.SecretKeyPrefix)
	privKeyBytes, err := formatting.Decode(formatting.CB58, privateKeyString)
	if err != nil {
		t.Fatalf("Failed to parse key: %s", err)
	}
	if !bytes.Equal(testPrivateKey, privKeyBytes) {
		t.Fatalf("Expected %v, got %v", testPrivateKey, privKeyBytes)
	}
}

func TestImportKey(t *testing.T) {
	jsonString := `{"username":"ScoobyUser","password":"ShaggyPassword1Zoinks!","privateKey":"PrivateKey-ewoqjP7PxY4yr3iLTpLisriqt94hdyDFNgchSxGGztUrTXtNN"}`
	args := ImportKeyArgs{}
	err := json.Unmarshal([]byte(jsonString), &args)
	if err != nil {
		t.Fatal(err)
	}

	service := defaultService(t)
	service.vm.Ctx.Lock.Lock()
	defer func() {
		if err := service.vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		service.vm.Ctx.Lock.Unlock()
	}()

	reply := api.JSONAddress{}
	if err := service.ImportKey(nil, &args, &reply); err != nil {
		t.Fatal(err)
	}
	if testAddress != reply.Address {
		t.Fatalf("Expected %q, got %q", testAddress, reply.Address)
	}
}

// Test issuing a tx, having it be dropped, and then re-issued and accepted
func TestGetTxStatus(t *testing.T) {
	service := defaultService(t)
	defaultAddress(t, service)
	service.vm.Ctx.Lock.Lock()
	defer func() {
		if err := service.vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		service.vm.Ctx.Lock.Unlock()
	}()

	// create a tx
	tx, err := service.vm.newCreateChainTx(
		testSubnet1.ID(),
		nil,
		avm.ID,
		nil,
		"chain name",
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	arg := &GetTxStatusArgs{TxID: tx.ID()}
	argIncludeReason := &GetTxStatusArgs{TxID: tx.ID(), IncludeReason: true}

	var resp GetTxStatusResponse
	err = service.GetTxStatus(nil, arg, &resp)
	switch {
	case err != nil:
		t.Fatal(err)
	case resp.Status != Unknown:
		t.Fatalf("status should be unknown but is %s", resp.Status)
	case resp.Reason != "":
		t.Fatalf("reason should be empty but is %s", resp.Reason)
	}

	resp = GetTxStatusResponse{} // reset

	err = service.GetTxStatus(nil, argIncludeReason, &resp)
	switch {
	case err != nil:
		t.Fatal(err)
	case resp.Status != Unknown:
		t.Fatalf("status should be unknown but is %s", resp.Status)
	case resp.Reason != "":
		t.Fatalf("reason should be empty but is %s", resp.Reason)
	}

	// put the chain in existing chain list
	if err := service.vm.mempool.IssueTx(tx); err != nil {
		t.Fatal(err)
	} else if err := service.vm.putChains(service.vm.DB, []*Tx{tx}); err != nil {
		t.Fatal(err)
	} else if _, err := service.vm.BuildBlock(); err == nil {
		t.Fatal("should have errored because chain already exists")
	}

	resp = GetTxStatusResponse{} // reset
	err = service.GetTxStatus(nil, arg, &resp)
	switch {
	case err != nil:
		t.Fatal(err)
	case resp.Status != Dropped:
		t.Fatalf("status should be Dropped but is %s", resp.Status)
	case resp.Reason != "":
		t.Fatal("reason should be empty when IncludeReason is false")
	}

	resp = GetTxStatusResponse{} // reset
	err = service.GetTxStatus(nil, argIncludeReason, &resp)
	switch {
	case err != nil:
		t.Fatal(err)
	case resp.Status != Dropped:
		t.Fatalf("status should be Dropped but is %s", resp.Status)
	case resp.Reason == "":
		t.Fatalf("reason shouldn't be empty")
	}

	// remove the chain from existing chain list
	if err := service.vm.putChains(service.vm.DB, []*Tx{}); err != nil {
		t.Fatal(err)
	} else if err := service.vm.mempool.IssueTx(tx); err != nil {
		t.Fatal(err)
	} else if block, err := service.vm.BuildBlock(); err != nil {
		t.Fatal(err)
	} else if blk, ok := block.(*StandardBlock); !ok {
		t.Fatalf("should be *StandardBlock but is %T", blk)
	} else if err := blk.Verify(); err != nil {
		t.Fatal(err)
	} else if err := blk.Accept(); err != nil {
		t.Fatal(err)
	}

	resp = GetTxStatusResponse{} // reset
	err = service.GetTxStatus(nil, arg, &resp)
	switch {
	case err != nil:
		t.Fatal(err)
	case resp.Status != Committed:
		t.Fatalf("status should be Committed but is %s", resp.Status)
	case resp.Reason != "":
		t.Fatalf("reason should be empty but is %s", resp.Reason)
	}
}

// Test issuing and then retrieving a transaction
func TestGetTx(t *testing.T) {
	service := defaultService(t)
	defaultAddress(t, service)
	service.vm.Ctx.Lock.Lock()
	defer func() {
		if err := service.vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		service.vm.Ctx.Lock.Unlock()
	}()

	type test struct {
		description string
		createTx    func() (*Tx, error)
	}

	tests := []test{
		{
			"standard block",
			func() (*Tx, error) {
				return service.vm.newCreateChainTx( // Test GetTx works for standard blocks
					testSubnet1.ID(),
					nil,
					avm.ID,
					nil,
					"chain name",
					[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
					keys[0].PublicKey().Address(), // change addr
				)
			},
		},
		{
			"proposal block",
			func() (*Tx, error) {
				return service.vm.newAddValidatorTx( // Test GetTx works for proposal blocks
					service.vm.minValidatorStake,
					uint64(service.vm.clock.Time().Add(syncBound).Unix()),
					uint64(service.vm.clock.Time().Add(syncBound).Add(defaultMinStakingDuration).Unix()),
					ids.GenerateTestShortID(),
					ids.GenerateTestShortID(),
					0,
					[]*crypto.PrivateKeySECP256K1R{keys[0]},
					keys[0].PublicKey().Address(), // change addr
				)
			},
		},
		{
			"atomic block",
			func() (*Tx, error) {
				return service.vm.newExportTx( // Test GetTx works for proposal blocks
					100,
					service.vm.Ctx.XChainID,
					ids.GenerateTestShortID(),
					[]*crypto.PrivateKeySECP256K1R{keys[0]},
					keys[0].PublicKey().Address(), // change addr
				)
			},
		},
	}

	for _, test := range tests {
		tx, err := test.createTx()
		if err != nil {
			t.Fatalf("failed test '%s': %s", test.description, err)
		}
		arg := &api.GetTxArgs{
			TxID:     tx.ID(),
			Encoding: formatting.CB58,
		}
		var response api.FormattedTx
		if err := service.GetTx(nil, arg, &response); err == nil {
			t.Fatalf("failed test '%s': haven't issued tx yet so shouldn't be able to get it", test.description)
		} else if err := service.vm.mempool.IssueTx(tx); err != nil {
			t.Fatalf("failed test '%s': %s", test.description, err)
		} else if block, err := service.vm.BuildBlock(); err != nil {
			t.Fatalf("failed test '%s': %s", test.description, err)
		} else if err := block.Verify(); err != nil {
			t.Fatalf("failed test '%s': %s", test.description, err)
		} else if err := block.Accept(); err != nil {
			t.Fatalf("failed test '%s': %s", test.description, err)
		} else if blk, ok := block.(*ProposalBlock); ok { // For proposal blocks, commit them
			if options, err := blk.Options(); err != nil {
				t.Fatalf("failed test '%s': %s", test.description, err)
			} else if commit, ok := options[0].(*Commit); !ok {
				t.Fatalf("failed test '%s': should prefer to commit", test.description)
			} else if err := commit.Verify(); err != nil {
				t.Fatalf("failed test '%s': %s", test.description, err)
			} else if err := commit.Accept(); err != nil {
				t.Fatalf("failed test '%s': %s", test.description, err)
			}
		} else if err := service.GetTx(nil, arg, &response); err != nil {
			t.Fatalf("failed test '%s': %s", test.description, err)
		} else {
			responseTxBytes, err := formatting.Decode(response.Encoding, response.Tx)
			if err != nil {
				t.Fatalf("failed test '%s': %s", test.description, err)
			}
			if !bytes.Equal(responseTxBytes, tx.Bytes()) {
				t.Fatalf("failed test '%s': byte representation of tx in response is incorrect", test.description)
			}
		}
	}
}

// Test method GetBalance
func TestGetBalance(t *testing.T) {
	service := defaultService(t)
	defaultAddress(t, service)
	service.vm.Ctx.Lock.Lock()
	defer func() {
		if err := service.vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		service.vm.Ctx.Lock.Unlock()
	}()

	// Ensure GetStake is correct for each of the genesis validators
	genesis, _ := defaultGenesis()
	for _, utxo := range genesis.UTXOs {
		request := api.JSONAddress{
			Address: fmt.Sprintf("P-%s", utxo.Address),
		}
		reply := GetBalanceResponse{}
		if err := service.GetBalance(nil, &request, &reply); err != nil {
			t.Fatal(err)
		}
		if reply.Balance != cjson.Uint64(defaultBalance) {
			t.Fatalf("Wrong balance. Expected %d ; Returned %d", reply.Balance, defaultBalance)
		}
		if reply.Unlocked != cjson.Uint64(defaultBalance) {
			t.Fatalf("Wrong unlocked balance. Expected %d ; Returned %d", reply.Unlocked, defaultBalance)
		}
		if reply.LockedStakeable != 0 {
			t.Fatalf("Wrong locked stakeable balance. Expected %d ; Returned %d", reply.LockedStakeable, 0)
		}
		if reply.LockedNotStakeable != 0 {
			t.Fatalf("Wrong locked not stakeable balance. Expected %d ; Returned %d", reply.LockedNotStakeable, 0)
		}
	}
}

// Test method GetStake
func TestGetStake(t *testing.T) {
	assert := assert.New(t)
	service := defaultService(t)
	defaultAddress(t, service)
	service.vm.Ctx.Lock.Lock()
	defer func() {
		err := service.vm.Shutdown()
		assert.NoError(err)
		service.vm.Ctx.Lock.Unlock()
	}()

	// Ensure GetStake is correct for each of the genesis validators
	genesis, _ := defaultGenesis()
	addrsStrs := []string{}
	for i, validator := range genesis.Validators {
		addr := fmt.Sprintf("P-%s", validator.RewardOwner.Addresses[0])
		addrsStrs = append(addrsStrs, addr)
		args := GetStakeArgs{
			api.JSONAddresses{
				Addresses: []string{addr},
			},
			formatting.Hex,
		}
		response := GetStakeReply{}
		err := service.GetStake(nil, &args, &response)
		assert.NoError(err)
		assert.EqualValues(uint64(defaultWeight), uint64(response.Staked))
		assert.Len(response.Outputs, 1)
		// Unmarshal into an output
		outputBytes, err := formatting.Decode(args.Encoding, response.Outputs[0])
		assert.NoError(err)
		var output avax.TransferableOutput
		_, err = service.vm.codec.Unmarshal(outputBytes, &output)
		assert.NoError(err)
		out, ok := output.Out.(*secp256k1fx.TransferOutput)
		assert.True(ok)
		assert.EqualValues(out.Amount(), defaultWeight)
		assert.EqualValues(out.Threshold, 1)
		assert.Len(out.Addrs, 1)
		assert.Equal(keys[i].PublicKey().Address(), out.Addrs[0])
		assert.EqualValues(out.Locktime, 0)
	}

	// Make sure this works for multiple addresses
	args := GetStakeArgs{
		api.JSONAddresses{
			Addresses: addrsStrs,
		},
		formatting.Hex,
	}
	response := GetStakeReply{}
	err := service.GetStake(nil, &args, &response)
	assert.NoError(err)
	assert.EqualValues(len(genesis.Validators)*defaultWeight, response.Staked)
	assert.Len(response.Outputs, len(genesis.Validators))
	for _, outputStr := range response.Outputs {
		outputBytes, err := formatting.Decode(args.Encoding, outputStr)
		assert.NoError(err)
		var output avax.TransferableOutput
		_, err = service.vm.codec.Unmarshal(outputBytes, &output)
		assert.NoError(err)
		out, ok := output.Out.(*secp256k1fx.TransferOutput)
		assert.True(ok)
		assert.EqualValues(defaultWeight, out.Amount())
		assert.EqualValues(out.Threshold, 1)
		assert.EqualValues(out.Locktime, 0)
		assert.Len(out.Addrs, 1)
	}

	oldStake := uint64(defaultWeight)

	// Add a delegator
	stakeAmt := service.vm.minDelegatorStake + 12345
	delegatorNodeID := ids.GenerateTestShortID()
	delegatorEndTime := uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix())
	tx, err := service.vm.newAddDelegatorTx(
		stakeAmt,
		uint64(defaultGenesisTime.Unix()),
		delegatorEndTime,
		delegatorNodeID,
		ids.GenerateTestShortID(),
		[]*crypto.PrivateKeySECP256K1R{keys[0]},
		keys[0].PublicKey().Address(), // change addr
	)
	assert.NoError(err)
	err = service.vm.addStaker(service.vm.DB, constants.PrimaryNetworkID, &rewardTx{
		Reward: 0,
		Tx:     *tx,
	})
	assert.NoError(err)

	// Make sure the delegator addr has the right stake (old stake + stakeAmt)
	addr, _ := service.vm.FormatLocalAddress(keys[0].PublicKey().Address())
	args.Addresses = []string{addr}
	err = service.GetStake(nil, &args, &response)
	assert.NoError(err)
	assert.EqualValues(oldStake+stakeAmt, uint64(response.Staked))
	assert.Len(response.Outputs, 2)
	// Unmarshal into transferableoutputs
	outputs := make([]avax.TransferableOutput, 2)
	for i := range outputs {
		outputBytes, err := formatting.Decode(args.Encoding, response.Outputs[i])
		assert.NoError(err)
		_, err = service.vm.codec.Unmarshal(outputBytes, &outputs[i])
		assert.NoError(err)
	}
	// Make sure the stake amount is as expected
	assert.EqualValues(stakeAmt+oldStake, outputs[0].Out.Amount()+outputs[1].Out.Amount())

	oldStake = uint64(response.Staked)

	// Make sure this works for pending stakers
	// Add a pending staker
	stakeAmt = service.vm.minValidatorStake + 54321
	pendingStakerNodeID := ids.GenerateTestShortID()
	pendingStakerEndTime := uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix())
	tx, err = service.vm.newAddValidatorTx(
		stakeAmt,
		uint64(defaultGenesisTime.Unix()),
		pendingStakerEndTime,
		pendingStakerNodeID,
		ids.GenerateTestShortID(),
		0,
		[]*crypto.PrivateKeySECP256K1R{keys[0]},
		keys[0].PublicKey().Address(), // change addr
	)
	assert.NoError(err)
	err = service.vm.enqueueStaker(service.vm.DB, constants.PrimaryNetworkID, tx)
	assert.NoError(err)
	// Make sure the delegator has the right stake (old stake + stakeAmt)
	err = service.GetStake(nil, &args, &response)
	assert.NoError(err)
	assert.EqualValues(oldStake+stakeAmt, response.Staked)
	assert.Len(response.Outputs, 3)
	outputs = make([]avax.TransferableOutput, 3)
	// Unmarshal
	for i := range outputs {
		outputBytes, err := formatting.Decode(args.Encoding, response.Outputs[i])
		assert.NoError(err)
		_, err = service.vm.codec.Unmarshal(outputBytes, &outputs[i])
		assert.NoError(err)
	}
	// Make sure the stake amount is as expected
	assert.EqualValues(stakeAmt+oldStake, outputs[0].Out.Amount()+outputs[1].Out.Amount()+outputs[2].Out.Amount())
}

// Test method GetCurrentValidators
func TestGetCurrentValidators(t *testing.T) {
	service := defaultService(t)
	defaultAddress(t, service)
	service.vm.Ctx.Lock.Lock()
	defer func() {
		if err := service.vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		service.vm.Ctx.Lock.Unlock()
	}()

	genesis, _ := defaultGenesis()

	// Call getValidators
	args := GetCurrentValidatorsArgs{SubnetID: constants.PrimaryNetworkID}
	response := GetCurrentValidatorsReply{}

	err := service.GetCurrentValidators(nil, &args, &response)
	switch {
	case err != nil:
		t.Fatal(err)
	case len(response.Validators) != len(genesis.Validators):
		t.Fatalf("should be %d validators but are %d", len(genesis.Validators), len(response.Validators))
	}

	for _, vdr := range genesis.Validators {
		found := false
		for i := 0; i < len(response.Validators) && !found; i++ {
			gotVdr, ok := response.Validators[i].(APIPrimaryValidator)
			switch {
			case !ok:
				t.Fatal("expected APIPrimaryValidator")
			case gotVdr.NodeID != vdr.NodeID:
			case gotVdr.EndTime != vdr.EndTime:
				t.Fatalf("expected end time of %s to be %v but got %v",
					vdr.NodeID,
					vdr.EndTime,
					gotVdr.EndTime,
				)
			case gotVdr.StartTime != vdr.StartTime:
				t.Fatalf("expected start time of %s to be %v but got %v",
					vdr.NodeID,
					vdr.StartTime,
					gotVdr.StartTime,
				)
			case gotVdr.Weight != vdr.Weight:
				t.Fatalf("expected weight of %s to be %v but got %v",
					vdr.NodeID,
					vdr.Weight,
					gotVdr.Weight,
				)
			default:
				found = true
			}
		}
		if !found {
			t.Fatalf("expected validators to contain %s but didn't", vdr.NodeID)
		}
	}

	// Add a delegator
	stakeAmt := service.vm.minDelegatorStake + 12345
	validatorNodeID := keys[1].PublicKey().Address()
	delegatorStartTime := uint64(defaultValidateStartTime.Unix())
	delegatorEndTime := uint64(defaultValidateStartTime.Add(defaultMinStakingDuration).Unix())

	tx, err := service.vm.newAddDelegatorTx(
		stakeAmt,
		delegatorStartTime,
		delegatorEndTime,
		validatorNodeID,
		ids.GenerateTestShortID(),
		[]*crypto.PrivateKeySECP256K1R{keys[0]},
		keys[0].PublicKey().Address(), // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.vm.addStaker(service.vm.DB, constants.PrimaryNetworkID, &rewardTx{
		Reward: 0,
		Tx:     *tx,
	}); err != nil {
		t.Fatal(err)
	}

	// Call getCurrentValidators
	args = GetCurrentValidatorsArgs{SubnetID: constants.PrimaryNetworkID}
	err = service.GetCurrentValidators(nil, &args, &response)
	switch {
	case err != nil:
		t.Fatal(err)
	case len(response.Validators) != len(genesis.Validators):
		t.Fatalf("should be %d validators but are %d", len(genesis.Validators), len(response.Validators))
	}

	// Make sure the delegator is there
	found := false
	for i := 0; i < len(response.Validators) && !found; i++ {
		vdr := response.Validators[i].(APIPrimaryValidator)
		if vdr.NodeID != validatorNodeID.PrefixedString(constants.NodeIDPrefix) {
			continue
		}
		found = true
		if len(vdr.Delegators) != 1 {
			t.Fatalf("%s should have 1 delegator", vdr.NodeID)
		}
		delegator := vdr.Delegators[0]
		switch {
		case delegator.NodeID != vdr.NodeID:
			t.Fatal("wrong node ID")
		case uint64(delegator.StartTime) != delegatorStartTime:
			t.Fatal("wrong start time")
		case uint64(delegator.EndTime) != delegatorEndTime:
			t.Fatal("wrong end time")
		case delegator.weight() != stakeAmt:
			t.Fatalf("wrong weight")
		}
	}
	if !found {
		t.Fatalf("didnt find delegator")
	}
}
