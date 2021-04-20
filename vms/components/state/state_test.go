// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"testing"

	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

// toy example of a block, just used for testing
type block struct {
	parentID ids.ID
	value    uint64
}

const blockSize = 40 // hashing.HashLen (32) + length of uin64 (8)

func (b *block) Bytes() ([]byte, error) {
	p := wrappers.Packer{Bytes: make([]byte, blockSize)}
	p.PackFixedBytes(b.parentID[:])
	p.PackLong(b.value)
	return p.Bytes, p.Err
}

func marshalBlock(blk interface{}) ([]byte, error) {
	return blk.(*block).Bytes()
}

func unmarshalBlock(bytes []byte) (interface{}, error) {
	p := wrappers.Packer{Bytes: bytes}

	parentID, err := ids.ToID(p.UnpackFixedBytes(hashing.HashLen))
	if err != nil {
		return nil, err
	}

	value := p.UnpackLong()

	if p.Errored() {
		return nil, p.Err
	}

	return &block{
		parentID: parentID,
		value:    value,
	}, nil
}

// toy example of an account, just used for testing
type account struct {
	id      ids.ID
	balance uint64
	nonce   uint64
}

const accountSize = 32 + 8 + 8

func (acc *account) Bytes() ([]byte, error) {
	p := wrappers.Packer{Bytes: make([]byte, accountSize)}
	p.PackFixedBytes(acc.id[:])
	p.PackLong(acc.balance)
	p.PackLong(acc.nonce)
	return p.Bytes, p.Err
}

func marshalAccount(acct interface{}) ([]byte, error) {
	return acct.(*account).Bytes()
}

func unmarshalAccount(bytes []byte) (interface{}, error) {
	p := wrappers.Packer{Bytes: bytes}

	id, err := ids.ToID(p.UnpackFixedBytes(hashing.HashLen))
	if err != nil {
		return nil, err
	}

	balance := p.UnpackLong()
	nonce := p.UnpackLong()

	if p.Errored() {
		return nil, p.Err
	}

	return &account{
		id:      id,
		balance: balance,
		nonce:   nonce,
	}, nil
}

