// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gorilla/rpc/v2"

	"github.com/ava-labs/gecko/cache"
	"github.com/ava-labs/gecko/database"
	"github.com/ava-labs/gecko/database/versiondb"
	"github.com/ava-labs/gecko/ids"
	"github.com/ava-labs/gecko/snow"
	"github.com/ava-labs/gecko/snow/choices"
	"github.com/ava-labs/gecko/snow/consensus/snowstorm"
	"github.com/ava-labs/gecko/snow/engine/common"
	"github.com/ava-labs/gecko/utils/codec"
	"github.com/ava-labs/gecko/utils/formatting"
	"github.com/ava-labs/gecko/utils/logging"
	"github.com/ava-labs/gecko/utils/timer"
	"github.com/ava-labs/gecko/utils/wrappers"
	"github.com/ava-labs/gecko/vms/components/ava"

	cjson "github.com/ava-labs/gecko/utils/json"
)

const (
	batchTimeout   = time.Second
	batchSize      = 30
	stateCacheSize = 10000
	idCacheSize    = 10000
	txCacheSize    = 10000
	addressSep     = "-"
)

var (
	errIncompatibleFx            = errors.New("incompatible feature extension")
	errUnknownFx                 = errors.New("unknown feature extension")
	errGenesisAssetMustHaveState = errors.New("genesis asset must have non-empty state")
	errInvalidAddress            = errors.New("invalid address")
	errWrongBlockchainID         = errors.New("wrong blockchain ID")
	errBootstrapping             = errors.New("chain is currently bootstrapping")
)

// VM implements the avalanche.DAGVM interface
type VM struct {
	ids.Aliaser

	ava      ids.ID
	platform ids.ID

	// Contains information of where this VM is executing
	ctx *snow.Context

	// Used to check local time
	clock timer.Clock

	codec codec.Codec

	pubsub *cjson.PubSubServer

	// State management
	state *prefixedState

	// Set to true once this VM is marked as `Bootstrapped` by the engine
	bootstrapped bool

	// Transaction issuing
	timer        *timer.Timer
	batchTimeout time.Duration
	txs          []snowstorm.Tx
	toEngine     chan<- common.Message

	baseDB database.Database
	db     *versiondb.Database

	typeToFxIndex map[reflect.Type]int
	fxs           []*parsedFx
}

type codecRegistry struct {
	index         int
	typeToFxIndex map[reflect.Type]int
	codec         codec.Codec
}

func (cr *codecRegistry) RegisterType(val interface{}) error {
	valType := reflect.TypeOf(val)
	cr.typeToFxIndex[valType] = cr.index
	return cr.codec.RegisterType(val)
}
func (cr *codecRegistry) Marshal(val interface{}) ([]byte, error) { return cr.codec.Marshal(val) }
func (cr *codecRegistry) Unmarshal(b []byte, val interface{}) error {
	return cr.codec.Unmarshal(b, val)
}

/*
 ******************************************************************************
 ******************************** Avalanche API *******************************
 ******************************************************************************
 */

// Initialize implements the avalanche.DAGVM interface
func (vm *VM) Initialize(
	ctx *snow.Context,
	db database.Database,
	genesisBytes []byte,
	toEngine chan<- common.Message,
	fxs []*common.Fx,
) error {
	vm.ctx = ctx
	vm.toEngine = toEngine
	vm.baseDB = db
	vm.db = versiondb.New(db)
	vm.typeToFxIndex = map[reflect.Type]int{}
	vm.Aliaser.Initialize()

	vm.pubsub = cjson.NewPubSubServer(ctx)
	c := codec.NewDefault()

	errs := wrappers.Errs{}
	errs.Add(
		vm.pubsub.Register("accepted"),
		vm.pubsub.Register("rejected"),
		vm.pubsub.Register("verified"),

		c.RegisterType(&BaseTx{}),
		c.RegisterType(&CreateAssetTx{}),
		c.RegisterType(&OperationTx{}),
		c.RegisterType(&ImportTx{}),
		c.RegisterType(&ExportTx{}),
	)
	if errs.Errored() {
		return errs.Err
	}

	vm.fxs = make([]*parsedFx, len(fxs))
	for i, fxContainer := range fxs {
		if fxContainer == nil {
			return errIncompatibleFx
		}
		fx, ok := fxContainer.Fx.(Fx)
		if !ok {
			return errIncompatibleFx
		}
		vm.fxs[i] = &parsedFx{
			ID: fxContainer.ID,
			Fx: fx,
		}
		vm.codec = &codecRegistry{
			index:         i,
			typeToFxIndex: vm.typeToFxIndex,
			codec:         c,
		}
		if err := fx.Initialize(vm); err != nil {
			return err
		}
	}

	vm.codec = c

	vm.state = &prefixedState{
		state: &state{State: ava.State{
			Cache: &cache.LRU{Size: stateCacheSize},
			DB:    vm.db,
			Codec: vm.codec,
		}},

		tx:       &cache.LRU{Size: idCacheSize},
		utxo:     &cache.LRU{Size: idCacheSize},
		txStatus: &cache.LRU{Size: idCacheSize},

		uniqueTx: &cache.EvictableLRU{Size: txCacheSize},
	}

	if err := vm.initAliases(genesisBytes); err != nil {
		return err
	}

	if dbStatus, err := vm.state.DBInitialized(); err != nil || dbStatus == choices.Unknown {
		if err := vm.initState(genesisBytes); err != nil {
			return err
		}
	}

	vm.timer = timer.NewTimer(func() {
		ctx.Lock.Lock()
		defer ctx.Lock.Unlock()

		vm.FlushTxs()
	})
	go ctx.Log.RecoverAndPanic(vm.timer.Dispatch)
	vm.batchTimeout = batchTimeout

	return vm.db.Commit()
}

