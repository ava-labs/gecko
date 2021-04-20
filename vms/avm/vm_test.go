// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/api/keystore"
	"github.com/ava-labs/avalanchego/chains/atomic"
	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/database/mockdb"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/nftfx"
	"github.com/ava-labs/avalanchego/vms/propertyfx"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/stretchr/testify/assert"
)

var networkID uint32 = 10
var chainID = ids.ID{5, 4, 3, 2, 1}
var platformChainID = ids.Empty.Prefix(0)
var testTxFee = uint64(1000)
var startBalance = uint64(50000)

var keys []*crypto.PrivateKeySECP256K1R
var addrs []ids.ShortID // addrs[i] corresponds to keys[i]

var assetID = ids.ID{1, 2, 3}
var username = "bobby"
var password = "StrnasfqewiurPasswdn56d" // #nosec G101

func init() {
	factory := crypto.FactorySECP256K1R{}

	for _, key := range []string{
		"24jUJ9vZexUM6expyMcT48LBx27k1m7xpraoV62oSQAHdziao5",
		"2MMvUMsxx6zsHSNXJdFD8yc5XkancvwyKPwpw4xUK3TCGDuNBY",
		"cxb7KpGWhDMALTjNNSJ7UQkkomPesyWAPUaWRGdyeBNzR6f35",
	} {
		keyBytes, _ := formatting.Decode(formatting.CB58, key)
		pk, _ := factory.ToPrivateKey(keyBytes)
		keys = append(keys, pk.(*crypto.PrivateKeySECP256K1R))
		addrs = append(addrs, pk.PublicKey().Address())
	}
}

type snLookup struct {
	chainsToSubnet map[ids.ID]ids.ID
}

func (sn *snLookup) SubnetID(chainID ids.ID) (ids.ID, error) {
	subnetID, ok := sn.chainsToSubnet[chainID]
	if !ok {
		return ids.ID{}, errors.New("")
	}
	return subnetID, nil
}

func NewContext(tb testing.TB) *snow.Context {
	genesisBytes := BuildGenesisTest(tb)
	tx := GetAVAXTxFromGenesisTest(genesisBytes, tb)

	ctx := snow.DefaultContextTest()
	ctx.NetworkID = networkID
	ctx.ChainID = chainID
	ctx.AVAXAssetID = tx.ID()
	ctx.XChainID = ids.Empty.Prefix(0)
	aliaser := ctx.BCLookup.(*ids.Aliaser)

	errs := wrappers.Errs{}
	errs.Add(
		aliaser.Alias(chainID, "X"),
		aliaser.Alias(chainID, chainID.String()),
		aliaser.Alias(platformChainID, "P"),
		aliaser.Alias(platformChainID, platformChainID.String()),
	)
	if errs.Errored() {
		tb.Fatal(errs.Err)
	}

	sn := &snLookup{
		chainsToSubnet: make(map[ids.ID]ids.ID),
	}
	sn.chainsToSubnet[chainID] = ctx.SubnetID
	sn.chainsToSubnet[platformChainID] = ctx.SubnetID
	ctx.SNLookup = sn
	return ctx
}

// Returns:
//   1) tx in genesis that creates AVAX
//   2) the index of the output
func GetAVAXTxFromGenesisTest(genesisBytes []byte, tb testing.TB) *Tx {
	_, c := setupCodec()
	genesis := Genesis{}
	if _, err := c.Unmarshal(genesisBytes, &genesis); err != nil {
		tb.Fatal(err)
	}

	if len(genesis.Txs) == 0 {
		tb.Fatal("genesis tx didn't have any txs")
	}

	var avaxTx *GenesisAsset
	for _, tx := range genesis.Txs {
		if tx.Name == "AVAX" {
			avaxTx = tx
			break
		}
	}
	if avaxTx == nil {
		tb.Fatal("there is no AVAX tx")
	}

	tx := Tx{
		UnsignedTx: &avaxTx.CreateAssetTx,
	}
	if err := tx.SignSECP256K1Fx(c, nil); err != nil {
		tb.Fatal(err)
	}

	return &tx
}

