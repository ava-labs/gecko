// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"container/heap"
	"errors"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/chains"
	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/engine/snowman/block"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/core"
	"github.com/ava-labs/avalanchego/vms/components/state"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"

	safemath "github.com/ava-labs/avalanchego/utils/math"
)

const (
	// TODO: remove skipped values on the next DB migration
	// For putting/getting values from state
	validatorsTypeID uint64 = iota
	chainsTypeID
	_
	subnetsTypeID
	utxoTypeID
	_
	txTypeID
	statusTypeID
	currentSupplyTypeID

	// PercentDenominator is the denominator used to calculate percentages
	PercentDenominator = 1000000

	droppedTxCacheSize = 50

	maxUTXOsToFetch = 1024

	// TODO: Turn these constants into governable parameters

	// MaxSubMinConsumptionRate is the % consumption that incentivizes staking
	// longer
	MaxSubMinConsumptionRate = 20000 // 2%
	// MinConsumptionRate is the minimum % consumption of the remaining tokens
	// to be minted
	MinConsumptionRate = 100000 // 10%

	// SupplyCap is the maximum amount of AVAX that should ever exist
	SupplyCap = 720 * units.MegaAvax

	// Maximum future start time for staking/delegating
	maxFutureStartTime = 24 * 7 * 2 * time.Hour
)

var (
	timestampKey     = ids.ID{'t', 'i', 'm', 'e'}
	chainsKey        = ids.ID{'c', 'h', 'a', 'i', 'n', 's'}
	subnetsKey       = ids.ID{'s', 'u', 'b', 'n', 'e', 't', 's'}
	currentSupplyKey = ids.ID{'c', 'u', 'r', 'r', 'e', 't', ' ', 's', 'u', 'p', 'p', 'l', 'y'}

	errRegisteringType          = errors.New("error registering type with database")
	errInvalidLastAcceptedBlock = errors.New("last accepted block must be a decision block")
	errInvalidID                = errors.New("invalid ID")
	errDSCantValidate           = errors.New("new blockchain can't be validated by primary network")
	errStartTimeTooLate         = errors.New("start time is too far in the future")
	errStartTimeTooEarly        = errors.New("start time is before the current chain time")
	errStartAfterEndTime        = errors.New("start time is after the end time")

	_ block.ChainVM        = &VM{}
	_ validators.Connector = &VM{}
	_ common.StaticVM      = &VM{}
)

// VM implements the snowman.ChainVM interface
type VM struct {
	metrics
	*core.SnowmanVM

	// Node's validator manager
	// Maps Subnets --> validators of the Subnet
	vdrMgr validators.Manager

	// true if the node is being run with staking enabled
	stakingEnabled bool

	// The node's chain manager
	chainManager chains.Manager

	fx            Fx
	codec         codec.Manager
	codecRegistry codec.Registry

	mempool Mempool

	// Used to create and use keys.
	factory crypto.FactorySECP256K1R

	// Used to get time. Useful for faking time during tests.
	clock timer.Clock

	// Key: block ID
	// Value: the block
	currentBlocks map[ids.ID]Block

	// fee that must be burned by every state creating transaction
	creationTxFee uint64
	// fee that must be burned by every non-state creating transaction
	txFee uint64

	// UptimePercentage is the minimum uptime required to be rewarded for staking.
	uptimePercentage float64

	// The minimum amount of tokens one must bond to be a validator
	minValidatorStake uint64

	// The maximum amount of tokens one can bond to a validator
	maxValidatorStake uint64

	// Minimum stake, in nAVAX, that can be delegated on the primary network
	minDelegatorStake uint64

	// Minimum fee that can be charged for delegation
	minDelegationFee uint32

	// Minimum amount of time to allow a validator to stake
	minStakeDuration time.Duration

	// Maximum amount of time to allow a validator to stake
	maxStakeDuration time.Duration

	// Consumption period for the minting function
	stakeMintingPeriod time.Duration

	// Time of the apricot phase 0 rule change
	apricotPhase0Time time.Time

	// Contains the IDs of transactions recently dropped because they failed verification.
	// These txs may be re-issued and put into accepted blocks, so check the database
	// to see if it was later committed/aborted before reporting that it's dropped.
	// Key: Tx ID
	// Value: String repr. of the verification error
	droppedTxCache cache.LRU

	// Bootstrapped remembers if this chain has finished bootstrapping or not
	bootstrapped bool

	bootstrappedTime time.Time

	connections map[ids.ShortID]time.Time
}

