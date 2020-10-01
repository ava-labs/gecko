// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto"
)

func TestTxHeapStart(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	txHeap := EventHeap{SortByStartTime: true}

	validator0, err := vm.newAddValidatorTx(
		vm.minValidatorStake,                                               // stake amount
		uint64(defaultGenesisTime.Unix()+1),                                // startTime
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()+1), // endTime
		ids.NewShortID([20]byte{}),                                         // node ID
		ids.NewShortID([20]byte{1, 2, 3, 4, 5, 6, 7}),                      // reward address
		0,                                       // shares
		[]*crypto.PrivateKeySECP256K1R{keys[0]}, // key
		ids.ShortEmpty,                          // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	vdr0Tx := validator0.UnsignedTx.(*UnsignedAddValidatorTx)

	validator1, err := vm.newAddValidatorTx(
		vm.minValidatorStake,                                               // stake amount
		uint64(defaultGenesisTime.Unix()+2),                                // startTime
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()+2), // endTime
		ids.NewShortID([20]byte{1}),                                        // node ID
		ids.NewShortID([20]byte{1, 2, 3, 4, 5, 6, 7}),                      // reward address
		0,                                       // shares
		[]*crypto.PrivateKeySECP256K1R{keys[0]}, // key
		ids.ShortEmpty,                          // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	vdr1Tx := validator1.UnsignedTx.(*UnsignedAddValidatorTx)

	validator2, err := vm.newAddValidatorTx(
		vm.minValidatorStake,                                               // stake amount
		uint64(defaultGenesisTime.Unix()+3),                                // startTime
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()+3), // endTime
		ids.NewShortID([20]byte{}),                                         // node ID
		ids.NewShortID([20]byte{1, 2, 3, 4, 5, 6, 7}),                      // reward address
		0,                                       // shares
		[]*crypto.PrivateKeySECP256K1R{keys[0]}, // key
		ids.ShortEmpty,                          // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	vdr2Tx := validator2.UnsignedTx.(*UnsignedAddValidatorTx)

	txHeap.Add(validator2)
	if timestamp := txHeap.Timestamp(); !timestamp.Equal(vdr2Tx.StartTime()) {
		t.Fatalf("TxHeap.Timestamp returned %s, expected %s", timestamp, vdr2Tx.StartTime())
	}

	txHeap.Add(validator1)
	if timestamp := txHeap.Timestamp(); !timestamp.Equal(vdr1Tx.StartTime()) {
		t.Fatalf("TxHeap.Timestamp returned %s, expected %s", timestamp, vdr1Tx.StartTime())
	}

	txHeap.Add(validator0)
	if timestamp := txHeap.Timestamp(); !timestamp.Equal(vdr0Tx.StartTime()) {
		t.Fatalf("TxHeap.Timestamp returned %s, expected %s", timestamp, vdr0Tx.StartTime())
	} else if top := txHeap.Peek(); !top.ID().Equals(validator0.ID()) {
		t.Fatalf("TxHeap prioritized %s, expected %s", top.ID(), validator0.ID())
	}
}