// BuildGenesisTest is the common Genesis builder for most tests
func BuildGenesisTest(tb testing.TB) []byte {
	addr0Str, _ := formatting.FormatBech32(testHRP, addrs[0].Bytes())
	addr1Str, _ := formatting.FormatBech32(testHRP, addrs[1].Bytes())
	addr2Str, _ := formatting.FormatBech32(testHRP, addrs[2].Bytes())

	defaultArgs := &BuildGenesisArgs{
		Encoding: formatting.Hex,
		GenesisData: map[string]AssetDefinition{
			"asset1": {
				Name:   "AVAX",
				Symbol: "SYMB",
				InitialState: map[string][]interface{}{
					"fixedCap": {
						Holder{
							Amount:  json.Uint64(startBalance),
							Address: addr0Str,
						},
						Holder{
							Amount:  json.Uint64(startBalance),
							Address: addr1Str,
						},
						Holder{
							Amount:  json.Uint64(startBalance),
							Address: addr2Str,
						},
					},
				},
			},
			"asset2": {
				Name:   "myVarCapAsset",
				Symbol: "MVCA",
				InitialState: map[string][]interface{}{
					"variableCap": {
						Owners{
							Threshold: 1,
							Minters: []string{
								addr0Str,
								addr1Str,
							},
						},
						Owners{
							Threshold: 2,
							Minters: []string{
								addr0Str,
								addr1Str,
								addr2Str,
							},
						},
					},
				},
			},
			"asset3": {
				Name: "myOtherVarCapAsset",
				InitialState: map[string][]interface{}{
					"variableCap": {
						Owners{
							Threshold: 1,
							Minters: []string{
								addr0Str,
							},
						},
					},
				},
			},
		}}

	return BuildGenesisTestWithArgs(tb, defaultArgs)
}

// BuildGenesisTestWithArgs allows building the genesis while injecting different starting points (args)
func BuildGenesisTestWithArgs(tb testing.TB, args *BuildGenesisArgs) []byte {
	ss := CreateStaticService()

	reply := BuildGenesisReply{}
	err := ss.BuildGenesis(nil, args, &reply)
	if err != nil {
		tb.Fatal(err)
	}

	b, err := formatting.Decode(reply.Encoding, reply.Bytes)
	if err != nil {
		tb.Fatal(err)
	}

	return b
}

func GenesisVM(tb testing.TB) ([]byte, chan common.Message, *VM, *atomic.Memory) {
	return GenesisVMWithArgs(tb, nil)
}

func GenesisVMWithArgs(tb testing.TB, args *BuildGenesisArgs) ([]byte, chan common.Message, *VM, *atomic.Memory) {
	var genesisBytes []byte

	if args != nil {
		genesisBytes = BuildGenesisTestWithArgs(tb, args)
	} else {
		genesisBytes = BuildGenesisTest(tb)
	}

	ctx := NewContext(tb)

	baseDB := memdb.New()

	m := &atomic.Memory{}
	err := m.Initialize(logging.NoLog{}, prefixdb.New([]byte{0}, baseDB))
	if err != nil {
		tb.Fatal(err)
	}
	ctx.SharedMemory = m.NewSharedMemory(ctx.ChainID)

	// NB: this lock is intentionally left locked when this function returns.
	// The caller of this function is responsible for unlocking.
	ctx.Lock.Lock()

	userKeystore := keystore.New(logging.NoLog{}, memdb.New())
	if err := userKeystore.CreateUser(username, password); err != nil {
		tb.Fatal(err)
	}
	ctx.Keystore = userKeystore.NewBlockchainKeyStore(ctx.ChainID)

	issuer := make(chan common.Message, 1)
	vm := &VM{
		txFee:         testTxFee,
		creationTxFee: testTxFee,
	}
	err = vm.Initialize(
		ctx,
		prefixdb.New([]byte{1}, baseDB),
		genesisBytes,
		issuer,
		[]*common.Fx{
			{
				ID: ids.Empty,
				Fx: &secp256k1fx.Fx{},
			},
			{
				ID: nftfx.ID,
				Fx: &nftfx.Fx{},
			},
		},
	)
	if err != nil {
		tb.Fatal(err)
	}
	vm.batchTimeout = 0

	if err := vm.Bootstrapping(); err != nil {
		tb.Fatal(err)
	}

	if err := vm.Bootstrapped(); err != nil {
		tb.Fatal(err)
	}

	return genesisBytes, issuer, vm, m
}