// Initialize this blockchain.
// [vm.ChainManager] and [vm.vdrMgr] must be set before this function is called.
func (vm *VM) Initialize(
	ctx *snow.Context,
	db database.Database,
	genesisBytes []byte,
	msgs chan<- common.Message,
	_ []*common.Fx,
) error {
	ctx.Log.Verbo("initializing platform chain")

	// Initialize metrics as soon as possible
	if err := vm.metrics.Initialize(ctx.Namespace, ctx.Metrics); err != nil {
		return err
	}

	// Initialize the inner VM, which has a lot of boiler-plate logic
	vm.SnowmanVM = &core.SnowmanVM{}
	if err := vm.SnowmanVM.Initialize(ctx, db, vm.unmarshalBlockFunc, msgs); err != nil {
		return err
	}
	vm.fx = &secp256k1fx.Fx{}

	vm.codec = Codec
	vm.codecRegistry = linearcodec.NewDefault()
	if err := vm.fx.Initialize(vm); err != nil {
		return err
	}

	vm.droppedTxCache = cache.LRU{Size: droppedTxCacheSize}
	vm.connections = make(map[ids.ShortID]time.Time)

	// Register this VM's types with the database so we can get/put structs to/from it
	vm.registerDBTypes()

	vm.mempool.Initialize(vm)

	// If the database is empty, create the platform chain anew using
	// the provided genesis state
	if !vm.DBInitialized() {
		genesis := &Genesis{}
		if _, err := GenesisCodec.Unmarshal(genesisBytes, genesis); err != nil {
			return err
		}
		if err := genesis.Initialize(); err != nil {
			return err
		}

		// Persist UTXOs that exist at genesis
		for _, utxo := range genesis.UTXOs {
			if err := vm.putUTXO(vm.DB, &utxo.UTXO); err != nil {
				return err
			}
		}

		// Persist the platform chain's timestamp at genesis
		genesisTime := time.Unix(int64(genesis.Timestamp), 0)
		if err := vm.State.PutTime(vm.DB, timestampKey, genesisTime); err != nil {
			return err
		}

		if err := vm.putCurrentSupply(vm.DB, genesis.InitialSupply); err != nil {
			return err
		}

		// Persist primary network validator set at genesis
		for _, vdrTx := range genesis.Validators {
			var (
				stakeAmount   uint64
				stakeDuration time.Duration
			)
			switch tx := vdrTx.UnsignedTx.(type) {
			case *UnsignedAddValidatorTx:
				stakeAmount = tx.Validator.Wght
				stakeDuration = tx.Validator.Duration()
			default:
				return errWrongTxType
			}
			reward, err := vm.calculateReward(vm.DB, stakeDuration, stakeAmount)
			if err != nil {
				return err
			}
			tx := rewardTx{
				Reward: reward,
				Tx:     *vdrTx,
			}
			if err := vm.addStaker(vm.DB, constants.PrimaryNetworkID, &tx); err != nil {
				return err
			}
		}

		// Persist the subnets that exist at genesis (none do)
		if err := vm.putSubnets(vm.DB, []*Tx{}); err != nil {
			return fmt.Errorf("error putting genesis subnets: %v", err)
		}

		// Ensure all chains that the genesis bytes say to create
		// have the right network ID
		filteredChains := []*Tx{}
		for _, chain := range genesis.Chains {
			unsignedChain, ok := chain.UnsignedTx.(*UnsignedCreateChainTx)
			if !ok {
				vm.Ctx.Log.Warn("invalid tx type in genesis chains")
				continue
			}
			if unsignedChain.NetworkID != vm.Ctx.NetworkID {
				vm.Ctx.Log.Warn("chain has networkID %d, expected %d", unsignedChain.NetworkID, vm.Ctx.NetworkID)
				continue
			}
			filteredChains = append(filteredChains, chain)
		}

		// Persist the chains that exist at genesis
		if err := vm.putChains(vm.DB, filteredChains); err != nil {
			return err
		}

		// Create the genesis block and save it as being accepted (We don't just
		// do genesisBlock.Accept() because then it'd look for genesisBlock's
		// non-existent parent)
		genesisID := hashing.ComputeHash256Array(genesisBytes)
		genesisBlock, err := vm.newCommitBlock(genesisID, 0)
		if err != nil {
			return err
		}
		if err := vm.State.PutBlock(vm.DB, genesisBlock); err != nil {
			return err
		}
		genesisBlock.onAcceptDB = versiondb.New(vm.DB)
		if err := genesisBlock.CommonBlock.Accept(); err != nil {
			return fmt.Errorf("error accepting genesis block: %w", err)
		}

		if err := vm.SetDBInitialized(); err != nil {
			return fmt.Errorf("error while setting db to initialized: %w", err)
		}

		if err := vm.DB.Commit(); err != nil {
			return err
		}
	}

	vm.currentBlocks = make(map[ids.ID]Block)

	if err := vm.initSubnets(); err != nil {
		ctx.Log.Error("failed to initialize Subnets: %s", err)
		return err
	}

	// Create all of the chains that the database says exist
	if err := vm.initBlockchains(); err != nil {
		vm.Ctx.Log.Warn("could not retrieve existing chains from database: %s", err)
		return err
	}

	lastAcceptedID, err := vm.LastAccepted()
	if err != nil {
		vm.Ctx.Log.Error("Error fetching the last accepted block ID (%s), %s", lastAcceptedID, err)
		return err
	}
	vm.Ctx.Log.Info("initializing last accepted block as %s", lastAcceptedID)

	// Build off the most recently accepted block
	if err := vm.SetPreference(lastAcceptedID); err != nil {
		vm.Ctx.Log.Error("Error setting the preference to the last accepted block (%s), %s", lastAcceptedID, err)
		return err
	}

	// Sanity check to make sure the DB is in a valid state
	lastAcceptedIntf, err := vm.getBlock(lastAcceptedID)
	if err != nil {
		vm.Ctx.Log.Error("Error fetching the last accepted block (%s), %s", vm.Preferred(), err)
		return err
	}
	if _, ok := lastAcceptedIntf.(decision); !ok {
		vm.Ctx.Log.Fatal("The last accepted block, %s, must always be a decision block", lastAcceptedID)
		return errInvalidLastAcceptedBlock
	}

	return nil
}

// Create all chains that exist that this node validates
// Can only be called after initSubnets()
func (vm *VM) initBlockchains() error {
	blockchains, err := vm.getChains(vm.DB) // get blockchains that exist
	if err != nil {
		return err
	}

	for _, chain := range blockchains {
		vm.createChain(chain)
	}
	return nil
}