// Bootstrapping is called by the consensus engine when it starts bootstrapping
// this chain
func (vm *VM) Bootstrapping() error {
	for _, fx := range vm.fxs {
		if err := fx.Fx.Bootstrapping(); err != nil {
			return err
		}
	}
	return nil
}

// Bootstrapped is called by the consensus engine when it is done bootstrapping
// this chain
func (vm *VM) Bootstrapped() error {
	for _, fx := range vm.fxs {
		if err := fx.Fx.Bootstrapped(); err != nil {
			return err
		}
	}
	vm.bootstrapped = true
	return nil
}

// Shutdown implements the avalanche.DAGVM interface
func (vm *VM) Shutdown() error {
	if vm.timer == nil {
		return nil
	}

	// There is a potential deadlock if the timer is about to execute a timeout.
	// So, the lock must be released before stopping the timer.
	vm.ctx.Lock.Unlock()
	vm.timer.Stop()
	vm.ctx.Lock.Lock()

	return vm.baseDB.Close()
}

var mapping map[string]string = map[string]string{
	"/tx":                        "GetTx",
	"/tx/issue":                  "IssueTx",
	"/tx/status":                 "GetTxStatus",
	"/tx/send":                   "Send",
	"/tx/mint":                   "CreateMintTx",
	"/tx/mint/sign":              "SignMintTx",
	"/utxo":                      "GetUTXOs",
	"/balance":                   "GetBalance",
	"/balance/all":               "GetAllBalances",
	"/asset/description":         "GetAssetDescription",
	"/asset/cap/fixed/create":    "CreateFixedCapAsset",
	"/asset/cap/variable/create": "CreateVariableCapAsset",
	"/address/create":            "CreateAddress",
	"/address/list":              "ListAddresses",
	"/key/import":                "ImportKey",
	"/key/export":                "ExportKey",
	"/ava/import":                "ImportAVA",
	"/ava/export":                "ExportAVA",
}

// CreateHandlers implements the avalanche.DAGVM interface
func (vm *VM) CreateHandlers() map[string]*common.HTTPHandler {
	rpcServer := rpc.NewServer()
	restMap := cjson.MappingGenerator(mapping, "avm")
	codec := cjson.RestCodec{Mapping: restMap}
	rpcServer.RegisterCodec(codec, "application/json")
	rpcServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	rpcServer.RegisterService(&Service{vm: vm}, "avm") // name this service "avm"

	return map[string]*common.HTTPHandler{
		"":        {Handler: rpcServer, RestEndpoints: restMap.GetKeys()},
		"/pubsub": {LockOptions: common.NoLock, Handler: vm.pubsub},
	}
}

var staticMapping map[string]string = map[string]string{
	"/genesis/build": "vm.BuildGenesis",
}

// CreateStaticHandlers implements the avalanche.DAGVM interface
func (vm *VM) CreateStaticHandlers() map[string]*common.HTTPHandler {
	newServer := rpc.NewServer()
	restMap := cjson.MappingGenerator(staticMapping, "avm")
	codec := cjson.RestCodec{Mapping: restMap}
	newServer.RegisterCodec(codec, "application/json")
	newServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	newServer.RegisterService(&StaticService{}, "avm") // name this service "avm"
	return map[string]*common.HTTPHandler{
		"": {LockOptions: common.WriteLock, Handler: newServer, RestEndpoints: restMap.GetKeys()},
	}
}

// PendingTxs implements the avalanche.DAGVM interface
func (vm *VM) PendingTxs() []snowstorm.Tx {
	vm.timer.Cancel()

	txs := vm.txs
	vm.txs = nil
	return txs
}

// ParseTx implements the avalanche.DAGVM interface
func (vm *VM) ParseTx(b []byte) (snowstorm.Tx, error) { return vm.parseTx(b) }