func NewTx(t *testing.T, genesisBytes []byte, vm *VM) *Tx {
	avaxTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	newTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        avaxTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: avaxTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
	}}}
	if err := newTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}
	return newTx
}

func TestTxSerialization(t *testing.T) {
	expected := []byte{
		// Codec version:
		0x00, 0x00,
		// txID:
		0x00, 0x00, 0x00, 0x01,
		// networkID:
		0x00, 0x00, 0x00, 0x0a,
		// chainID:
		0x05, 0x04, 0x03, 0x02, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// number of outs:
		0x00, 0x00, 0x00, 0x03,
		// output[0]:
		// assetID:
		0x01, 0x02, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// fxID:
		0x00, 0x00, 0x00, 0x07,
		// secp256k1 Transferable Output:
		// amount:
		0x00, 0x00, 0x12, 0x30, 0x9c, 0xe5, 0x40, 0x00,
		// locktime:
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// threshold:
		0x00, 0x00, 0x00, 0x01,
		// number of addresses
		0x00, 0x00, 0x00, 0x01,
		// address[0]
		0xfc, 0xed, 0xa8, 0xf9, 0x0f, 0xcb, 0x5d, 0x30,
		0x61, 0x4b, 0x99, 0xd7, 0x9f, 0xc4, 0xba, 0xa2,
		0x93, 0x07, 0x76, 0x26,
		// output[1]:
		// assetID:
		0x01, 0x02, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// fxID:
		0x00, 0x00, 0x00, 0x07,
		// secp256k1 Transferable Output:
		// amount:
		0x00, 0x00, 0x12, 0x30, 0x9c, 0xe5, 0x40, 0x00,
		// locktime:
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// threshold:
		0x00, 0x00, 0x00, 0x01,
		// number of addresses:
		0x00, 0x00, 0x00, 0x01,
		// address[0]:
		0x6e, 0xad, 0x69, 0x3c, 0x17, 0xab, 0xb1, 0xbe,
		0x42, 0x2b, 0xb5, 0x0b, 0x30, 0xb9, 0x71, 0x1f,
		0xf9, 0x8d, 0x66, 0x7e,
		// output[2]:
		// assetID:
		0x01, 0x02, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// fxID:
		0x00, 0x00, 0x00, 0x07,
		// secp256k1 Transferable Output:
		// amount:
		0x00, 0x00, 0x12, 0x30, 0x9c, 0xe5, 0x40, 0x00,
		// locktime:
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// threshold:
		0x00, 0x00, 0x00, 0x01,
		// number of addresses:
		0x00, 0x00, 0x00, 0x01,
		// address[0]:
		0xf2, 0x42, 0x08, 0x46, 0x87, 0x6e, 0x69, 0xf4,
		0x73, 0xdd, 0xa2, 0x56, 0x17, 0x29, 0x67, 0xe9,
		0x92, 0xf0, 0xee, 0x31,
		// number of inputs:
		0x00, 0x00, 0x00, 0x00,
		// Memo length:
		0x00, 0x00, 0x00, 0x04,
		// Memo:
		0x00, 0x01, 0x02, 0x03,
		// name length:
		0x00, 0x04,
		// name:
		'n', 'a', 'm', 'e',
		// symbol length:
		0x00, 0x04,
		// symbol:
		's', 'y', 'm', 'b',
		// denomination
		0x00,
		// number of initial states:
		0x00, 0x00, 0x00, 0x01,
		// fx index:
		0x00, 0x00, 0x00, 0x00,
		// number of outputs:
		0x00, 0x00, 0x00, 0x01,
		// fxID:
		0x00, 0x00, 0x00, 0x06,
		// secp256k1 Mint Output:
		// locktime:
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// threshold:
		0x00, 0x00, 0x00, 0x01,
		// number of addresses:
		0x00, 0x00, 0x00, 0x01,
		// address[0]:
		0xfc, 0xed, 0xa8, 0xf9, 0x0f, 0xcb, 0x5d, 0x30,
		0x61, 0x4b, 0x99, 0xd7, 0x9f, 0xc4, 0xba, 0xa2,
		0x93, 0x07, 0x76, 0x26,
		// number of credentials:
		0x00, 0x00, 0x00, 0x00,
	}

	unsignedTx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
			Memo:         []byte{0x00, 0x01, 0x02, 0x03},
		}},
		Name:         "name",
		Symbol:       "symb",
		Denomination: 0,
		States: []*InitialState{
			{
				FxID: 0,
				Outs: []verify.State{
					&secp256k1fx.MintOutput{
						OutputOwners: secp256k1fx.OutputOwners{
							Threshold: 1,
							Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
						},
					},
				},
			},
		},
	}
	tx := &Tx{UnsignedTx: unsignedTx}
	for _, key := range keys {
		addr := key.PublicKey().Address()

		unsignedTx.Outs = append(unsignedTx.Outs, &avax.TransferableOutput{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 20 * units.KiloAvax,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{addr},
				},
			},
		})
	}

	_, c := setupCodec()
	if err := tx.SignSECP256K1Fx(c, nil); err != nil {
		t.Fatal(err)
	}

	result := tx.Bytes()
	if !bytes.Equal(expected, result) {
		t.Fatalf("\nExpected: 0x%x\nResult:   0x%x", expected, result)
	}
}