// Set the node's validator manager to be up to date
func (vm *VM) initSubnets() error {
	if err := vm.updateValidators(vm.DB); err != nil {
		return err
	}
	return vm.updateVdrMgr(true)
}

// Create the blockchain described in [tx], but only if this node is a member of
// the Subnet that validates the chain
func (vm *VM) createChain(tx *Tx) {
	unsignedTx, ok := tx.UnsignedTx.(*UnsignedCreateChainTx)
	if !ok {
		// Invalid tx type
		return
	}
	// The validators that compose the Subnet that validates this chain
	validators, subnetExists := vm.vdrMgr.GetValidators(unsignedTx.SubnetID)
	if !subnetExists {
		vm.Ctx.Log.Error("blockchain %s validated by Subnet %s but couldn't get that Subnet. Blockchain not created",
			tx.ID(), unsignedTx.SubnetID)
		return
	}
	if vm.stakingEnabled && // Staking is enabled, so nodes might not validate all chains
		constants.PrimaryNetworkID != unsignedTx.SubnetID && // All nodes must validate the primary network
		!validators.Contains(vm.Ctx.NodeID) { // This node doesn't validate this blockchain
		return
	}

	chainParams := chains.ChainParameters{
		ID:          tx.ID(),
		SubnetID:    unsignedTx.SubnetID,
		GenesisData: unsignedTx.GenesisData,
		VMAlias:     unsignedTx.VMID.String(),
	}
	for _, fxID := range unsignedTx.FxIDs {
		chainParams.FxAliases = append(chainParams.FxAliases, fxID.String())
	}
	vm.chainManager.CreateChain(chainParams)
}

// Bootstrapping marks this VM as bootstrapping
func (vm *VM) Bootstrapping() error { vm.bootstrapped = false; return vm.fx.Bootstrapping() }

// Bootstrapped marks this VM as bootstrapped
func (vm *VM) Bootstrapped() error {
	vm.bootstrapped = true
	vm.bootstrappedTime = time.Unix(vm.clock.Time().Unix(), 0)

	errs := wrappers.Errs{}
	errs.Add(
		vm.updateVdrMgr(false),
		vm.fx.Bootstrapped(),
	)
	if errs.Errored() {
		return errs.Err
	}

	stopPrefix := []byte(fmt.Sprintf("%s%s", constants.PrimaryNetworkID, stopDBPrefix))
	stopDB := prefixdb.NewNested(stopPrefix, vm.DB)
	defer stopDB.Close()

	stopIter := stopDB.NewIterator()
	defer stopIter.Release()

	for stopIter.Next() { // Iterates in order of increasing stop time
		txBytes := stopIter.Value()

		tx := rewardTx{}
		if _, err := vm.codec.Unmarshal(txBytes, &tx); err != nil {
			return fmt.Errorf("couldn't unmarshal validator tx: %w", err)
		}
		if err := tx.Tx.Sign(vm.codec, nil); err != nil {
			return err
		}

		unsignedTx, ok := tx.Tx.UnsignedTx.(*UnsignedAddValidatorTx)
		if !ok {
			continue
		}

		nodeID := unsignedTx.Validator.ID()

		uptime, err := vm.uptime(vm.DB, nodeID)
		switch {
		case err == database.ErrNotFound:
			uptime = &validatorUptime{
				LastUpdated: uint64(unsignedTx.StartTime().Unix()),
			}
		case err != nil:
			return err
		}

		lastUpdated := time.Unix(int64(uptime.LastUpdated), 0)
		if !vm.bootstrappedTime.After(lastUpdated) {
			continue
		}

		durationOffline := vm.bootstrappedTime.Sub(lastUpdated)

		uptime.UpDuration += uint64(durationOffline / time.Second)
		uptime.LastUpdated = uint64(vm.bootstrappedTime.Unix())

		if err := vm.setUptime(vm.DB, nodeID, uptime); err != nil {
			return err
		}
	}
	if err := stopIter.Error(); err != nil {
		return err
	}

	errs.Add(
		vm.DB.Commit(),
		stopDB.Close(),
	)
	return errs.Err
}

// Shutdown this blockchain
func (vm *VM) Shutdown() error {
	if vm.DB == nil {
		return nil
	}

	vm.mempool.Shutdown()

	stopPrefix := []byte(fmt.Sprintf("%s%s", constants.PrimaryNetworkID, stopDBPrefix))
	stopDB := prefixdb.NewNested(stopPrefix, vm.DB)
	defer stopDB.Close()

	stopIter := stopDB.NewIterator()
	defer stopIter.Release()

	for stopIter.Next() { // Iterates in order of increasing stop time
		txBytes := stopIter.Value()

		tx := rewardTx{}
		if _, err := vm.codec.Unmarshal(txBytes, &tx); err != nil {
			return fmt.Errorf("couldn't unmarshal validator tx: %w", err)
		}
		if err := tx.Tx.Sign(vm.codec, nil); err != nil {
			return err
		}

		staker, ok := tx.Tx.UnsignedTx.(*UnsignedAddValidatorTx)
		if !ok {
			continue
		}
		nodeID := staker.Validator.ID()
		startTime := staker.StartTime()

		uptime, err := vm.uptime(vm.DB, nodeID)
		switch {
		case err == database.ErrNotFound:
			uptime = &validatorUptime{
				LastUpdated: uint64(startTime.Unix()),
			}
		case err != nil:
			return err
		}

		currentLocalTime := vm.clock.Time()
		timeConnected := currentLocalTime
		if realTimeConnected, isConnected := vm.connections[nodeID]; isConnected {
			timeConnected = realTimeConnected
		}
		if timeConnected.Before(vm.bootstrappedTime) {
			timeConnected = vm.bootstrappedTime
		}

		lastUpdated := time.Unix(int64(uptime.LastUpdated), 0)
		if timeConnected.Before(lastUpdated) {
			timeConnected = lastUpdated
		}

		// If the current local time is before the time this peer
		// was marked as connnected skip updating its uptime.
		if currentLocalTime.Before(timeConnected) {
			continue
		}

		uptime.UpDuration += uint64(currentLocalTime.Sub(timeConnected) / time.Second)
		uptime.LastUpdated = uint64(currentLocalTime.Unix())

		if err := vm.setUptime(vm.DB, nodeID, uptime); err != nil {
			vm.Ctx.Log.Error("failed to write back uptime data")
		}
	}
	if err := stopIter.Error(); err != nil {
		return err
	}

	errs := wrappers.Errs{}
	errs.Add(
		vm.DB.Commit(),
		stopDB.Close(),
		vm.DB.Close(),
	)
	return errs.Err
}

