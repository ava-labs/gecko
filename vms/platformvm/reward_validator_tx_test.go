// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/chains"
	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/math"
	"github.com/ava-labs/avalanchego/vms/components/core"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

func TestUnsignedRewardValidatorTxSemanticVerify(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	// ID of validator that should leave DS validator set next
	toRemoveIntf, err := vm.nextStakerStop(vm.DB, constants.PrimaryNetworkID)
	if err != nil {
		t.Fatal(err)
	}
	toRemove := toRemoveIntf.Tx.UnsignedTx.(*UnsignedAddValidatorTx)

	// Case 1: Chain timestamp is wrong
	if tx, err := vm.newRewardValidatorTx(toRemove.ID()); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err := toRemove.SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatalf("should have failed because validator end time doesn't match chain timestamp")
	}

	// Case 2: Wrong validator
	if tx, err := vm.newRewardValidatorTx(ids.GenerateTestID()); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err := toRemove.SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatalf("should have failed because validator ID is wrong")
	}

	// Case 3: Happy path
	// Advance chain timestamp to time that next validator leaves
	if err := vm.putTimestamp(vm.DB, toRemove.EndTime()); err != nil {
		t.Fatal(err)
	}
	tx, err := vm.newRewardValidatorTx(toRemove.ID())
	if err != nil {
		t.Fatal(err)
	}
	onCommitDB, onAbortDB, _, _, err := tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx)
	if err != nil {
		t.Fatal(err)
	}

	// ID of validator that should leave DS validator set next

	if nextToRemove, err := vm.nextStakerStop(onCommitDB, constants.PrimaryNetworkID); err != nil {
		t.Fatal(err)
	} else if toRemove.ID().Equals(nextToRemove.Tx.ID()) {
		t.Fatalf("Should have removed the previous validator")
	}

	// check that stake/reward is given back
	stakeOwners := toRemove.Stake[0].Out.(*secp256k1fx.TransferOutput).AddressesSet()
	// Get old balances, balances if tx abort, balances if tx committed
	for _, stakeOwner := range stakeOwners.List() {
		stakeOwnerSet := ids.ShortSet{}
		stakeOwnerSet.Add(stakeOwner)

		oldBalance, err := vm.getBalance(vm.DB, stakeOwnerSet)
		if err != nil {
			t.Fatal(err)
		}
		onAbortBalance, err := vm.getBalance(onAbortDB, stakeOwnerSet)
		if err != nil {
			t.Fatal(err)
		}
		onCommitBalance, err := vm.getBalance(onCommitDB, stakeOwnerSet)
		if err != nil {
			t.Fatal(err)
		}
		if onAbortBalance != oldBalance+toRemove.Validator.Weight() {
			t.Fatalf("on abort, should have got back staked amount")
		}
		if onCommitBalance != oldBalance+toRemove.Validator.Weight()+27 {
			t.Fatalf("on commit, should have old balance (%d) + staked amount (%d) + reward (%d) but have %d",
				oldBalance, toRemove.Validator.Weight(), 27, onCommitBalance)
		}
	}
}

