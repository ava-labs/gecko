// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/gorilla/rpc/v2"

	"github.com/ava-labs/avalanchego/api/pubsub"
	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/codec/reflectcodec"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/vertex"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/nftfx"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"

	cjson "github.com/ava-labs/avalanchego/utils/json"
	safemath "github.com/ava-labs/avalanchego/utils/math"
)

const (
	batchTimeout       = time.Second
	batchSize          = 30
	stateCacheSize     = 30000
	idCacheSize        = 30000
	txCacheSize        = 30000
	assetToFxCacheSize = 1024
	maxUTXOsToFetch    = 1024

	codecVersion = 0
)

var (
	errIncompatibleFx            = errors.New("incompatible feature extension")
	errUnknownFx                 = errors.New("unknown feature extension")
	errGenesisAssetMustHaveState = errors.New("genesis asset must have non-empty state")
	errWrongBlockchainID         = errors.New("wrong blockchain ID")
	errBootstrapping             = errors.New("chain is currently bootstrapping")
	errInsufficientFunds         = errors.New("insufficient funds")

	_ vertex.DAGVM    = &VM{}
	_ common.StaticVM = &VM{}
)

// VM implements the avalanche.DAGVM interface
type VM struct {
	metrics
	ids.Aliaser

	// Contains information of where this VM is executing
	ctx *snow.Context

	// Used to check local time
	clock timer.Clock

	genesisCodec  codec.Manager
	codec         codec.Manager
	codecRegistry codec.Registry

	pubsub *pubsub.Server

	// State management
	state *prefixedState

	// Set to true once this VM is marked as `Bootstrapped` by the engine
	bootstrapped bool

	// fee that must be burned by every state creating transaction
	creationTxFee uint64
	// fee that must be burned by every non-state creating transaction
	txFee uint64

	// Asset ID --> Bit set with fx IDs the asset supports
	assetToFxCache *cache.LRU

	// Transaction issuing
	timer        *timer.Timer
	batchTimeout time.Duration
	txs          []snowstorm.Tx
	toEngine     chan<- common.Message

	baseDB database.Database
	db     *versiondb.Database

	typeToFxIndex map[reflect.Type]int
	fxs           []*parsedFx

	walletService WalletService
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
	vm.assetToFxCache = &cache.LRU{Size: assetToFxCacheSize}

	vm.pubsub = pubsub.NewServer(ctx)

	genesisCodec := linearcodec.New(reflectcodec.DefaultTagName, 1<<20)
	c := linearcodec.NewDefault()

	vm.genesisCodec = codec.NewManager(math.MaxInt32)
	vm.codec = codec.NewDefaultManager()

	errs := wrappers.Errs{}
	errs.Add(
		vm.metrics.Initialize(ctx.Namespace, ctx.Metrics),

		vm.pubsub.Register("accepted"),
		vm.pubsub.Register("rejected"),
		vm.pubsub.Register("verified"),

		c.RegisterType(&BaseTx{}),
		c.RegisterType(&CreateAssetTx{}),
		c.RegisterType(&OperationTx{}),
		c.RegisterType(&ImportTx{}),
		c.RegisterType(&ExportTx{}),
		vm.codec.RegisterCodec(codecVersion, c),

		genesisCodec.RegisterType(&BaseTx{}),
		genesisCodec.RegisterType(&CreateAssetTx{}),
		genesisCodec.RegisterType(&OperationTx{}),
		genesisCodec.RegisterType(&ImportTx{}),
		genesisCodec.RegisterType(&ExportTx{}),
		vm.genesisCodec.RegisterCodec(codecVersion, genesisCodec),
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
		vm.codecRegistry = &codecRegistry{
			codecs:      []codec.Registry{genesisCodec, c},
			index:       i,
			typeToIndex: vm.typeToFxIndex,
		}
		if err := fx.Initialize(vm); err != nil {
			return err
		}
	}

	vm.state = &prefixedState{
		state: &state{State: avax.State{
			Cache:        &cache.LRU{Size: stateCacheSize},
			DB:           vm.db,
			GenesisCodec: vm.genesisCodec,
			Codec:        vm.codec,
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

	vm.walletService.vm = vm
	vm.walletService.pendingTxMap = make(map[ids.ID]*list.Element)
	vm.walletService.pendingTxOrdering = list.New()

	return vm.db.Commit()
}

// Bootstrapping is called by the consensus engine when it starts bootstrapping
// this chain
func (vm *VM) Bootstrapping() error {
	vm.metrics.numBootstrappingCalls.Inc()

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
	vm.metrics.numBootstrappedCalls.Inc()

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

// CreateHandlers implements the avalanche.DAGVM interface
func (vm *VM) CreateHandlers() (map[string]*common.HTTPHandler, error) {
	vm.metrics.numCreateHandlersCalls.Inc()

	codec := cjson.NewCodec()

	rpcServer := rpc.NewServer()
	rpcServer.RegisterCodec(codec, "application/json")
	rpcServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	// name this service "avm"
	if err := rpcServer.RegisterService(&Service{vm: vm}, "avm"); err != nil {
		return nil, err
	}

	walletServer := rpc.NewServer()
	walletServer.RegisterCodec(codec, "application/json")
	walletServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	// name this service "wallet"
	err := walletServer.RegisterService(&vm.walletService, "wallet")

	return map[string]*common.HTTPHandler{
		"":        {Handler: rpcServer},
		"/wallet": {Handler: walletServer},
		"/pubsub": {LockOptions: common.NoLock, Handler: vm.pubsub},
	}, err
}

// CreateStaticHandlers implements the common.StaticVM interface
func (vm *VM) CreateStaticHandlers() (map[string]*common.HTTPHandler, error) {
	newServer := rpc.NewServer()
	codec := cjson.NewCodec()
	newServer.RegisterCodec(codec, "application/json")
	newServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	// name this service "avm"
	staticService := CreateStaticService()
	return map[string]*common.HTTPHandler{
		"": {LockOptions: common.WriteLock, Handler: newServer},
	}, newServer.RegisterService(staticService, "avm")
}

// Pending implements the avalanche.DAGVM interface
func (vm *VM) PendingTxs() []snowstorm.Tx {
	vm.metrics.numPendingCalls.Inc()

	vm.timer.Cancel()

	txs := vm.txs
	vm.txs = nil
	return txs
}

// Parse implements the avalanche.DAGVM interface
func (vm *VM) ParseTx(b []byte) (snowstorm.Tx, error) {
	vm.metrics.numParseCalls.Inc()

	return vm.parseTx(b)
}

// Get implements the avalanche.DAGVM interface
func (vm *VM) GetTx(txID ids.ID) (snowstorm.Tx, error) {
	vm.metrics.numGetCalls.Inc()

	tx := &UniqueTx{
		vm:   vm,
		txID: txID,
	}
	// Verify must be called in the case the that tx was flushed from the unique
	// cache.
	return tx, tx.verifyWithoutCacheWrites()
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
func (vm *VM) IssueTx(b []byte) (ids.ID, error) {
	if !vm.bootstrapped {
		return ids.ID{}, errBootstrapping
	}
	tx, err := vm.parseTx(b)
	if err != nil {
		return ids.ID{}, err
	}
	if err := tx.verifyWithoutCacheWrites(); err != nil {
		return ids.ID{}, err
	}
	vm.issueTx(tx)
	return tx.ID(), nil
}

// GetAtomicUTXOs returns imported/exports UTXOs such that at least one of the addresses in [addrs] is referenced.
// Returns at most [limit] UTXOs.
// If [limit] <= 0 or [limit] > maxUTXOsToFetch, it is set to [maxUTXOsToFetch].
// Returns:
// * The fetched UTXOs
// * true if all there are no more UTXOs in this range to fetch
// * The address associated with the last UTXO fetched
// * The ID of the last UTXO fetched
func (vm *VM) GetAtomicUTXOs(
	chainID ids.ID,
	addrs ids.ShortSet,
	startAddr ids.ShortID,
	startUTXOID ids.ID,
	limit int,
) ([]*avax.UTXO, ids.ShortID, ids.ID, error) {
	if limit <= 0 || limit > maxUTXOsToFetch {
		limit = maxUTXOsToFetch
	}

	addrsList := make([][]byte, addrs.Len())
	i := 0
	for addr := range addrs {
		copied := addr
		addrsList[i] = copied[:]
		i++
	}

	allUTXOBytes, lastAddr, lastUTXO, err := vm.ctx.SharedMemory.Indexed(
		chainID,
		addrsList,
		startAddr.Bytes(),
		startUTXOID[:],
		limit,
	)
	if err != nil {
		return nil, ids.ShortID{}, ids.ID{}, fmt.Errorf("error fetching atomic UTXOs: %w", err)
	}

	lastAddrID, err := ids.ToShortID(lastAddr)
	if err != nil {
		lastAddrID = ids.ShortEmpty
	}
	lastUTXOID, err := ids.ToID(lastUTXO)
	if err != nil {
		lastUTXOID = ids.Empty
	}

	utxos := make([]*avax.UTXO, len(allUTXOBytes))
	for i, utxoBytes := range allUTXOBytes {
		utxo := &avax.UTXO{}
		if _, err := vm.codec.Unmarshal(utxoBytes, utxo); err != nil {
			return nil, ids.ShortID{}, ids.ID{}, fmt.Errorf("error parsing UTXO: %w", err)
		}
		utxos[i] = utxo
	}
	return utxos, lastAddrID, lastUTXOID, nil
}

// GetUTXOs returns UTXOs such that at least one of the addresses in [addrs] is referenced.
// Returns at most [limit] UTXOs.
// If [limit] <= 0 or [limit] > maxUTXOsToFetch, it is set to [maxUTXOsToFetch].
// Only returns UTXOs associated with addresses >= [startAddr].
// For address [startAddr], only returns UTXOs whose IDs are greater than [startUTXOID].
// Given a ![paginate] input all utxos will be fetched
// Returns:
// * The fetched UTXOs
// * The address associated with the last UTXO fetched
// * The ID of the last UTXO fetched
func (vm *VM) GetUTXOs(
	addrs ids.ShortSet,
	startAddr ids.ShortID,
	startUTXOID ids.ID,
	limit int,
	paginate bool,
) ([]*avax.UTXO, ids.ShortID, ids.ID, error) {
	if limit <= 0 || limit > maxUTXOsToFetch {
		limit = maxUTXOsToFetch
	}

	if paginate {
		return vm.getPaginatedUTXOs(addrs, startAddr, startUTXOID, limit)
	}
	return vm.getAllUTXOs(addrs)
}

func (vm *VM) getPaginatedUTXOs(addrs ids.ShortSet,
	startAddr ids.ShortID,
	startUTXOID ids.ID,
	limit int,
) ([]*avax.UTXO, ids.ShortID, ids.ID, error) {
	lastAddr := ids.ShortEmpty
	lastIndex := ids.Empty

	utxos := make([]*avax.UTXO, 0, limit)
	seen := make(ids.Set, limit) // IDs of UTXOs already in the list
	searchSize := limit          // the limit diminishes which can impact the expected return

	// enforces the same ordering for pagination
	addrsList := addrs.List()
	ids.SortShortIDs(addrsList)

	for _, addr := range addrsList {
		start := ids.Empty
		if comp := bytes.Compare(addr.Bytes(), startAddr.Bytes()); comp == -1 { // Skip addresses before [startAddr]
			continue
		} else if comp == 0 {
			start = startUTXOID
		}

		// Get UTXOs associated with [addr]. [searchSize] is used here to ensure
		// that no UTXOs are dropped due to duplicated fetching.
		utxoIDs, err := vm.state.Funds(addr.Bytes(), start, searchSize)
		if err != nil {
			return nil, ids.ShortID{}, ids.ID{}, fmt.Errorf("couldn't get UTXOs for address %s: %w", addr, err)
		}
		for _, utxoID := range utxoIDs {
			lastIndex = utxoID // The last searched UTXO - not the last found
			lastAddr = addr    // The last address searched that has UTXOs (even duplicated) - not the last found

			if seen.Contains(utxoID) { // Already have this UTXO in the list
				continue
			}

			utxo, err := vm.state.UTXO(utxoID)
			if err != nil {
				return nil, ids.ShortID{}, ids.ID{}, fmt.Errorf("couldn't get UTXO %s: %w", utxoID, err)
			}

			utxos = append(utxos, utxo)
			seen.Add(utxoID)
			limit--
			if limit <= 0 {
				return utxos, lastAddr, lastIndex, nil // Found [limit] utxos; stop.
			}
		}
	}
	return utxos, lastAddr, lastIndex, nil // Didnt reach the [limit] utxos; no more were found
}

func (vm *VM) getAllUTXOs(addrs ids.ShortSet) ([]*avax.UTXO, ids.ShortID, ids.ID, error) {
	var err error
	lastAddr := ids.ShortEmpty
	lastIndex := ids.Empty
	seen := make(ids.Set, maxUTXOsToFetch) // IDs of UTXOs already in the list
	utxos := make([]*avax.UTXO, 0, maxUTXOsToFetch)

	// enforces the same ordering for pagination
	addrsList := addrs.List()
	ids.SortShortIDs(addrsList)

	// iterate over the addresses and get all the utxos
	for _, addr := range addrsList {
		lastIndex, err = vm.getAllUniqueAddressUTXOs(addr, &seen, &utxos)
		if err != nil {
			return nil, ids.ShortID{}, ids.ID{}, fmt.Errorf("couldn't get UTXOs for address %s: %w", addr, err)
		}

		if lastIndex != ids.Empty {
			lastAddr = addr // The last address searched that has UTXOs (even duplicated) - not the last found
		}
	}
	return utxos, lastAddr, lastIndex, nil
}

func (vm *VM) getAllUniqueAddressUTXOs(addr ids.ShortID, seen *ids.Set, utxos *[]*avax.UTXO) (ids.ID, error) {
	lastIndex := ids.Empty

	for {
		utxoIDs, err := vm.state.Funds(addr.Bytes(), lastIndex, maxUTXOsToFetch) // Get UTXOs associated with [addr]
		if err != nil {
			return ids.ID{}, err
		}

		if len(utxoIDs) == 0 {
			return lastIndex, nil
		}

		for _, utxoID := range utxoIDs {
			lastIndex = utxoID // The last searched UTXO - not the last found

			if seen.Contains(utxoID) { // Already have this UTXO in the list
				continue
			}

			utxo, err := vm.state.UTXO(utxoID)
			if err != nil {
				return ids.ID{}, err
			}
			*utxos = append(*utxos, utxo)
			seen.Add(utxoID)
		}
	}
}

/*
 ******************************************************************************
 *********************************** Fx API ***********************************
 ******************************************************************************
 */

// Clock returns a reference to the internal clock of this VM
func (vm *VM) Clock() *timer.Clock { return &vm.clock }

// Codec returns a reference to the internal codec of this VM
func (vm *VM) Codec() codec.Manager { return vm.codec }

// CodecRegistry returns a reference to the internal codec registry of this VM
func (vm *VM) CodecRegistry() codec.Registry { return vm.codecRegistry }

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
	if _, err := vm.genesisCodec.Unmarshal(genesisBytes, &genesis); err != nil {
		return err
	}

	for _, genesisTx := range genesis.Txs {
		if len(genesisTx.Outs) != 0 {
			return errGenesisAssetMustHaveState
		}

		tx := Tx{
			UnsignedTx: &genesisTx.CreateAssetTx,
		}
		if err := tx.SignSECP256K1Fx(vm.genesisCodec, nil); err != nil {
			return err
		}

		txID := tx.ID()
		if err := vm.Alias(txID, genesisTx.Alias); err != nil {
			return err
		}
	}

	return nil
}

func (vm *VM) initState(genesisBytes []byte) error {
	genesis := Genesis{}
	if _, err := vm.genesisCodec.Unmarshal(genesisBytes, &genesis); err != nil {
		return err
	}

	for _, genesisTx := range genesis.Txs {
		if len(genesisTx.Outs) != 0 {
			return errGenesisAssetMustHaveState
		}

		tx := Tx{
			UnsignedTx: &genesisTx.CreateAssetTx,
		}
		if err := tx.SignSECP256K1Fx(vm.genesisCodec, nil); err != nil {
			return err
		}

		txID := tx.ID()
		vm.ctx.Log.Info("initializing with AssetID %s", txID)
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

func (vm *VM) parseTx(bytes []byte) (*UniqueTx, error) {
	rawTx, err := vm.parsePrivateTx(bytes)
	if err != nil {
		return nil, err
	}

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

func (vm *VM) parsePrivateTx(txBytes []byte) (*Tx, error) {
	tx := &Tx{}
	_, err := vm.codec.Unmarshal(txBytes, tx)
	if err != nil {
		return nil, err
	}
	unsignedBytes, err := vm.codec.Marshal(codecVersion, &tx.UnsignedTx)
	if err != nil {
		return nil, err
	}
	tx.Initialize(unsignedBytes, txBytes)
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

func (vm *VM) getUTXO(utxoID *avax.UTXOID) (*avax.UTXO, error) {
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

	if err := parent.verifyWithoutCacheWrites(); err != nil {
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
	// Check cache to see whether this asset supports this fx
	fxIDsIntf, assetInCache := vm.assetToFxCache.Get(assetID)
	if assetInCache {
		return fxIDsIntf.(ids.BitSet).Contains(uint(fxID))
	}
	// Caches doesn't say whether this asset support this fx.
	// Get the tx that created the asset and check.
	tx := &UniqueTx{
		vm:   vm,
		txID: assetID,
	}
	if status := tx.Status(); !status.Fetched() {
		return false
	}
	createAssetTx, ok := tx.UnsignedTx.(*CreateAssetTx)
	if !ok {
		// This transaction was not an asset creation tx
		return false
	}
	fxIDs := ids.BitSet(0)
	for _, state := range createAssetTx.States {
		if state.FxID == uint32(fxID) {
			// Cache that this asset supports this fx
			fxIDs.Add(uint(fxID))
		}
	}
	vm.assetToFxCache.Put(assetID, fxIDs)
	return fxIDs.Contains(uint(fxID))
}

func (vm *VM) verifyTransferOfUTXO(tx UnsignedTx, in *avax.TransferableInput, cred verify.Verifiable, utxo *avax.UTXO) error {
	fxIndex, err := vm.getFx(cred)
	if err != nil {
		return err
	}
	fx := vm.fxs[fxIndex].Fx

	utxoAssetID := utxo.AssetID()
	inAssetID := in.AssetID()
	if utxoAssetID != inAssetID {
		return errAssetIDMismatch
	}

	if !vm.verifyFxUsage(fxIndex, inAssetID) {
		return errIncompatibleFx
	}

	return fx.VerifyTransfer(tx, in.In, cred, utxo.Out)
}

func (vm *VM) verifyTransfer(tx UnsignedTx, in *avax.TransferableInput, cred verify.Verifiable) error {
	utxo, err := vm.getUTXO(&in.UTXOID)
	if err != nil {
		return err
	}
	return vm.verifyTransferOfUTXO(tx, in, cred, utxo)
}

func (vm *VM) verifyOperation(tx UnsignedTx, op *Operation, cred verify.Verifiable) error {
	opAssetID := op.AssetID()

	numUTXOs := len(op.UTXOIDs)
	utxos := make([]interface{}, numUTXOs)
	for i, utxoID := range op.UTXOIDs {
		utxo, err := vm.getUTXO(utxoID)
		if err != nil {
			return err
		}

		utxoAssetID := utxo.AssetID()
		if utxoAssetID != opAssetID {
			return errAssetIDMismatch
		}
		utxos[i] = utxo.Out
	}

	fxIndex, err := vm.getFx(op.Op)
	if err != nil {
		return err
	}
	fx := vm.fxs[fxIndex].Fx

	if !vm.verifyFxUsage(fxIndex, opAssetID) {
		return errIncompatibleFx
	}
	return fx.VerifyOperation(tx, op.Op, cred, utxos)
}

// LoadUser returns:
// 1) The UTXOs that reference one or more addresses controlled by the given user
// 2) A keychain that contains this user's keys
// If [addrsToUse] has positive length, returns UTXOs that reference one or more
// addresses controlled by the given user that are also in [addrsToUse].
func (vm *VM) LoadUser(
	username string,
	password string,
	addrsToUse ids.ShortSet,
) (
	[]*avax.UTXO,
	*secp256k1fx.Keychain,
	error,
) {
	db, err := vm.ctx.Keystore.GetDatabase(username, password)
	if err != nil {
		return nil, nil, fmt.Errorf("problem retrieving user: %w", err)
	}
	// Drop any potential error closing the database to report the original
	// error
	defer db.Close()

	user := userState{vm: vm}

	kc, err := user.Keychain(db, addrsToUse)
	if err != nil {
		return nil, nil, err
	}

	utxos, _, _, err := vm.GetUTXOs(kc.Addresses(), ids.ShortEmpty, ids.Empty, -1, false)
	if err != nil {
		return nil, nil, fmt.Errorf("problem retrieving user's UTXOs: %w", err)
	}

	return utxos, kc, db.Close()
}

// Spend ...
func (vm *VM) Spend(
	utxos []*avax.UTXO,
	kc *secp256k1fx.Keychain,
	amounts map[ids.ID]uint64,
) (
	map[ids.ID]uint64,
	[]*avax.TransferableInput,
	[][]*crypto.PrivateKeySECP256K1R,
	error,
) {
	amountsSpent := make(map[ids.ID]uint64, len(amounts))
	time := vm.clock.Unix()

	ins := []*avax.TransferableInput{}
	keys := [][]*crypto.PrivateKeySECP256K1R{}
	for _, utxo := range utxos {
		assetID := utxo.AssetID()
		amount := amounts[assetID]
		amountSpent := amountsSpent[assetID]

		if amountSpent >= amount {
			// we already have enough inputs allocated to this asset
			continue
		}

		inputIntf, signers, err := kc.Spend(utxo.Out, time)
		if err != nil {
			// this utxo can't be spent with the current keys right now
			continue
		}
		input, ok := inputIntf.(avax.TransferableIn)
		if !ok {
			// this input doesn't have an amount, so I don't care about it here
			continue
		}
		newAmountSpent, err := safemath.Add64(amountSpent, input.Amount())
		if err != nil {
			// there was an error calculating the consumed amount, just error
			return nil, nil, nil, errSpendOverflow
		}
		amountsSpent[assetID] = newAmountSpent

		// add the new input to the array
		ins = append(ins, &avax.TransferableInput{
			UTXOID: utxo.UTXOID,
			Asset:  avax.Asset{ID: assetID},
			In:     input,
		})
		// add the required keys to the array
		keys = append(keys, signers)
	}

	for asset, amount := range amounts {
		if amountsSpent[asset] < amount {
			return nil, nil, nil, fmt.Errorf("want to spend %d of asset %s but only have %d",
				amount,
				asset,
				amountsSpent[asset],
			)
		}
	}

	avax.SortTransferableInputsWithSigners(ins, keys)
	return amountsSpent, ins, keys, nil
}

// SpendNFT ...
func (vm *VM) SpendNFT(
	utxos []*avax.UTXO,
	kc *secp256k1fx.Keychain,
	assetID ids.ID,
	groupID uint32,
	to ids.ShortID,
) (
	[]*Operation,
	[][]*crypto.PrivateKeySECP256K1R,
	error,
) {
	time := vm.clock.Unix()

	ops := []*Operation{}
	keys := [][]*crypto.PrivateKeySECP256K1R{}

	for _, utxo := range utxos {
		// makes sure that the variable isn't overwritten with the next iteration
		utxo := utxo

		if len(ops) > 0 {
			// we have already been able to create the operation needed
			break
		}

		if utxo.AssetID() != assetID {
			// wrong asset ID
			continue
		}
		out, ok := utxo.Out.(*nftfx.TransferOutput)
		if !ok {
			// wrong output type
			continue
		}
		if out.GroupID != groupID {
			// wrong group id
			continue
		}
		indices, signers, ok := kc.Match(&out.OutputOwners, time)
		if !ok {
			// unable to spend the output
			continue
		}

		// add the new operation to the array
		ops = append(ops, &Operation{
			Asset:   utxo.Asset,
			UTXOIDs: []*avax.UTXOID{&utxo.UTXOID},
			Op: &nftfx.TransferOperation{
				Input: secp256k1fx.Input{
					SigIndices: indices,
				},
				Output: nftfx.TransferOutput{
					GroupID: out.GroupID,
					Payload: out.Payload,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{to},
					},
				},
			},
		})
		// add the required keys to the array
		keys = append(keys, signers)
	}

	if len(ops) == 0 {
		return nil, nil, errInsufficientFunds
	}

	sortOperationsWithSigners(ops, keys, vm.codec)
	return ops, keys, nil
}

// SpendAll ...
func (vm *VM) SpendAll(
	utxos []*avax.UTXO,
	kc *secp256k1fx.Keychain,
) (
	map[ids.ID]uint64,
	[]*avax.TransferableInput,
	[][]*crypto.PrivateKeySECP256K1R,
	error,
) {
	amountsSpent := make(map[ids.ID]uint64)
	time := vm.clock.Unix()

	ins := []*avax.TransferableInput{}
	keys := [][]*crypto.PrivateKeySECP256K1R{}
	for _, utxo := range utxos {
		assetID := utxo.AssetID()
		amountSpent := amountsSpent[assetID]

		inputIntf, signers, err := kc.Spend(utxo.Out, time)
		if err != nil {
			// this utxo can't be spent with the current keys right now
			continue
		}
		input, ok := inputIntf.(avax.TransferableIn)
		if !ok {
			// this input doesn't have an amount, so I don't care about it here
			continue
		}
		newAmountSpent, err := safemath.Add64(amountSpent, input.Amount())
		if err != nil {
			// there was an error calculating the consumed amount, just error
			return nil, nil, nil, errSpendOverflow
		}
		amountsSpent[assetID] = newAmountSpent

		// add the new input to the array
		ins = append(ins, &avax.TransferableInput{
			UTXOID: utxo.UTXOID,
			Asset:  avax.Asset{ID: assetID},
			In:     input,
		})
		// add the required keys to the array
		keys = append(keys, signers)
	}

	avax.SortTransferableInputsWithSigners(ins, keys)
	return amountsSpent, ins, keys, nil
}

// Mint ...
func (vm *VM) Mint(
	utxos []*avax.UTXO,
	kc *secp256k1fx.Keychain,
	amounts map[ids.ID]uint64,
	to ids.ShortID,
) (
	[]*Operation,
	[][]*crypto.PrivateKeySECP256K1R,
	error,
) {
	time := vm.clock.Unix()

	ops := []*Operation{}
	keys := [][]*crypto.PrivateKeySECP256K1R{}

	for _, utxo := range utxos {
		// makes sure that the variable isn't overwritten with the next iteration
		utxo := utxo

		assetID := utxo.AssetID()
		amount := amounts[assetID]
		if amount == 0 {
			continue
		}

		out, ok := utxo.Out.(*secp256k1fx.MintOutput)
		if !ok {
			continue
		}

		inIntf, signers, err := kc.Spend(out, time)
		if err != nil {
			continue
		}

		in, ok := inIntf.(*secp256k1fx.Input)
		if !ok {
			continue
		}

		// add the operation to the array
		ops = append(ops, &Operation{
			Asset:   utxo.Asset,
			UTXOIDs: []*avax.UTXOID{&utxo.UTXOID},
			Op: &secp256k1fx.MintOperation{
				MintInput:  *in,
				MintOutput: *out,
				TransferOutput: secp256k1fx.TransferOutput{
					Amt: amount,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{to},
					},
				},
			},
		})
		// add the required keys to the array
		keys = append(keys, signers)

		// remove the asset from the required amounts to mint
		delete(amounts, assetID)
	}

	for _, amount := range amounts {
		if amount > 0 {
			return nil, nil, errAddressesCantMintAsset
		}
	}

	sortOperationsWithSigners(ops, keys, vm.codec)
	return ops, keys, nil
}

// MintNFT ...
func (vm *VM) MintNFT(
	utxos []*avax.UTXO,
	kc *secp256k1fx.Keychain,
	assetID ids.ID,
	payload []byte,
	to ids.ShortID,
) (
	[]*Operation,
	[][]*crypto.PrivateKeySECP256K1R,
	error,
) {
	time := vm.clock.Unix()

	ops := []*Operation{}
	keys := [][]*crypto.PrivateKeySECP256K1R{}

	for _, utxo := range utxos {
		// makes sure that the variable isn't overwritten with the next iteration
		utxo := utxo

		if len(ops) > 0 {
			// we have already been able to create the operation needed
			break
		}

		if utxo.AssetID() != assetID {
			// wrong asset id
			continue
		}
		out, ok := utxo.Out.(*nftfx.MintOutput)
		if !ok {
			// wrong output type
			continue
		}

		indices, signers, ok := kc.Match(&out.OutputOwners, time)
		if !ok {
			// unable to spend the output
			continue
		}

		// add the operation to the array
		ops = append(ops, &Operation{
			Asset: avax.Asset{ID: assetID},
			UTXOIDs: []*avax.UTXOID{
				&utxo.UTXOID,
			},
			Op: &nftfx.MintOperation{
				MintInput: secp256k1fx.Input{
					SigIndices: indices,
				},
				GroupID: out.GroupID,
				Payload: payload,
				Outputs: []*secp256k1fx.OutputOwners{{
					Threshold: 1,
					Addrs:     []ids.ShortID{to},
				}},
			},
		})
		// add the required keys to the array
		keys = append(keys, signers)
	}

	if len(ops) == 0 {
		return nil, nil, errAddressesCantMintAsset
	}

	sortOperationsWithSigners(ops, keys, vm.codec)
	return ops, keys, nil
}

// ParseLocalAddress takes in an address for this chain and produces the ID
func (vm *VM) ParseLocalAddress(addrStr string) (ids.ShortID, error) {
	chainID, addr, err := vm.ParseAddress(addrStr)
	if err != nil {
		return ids.ShortID{}, err
	}
	if chainID != vm.ctx.ChainID {
		return ids.ShortID{}, fmt.Errorf("expected chainID to be %q but was %q",
			vm.ctx.ChainID, chainID)
	}
	return addr, nil
}

// ParseAddress takes in an address and produces the ID of the chain it's for
// the ID of the address
func (vm *VM) ParseAddress(addrStr string) (ids.ID, ids.ShortID, error) {
	chainIDAlias, hrp, addrBytes, err := formatting.ParseAddress(addrStr)
	if err != nil {
		return ids.ID{}, ids.ShortID{}, err
	}

	chainID, err := vm.ctx.BCLookup.Lookup(chainIDAlias)
	if err != nil {
		return ids.ID{}, ids.ShortID{}, err
	}

	expectedHRP := constants.GetHRP(vm.ctx.NetworkID)
	if hrp != expectedHRP {
		return ids.ID{}, ids.ShortID{}, fmt.Errorf("expected hrp %q but got %q",
			expectedHRP, hrp)
	}

	addr, err := ids.ToShortID(addrBytes)
	if err != nil {
		return ids.ID{}, ids.ShortID{}, err
	}
	return chainID, addr, nil
}

// FormatLocalAddress takes in a raw address and produces the formatted address
func (vm *VM) FormatLocalAddress(addr ids.ShortID) (string, error) {
	return vm.FormatAddress(vm.ctx.ChainID, addr)
}

// FormatAddress takes in a chainID and a raw address and produces the formatted
// address
func (vm *VM) FormatAddress(chainID ids.ID, addr ids.ShortID) (string, error) {
	chainIDAlias, err := vm.ctx.BCLookup.PrimaryAlias(chainID)
	if err != nil {
		return "", err
	}
	hrp := constants.GetHRP(vm.ctx.NetworkID)
	return formatting.FormatAddress(chainIDAlias, hrp, addr.Bytes())
}

// selectChangeAddr returns the change address to be used for [kc] when [changeAddr] is given
// as the optional change address argument
func (vm *VM) selectChangeAddr(defaultAddr ids.ShortID, changeAddr string) (ids.ShortID, error) {
	if changeAddr == "" {
		return defaultAddr, nil
	}
	addr, err := vm.ParseLocalAddress(changeAddr)
	if err != nil {
		return ids.ShortID{}, fmt.Errorf("couldn't parse changeAddr: %w", err)
	}
	return addr, nil
}

// lookupAssetID looks for an ID aliased by [asset] and if it fails
// attempts to parse [asset] into an ID
func (vm *VM) lookupAssetID(asset string) (ids.ID, error) {
	if assetID, err := vm.Lookup(asset); err == nil {
		return assetID, nil
	}
	if assetID, err := ids.FromString(asset); err == nil {
		return assetID, nil
	}
	return ids.ID{}, fmt.Errorf("asset '%s' not found", asset)
}