func TestInvalidGenesis(t *testing.T) {
	vm := &VM{}
	ctx := NewContext(t)
	ctx.Lock.Lock()
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	err := vm.Initialize(
		/*context=*/ ctx,
		/*db=*/ memdb.New(),
		/*genesisState=*/ nil,
		/*engineMessenger=*/ make(chan common.Message, 1),
		/*fxs=*/ nil,
	)
	if err == nil {
		t.Fatalf("Should have errored due to an invalid genesis")
	}
}

func TestInvalidFx(t *testing.T) {
	vm := &VM{}
	ctx := NewContext(t)
	ctx.Lock.Lock()
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	genesisBytes := BuildGenesisTest(t)
	err := vm.Initialize(
		/*context=*/ ctx,
		/*db=*/ memdb.New(),
		/*genesisState=*/ genesisBytes,
		/*engineMessenger=*/ make(chan common.Message, 1),
		/*fxs=*/ []*common.Fx{
			nil,
		},
	)
	if err == nil {
		t.Fatalf("Should have errored due to an invalid interface")
	}
}

func TestFxInitializationFailure(t *testing.T) {
	vm := &VM{}
	ctx := NewContext(t)
	ctx.Lock.Lock()
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	genesisBytes := BuildGenesisTest(t)
	err := vm.Initialize(
		/*context=*/ ctx,
		/*db=*/ memdb.New(),
		/*genesisState=*/ genesisBytes,
		/*engineMessenger=*/ make(chan common.Message, 1),
		/*fxs=*/ []*common.Fx{{
			ID: ids.Empty,
			Fx: &FxTest{
				InitializeF: func(interface{}) error {
					return errUnknownFx
				},
			},
		}},
	)
	if err == nil {
		t.Fatalf("Should have errored due to an invalid fx initialization")
	}
}

