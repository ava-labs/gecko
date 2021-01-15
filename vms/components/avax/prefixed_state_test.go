// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avax

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/ids"
)

func TestPrefixedFunds(t *testing.T) {
	c := linearcodec.NewDefault()
	codecVersion := uint16(1)
	if err := c.RegisterType(&TestAddressable{}); err != nil {
		t.Fatal(err)
	}
	manager := codec.NewDefaultManager()
	if err := manager.RegisterCodec(codecVersion, c); err != nil {
		t.Fatal(err)
	}

	chain0ID := ids.Empty.Prefix(0)
	chain1ID := ids.Empty.Prefix(1)

	db := memdb.New()

	st0 := NewPrefixedState(db, manager, manager, func() uint16 { return codecVersion }, chain0ID, chain1ID)
	st1 := NewPrefixedState(db, manager, manager, func() uint16 { return codecVersion }, chain1ID, chain0ID)

	addr := ids.GenerateTestShortID()
	addrBytes := addr.Bytes()

	utxo := &UTXO{
		UTXOID: UTXOID{
			TxID:        ids.Empty,
			OutputIndex: 0,
		},
		Asset: Asset{
			ID: ids.Empty,
		},
		Out: &TestAddressable{
			Addrs: [][]byte{
				addrBytes,
			},
		},
	}

	assert.NoError(t, st0.FundUTXO(utxo))

	utxoIDs, err := st1.Funds(addr.Bytes(), ids.Empty, math.MaxInt32)
	assert.NoError(t, err)
	assert.Equal(t, []ids.ID{utxo.InputID()}, utxoIDs)

	assert.NoError(t, st1.SpendUTXO(utxo.InputID()))

	utxoIDs, err = st1.Funds(addr.Bytes(), ids.Empty, math.MaxInt32)
	assert.NoError(t, err)
	assert.Len(t, utxoIDs, 0)
}