func TestTxHeapStop(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	txHeap := EventHeap{}

	validator0, err := vm.newAddValidatorTx(
		vm.minValidatorStake,                                               // stake amount
		uint64(defaultGenesisTime.Unix()+1),                                // startTime
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()+1), // endTime
		ids.NewShortID([20]byte{}),                                         // node ID
		ids.NewShortID([20]byte{1, 2, 3, 4, 5, 6, 7}),                      // reward address
		0,                                       // shares
		[]*crypto.PrivateKeySECP256K1R{keys[0]}, // key
		ids.ShortEmpty,                          // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	vdr0Tx := validator0.UnsignedTx.(*UnsignedAddValidatorTx)

	validator1, err := vm.newAddValidatorTx(
		vm.minValidatorStake,                                               // stake amount
		uint64(defaultGenesisTime.Unix()+1),                                // startTime
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()+2), // endTime
		ids.NewShortID([20]byte{1}),                                        // node ID
		ids.NewShortID([20]byte{1, 2, 3, 4, 5, 6, 7}),                      // reward address
		0,                                       // shares
		[]*crypto.PrivateKeySECP256K1R{keys[0]}, // key
		ids.ShortEmpty,                          // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	vdr1Tx := validator1.UnsignedTx.(*UnsignedAddValidatorTx)

	validator2, err := vm.newAddValidatorTx(
		vm.minValidatorStake,                                               // stake amount
		uint64(defaultGenesisTime.Unix()+1),                                // startTime
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()+3), // endTime
		ids.NewShortID([20]byte{}),                                         // node ID
		ids.NewShortID([20]byte{1, 2, 3, 4, 5, 6, 7}),                      // reward address
		0,                                       // shares
		[]*crypto.PrivateKeySECP256K1R{keys[0]}, // key
		ids.ShortEmpty,                          // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	vdr2Tx := validator2.UnsignedTx.(*UnsignedAddValidatorTx)

	txHeap.Add(validator2)
	if timestamp := txHeap.Timestamp(); !timestamp.Equal(vdr2Tx.EndTime()) {
		t.Fatalf("TxHeap.Timestamp returned %s, expected %s", timestamp, vdr2Tx.EndTime())
	}

	txHeap.Add(validator1)
	if timestamp := txHeap.Timestamp(); !timestamp.Equal(vdr1Tx.EndTime()) {
		t.Fatalf("TxHeap.Timestamp returned %s, expected %s", timestamp, vdr1Tx.EndTime())
	}

	txHeap.Add(validator0)
	if timestamp := txHeap.Timestamp(); !timestamp.Equal(vdr0Tx.EndTime()) {
		t.Fatalf("TxHeap.Timestamp returned %s, expected %s", timestamp, vdr0Tx.EndTime())
	} else if top := txHeap.Txs[0]; !top.ID().Equals(validator0.ID()) {
		t.Fatalf("TxHeap prioritized %s, expected %s", top.ID(), validator0.ID())
	}
}

func TestTxHeapStartValidatorVsDelegatorOrdering(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	txHeap := EventHeap{SortByStartTime: true}

	validator, err := vm.newAddValidatorTx(
		vm.minValidatorStake,                                               // stake amount
		uint64(defaultGenesisTime.Unix()+1),                                // startTime
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()+1), // endTime
		ids.NewShortID([20]byte{}),                                         // node ID
		ids.NewShortID([20]byte{1, 2, 3, 4, 5, 6, 7}),                      // reward address
		0,                                       // shares
		[]*crypto.PrivateKeySECP256K1R{keys[0]}, // key
		ids.ShortEmpty,                          // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	delegator, err := vm.newAddDelegatorTx(
		vm.minValidatorStake,                                               // stake amount
		uint64(defaultGenesisTime.Unix()+1),                                // startTime
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()+1), // endTime
		ids.NewShortID([20]byte{}),                                         // node ID
		ids.NewShortID([20]byte{1, 2, 3, 4, 5, 6, 7}),                      // reward address
		[]*crypto.PrivateKeySECP256K1R{keys[0]},                            // key
		ids.ShortEmpty,                                                     // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	txHeap.Add(validator)
	txHeap.Add(delegator)

	if top := txHeap.Txs[0]; !top.ID().Equals(validator.ID()) {
		t.Fatalf("TxHeap prioritized %s, expected %s", top.ID(), validator.ID())
	}
}

func TestTxHeapStopValidatorVsDelegatorOrdering(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	txHeap := EventHeap{}

	validator, err := vm.newAddValidatorTx(
		vm.minValidatorStake,                                               // stake amount
		uint64(defaultGenesisTime.Unix()+1),                                // startTime
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()+1), // endTime
		ids.NewShortID([20]byte{}),                                         // node ID
		ids.NewShortID([20]byte{1, 2, 3, 4, 5, 6, 7}),                      // reward address
		0,                                       // shares
		[]*crypto.PrivateKeySECP256K1R{keys[0]}, // key
		ids.ShortEmpty,                          // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	delegator, err := vm.newAddDelegatorTx(
		vm.minValidatorStake,                                               // stake amount
		uint64(defaultGenesisTime.Unix()+1),                                // startTime
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()+1), // endTime
		ids.NewShortID([20]byte{}),                                         // node ID
		ids.NewShortID([20]byte{1, 2, 3, 4, 5, 6, 7}),                      // reward address
		[]*crypto.PrivateKeySECP256K1R{keys[0]},                            // key
		ids.ShortEmpty,                                                     // change addr

	)
	if err != nil {
		t.Fatal(err)
	}

	txHeap.Add(validator)
	txHeap.Add(delegator)

	if top := txHeap.Txs[0]; !top.ID().Equals(delegator.ID()) {
		t.Fatalf("TxHeap prioritized %s, expected %s", top.ID(), delegator.ID())
	}
}