// GetTx implements the avalanche.DAGVM interface
func (vm *VM) GetTx(txID ids.ID) (snowstorm.Tx, error) {
	tx := &UniqueTx{
		vm:   vm,
		txID: txID,
	}
	// Verify must be called in the case the that tx was flushed from the unique
	// cache.
	return tx, tx.Verify()
}

/*
 ******************************************************************************
 ********************************** JSON API **********************************
 ******************************************************************************
 */

// IssueTx attempts to send a transaction to consensus.
// If onDecide is specified, the function will be called when the transaction is
// either accepted or rejected with the appropriate status. This function will
// go out of scope when the transaction is removed from memory.
func (vm *VM) IssueTx(b []byte, onDecide func(choices.Status)) (ids.ID, error) {
	if !vm.bootstrapped {
		return ids.ID{}, errBootstrapping
	}
	tx, err := vm.parseTx(b)
	if err != nil {
		return ids.ID{}, err
	}
	if err := tx.Verify(); err != nil {
		return ids.ID{}, err
	}
	vm.issueTx(tx)
	tx.onDecide = onDecide
	return tx.ID(), nil
}

// GetAtomicUTXOs returns the utxos that at least one of the provided addresses is
// referenced in.
func (vm *VM) GetAtomicUTXOs(addrs ids.Set) ([]*ava.UTXO, error) {
	smDB := vm.ctx.SharedMemory.GetDatabase(vm.platform)
	defer vm.ctx.SharedMemory.ReleaseDatabase(vm.platform)

	state := ava.NewPrefixedState(smDB, vm.codec)

	utxoIDs := ids.Set{}
	for _, addr := range addrs.List() {
		utxos, _ := state.PlatformFunds(addr)
		utxoIDs.Add(utxos...)
	}

	utxos := []*ava.UTXO{}
	for _, utxoID := range utxoIDs.List() {
		utxo, err := state.PlatformUTXO(utxoID)
		if err != nil {
			return nil, err
		}
		utxos = append(utxos, utxo)
	}
	return utxos, nil
}

// GetUTXOs returns the utxos that at least one of the provided addresses is
// referenced in.
func (vm *VM) GetUTXOs(addrs ids.Set) ([]*ava.UTXO, error) {
	utxoIDs := ids.Set{}
	for _, addr := range addrs.List() {
		utxos, _ := vm.state.Funds(addr)
		utxoIDs.Add(utxos...)
	}

	utxos := []*ava.UTXO{}
	for _, utxoID := range utxoIDs.List() {
		utxo, err := vm.state.UTXO(utxoID)
		if err != nil {
			return nil, err
		}
		utxos = append(utxos, utxo)
	}
	return utxos, nil
}

/*
 ******************************************************************************
 *********************************** Fx API ***********************************
 ******************************************************************************
 */

// Clock returns a reference to the internal clock of this VM
func (vm *VM) Clock() *timer.Clock { return &vm.clock }

// Codec returns a reference to the internal codec of this VM
func (vm *VM) Codec() codec.Codec { return vm.codec }

// Logger returns a reference to the internal logger of this VM
func (vm *VM) Logger() logging.Logger { return vm.ctx.Log }

/*
 ******************************************************************************
 ********************************** Timer API *********************************
 ******************************************************************************
 */

// FlushTxs into consensus
func (vm *VM) FlushTxs() {
	vm.timer.Cancel()
	if len(vm.txs) != 0 {
		select {
		case vm.toEngine <- common.PendingTxs:
		default:
			vm.ctx.Log.Warn("Delaying issuance of transactions due to contention")
			vm.timer.SetTimeoutIn(vm.batchTimeout)
		}
	}
}

/*
 ******************************************************************************
 ********************************** Helpers ***********************************
 ******************************************************************************
 */

func (vm *VM) initAliases(genesisBytes []byte) error {
	genesis := Genesis{}
	if err := vm.codec.Unmarshal(genesisBytes, &genesis); err != nil {
		return err
	}

	for _, genesisTx := range genesis.Txs {
		if len(genesisTx.Outs) != 0 {
			return errGenesisAssetMustHaveState
		}

		tx := Tx{
			UnsignedTx: &genesisTx.CreateAssetTx,
		}
		txBytes, err := vm.codec.Marshal(&tx)
		if err != nil {
			return err
		}
		tx.Initialize(txBytes)

		txID := tx.ID()

		if err = vm.Alias(txID, genesisTx.Alias); err != nil {
			return err
		}
	}

	return nil
}