func TestRewardDelegatorTxSemanticVerify(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	initialSupply, err := vm.getCurrentSupply(vm.DB)
	if err != nil {
		t.Fatal(err)
	}

	vdrRewardAddress := ids.GenerateTestShortID()
	delRewardAddress := ids.GenerateTestShortID()

	vdrStartTime := uint64(defaultValidateStartTime.Unix()) + 1
	vdrEndTime := uint64(defaultValidateStartTime.Add(2 * defaultMinStakingDuration).Unix())
	vdrNodeID := ids.GenerateTestShortID()
	vdrTx, err := vm.newAddValidatorTx(
		vm.minValidatorStake, // stakeAmt
		vdrStartTime,
		vdrEndTime,
		vdrNodeID,        // node ID
		vdrRewardAddress, // reward address
		PercentDenominator/4,
		[]*crypto.PrivateKeySECP256K1R{keys[0]}, // fee payer
		ids.ShortEmpty,                          // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	delStartTime := vdrStartTime
	delEndTime := vdrEndTime
	delTx, err := vm.newAddDelegatorTx(
		vm.minDelegatorStake, // stakeAmt
		delStartTime,
		delEndTime,
		vdrNodeID,                               // node ID
		delRewardAddress,                        // reward address
		[]*crypto.PrivateKeySECP256K1R{keys[0]}, // fee payer
		ids.ShortEmpty,                          // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := vm.addStaker(vm.DB, constants.PrimaryNetworkID, &rewardTx{
		Reward: 0,
		Tx:     *vdrTx,
	}); err != nil {
		t.Fatal(err)
	}
	if err := vm.addStaker(vm.DB, constants.PrimaryNetworkID, &rewardTx{
		Reward: 1000000,
		Tx:     *delTx,
	}); err != nil {
		t.Fatal(err)
	}
	if err := vm.putTimestamp(vm.DB, time.Unix(int64(delEndTime), 0)); err != nil {
		t.Fatal(err)
	}

	tx, err := vm.newRewardValidatorTx(delTx.ID())
	if err != nil {
		t.Fatal(err)
	}
	onCommitDB, onAbortDB, _, _, err := tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx)
	if err != nil {
		t.Fatal(err)
	}

	vdrDestSet := ids.ShortSet{}
	vdrDestSet.Add(vdrRewardAddress)
	delDestSet := ids.ShortSet{}
	delDestSet.Add(delRewardAddress)

	expectedReward := uint64(1000000)

	// If tx is committed, delegator and delegatee should get reward
	// and the delegator's reward should be greater because the delegatee's share is 25%
	oldVdrBalance, err := vm.getBalance(vm.DB, vdrDestSet)
	assert.NoError(t, err)
	commitVdrBalance, err := vm.getBalance(onCommitDB, vdrDestSet)
	assert.NoError(t, err)
	vdrReward, err := math.Sub64(commitVdrBalance, oldVdrBalance)
	assert.NoError(t, err)
	if vdrReward == 0 {
		t.Fatal("expected delegatee balance to increase because of reward")
	}

	oldDelBalance, err := vm.getBalance(vm.DB, delDestSet)
	assert.NoError(t, err)
	commitDelBalance, err := vm.getBalance(onCommitDB, delDestSet)
	assert.NoError(t, err)
	delReward, err := math.Sub64(commitDelBalance, oldDelBalance)
	assert.NoError(t, err)
	if delReward == 0 {
		t.Fatal("expected delegator balance to increase because of reward")
	}

	if delReward < vdrReward {
		t.Fatal("the delegator's reward should be greater than the delegatee's because the delegatee's share is 25%")
	}
	if delReward+vdrReward != expectedReward {
		t.Fatalf("expected total reward to be %d but is %d", expectedReward, delReward+vdrReward)
	}

	abortVdrBalance, err := vm.getBalance(onAbortDB, vdrDestSet)
	assert.NoError(t, err)
	vdrReward, err = math.Sub64(abortVdrBalance, oldVdrBalance)
	assert.NoError(t, err)
	if vdrReward != 0 {
		t.Fatal("expected delegatee balance to stay the same")
	}

	abortDelBalance, err := vm.getBalance(onAbortDB, delDestSet)
	assert.NoError(t, err)
	delReward, err = math.Sub64(abortDelBalance, oldDelBalance)
	assert.NoError(t, err)
	if delReward != 0 {
		t.Fatal("expected delegatee balance to stay the same")
	}

	if supply, err := vm.getCurrentSupply(onAbortDB); err != nil {
		t.Fatal(err)
	} else if supply != initialSupply-expectedReward {
		t.Fatalf("should have removed un-rewarded tokens from the potential supply")
	}
}