func TestIssueTx(t *testing.T) {
	genesisBytes, issuer, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	newTx := NewTx(t, genesisBytes, vm)

	txID, err := vm.IssueTx(newTx.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if txID != newTx.ID() {
		t.Fatalf("Issue Tx returned wrong TxID")
	}
	ctx.Lock.Unlock()

	msg := <-issuer
	if msg != common.PendingTxs {
		t.Fatalf("Wrong message")
	}
	ctx.Lock.Lock()

	if txs := vm.PendingTxs(); len(txs) != 1 {
		t.Fatalf("Should have returned %d tx(s)", 1)
	}
}

func TestGenesisGetUTXOs(t *testing.T) {
	_, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	addrsSet := ids.ShortSet{}
	addrsSet.Add(addrs[0])
	utxos, _, _, err := vm.GetUTXOs(addrsSet, ids.ShortEmpty, ids.Empty, -1, true)
	if err != nil {
		t.Fatal(err)
	}

	if len(utxos) != 4 {
		t.Fatalf("Wrong number of utxos. Expected (%d) returned (%d)", 4, len(utxos))
	}
}

// TestGenesisGetPaginatedUTXOs tests
// - Pagination when the total UTXOs exceed maxUTXOsToFetch (1024)
// - Fetching all UTXOs when they exceed maxUTXOsToFetch (1024)
func TestGenesisGetPaginatedUTXOs(t *testing.T) {
	addr0Str, _ := formatting.FormatBech32(testHRP, addrs[0].Bytes())
	addr1Str, _ := formatting.FormatBech32(testHRP, addrs[1].Bytes())
	addr2Str, _ := formatting.FormatBech32(testHRP, addrs[2].Bytes())

	// Create a starting point of 3000 UTXOs on different addresses
	utxoCount := 2345
	holder := map[string][]interface{}{}
	for i := 0; i < utxoCount; i++ {
		holder["fixedCap"] = append(holder["fixedCap"],
			Holder{
				Amount:  json.Uint64(startBalance),
				Address: addr0Str,
			},
			Holder{
				Amount:  json.Uint64(startBalance),
				Address: addr1Str,
			},
			Holder{
				Amount:  json.Uint64(startBalance),
				Address: addr2Str,
			})
	}

	// Inject them in the Genesis build
	genesisArgs := &BuildGenesisArgs{
		Encoding: formatting.Hex,
		GenesisData: map[string]AssetDefinition{
			"asset1": {
				Name:         "AVAX",
				Symbol:       "SYMB",
				InitialState: holder,
			},
		}}
	_, _, vm, _ := GenesisVMWithArgs(t, genesisArgs)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	addrsSet := ids.ShortSet{}
	addrsSet.Add(addrs[0], addrs[1])

	var (
		fetchedUTXOs []*avax.UTXO
		err          error
	)

	lastAddr := ids.ShortEmpty
	lastIdx := ids.Empty

	var totalUTXOs []*avax.UTXO
	for i := 0; i <= 3; i++ {
		fetchedUTXOs, lastAddr, lastIdx, err = vm.GetUTXOs(addrsSet, lastAddr, lastIdx, -1, true)
		if err != nil {
			t.Fatal(err)
		}

		if len(fetchedUTXOs) == utxoCount {
			t.Fatalf("Wrong number of utxos. Should be Paginated. Expected (%d) returned (%d)", maxUTXOsToFetch, len(fetchedUTXOs))
		}
		totalUTXOs = append(totalUTXOs, fetchedUTXOs...)
	}

	if len(totalUTXOs) != 4*maxUTXOsToFetch {
		t.Fatalf("Wrong number of utxos. Should have paginated through all. Expected (%d) returned (%d)", 4*maxUTXOsToFetch, len(totalUTXOs))
	}

	// Fetch all UTXOs
	notPaginatedUTXOs, _, _, err := vm.GetUTXOs(addrsSet, ids.ShortEmpty, ids.Empty, -1, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(notPaginatedUTXOs) != 2*utxoCount {
		t.Fatalf("Wrong number of utxos. Expected (%d) returned (%d)", 2*utxoCount, len(notPaginatedUTXOs))
	}
}

// Test issuing a transaction that consumes a currently pending UTXO. The
// transaction should be issued successfully.
func TestIssueDependentTx(t *testing.T) {
	genesisBytes, issuer, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	avaxTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	key := keys[0]

	firstTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        avaxTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: avaxTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: avaxTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: startBalance - vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{key.PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := firstTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{key}}); err != nil {
		t.Fatal(err)
	}

	if _, err := vm.IssueTx(firstTx.Bytes()); err != nil {
		t.Fatal(err)
	}

	secondTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        firstTx.ID(),
				OutputIndex: 0,
			},
			Asset: avax.Asset{ID: avaxTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance - vm.txFee,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
	}}}
	if err := secondTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{key}}); err != nil {
		t.Fatal(err)
	}

	if _, err := vm.IssueTx(secondTx.Bytes()); err != nil {
		t.Fatal(err)
	}
	ctx.Lock.Unlock()

	msg := <-issuer
	if msg != common.PendingTxs {
		t.Fatalf("Wrong message")
	}
	ctx.Lock.Lock()

	if txs := vm.PendingTxs(); len(txs) != 2 {
		t.Fatalf("Should have returned %d tx(s)", 2)
	}
}