// Ensure there is an error if someone tries to do a put without registering the type
func TestPutUnregistered(t *testing.T) {
	// make a state and a database
	state, err := NewState()
	if err != nil {
		t.Fatal(err)
	}
	db := memdb.New()

	// make an account
	acc1 := &account{
		id:      ids.ID{1, 2, 3},
		balance: 1,
		nonce:   2,
	}

	if err := state.Put(db, 1, ids.ID{1, 2, 3}, acc1); err == nil {
		t.Fatal("should have failed because type ID is unregistred")
	}

	// register type
	if err := state.RegisterType(1, marshalAccount, unmarshalAccount); err != nil {
		t.Fatal(err)
	}

	// should not error now
	if err := state.Put(db, 1, ids.ID{1, 2, 3}, acc1); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// Ensure there is an error if someone tries to get the value associated with a
// key that doesn't exist
func TestKeyDoesNotExist(t *testing.T) {
	// make a state and a database
	state, err := NewState()
	if err != nil {
		t.Fatal(err)
	}
	db := memdb.New()

	if _, err := state.Get(db, 1, ids.ID{1, 2, 3}); err == nil {
		t.Fatal("should have failed because no such key or typeID exists")
	}

	// register type with ID 1
	typeID := uint64(1)
	if err := state.RegisterType(typeID, marshalAccount, unmarshalAccount); err != nil {
		t.Fatal(err)
	}

	// Should still fail because there is no value with this key
	if _, err := state.Get(db, typeID, ids.ID{1, 2, 3}); err == nil {
		t.Fatal("should have failed because no such key exists")
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// Ensure there is an error if someone tries to register a type ID that already exists
func TestRegisterExistingTypeID(t *testing.T) {
	// make a state and a database
	state, err := NewState()
	if err != nil {
		t.Fatal(err)
	}
	db := memdb.New()

	// register type with ID 1
	typeID := uint64(1)
	if err := state.RegisterType(typeID, marshalBlock, unmarshalBlock); err != nil {
		t.Fatal(err)
	}

	// try to register the same type ID
	if err := state.RegisterType(typeID, marshalAccount, unmarshalAccount); err == nil {
		t.Fatal("Should have errored because typeID already registered")
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// Ensure there is an error when someone tries to get a value using the wrong typeID
func TestGetWrongTypeID(t *testing.T) {
	// make a state and a database
	state, err := NewState()
	if err != nil {
		t.Fatal(err)
	}
	db := memdb.New()

	// register type with ID 1
	blockTypeID := uint64(1)
	if err := state.RegisterType(blockTypeID, marshalBlock, unmarshalBlock); err != nil {
		t.Fatal(err)
	}

	// make and put a block
	block := &block{
		parentID: ids.ID{4, 5, 6},
		value:    5,
	}
	blockID := ids.ID{1, 2, 3}
	if err = state.Put(db, blockTypeID, blockID, block); err != nil {
		t.Fatal(err)
	}

	// try to get it using the right key but wrong typeID
	if _, err := state.Get(db, 2, blockID); err == nil {
		t.Fatal("should have failed because type ID is wrong")
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// Ensure that there is no error when someone puts two values with the same
// key but different type IDs
func TestSameKeyDifferentTypeID(t *testing.T) {
	// make a state and a database
	state, err := NewState()
	if err != nil {
		t.Fatal(err)
	}
	db := memdb.New()

	// register block type with ID 1
	blockTypeID := uint64(1)
	if err := state.RegisterType(blockTypeID, marshalBlock, unmarshalBlock); err != nil {
		t.Fatal(err)
	}

	// register account type with ID 2
	accountTypeID := uint64(2)
	if err := state.RegisterType(accountTypeID, marshalAccount, unmarshalAccount); err != nil {
		t.Fatal(err)
	}

	sharedKey := ids.ID{1, 2, 3}

	// make an account
	acc := &account{
		id:      ids.ID{1, 2, 3},
		balance: 1,
		nonce:   2,
	}

	// put it using sharedKey
	if err = state.Put(db, accountTypeID, sharedKey, acc); err != nil {
		t.Fatal(err)
	}

	// make a block
	block1 := &block{
		parentID: ids.ID{4, 5, 6},
		value:    5,
	}

	// put it using sharedKey
	if err = state.Put(db, blockTypeID, sharedKey, block1); err != nil {
		t.Fatal(err)
	}

	// ensure the account is still there and correct
	accInterface, err := state.Get(db, accountTypeID, sharedKey)
	if err != nil {
		t.Fatal(err)
	}
	accFromState, ok := accInterface.(*account)
	switch {
	case !ok:
		t.Fatal("should have been type *account")
	case accFromState.balance != acc.balance:
		t.Fatal("balances should be same")
	case accFromState.id != acc.id:
		t.Fatal("ids should be the same")
	case accFromState.nonce != acc.nonce:
		t.Fatal("nonces should be same")
	}

	// ensure the block is still there and correct
	blockInterface, err := state.Get(db, blockTypeID, sharedKey)
	if err != nil {
		t.Fatal(err)
	}

	blockFromState, ok := blockInterface.(*block)
	switch {
	case !ok:
		t.Fatal("should have been type *block")
	case blockFromState.parentID != block1.parentID:
		t.Fatal("parentIDs should be same")
	case blockFromState.value != block1.value:
		t.Fatal("values should be same")
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// Ensure that overwriting a value works
func TestOverwrite(t *testing.T) {
	// make a state and a database
	state, err := NewState()
	if err != nil {
		t.Fatal(err)
	}
	db := memdb.New()

	// register block type with ID 1
	blockTypeID := uint64(1)
	if err := state.RegisterType(blockTypeID, marshalBlock, unmarshalBlock); err != nil {
		t.Fatal(err)
	}

	// make a block
	block1 := &block{
		parentID: ids.ID{4, 5, 6},
		value:    5,
	}

	key := ids.ID{1, 2, 3}

	// put it
	if err = state.Put(db, blockTypeID, key, block1); err != nil {
		t.Fatal(err)
	}

	// make another block
	block2 := &block{
		parentID: ids.ID{100, 200, 1},
		value:    6,
	}

	// put it with the same key
	if err = state.Put(db, blockTypeID, key, block2); err != nil {
		t.Fatal(err)
	}

	// ensure the first value was over-written
	// get it and make sure it's right
	blockInterface, err := state.Get(db, blockTypeID, key)
	if err != nil {
		t.Fatal(err)
	}

	blockFromState, ok := blockInterface.(*block)
	switch {
	case !ok:
		t.Fatal("should have been type *block")
	case blockFromState.parentID != block2.parentID:
		t.Fatal("parentIDs should be same")
	case blockFromState.value != block2.value:
		t.Fatal("values should be same")
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// Put 4 values, 2 of one type and 2 of another
func TestHappyPath(t *testing.T) {
	// make a state and a database
	state, err := NewState()
	if err != nil {
		t.Fatal(err)
	}
	db := memdb.New()

	accountTypeID := uint64(1)

	// register type account
	if err := state.RegisterType(accountTypeID, marshalAccount, unmarshalAccount); err != nil {
		t.Fatal(err)
	}

	// make an account
	acc1 := &account{
		id:      ids.ID{1, 2, 3},
		balance: 1,
		nonce:   2,
	}

	// put it
	if err = state.Put(db, accountTypeID, acc1.id, acc1); err != nil {
		t.Fatal(err)
	}

	// get it and make sure it's right
	acc1Interface, err := state.Get(db, accountTypeID, acc1.id)
	if err != nil {
		t.Fatal(err)
	}

	acc1FromState, ok := acc1Interface.(*account)
	switch {
	case !ok:
		t.Fatal("should have been type *account")
	case acc1FromState.balance != acc1.balance:
		t.Fatal("balances should be same")
	case acc1FromState.id != acc1.id:
		t.Fatal("ids should be the same")
	case acc1FromState.nonce != acc1.nonce:
		t.Fatal("nonces should be same")
	}

	// make another account
	acc2 := &account{
		id:      ids.ID{9, 2, 1},
		balance: 7,
		nonce:   44,
	}

	// put it
	if err = state.Put(db, accountTypeID, acc2.id, acc2); err != nil {
		t.Fatal(err)
	}

	// get it and make sure it's right
	acc2Interface, err := state.Get(db, accountTypeID, acc2.id)
	if err != nil {
		t.Fatal(err)
	}

	acc2FromState, ok := acc2Interface.(*account)
	switch {
	case !ok:
		t.Fatal("should have been type *account")
	case acc2FromState.balance != acc2.balance:
		t.Fatal("balances should be same")
	case acc2FromState.id != acc2.id:
		t.Fatal("ids should be the same")
	case acc2FromState.nonce != acc2.nonce:
		t.Fatal("nonces should be same")
	}

	// register type block
	blockTypeID := uint64(2)
	if err := state.RegisterType(blockTypeID, marshalBlock, unmarshalBlock); err != nil {
		t.Fatal(err)
	}

	// make a block
	block1ID := ids.ID{9, 9, 9}
	block1 := &block{
		parentID: ids.ID{4, 5, 6},
		value:    5,
	}

	// put it
	if err = state.Put(db, blockTypeID, block1ID, block1); err != nil {
		t.Fatal(err)
	}

	// get it and make sure it's right
	block1Interface, err := state.Get(db, blockTypeID, block1ID)
	if err != nil {
		t.Fatal(err)
	}

	block1FromState, ok := block1Interface.(*block)
	switch {
	case !ok:
		t.Fatal("should have been type *block")
	case block1FromState.parentID != block1.parentID:
		t.Fatal("parentIDs should be same")
	case block1FromState.value != block1.value:
		t.Fatal("values should be same")
	}

	// make another block
	block2ID := ids.ID{1, 2, 3, 4, 5, 6, 7, 8, 9}
	block2 := &block{
		parentID: ids.ID{10, 1, 2},
		value:    67,
	}

	// put it
	if err = state.Put(db, blockTypeID, block2ID, block2); err != nil {
		t.Fatal(err)
	}

	// get it and make sure it's right
	block2Interface, err := state.Get(db, blockTypeID, block2ID)
	if err != nil {
		t.Fatal(err)
	}

	block2FromState, ok := block2Interface.(*block)
	switch {
	case !ok:
		t.Fatal("should have been type *block")
	case block2FromState.parentID != block2.parentID:
		t.Fatal("parentIDs should be same")
	case block2FromState.value != block2.value:
		t.Fatal("values should be same")
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}