// BuildBlock builds a block to be added to consensus
func (vm *VM) BuildBlock() (snowman.Block, error) { return vm.mempool.BuildBlock() }

// ParseBlock implements the snowman.ChainVM interface
func (vm *VM) ParseBlock(bytes []byte) (snowman.Block, error) {
	blockInterface, err := vm.unmarshalBlockFunc(bytes)
	if err != nil {
		return nil, errors.New("problem parsing block")
	}
	block, ok := blockInterface.(snowman.Block)
	if !ok { // in practice should never happen because unmarshalBlockFunc returns a snowman.Block
		return nil, errors.New("problem parsing block")
	}
	if block, err := vm.GetBlock(block.ID()); err == nil {
		// If we have seen this block before, return it with the most up-to-date info
		return block, nil
	}
	if err := vm.State.PutBlock(vm.DB, block); err != nil { // Persist the block
		return nil, fmt.Errorf("failed to put block due to %w", err)
	}

	return block, vm.DB.Commit()
}

// GetBlock implements the snowman.ChainVM interface
func (vm *VM) GetBlock(blkID ids.ID) (snowman.Block, error) { return vm.getBlock(blkID) }

func (vm *VM) getBlock(blkID ids.ID) (Block, error) {
	// If block is in memory, return it.
	if blk, exists := vm.currentBlocks[blkID]; exists {
		return blk, nil
	}
	// Block isn't in memory. If block is in database, return it.
	blkInterface, err := vm.State.GetBlock(vm.DB, blkID)
	if err != nil {
		return nil, err
	}
	if block, ok := blkInterface.(Block); ok {
		return block, nil
	}
	return nil, errors.New("block not found")
}

// SetPreference sets the preferred block to be the one with ID [blkID]
func (vm *VM) SetPreference(blkID ids.ID) error {
	if blkID != vm.Preferred() {
		if err := vm.SnowmanVM.SetPreference(blkID); err != nil {
			return err
		}
		vm.mempool.ResetTimer()
	}
	return nil
}

// CreateHandlers returns a map where:
// * keys are API endpoint extensions
// * values are API handlers
// See API documentation for more information
func (vm *VM) CreateHandlers() (map[string]*common.HTTPHandler, error) {
	// Create a service with name "platform"
	handler, err := vm.SnowmanVM.NewHandler("platform", &Service{vm: vm})
	return map[string]*common.HTTPHandler{"": handler}, err
}

// CreateStaticHandlers implements the snowman.ChainVM interface
func (vm *VM) CreateStaticHandlers() (map[string]*common.HTTPHandler, error) {
	// Static service's name is platform
	staticService := CreateStaticService()
	handler, err := vm.SnowmanVM.NewHandler("platform", staticService)
	return map[string]*common.HTTPHandler{
		"": handler,
	}, err
}

// Connected implements validators.Connector
func (vm *VM) Connected(vdrID ids.ShortID) {
	vm.connections[vdrID] = time.Unix(vm.clock.Time().Unix(), 0)
}

// Disconnected implements validators.Connector
func (vm *VM) Disconnected(vdrID ids.ShortID) {
	timeConnected, ok := vm.connections[vdrID]
	if !ok {
		return
	}
	delete(vm.connections, vdrID)

	if !vm.bootstrapped {
		return
	}

	txIntf, isValidator, err := vm.isValidator(vm.DB, constants.PrimaryNetworkID, vdrID)
	if err != nil || !isValidator {
		return
	}
	tx, ok := txIntf.(*UnsignedAddValidatorTx)
	if !ok {
		return
	}

	uptime, err := vm.uptime(vm.DB, vdrID)
	switch {
	case err == database.ErrNotFound:
		uptime = &validatorUptime{
			LastUpdated: uint64(tx.StartTime().Unix()),
		}
	case err != nil:
		return
	}

	if timeConnected.Before(vm.bootstrappedTime) {
		timeConnected = vm.bootstrappedTime
	}

	lastUpdated := time.Unix(int64(uptime.LastUpdated), 0)
	if timeConnected.Before(lastUpdated) {
		timeConnected = lastUpdated
	}

	currentLocalTime := vm.clock.Time()
	if !currentLocalTime.After(lastUpdated) {
		return
	}

	uptime.UpDuration += uint64(currentLocalTime.Sub(timeConnected) / time.Second)
	uptime.LastUpdated = uint64(currentLocalTime.Unix())

	if err := vm.setUptime(vm.DB, vdrID, uptime); err != nil {
		vm.Ctx.Log.Error("failed to write back uptime data")
	}
	if err := vm.DB.Commit(); err != nil {
		vm.Ctx.Log.Error("failed to commit database changes")
	}
}