func TestOptimisticUptime(t *testing.T) {
	_, genesisBytes := defaultGenesis()
	db := memdb.New()

	firstVM := &VM{
		SnowmanVM:          &core.SnowmanVM{},
		chainManager:       chains.MockManager{},
		uptimePercentage:   .2,
		stakeMintingPeriod: defaultMaxStakingDuration,
	}
	firstVM.vdrMgr = validators.NewManager()
	firstVM.clock.Set(defaultGenesisTime)

	firstCtx := defaultContext()
	firstCtx.Lock.Lock()

	firstMsgChan := make(chan common.Message, 1)
	if err := firstVM.Initialize(firstCtx, db, genesisBytes, firstMsgChan, nil); err != nil {
		t.Fatal(err)
	}

	if err := firstVM.Bootstrapping(); err != nil {
		t.Fatal(err)
	}

	// Fast forward clock to time for genesis validators to leave
	firstVM.clock.Set(defaultValidateStartTime.Add(2 * defaultMinStakingDuration))

	if err := firstVM.Bootstrapped(); err != nil {
		t.Fatal(err)
	}

	if err := firstVM.Shutdown(); err != nil {
		t.Fatal(err)
	}
	firstCtx.Lock.Unlock()

	secondVM := &VM{
		SnowmanVM:        &core.SnowmanVM{},
		chainManager:     chains.MockManager{},
		uptimePercentage: .20,
	}

	secondVM.vdrMgr = validators.NewManager()

	secondVM.clock.Set(defaultValidateStartTime.Add(2 * defaultMinStakingDuration))
	secondCtx := defaultContext()
	secondCtx.Lock.Lock()
	defer func() {
		secondVM.Shutdown()
		secondCtx.Lock.Unlock()
	}()

	secondMsgChan := make(chan common.Message, 1)
	if err := secondVM.Initialize(secondCtx, db, genesisBytes, secondMsgChan, nil); err != nil {
		t.Fatal(err)
	}

	if err := secondVM.Bootstrapping(); err != nil {
		t.Fatal(err)
	}

	if err := secondVM.Bootstrapped(); err != nil {
		t.Fatal(err)
	}

	secondVM.clock.Set(defaultValidateEndTime)

	blk, err := secondVM.BuildBlock() // should contain proposal to advance time
	if err != nil {
		t.Fatal(err)
	} else if err := blk.Verify(); err != nil {
		t.Fatal(err)
	}

	// Assert preferences are correct
	block := blk.(*ProposalBlock)
	if options, err := block.Options(); err != nil {
		t.Fatal(err)
	} else if commit, ok := options[0].(*Commit); !ok {
		t.Fatal(errShouldPrefCommit)
	} else if abort, ok := options[1].(*Abort); !ok {
		t.Fatal(errShouldPrefCommit)
	} else if err := block.Accept(); err != nil {
		t.Fatal(err)
	} else if err := commit.Verify(); err != nil {
		t.Fatal(err)
	} else if err := abort.Verify(); err != nil {
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(abort.onAccept(), block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Aborted {
		t.Fatalf("status should be Aborted but is %s", status)
	} else if err := commit.Accept(); err != nil { // advance the timestamp
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(secondVM.DB, block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Committed {
		t.Fatalf("status should be Committed but is %s", status)
	} else if timestamp, err := secondVM.getTimestamp(secondVM.DB); err != nil { // Verify that chain's timestamp has advanced
		t.Fatal(err)
	} else if !timestamp.Equal(defaultValidateEndTime) {
		t.Fatal("expected timestamp to have advanced")
	}

	if blk, err = secondVM.BuildBlock(); err != nil { // should contain proposal to reward genesis validator
		t.Fatal(err)
	} else if err := blk.Verify(); err != nil {
		t.Fatal(err)
	}
	block = blk.(*ProposalBlock)
	if options, err := block.Options(); err != nil { // Assert preferences are correct
		t.Fatal(err)
	} else if commit, ok := options[0].(*Commit); !ok {
		t.Fatal(errShouldPrefCommit)
	} else if abort, ok := options[1].(*Abort); !ok {
		t.Fatal(errShouldPrefCommit)
	} else if err := blk.Accept(); err != nil {
		t.Fatal(err)
	} else if err := commit.Verify(); err != nil {
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(commit.onAccept(), block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Committed {
		t.Fatalf("status should be Committed but is %s", status)
	} else if err := abort.Verify(); err != nil {
		t.Fatal(err)
	} else if err := abort.Accept(); err != nil { // do not reward the genesis validator
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(secondVM.DB, block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Aborted {
		t.Fatalf("status should be Aborted but is %s", status)
	} else if _, isValidator, err := secondVM.isValidator(secondVM.DB, constants.PrimaryNetworkID, keys[1].PublicKey().Address()); err != nil {
		// Verify that genesis validator was removed from current validator set
		t.Fatal(err)
	} else if isValidator {
		t.Fatal("should have removed a genesis validator")
	}
}

func TestObservedUptime(t *testing.T) {
	_, genesisBytes := defaultGenesis()
	db := memdb.New()

	firstVM := &VM{
		SnowmanVM:          &core.SnowmanVM{},
		chainManager:       chains.MockManager{},
		uptimePercentage:   .2,
		stakeMintingPeriod: defaultMaxStakingDuration,
	}
	firstVM.vdrMgr = validators.NewManager()
	firstVM.clock.Set(defaultGenesisTime)

	firstCtx := defaultContext()
	firstCtx.Lock.Lock()

	firstMsgChan := make(chan common.Message, 1)
	if err := firstVM.Initialize(firstCtx, db, genesisBytes, firstMsgChan, nil); err != nil {
		t.Fatal(err)
	}

	if err := firstVM.Bootstrapping(); err != nil {
		t.Fatal(err)
	}

	if err := firstVM.Bootstrapped(); err != nil {
		t.Fatal(err)
	}
	firstCtx.Lock.Unlock()

	firstVM.Connected(keys[1].PublicKey().Address())

	firstCtx.Lock.Lock()
	// Fast forward clock to time for genesis validators to leave
	firstVM.clock.Set(defaultValidateStartTime.Add(2 * defaultMinStakingDuration))

	if err := firstVM.Shutdown(); err != nil {
		t.Fatal(err)
	}
	firstCtx.Lock.Unlock()

	secondVM := &VM{
		SnowmanVM:        &core.SnowmanVM{},
		chainManager:     chains.MockManager{},
		uptimePercentage: .20,
	}

	secondVM.vdrMgr = validators.NewManager()

	secondVM.clock.Set(defaultValidateStartTime.Add(2 * defaultMinStakingDuration))
	secondCtx := defaultContext()
	secondCtx.Lock.Lock()
	defer func() {
		secondVM.Shutdown()
		secondCtx.Lock.Unlock()
	}()

	secondMsgChan := make(chan common.Message, 1)
	if err := secondVM.Initialize(secondCtx, db, genesisBytes, secondMsgChan, nil); err != nil {
		t.Fatal(err)
	}

	if err := secondVM.Bootstrapping(); err != nil {
		t.Fatal(err)
	}

	if err := secondVM.Bootstrapped(); err != nil {
		t.Fatal(err)
	}

	secondVM.clock.Set(defaultValidateEndTime)

	blk, err := secondVM.BuildBlock() // should contain proposal to advance time
	if err != nil {
		t.Fatal(err)
	} else if err := blk.Verify(); err != nil {
		t.Fatal(err)
	}

	// Assert preferences are correct
	block := blk.(*ProposalBlock)
	if options, err := block.Options(); err != nil {
		t.Fatal(err)
	} else if commit, ok := options[0].(*Commit); !ok {
		t.Fatal(errShouldPrefCommit)
	} else if abort, ok := options[1].(*Abort); !ok {
		t.Fatal(errShouldPrefCommit)
	} else if err := block.Accept(); err != nil {
		t.Fatal(err)
	} else if err := commit.Verify(); err != nil {
		t.Fatal(err)
	} else if err := abort.Verify(); err != nil {
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(abort.onAccept(), block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Aborted {
		t.Fatalf("status should be Aborted but is %s", status)
	} else if err := commit.Accept(); err != nil { // advance the timestamp
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(secondVM.DB, block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Committed {
		t.Fatalf("status should be Committed but is %s", status)
	} else if timestamp, err := secondVM.getTimestamp(secondVM.DB); err != nil { // Verify that chain's timestamp has advanced
		t.Fatal(err)
	} else if !timestamp.Equal(defaultValidateEndTime) {
		t.Fatal("expected timestamp to have advanced")
	}

	if blk, err = secondVM.BuildBlock(); err != nil { // should contain proposal to reward genesis validator
		t.Fatal(err)
	} else if err := blk.Verify(); err != nil {
		t.Fatal(err)
	}
	block = blk.(*ProposalBlock)
	if options, err := block.Options(); err != nil { // Assert preferences are correct
		t.Fatal(err)
	} else if commit, ok := options[0].(*Commit); !ok {
		t.Fatal(errShouldPrefCommit)
	} else if abort, ok := options[1].(*Abort); !ok {
		t.Fatal(errShouldPrefCommit)
	} else if err := blk.Accept(); err != nil {
		t.Fatal(err)
	} else if err := commit.Verify(); err != nil {
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(commit.onAccept(), block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Committed {
		t.Fatalf("status should be Committed but is %s", status)
	} else if err := abort.Verify(); err != nil {
		t.Fatal(err)
	} else if err := abort.Accept(); err != nil { // do not reward the genesis validator
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(secondVM.DB, block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Aborted {
		t.Fatalf("status should be Aborted but is %s", status)
	} else if _, isValidator, err := secondVM.isValidator(secondVM.DB, constants.PrimaryNetworkID, keys[1].PublicKey().Address()); err != nil {
		// Verify that genesis validator was removed from current validator set
		t.Fatal(err)
	} else if isValidator {
		t.Fatal("should have removed a genesis validator")
	}
}

func TestUptimeDisallowed(t *testing.T) {
	_, genesisBytes := defaultGenesis()
	db := memdb.New()

	firstVM := &VM{
		SnowmanVM:          &core.SnowmanVM{},
		chainManager:       chains.MockManager{},
		uptimePercentage:   .2,
		stakeMintingPeriod: defaultMaxStakingDuration,
	}
	firstVM.vdrMgr = validators.NewManager()
	firstVM.clock.Set(defaultGenesisTime)

	firstCtx := defaultContext()
	firstCtx.Lock.Lock()

	firstMsgChan := make(chan common.Message, 1)
	if err := firstVM.Initialize(firstCtx, db, genesisBytes, firstMsgChan, nil); err != nil {
		t.Fatal(err)
	}

	// Fast forward clock to time for genesis validators to leave
	firstVM.clock.Set(defaultValidateStartTime.Add(2 * defaultMinStakingDuration))

	if err := firstVM.Bootstrapping(); err != nil {
		t.Fatal(err)
	}

	if err := firstVM.Bootstrapped(); err != nil {
		t.Fatal(err)
	}

	if err := firstVM.Shutdown(); err != nil {
		t.Fatal(err)
	}
	firstCtx.Lock.Unlock()

	secondVM := &VM{
		SnowmanVM:        &core.SnowmanVM{},
		chainManager:     chains.MockManager{},
		uptimePercentage: .21,
	}

	secondVM.vdrMgr = validators.NewManager()

	secondVM.clock.Set(defaultValidateStartTime.Add(2 * defaultMinStakingDuration))
	secondCtx := defaultContext()
	secondCtx.Lock.Lock()
	defer func() {
		secondVM.Shutdown()
		secondCtx.Lock.Unlock()
	}()

	secondMsgChan := make(chan common.Message, 1)
	if err := secondVM.Initialize(secondCtx, db, genesisBytes, secondMsgChan, nil); err != nil {
		t.Fatal(err)
	}

	if err := secondVM.Bootstrapping(); err != nil {
		t.Fatal(err)
	}

	if err := secondVM.Bootstrapped(); err != nil {
		t.Fatal(err)
	}

	secondVM.clock.Set(defaultValidateEndTime)

	blk, err := secondVM.BuildBlock() // should contain proposal to advance time
	if err != nil {
		t.Fatal(err)
	} else if err := blk.Verify(); err != nil {
		t.Fatal(err)
	}

	// Assert preferences are correct
	block := blk.(*ProposalBlock)
	if options, err := block.Options(); err != nil {
		t.Fatal(err)
	} else if commit, ok := options[0].(*Commit); !ok {
		t.Fatal(errShouldPrefCommit)
	} else if abort, ok := options[1].(*Abort); !ok {
		t.Fatal(errShouldPrefCommit)
	} else if err := block.Accept(); err != nil {
		t.Fatal(err)
	} else if err := commit.Verify(); err != nil {
		t.Fatal(err)
	} else if err := abort.Verify(); err != nil {
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(abort.onAccept(), block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Aborted {
		t.Fatalf("status should be Aborted but is %s", status)
	} else if err := commit.Accept(); err != nil { // advance the timestamp
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(secondVM.DB, block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Committed {
		t.Fatalf("status should be Committed but is %s", status)
	} else if timestamp, err := secondVM.getTimestamp(secondVM.DB); err != nil { // Verify that chain's timestamp has advanced
		t.Fatal(err)
	} else if !timestamp.Equal(defaultValidateEndTime) {
		t.Fatal("expected timestamp to have advanced")
	}

	if blk, err = secondVM.BuildBlock(); err != nil { // should contain proposal to reward genesis validator
		t.Fatal(err)
	} else if err := blk.Verify(); err != nil {
		t.Fatal(err)
	}
	block = blk.(*ProposalBlock)
	if options, err := block.Options(); err != nil { // Assert preferences are correct
		t.Fatal(err)
	} else if commit, ok := options[1].(*Commit); !ok {
		t.Fatal(errShouldPrefAbort)
	} else if abort, ok := options[0].(*Abort); !ok {
		t.Fatal(errShouldPrefAbort)
	} else if err := blk.Accept(); err != nil {
		t.Fatal(err)
	} else if err := commit.Verify(); err != nil {
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(commit.onAccept(), block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Committed {
		t.Fatalf("status should be Committed but is %s", status)
	} else if err := abort.Verify(); err != nil {
		t.Fatal(err)
	} else if err := abort.Accept(); err != nil { // do not reward the genesis validator
		t.Fatal(err)
	} else if status, err := secondVM.getStatus(secondVM.DB, block.Tx.ID()); err != nil {
		t.Fatal(err)
	} else if status != Aborted {
		t.Fatalf("status should be Aborted but is %s", status)
	} else if _, isValidator, err := secondVM.isValidator(secondVM.DB, constants.PrimaryNetworkID, keys[1].PublicKey().Address()); err != nil {
		// Verify that genesis validator was removed from current validator set
		t.Fatal(err)
	} else if isValidator {
		t.Fatal("should have removed a genesis validator")
	}
}