// Test issuing a transaction that creates an NFT family
func TestIssueNFT(t *testing.T) {
	vm := &VM{}
	ctx := NewContext(t)
	ctx.Lock.Lock()
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	genesisBytes := BuildGenesisTest(t)
	issuer := make(chan common.Message, 1)
	err := vm.Initialize(
		ctx,
		memdb.New(),
		genesisBytes,
		issuer,
		[]*common.Fx{
			{
				ID: ids.Empty.Prefix(0),
				Fx: &secp256k1fx.Fx{},
			},
			{
				ID: ids.Empty.Prefix(1),
				Fx: &nftfx.Fx{},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	vm.batchTimeout = 0

	err = vm.Bootstrapping()
	if err != nil {
		t.Fatal(err)
	}

	err = vm.Bootstrapped()
	if err != nil {
		t.Fatal(err)
	}

	createAssetTx := &Tx{UnsignedTx: &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "Team Rocket",
		Symbol:       "TR",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 1,
			Outs: []verify.State{
				&nftfx.MintOutput{
					GroupID: 1,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
				&nftfx.MintOutput{
					GroupID: 2,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
			},
		}},
	}}
	if err := createAssetTx.SignSECP256K1Fx(vm.codec, nil); err != nil {
		t.Fatal(err)
	}

	if _, err = vm.IssueTx(createAssetTx.Bytes()); err != nil {
		t.Fatal(err)
	}

	mintNFTTx := &Tx{UnsignedTx: &OperationTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Ops: []*Operation{{
			Asset: avax.Asset{ID: createAssetTx.ID()},
			UTXOIDs: []*avax.UTXOID{{
				TxID:        createAssetTx.ID(),
				OutputIndex: 0,
			}},
			Op: &nftfx.MintOperation{
				MintInput: secp256k1fx.Input{
					SigIndices: []uint32{0},
				},
				GroupID: 1,
				Payload: []byte{'h', 'e', 'l', 'l', 'o'},
				Outputs: []*secp256k1fx.OutputOwners{{}},
			},
		}},
	}}
	if err := mintNFTTx.SignNFTFx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	if _, err = vm.IssueTx(mintNFTTx.Bytes()); err != nil {
		t.Fatal(err)
	}

	transferNFTTx := &Tx{
		UnsignedTx: &OperationTx{
			BaseTx: BaseTx{BaseTx: avax.BaseTx{
				NetworkID:    networkID,
				BlockchainID: chainID,
			}},
			Ops: []*Operation{{
				Asset: avax.Asset{ID: createAssetTx.ID()},
				UTXOIDs: []*avax.UTXOID{{
					TxID:        mintNFTTx.ID(),
					OutputIndex: 0,
				}},
				Op: &nftfx.TransferOperation{
					Input: secp256k1fx.Input{},
					Output: nftfx.TransferOutput{
						GroupID:      1,
						Payload:      []byte{'h', 'e', 'l', 'l', 'o'},
						OutputOwners: secp256k1fx.OutputOwners{},
					},
				},
			}},
		},
		Creds: []verify.Verifiable{
			&nftfx.Credential{},
		},
	}
	if err := transferNFTTx.SignNFTFx(vm.codec, nil); err != nil {
		t.Fatal(err)
	}

	if _, err = vm.IssueTx(transferNFTTx.Bytes()); err != nil {
		t.Fatal(err)
	}
}

// Test issuing a transaction that creates an Property family
func TestIssueProperty(t *testing.T) {
	vm := &VM{}
	ctx := NewContext(t)
	ctx.Lock.Lock()
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	genesisBytes := BuildGenesisTest(t)
	issuer := make(chan common.Message, 1)
	err := vm.Initialize(
		ctx,
		memdb.New(),
		genesisBytes,
		issuer,
		[]*common.Fx{
			{
				ID: ids.Empty.Prefix(0),
				Fx: &secp256k1fx.Fx{},
			},
			{
				ID: ids.Empty.Prefix(1),
				Fx: &nftfx.Fx{},
			},
			{
				ID: ids.Empty.Prefix(2),
				Fx: &propertyfx.Fx{},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	vm.batchTimeout = 0

	err = vm.Bootstrapping()
	if err != nil {
		t.Fatal(err)
	}

	err = vm.Bootstrapped()
	if err != nil {
		t.Fatal(err)
	}

	createAssetTx := &Tx{UnsignedTx: &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "Team Rocket",
		Symbol:       "TR",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 2,
			Outs: []verify.State{
				&propertyfx.MintOutput{
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
			},
		}},
	}}
	if err := createAssetTx.SignSECP256K1Fx(vm.codec, nil); err != nil {
		t.Fatal(err)
	}

	if _, err = vm.IssueTx(createAssetTx.Bytes()); err != nil {
		t.Fatal(err)
	}

	mintPropertyTx := &Tx{UnsignedTx: &OperationTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Ops: []*Operation{{
			Asset: avax.Asset{ID: createAssetTx.ID()},
			UTXOIDs: []*avax.UTXOID{{
				TxID:        createAssetTx.ID(),
				OutputIndex: 0,
			}},
			Op: &propertyfx.MintOperation{
				MintInput: secp256k1fx.Input{
					SigIndices: []uint32{0},
				},
				MintOutput: propertyfx.MintOutput{
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
				OwnedOutput: propertyfx.OwnedOutput{},
			},
		}},
	}}

	unsignedBytes, err := vm.codec.Marshal(codecVersion, &mintPropertyTx.UnsignedTx)
	if err != nil {
		t.Fatal(err)
	}

	key := keys[0]
	sig, err := key.Sign(unsignedBytes)
	if err != nil {
		t.Fatal(err)
	}
	fixedSig := [crypto.SECP256K1RSigLen]byte{}
	copy(fixedSig[:], sig)

	mintPropertyTx.Creds = append(mintPropertyTx.Creds, &propertyfx.Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			fixedSig,
		}},
	})

	signedBytes, err := vm.codec.Marshal(codecVersion, mintPropertyTx)
	if err != nil {
		t.Fatal(err)
	}
	mintPropertyTx.Initialize(unsignedBytes, signedBytes)

	if _, err = vm.IssueTx(mintPropertyTx.Bytes()); err != nil {
		t.Fatal(err)
	}

	burnPropertyTx := &Tx{UnsignedTx: &OperationTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Ops: []*Operation{{
			Asset: avax.Asset{ID: createAssetTx.ID()},
			UTXOIDs: []*avax.UTXOID{{
				TxID:        mintPropertyTx.ID(),
				OutputIndex: 1,
			}},
			Op: &propertyfx.BurnOperation{Input: secp256k1fx.Input{}},
		}},
	}}

	burnPropertyTx.Creds = append(burnPropertyTx.Creds, &propertyfx.Credential{})

	unsignedBytes, err = vm.codec.Marshal(codecVersion, burnPropertyTx.UnsignedTx)
	if err != nil {
		t.Fatal(err)
	}
	signedBytes, err = vm.codec.Marshal(codecVersion, burnPropertyTx)
	if err != nil {
		t.Fatal(err)
	}
	burnPropertyTx.Initialize(unsignedBytes, signedBytes)

	if _, err = vm.IssueTx(burnPropertyTx.Bytes()); err != nil {
		t.Fatal(err)
	}
}