// Returns the time when the next staker of any subnet starts/stops staking
// after the current timestamp
func (vm *VM) nextStakerChangeTime(db database.Database) (time.Time, error) {
	subnets, err := vm.getSubnets(db)
	if err != nil {
		return time.Time{}, fmt.Errorf("couldn't get subnets: %w", err)
	}
	earliest := timer.MaxTime

	for _, subnet := range subnets {
		subnetID := subnet.ID()
		if tx, err := vm.nextStakerStart(db, subnetID); err == nil {
			if staker, ok := tx.UnsignedTx.(TimedTx); ok {
				if startTime := staker.StartTime(); startTime.Before(earliest) {
					earliest = startTime
				}
			}
		}
		if tx, err := vm.nextStakerStop(db, subnetID); err == nil {
			if staker, ok := tx.Tx.UnsignedTx.(TimedTx); ok {
				if endTime := staker.EndTime(); endTime.Before(earliest) {
					earliest = endTime
				}
			}
		}
	}
	if tx, err := vm.nextStakerStart(db, constants.PrimaryNetworkID); err == nil {
		if staker, ok := tx.UnsignedTx.(TimedTx); ok {
			if startTime := staker.StartTime(); startTime.Before(earliest) {
				earliest = startTime
			}
		}
	}
	if tx, err := vm.nextStakerStop(db, constants.PrimaryNetworkID); err == nil {
		if staker, ok := tx.Tx.UnsignedTx.(TimedTx); ok {
			if endTime := staker.EndTime(); endTime.Before(earliest) {
				earliest = endTime
			}
		}
	}
	return earliest, nil
}

// update validator set of [subnetID] based on the current chain timestamp
func (vm *VM) updateValidators(db database.Database) error {
	timestamp, err := vm.getTimestamp(db)
	if err != nil {
		return fmt.Errorf("can't get timestamp: %w", err)
	}

	subnets, err := vm.getSubnets(db)
	if err != nil {
		return err
	}

	if err := vm.updateSubnetValidators(db, constants.PrimaryNetworkID, timestamp); err != nil {
		return err
	}
	for _, subnet := range subnets {
		if err := vm.updateSubnetValidators(db, subnet.ID(), timestamp); err != nil {
			return err
		}
	}
	return nil
}

func (vm *VM) calculateReward(db database.Database, duration time.Duration, stakeAmount uint64) (uint64, error) {
	currentSupply, err := vm.getCurrentSupply(db)
	if err != nil {
		return 0, err
	}
	reward := Reward(duration, stakeAmount, currentSupply, vm.stakeMintingPeriod)
	newSupply, err := safemath.Add64(currentSupply, reward)
	if err != nil {
		return 0, err
	}
	return reward, vm.putCurrentSupply(db, newSupply)
}