func (vm *VM) initState(genesisBytes []byte) error {
	genesis := Genesis{}
	if err := vm.codec.Unmarshal(genesisBytes, &genesis); err != nil {
		return err
	}

	for _, genesisTx := range genesis.Txs {
		if len(genesisTx.Outs) != 0 {
			return errGenesisAssetMustHaveState
		}

		tx := Tx{
			UnsignedTx: &genesisTx.CreateAssetTx,
		}
		txBytes, err := vm.codec.Marshal(&tx)
		if err != nil {
			return err
		}
		tx.Initialize(txBytes)

		txID := tx.ID()

		vm.ctx.Log.Info("Initializing with AssetID %s", txID)

		if err := vm.state.SetTx(txID, &tx); err != nil {
			return err
		}
		if err := vm.state.SetStatus(txID, choices.Accepted); err != nil {
			return err
		}
		for _, utxo := range tx.UTXOs() {
			if err := vm.state.FundUTXO(utxo); err != nil {
				return err
			}
		}
	}

	return vm.state.SetDBInitialized(choices.Processing)
}

func (vm *VM) parseTx(b []byte) (*UniqueTx, error) {
	rawTx := &Tx{}
	err := vm.codec.Unmarshal(b, rawTx)
	if err != nil {
		return nil, err
	}
	rawTx.Initialize(b)

	tx := &UniqueTx{
		TxState: &TxState{
			Tx: rawTx,
		},
		vm:   vm,
		txID: rawTx.ID(),
	}
	if err := tx.SyntacticVerify(); err != nil {
		return nil, err
	}

	if tx.Status() == choices.Unknown {
		if err := vm.state.SetTx(tx.ID(), tx.Tx); err != nil {
			return nil, err
		}
		if err := tx.setStatus(choices.Processing); err != nil {
			return nil, err
		}
		return tx, vm.db.Commit()
	}

	return tx, nil
}

func (vm *VM) issueTx(tx snowstorm.Tx) {
	vm.txs = append(vm.txs, tx)
	switch {
	case len(vm.txs) == batchSize:
		vm.FlushTxs()
	case len(vm.txs) == 1:
		vm.timer.SetTimeoutIn(vm.batchTimeout)
	}
}

func (vm *VM) getUTXO(utxoID *ava.UTXOID) (*ava.UTXO, error) {
	inputID := utxoID.InputID()
	utxo, err := vm.state.UTXO(inputID)
	if err == nil {
		return utxo, nil
	}

	inputTx, inputIndex := utxoID.InputSource()
	parent := UniqueTx{
		vm:   vm,
		txID: inputTx,
	}

	if err := parent.Verify(); err != nil {
		return nil, errMissingUTXO
	} else if status := parent.Status(); status.Decided() {
		return nil, errMissingUTXO
	}

	parentUTXOs := parent.UTXOs()
	if uint32(len(parentUTXOs)) <= inputIndex || int(inputIndex) < 0 {
		return nil, errInvalidUTXO
	}
	return parentUTXOs[int(inputIndex)], nil
}

func (vm *VM) getFx(val interface{}) (int, error) {
	valType := reflect.TypeOf(val)
	fx, exists := vm.typeToFxIndex[valType]
	if !exists {
		return 0, errUnknownFx
	}
	return fx, nil
}

func (vm *VM) verifyFxUsage(fxID int, assetID ids.ID) bool {
	tx := &UniqueTx{
		vm:   vm,
		txID: assetID,
	}
	if status := tx.Status(); !status.Fetched() {
		return false
	}
	createAssetTx, ok := tx.UnsignedTx.(*CreateAssetTx)
	if !ok {
		return false
	}
	// TODO: This could be a binary search to import performance... Or perhaps
	// make a map
	for _, state := range createAssetTx.States {
		if state.FxID == uint32(fxID) {
			return true
		}
	}
	return false
}

// Parse ...
func (vm *VM) Parse(addrStr string) ([]byte, error) {
	if count := strings.Count(addrStr, addressSep); count != 1 {
		return nil, errInvalidAddress
	}
	addressParts := strings.SplitN(addrStr, addressSep, 2)
	bcAlias := addressParts[0]
	rawAddr := addressParts[1]
	bcID, err := vm.ctx.BCLookup.Lookup(bcAlias)
	if err != nil {
		bcID, err = ids.FromString(bcAlias)
		if err != nil {
			return nil, err
		}
	}
	if !bcID.Equals(vm.ctx.ChainID) {
		return nil, errWrongBlockchainID
	}
	cb58 := formatting.CB58{}
	err = cb58.FromString(rawAddr)
	return cb58.Bytes, err
}

// Format ...
func (vm *VM) Format(b []byte) string {
	var bcAlias string
	if alias, err := vm.ctx.BCLookup.PrimaryAlias(vm.ctx.ChainID); err == nil {
		bcAlias = alias
	} else {
		bcAlias = vm.ctx.ChainID.String()
	}
	return fmt.Sprintf("%s%s%s", bcAlias, addressSep, formatting.CB58{Bytes: b})
}