func TestVMFormat(t *testing.T) {
	_, _, vm, _ := GenesisVM(t)
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		vm.ctx.Lock.Unlock()
	}()

	tests := []struct {
		in       ids.ShortID
		expected string
	}{
		{ids.ShortEmpty, "X-testing1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqtu2yas"},
	}
	for _, test := range tests {
		t.Run(test.in.String(), func(t *testing.T) {
			addrStr, err := vm.FormatLocalAddress(test.in)
			if err != nil {
				t.Error(err)
			}
			if test.expected != addrStr {
				t.Errorf("Expected %q, got %q", test.expected, addrStr)
			}
		})
	}
}

func TestTxCached(t *testing.T) {
	genesisBytes, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	newTx := NewTx(t, genesisBytes, vm)
	txBytes := newTx.Bytes()

	_, err := vm.ParseTx(txBytes)
	assert.NoError(t, err)

	db := mockdb.New()
	called := new(bool)
	db.OnGet = func([]byte) ([]byte, error) {
		*called = true
		return nil, errors.New("")
	}
	vm.state.state.DB = db
	vm.state.state.Cache.Flush()

	_, err = vm.ParseTx(txBytes)
	assert.NoError(t, err)
	assert.False(t, *called, "shouldn't have called the DB")
}

func TestTxNotCached(t *testing.T) {
	genesisBytes, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	newTx := NewTx(t, genesisBytes, vm)
	txBytes := newTx.Bytes()

	_, err := vm.ParseTx(txBytes)
	assert.NoError(t, err)

	db := mockdb.New()
	called := new(bool)
	db.OnGet = func([]byte) ([]byte, error) {
		*called = true
		return nil, errors.New("")
	}
	db.OnPut = func([]byte, []byte) error { return nil }
	vm.state.state.DB = db
	vm.state.uniqueTx.Flush()
	vm.state.state.Cache.Flush()

	_, err = vm.ParseTx(txBytes)
	assert.NoError(t, err)
	assert.True(t, *called, "should have called the DB")
}