func (vm *VM) updateSubnetValidators(db database.Database, subnetID ids.ID, timestamp time.Time) error {
	startPrefix := []byte(fmt.Sprintf("%s%s", subnetID, startDBPrefix))
	startDB := prefixdb.NewNested(startPrefix, db)
	defer startDB.Close()

	startIter := startDB.NewIterator()
	defer startIter.Release()

pendingStakerLoop:
	for startIter.Next() { // Iterates in order of increasing start time
		txBytes := startIter.Value()

		tx := Tx{}
		if _, err := vm.codec.Unmarshal(txBytes, &tx); err != nil {
			return fmt.Errorf("couldn't unmarshal validator tx: %w", err)
		}

		switch staker := tx.UnsignedTx.(type) {
		case *UnsignedAddDelegatorTx:
			if subnetID != constants.PrimaryNetworkID {
				return fmt.Errorf("AddDelegatorTx is invalid for subnet %s",
					subnetID)
			}
			if staker.StartTime().After(timestamp) {
				break pendingStakerLoop
			}

			if err := tx.Sign(vm.codec, nil); err != nil {
				return err
			}

			if err := vm.dequeueStaker(db, subnetID, &tx); err != nil {
				return fmt.Errorf("couldn't dequeue staker: %w", err)
			}

			reward, err := vm.calculateReward(db, staker.Validator.Duration(), staker.Validator.Wght)
			if err != nil {
				return fmt.Errorf("couldn't calculate reward for staker: %w", err)
			}

			rTx := rewardTx{
				Reward: reward,
				Tx:     tx,
			}
			if err := vm.addStaker(db, subnetID, &rTx); err != nil {
				return fmt.Errorf("couldn't add staker: %w", err)
			}
		case *UnsignedAddValidatorTx:
			if subnetID != constants.PrimaryNetworkID {
				return fmt.Errorf("AddValidatorTx is invalid for subnet %s",
					subnetID)
			}
			if staker.StartTime().After(timestamp) {
				break pendingStakerLoop
			}

			if err := tx.Sign(vm.codec, nil); err != nil {
				return err
			}

			if err := vm.dequeueStaker(db, subnetID, &tx); err != nil {
				return fmt.Errorf("couldn't dequeue staker: %w", err)
			}

			reward, err := vm.calculateReward(db, staker.Validator.Duration(), staker.Validator.Wght)
			if err != nil {
				return fmt.Errorf("couldn't calculate reward for staker: %w", err)
			}

			rTx := rewardTx{
				Reward: reward,
				Tx:     tx,
			}
			if err := vm.addStaker(db, subnetID, &rTx); err != nil {
				return fmt.Errorf("couldn't add staker: %w", err)
			}
		case *UnsignedAddSubnetValidatorTx:
			if txSubnetID := staker.Validator.SubnetID(); subnetID != txSubnetID {
				return fmt.Errorf("AddSubnetValidatorTx references the incorrect subnet. Expected %s; Got %s",
					subnetID, txSubnetID)
			}
			if staker.StartTime().After(timestamp) {
				break pendingStakerLoop
			}

			if err := tx.Sign(vm.codec, nil); err != nil {
				return err
			}

			if err := vm.dequeueStaker(db, subnetID, &tx); err != nil {
				return fmt.Errorf("couldn't dequeue staker: %w", err)
			}

			rTx := rewardTx{
				Reward: 0,
				Tx:     tx,
			}
			if err := vm.addStaker(db, subnetID, &rTx); err != nil {
				return fmt.Errorf("couldn't add staker: %w", err)
			}
		default:
			return fmt.Errorf("expected validator but got %T", tx.UnsignedTx)
		}
	}

	stopPrefix := []byte(fmt.Sprintf("%s%s", subnetID, stopDBPrefix))
	stopDB := prefixdb.NewNested(stopPrefix, db)
	defer stopDB.Close()

	stopIter := stopDB.NewIterator()
	defer stopIter.Release()

currentStakerLoop:
	for stopIter.Next() { // Iterates in order of increasing stop time
		txBytes := stopIter.Value()

		tx := rewardTx{}
		if _, err := vm.codec.Unmarshal(txBytes, &tx); err != nil {
			return fmt.Errorf("couldn't unmarshal validator tx: %w", err)
		}

		switch staker := tx.Tx.UnsignedTx.(type) {
		case *UnsignedAddDelegatorTx:
			if subnetID != constants.PrimaryNetworkID {
				return fmt.Errorf("AddDelegatorTx is invalid for subnet %s",
					subnetID)
			}
			if staker.EndTime().After(timestamp) {
				break currentStakerLoop
			}
		case *UnsignedAddValidatorTx:
			if subnetID != constants.PrimaryNetworkID {
				return fmt.Errorf("AddValidatorTx is invalid for subnet %s",
					subnetID)
			}
			if staker.EndTime().After(timestamp) {
				break currentStakerLoop
			}
		case *UnsignedAddSubnetValidatorTx:
			if txSubnetID := staker.Validator.SubnetID(); subnetID != txSubnetID {
				return fmt.Errorf("AddSubnetValidatorTx references the incorrect subnet. Expected %s; Got %s",
					subnetID, txSubnetID)
			}
			if staker.EndTime().After(timestamp) {
				break currentStakerLoop
			}

			if err := tx.Tx.Sign(vm.codec, nil); err != nil {
				return err
			}

			if err := vm.removeStaker(db, subnetID, &tx); err != nil {
				return fmt.Errorf("couldn't remove staker: %w", err)
			}
		default:
			return fmt.Errorf("expected validator but got %T", tx.Tx.UnsignedTx)
		}
	}

	errs := wrappers.Errs{}
	errs.Add(
		startIter.Error(),
		startDB.Close(),
		stopIter.Error(),
		stopDB.Close(),
	)
	return errs.Err
}

func (vm *VM) updateVdrMgr(force bool) error {
	if !force && !vm.bootstrapped {
		return nil
	}

	subnets, err := vm.getSubnets(vm.DB)
	if err != nil {
		return err
	}

	if err := vm.updateVdrSet(constants.PrimaryNetworkID); err != nil {
		return err
	}
	for _, subnet := range subnets {
		if err := vm.updateVdrSet(subnet.ID()); err != nil {
			return err
		}
	}
	return vm.initBlockchains()
}

func (vm *VM) updateVdrSet(subnetID ids.ID) error {
	vdrs := validators.NewSet()

	stopPrefix := []byte(fmt.Sprintf("%s%s", subnetID, stopDBPrefix))
	stopDB := prefixdb.NewNested(stopPrefix, vm.DB)
	defer stopDB.Close()
	stopIter := stopDB.NewIterator()
	defer stopIter.Release()

	for stopIter.Next() { // Iterates in order of increasing stop time
		txBytes := stopIter.Value()

		tx := rewardTx{}
		if _, err := vm.codec.Unmarshal(txBytes, &tx); err != nil {
			return fmt.Errorf("couldn't unmarshal validator tx: %w", err)
		}
		if err := tx.Tx.Sign(vm.codec, nil); err != nil {
			return err
		}

		var err error
		switch staker := tx.Tx.UnsignedTx.(type) {
		case *UnsignedAddDelegatorTx:
			err = vdrs.AddWeight(staker.Validator.NodeID, staker.Validator.Weight())
		case *UnsignedAddValidatorTx:
			err = vdrs.AddWeight(staker.Validator.NodeID, staker.Validator.Weight())
		case *UnsignedAddSubnetValidatorTx:
			err = vdrs.AddWeight(staker.Validator.NodeID, staker.Validator.Weight())
		default:
			err = fmt.Errorf("expected validator but got %T", tx.Tx.UnsignedTx)
		}
		if err != nil {
			return err
		}
	}

	if subnetID == constants.PrimaryNetworkID {
		vm.totalStake.Set(float64(vdrs.Weight()) / float64(units.Avax))
	}

	errs := wrappers.Errs{}
	errs.Add(
		vm.vdrMgr.Set(subnetID, vdrs),
		stopIter.Error(),
		stopDB.Close(),
	)
	return errs.Err
}

// Codec ...
func (vm *VM) Codec() codec.Manager { return vm.codec }

// CodecRegistry ...
func (vm *VM) CodecRegistry() codec.Registry { return vm.codecRegistry }

// Clock ...
func (vm *VM) Clock() *timer.Clock { return &vm.clock }

// Logger ...
func (vm *VM) Logger() logging.Logger { return vm.Ctx.Log }

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
	startAddr state.Marshaller,
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

	allUTXOBytes, lastAddr, lastUTXO, err := vm.Ctx.SharedMemory.Indexed(
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

// ParseLocalAddress takes in an address for this chain and produces the ID
func (vm *VM) ParseLocalAddress(addrStr string) (ids.ShortID, error) {
	chainID, addr, err := vm.ParseAddress(addrStr)
	if err != nil {
		return ids.ShortID{}, err
	}
	if chainID != vm.Ctx.ChainID {
		return ids.ShortID{}, fmt.Errorf("expected chainID to be %q but was %q",
			vm.Ctx.ChainID, chainID)
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

	chainID, err := vm.Ctx.BCLookup.Lookup(chainIDAlias)
	if err != nil {
		return ids.ID{}, ids.ShortID{}, err
	}

	expectedHRP := constants.GetHRP(vm.Ctx.NetworkID)
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
	return vm.FormatAddress(vm.Ctx.ChainID, addr)
}

// FormatAddress takes in a chainID and a raw address and produces the formatted
// address
func (vm *VM) FormatAddress(chainID ids.ID, addr ids.ShortID) (string, error) {
	chainIDAlias, err := vm.Ctx.BCLookup.PrimaryAlias(chainID)
	if err != nil {
		return "", err
	}
	hrp := constants.GetHRP(vm.Ctx.NetworkID)
	return formatting.FormatAddress(chainIDAlias, hrp, addr.Bytes())
}

func (vm *VM) calculateUptime(db database.Database, nodeID ids.ShortID, startTime time.Time) (float64, error) {
	uptime, err := vm.uptime(db, nodeID)
	switch {
	case err == database.ErrNotFound:
		uptime = &validatorUptime{
			LastUpdated: uint64(startTime.Unix()),
		}
	case err != nil:
		return 0, err
	}

	upDuration := uptime.UpDuration
	currentLocalTime := vm.clock.Time()
	if timeConnected, isConnected := vm.connections[nodeID]; isConnected {
		if timeConnected.Before(vm.bootstrappedTime) {
			timeConnected = vm.bootstrappedTime
		}

		lastUpdated := time.Unix(int64(uptime.LastUpdated), 0)
		if timeConnected.Before(lastUpdated) {
			timeConnected = lastUpdated
		}

		durationConnected := currentLocalTime.Sub(timeConnected)
		if durationConnected > 0 {
			upDuration += uint64(durationConnected / time.Second)
		}
	}
	bestPossibleUpDuration := uint64(currentLocalTime.Sub(startTime) / time.Second)
	return float64(upDuration) / float64(bestPossibleUpDuration), nil
}

// Returns the current staker set of the Primary Network.
// Each element corresponds to a staking transaction.
// There may be multiple elements with the same node ID.
// TODO implement this more efficiently
func (vm *VM) getStakers() ([]validators.Validator, error) {
	stopPrefix := []byte(fmt.Sprintf("%s%s", constants.PrimaryNetworkID, stopDBPrefix))
	stopDB := prefixdb.NewNested(stopPrefix, vm.DB)
	defer stopDB.Close()
	stopIter := stopDB.NewIterator()
	defer stopIter.Release()

	stakers := []validators.Validator{}
	for stopIter.Next() { // Iterates in order of increasing stop time
		txBytes := stopIter.Value()
		tx := rewardTx{}
		if _, err := vm.codec.Unmarshal(txBytes, &tx); err != nil {
			return nil, fmt.Errorf("couldn't unmarshal validator tx: %w", err)
		} else if err := tx.Tx.Sign(vm.codec, nil); err != nil {
			return nil, err
		}

		switch staker := tx.Tx.UnsignedTx.(type) {
		case *UnsignedAddDelegatorTx:
			stakers = append(stakers, &staker.Validator)
		case *UnsignedAddValidatorTx:
			stakers = append(stakers, &staker.Validator)
		}
	}

	errs := wrappers.Errs{}
	errs.Add(
		stopIter.Error(),
		stopDB.Close(),
	)
	return stakers, errs.Err
}

// Returns the percentage of the total stake on the Primary Network
// of nodes connected to this node.
func (vm *VM) getPercentConnected() (float64, error) {
	stakers, err := vm.getStakers()
	if err != nil {
		return 0, fmt.Errorf("couldn't get stakers: %w", err)
	}

	connectedStake := uint64(0)
	totalStake := uint64(0)
	for _, staker := range stakers {
		totalStake, err = safemath.Add64(totalStake, staker.Weight())
		if err != nil {
			return 0, err
		}
		if _, connected := vm.connections[staker.ID()]; !connected {
			continue // not connected to use --> don't include
		}
		connectedStake, err = safemath.Add64(connectedStake, staker.Weight())
		if err != nil {
			return 0, err
		}
	}

	return float64(connectedStake) / float64(totalStake), nil
}

func (vm *VM) maxStakeAmount(db database.Database, subnetID ids.ID, nodeID ids.ShortID, startTime time.Time, endTime time.Time) (uint64, error) {
	currentTime, err := vm.getTimestamp(db)
	if err != nil {
		return 0, err
	}
	if currentTime.After(startTime) {
		return 0, errStartTimeTooEarly
	}
	if startTime.After(endTime) {
		return 0, errStartAfterEndTime
	}

	stopPrefix := []byte(fmt.Sprintf("%s%s", subnetID, stopDBPrefix))
	stopDB := prefixdb.NewNested(stopPrefix, db)
	defer stopDB.Close()

	stopIter := stopDB.NewIterator()
	defer stopIter.Release()

	currentWeight := uint64(0)
	toRemoveHeap := validatorHeap{}
	for stopIter.Next() { // Iterates in order of increasing stop time
		txBytes := stopIter.Value()

		tx := rewardTx{}
		if _, err := vm.codec.Unmarshal(txBytes, &tx); err != nil {
			return 0, fmt.Errorf("couldn't unmarshal validator tx: %w", err)
		}

		validator := (*Validator)(nil)
		switch staker := tx.Tx.UnsignedTx.(type) {
		case *UnsignedAddDelegatorTx:
			validator = &staker.Validator
		case *UnsignedAddValidatorTx:
			validator = &staker.Validator
		case *UnsignedAddSubnetValidatorTx:
			validator = &staker.Validator.Validator
		default:
			return 0, fmt.Errorf("expected validator but got %T", tx.Tx.UnsignedTx)
		}

		if validator.NodeID != nodeID {
			continue
		}

		if err := tx.Tx.Sign(vm.codec, nil); err != nil {
			return 0, err
		}

		newWeight, err := safemath.Add64(currentWeight, validator.Wght)
		if err != nil {
			return 0, err
		}
		currentWeight = newWeight
		toRemoveHeap.Add(validator)
	}

	startPrefix := []byte(fmt.Sprintf("%s%s", subnetID, startDBPrefix))
	startDB := prefixdb.NewNested(startPrefix, db)
	defer startDB.Close()

	startIter := startDB.NewIterator()
	defer startIter.Release()

	maxWeight := uint64(0)
	for startIter.Next() { // Iterates in order of increasing start time
		txBytes := startIter.Value()

		tx := Tx{}
		if _, err := vm.codec.Unmarshal(txBytes, &tx); err != nil {
			return 0, fmt.Errorf("couldn't unmarshal validator tx: %w", err)
		}

		validator := (*Validator)(nil)
		switch staker := tx.UnsignedTx.(type) {
		case *UnsignedAddDelegatorTx:
			validator = &staker.Validator
		case *UnsignedAddValidatorTx:
			validator = &staker.Validator
		case *UnsignedAddSubnetValidatorTx:
			validator = &staker.Validator.Validator
		default:
			return 0, fmt.Errorf("expected validator but got %T", tx.UnsignedTx)
		}

		if validator.StartTime().After(endTime) {
			break
		}

		if validator.NodeID != nodeID {
			continue
		}

		if err := tx.Sign(vm.codec, nil); err != nil {
			return 0, err
		}

		for len(toRemoveHeap) > 0 && !toRemoveHeap[0].EndTime().After(validator.StartTime()) {
			toRemove := toRemoveHeap[0]
			toRemoveHeap = toRemoveHeap[1:]

			if currentWeight > maxWeight && !startTime.After(toRemove.EndTime()) {
				maxWeight = currentWeight
			}

			newWeight, err := safemath.Sub64(currentWeight, toRemove.Wght)
			if err != nil {
				return 0, err
			}
			currentWeight = newWeight
		}

		newWeight, err := safemath.Add64(currentWeight, validator.Wght)
		if err != nil {
			return 0, err
		}
		currentWeight = newWeight
		if currentWeight > maxWeight && !startTime.After(validator.StartTime()) {
			maxWeight = currentWeight
		}

		toRemoveHeap.Add(validator)
	}

	for len(toRemoveHeap) > 0 && toRemoveHeap[0].EndTime().Before(startTime) {
		toRemove := toRemoveHeap[0]
		toRemoveHeap = toRemoveHeap[1:]

		newWeight, err := safemath.Sub64(currentWeight, toRemove.Wght)
		if err != nil {
			return 0, err
		}
		currentWeight = newWeight
	}

	if currentWeight > maxWeight {
		maxWeight = currentWeight
	}

	errs := wrappers.Errs{}
	errs.Add(
		stopIter.Error(),
		stopDB.Close(),
		startIter.Error(),
		startDB.Close(),
	)
	return maxWeight, errs.Err
}

type validatorHeap []*Validator

func (h *validatorHeap) Len() int                 { return len(*h) }
func (h *validatorHeap) Less(i, j int) bool       { return (*h)[i].EndTime().Before((*h)[j].EndTime()) }
func (h *validatorHeap) Swap(i, j int)            { (*h)[i], (*h)[j] = (*h)[j], (*h)[i] }
func (h *validatorHeap) Add(validator *Validator) { heap.Push(h, validator) }
func (h *validatorHeap) Peek() *Validator         { return (*h)[0] }
func (h *validatorHeap) Remove() *Validator       { return heap.Pop(h).(*Validator) }
func (h *validatorHeap) Push(x interface{})       { *h = append(*h, x.(*Validator)) }
func (h *validatorHeap) Pop() interface{} {
	newLen := len(*h) - 1
	val := (*h)[newLen]
	*h = (*h)[:newLen]
	return val
}