func TestTxVerifyAfterIssueTx(t *testing.T) {
	genesisBytes, issuer, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	avaxTx := GetAVAXTxFromGenesisTest(genesisBytes, t)
	key := keys[0]
	firstTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        avaxTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: avaxTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: avaxTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: startBalance - vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{key.PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := firstTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{key}}); err != nil {
		t.Fatal(err)
	}

	secondTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        avaxTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: avaxTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: avaxTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: 1,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{key.PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := secondTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{key}}); err != nil {
		t.Fatal(err)
	}

	parsedSecondTx, err := vm.ParseTx(secondTx.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := parsedSecondTx.Verify(); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.IssueTx(firstTx.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := parsedSecondTx.Accept(); err != nil {
		t.Fatal(err)
	}
	ctx.Lock.Unlock()

	msg := <-issuer
	if msg != common.PendingTxs {
		t.Fatalf("Wrong message")
	}
	ctx.Lock.Lock()

	txs := vm.PendingTxs()
	if len(txs) != 1 {
		t.Fatalf("Should have returned %d tx(s)", 1)
	}
	parsedFirstTx := txs[0]

	if err := parsedFirstTx.Verify(); err == nil {
		t.Fatalf("Should have errored due to a missing UTXO")
	}
}

func TestTxVerifyAfterGet(t *testing.T) {
	genesisBytes, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	avaxTx := GetAVAXTxFromGenesisTest(genesisBytes, t)
	key := keys[0]
	firstTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        avaxTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: avaxTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: avaxTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: startBalance - vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{key.PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := firstTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{key}}); err != nil {
		t.Fatal(err)
	}

	secondTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        avaxTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: avaxTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: avaxTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: 1,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{key.PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := secondTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{key}}); err != nil {
		t.Fatal(err)
	}

	parsedSecondTx, err := vm.ParseTx(secondTx.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := parsedSecondTx.Verify(); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.IssueTx(firstTx.Bytes()); err != nil {
		t.Fatal(err)
	}
	parsedFirstTx, err := vm.GetTx(firstTx.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := parsedSecondTx.Accept(); err != nil {
		t.Fatal(err)
	}
	if err := parsedFirstTx.Verify(); err == nil {
		t.Fatalf("Should have errored due to a missing UTXO")
	}
}

func TestTxVerifyAfterVerifyAncestorTx(t *testing.T) {
	genesisBytes, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	avaxTx := GetAVAXTxFromGenesisTest(genesisBytes, t)
	key := keys[0]
	firstTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        avaxTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: avaxTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: avaxTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: startBalance - vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{key.PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := firstTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{key}}); err != nil {
		t.Fatal(err)
	}

	firstTxDescendant := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        firstTx.ID(),
				OutputIndex: 0,
			},
			Asset: avax.Asset{ID: avaxTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance - vm.txFee,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: avaxTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: startBalance - 2*vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{key.PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := firstTxDescendant.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{key}}); err != nil {
		t.Fatal(err)
	}

	secondTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        avaxTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: avaxTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: avaxTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: 1,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{key.PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := secondTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{key}}); err != nil {
		t.Fatal(err)
	}

	parsedSecondTx, err := vm.ParseTx(secondTx.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := parsedSecondTx.Verify(); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.IssueTx(firstTx.Bytes()); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.IssueTx(firstTxDescendant.Bytes()); err != nil {
		t.Fatal(err)
	}
	parsedFirstTx, err := vm.GetTx(firstTx.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := parsedSecondTx.Accept(); err != nil {
		t.Fatal(err)
	}
	if err := parsedFirstTx.Verify(); err == nil {
		t.Fatalf("Should have errored due to a missing UTXO")
	}
}
