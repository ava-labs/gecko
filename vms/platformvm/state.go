// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/state"

	safemath "github.com/ava-labs/avalanchego/utils/math"
)

// This file contains methods of VM that deal with getting/putting values from database

// TODO: Cache prefixed IDs or use different way of keying into database
const (
	startDBPrefix  = "start"
	stopDBPrefix   = "stop"
	uptimeDBPrefix = "uptime"
)

var (
	errNoValidators = errors.New("there are no validators")
)

// persist a tx
func (vm *VM) putTx(db database.Database, id ids.ID, tx []byte) error {
	return vm.State.Put(db, txTypeID, id, tx)
}

// retrieve a tx
func (vm *VM) getTx(db database.Database, txID ids.ID) ([]byte, error) {
	txIntf, err := vm.State.Get(db, txTypeID, txID)
	if err != nil {
		return nil, err
	}
	if tx, ok := txIntf.([]byte); ok {
		return tx, nil
	}
	return nil, fmt.Errorf("expected tx to be []byte but is type %T", txIntf)
}

// Persist a status
func (vm *VM) putStatus(db database.Database, id ids.ID, status Status) error {
	return vm.State.Put(db, statusTypeID, id, status)
}

// Retrieve a status
func (vm *VM) getStatus(db database.Database, id ids.ID) (Status, error) {
	statusIntf, err := vm.State.Get(db, statusTypeID, id)
	if err != nil {
		return Unknown, err
	}
	if status, ok := statusIntf.(Status); ok {
		return status, nil
	}
	return Unknown, fmt.Errorf("expected status to be type Status but is type %T", statusIntf)
}

// Add a staker to subnet [subnetID]'s pending validator queue. A staker may be
// a validator or a delegator
func (vm *VM) enqueueStaker(db database.Database, subnetID ids.ID, stakerTx *Tx) error {
	var (
		staker   TimedTx
		priority byte
	)
	switch unsignedTx := stakerTx.UnsignedTx.(type) {
	case *UnsignedAddDelegatorTx:
		staker = unsignedTx
		priority = 1
	case *UnsignedAddSubnetValidatorTx:
		staker = unsignedTx
		priority = 0
	case *UnsignedAddValidatorTx:
		staker = unsignedTx
		priority = 2
	default:
		return fmt.Errorf("staker is unexpected type %T", stakerTx)
	}
	stakerID := staker.ID() // Tx ID of this tx
	txBytes := stakerTx.Bytes()

	// Sorted by subnet ID then start time then tx ID
	prefixStart := []byte(fmt.Sprintf("%s%s", subnetID, startDBPrefix))
	prefixStartDB := prefixdb.NewNested(prefixStart, db)

	p := wrappers.Packer{MaxSize: wrappers.LongLen + wrappers.ByteLen + hashing.HashLen}
	p.PackLong(uint64(staker.StartTime().Unix()))
	p.PackByte(priority)
	p.PackFixedBytes(stakerID[:])
	if p.Err != nil {
		// Close the DB, but ignore the error, as the parent error needs to be
		// returned.
		_ = prefixStartDB.Close()
		return fmt.Errorf("couldn't serialize validator key: %w", p.Err)
	}
	startKey := p.Bytes

	errs := wrappers.Errs{}
	errs.Add(
		prefixStartDB.Put(startKey, txBytes),
		prefixStartDB.Close(),
	)
	return errs.Err
}

// Remove a staker from subnet [subnetID]'s pending validator queue. A staker
// may be a validator or a delegator
func (vm *VM) dequeueStaker(db database.Database, subnetID ids.ID, stakerTx *Tx) error {
	var (
		staker   TimedTx
		priority byte
	)
	switch unsignedTx := stakerTx.UnsignedTx.(type) {
	case *UnsignedAddDelegatorTx:
		staker = unsignedTx
		priority = 1
	case *UnsignedAddSubnetValidatorTx:
		staker = unsignedTx
		priority = 0
	case *UnsignedAddValidatorTx:
		staker = unsignedTx
		priority = 2
	default:
		return fmt.Errorf("staker is unexpected type %T", stakerTx)
	}
	stakerID := staker.ID() // Tx ID of this tx

	// Sorted by subnet ID then start time then ID
	prefixStart := []byte(fmt.Sprintf("%s%s", subnetID, startDBPrefix))
	prefixStartDB := prefixdb.NewNested(prefixStart, db)

	p := wrappers.Packer{MaxSize: wrappers.LongLen + wrappers.ByteLen + hashing.HashLen}
	p.PackLong(uint64(staker.StartTime().Unix()))
	p.PackByte(priority)
	p.PackFixedBytes(stakerID[:])
	if p.Err != nil {
		// Close the DB, but ignore the error, as the parent error needs to be
		// returned.
		_ = prefixStartDB.Close()
		return fmt.Errorf("couldn't serialize validator key: %w", p.Err)
	}
	startKey := p.Bytes

	errs := wrappers.Errs{}
	errs.Add(
		prefixStartDB.Delete(startKey),
		prefixStartDB.Close(),
	)
	return errs.Err
}

// Add a staker to subnet [subnetID]
// A staker may be a validator or a delegator
func (vm *VM) addStaker(db database.Database, subnetID ids.ID, tx *rewardTx) error {
	var (
		staker   TimedTx
		priority byte
	)
	switch unsignedTx := tx.Tx.UnsignedTx.(type) {
	case *UnsignedAddDelegatorTx:
		staker = unsignedTx
		priority = 0
	case *UnsignedAddSubnetValidatorTx:
		staker = unsignedTx
		priority = 1
	case *UnsignedAddValidatorTx:
		staker = unsignedTx
		priority = 2
	default:
		return fmt.Errorf("staker is unexpected type %T", tx.Tx.UnsignedTx)
	}

	txBytes, err := vm.codec.Marshal(codecVersion, tx)
	if err != nil {
		return err
	}

	txID := tx.Tx.ID() // Tx ID of this tx

	// Sorted by subnet ID then stop time then tx ID
	prefixStop := []byte(fmt.Sprintf("%s%s", subnetID, stopDBPrefix))
	prefixStopDB := prefixdb.NewNested(prefixStop, db)

	p := wrappers.Packer{MaxSize: wrappers.LongLen + wrappers.ByteLen + hashing.HashLen}
	p.PackLong(uint64(staker.EndTime().Unix()))
	p.PackByte(priority)
	p.PackFixedBytes(txID[:])
	if p.Err != nil {
		// Close the DB, but ignore the error, as the parent error needs to be
		// returned.
		_ = prefixStopDB.Close()
		return fmt.Errorf("couldn't serialize validator key: %w", p.Err)
	}
	stopKey := p.Bytes

	errs := wrappers.Errs{}
	errs.Add(
		prefixStopDB.Put(stopKey, txBytes),
		prefixStopDB.Close(),
	)
	return errs.Err
}

// Remove a staker from subnet [subnetID]
// A staker may be a validator or a delegator
func (vm *VM) removeStaker(db database.Database, subnetID ids.ID, tx *rewardTx) error {
	var (
		staker   TimedTx
		priority byte
	)
	switch unsignedTx := tx.Tx.UnsignedTx.(type) {
	case *UnsignedAddDelegatorTx:
		staker = unsignedTx
		priority = 0
	case *UnsignedAddSubnetValidatorTx:
		staker = unsignedTx
		priority = 1
	case *UnsignedAddValidatorTx:
		staker = unsignedTx
		priority = 2
	default:
		return fmt.Errorf("staker is unexpected type %T", tx.Tx.UnsignedTx)
	}

	txID := tx.Tx.ID() // Tx ID of this tx

	// Sorted by subnet ID then stop time
	prefixStop := []byte(fmt.Sprintf("%s%s", subnetID, stopDBPrefix))
	prefixStopDB := prefixdb.NewNested(prefixStop, db)

	p := wrappers.Packer{MaxSize: wrappers.LongLen + wrappers.ByteLen + hashing.HashLen}
	p.PackLong(uint64(staker.EndTime().Unix()))
	p.PackByte(priority)
	p.PackFixedBytes(txID[:])
	if p.Err != nil {
		// Close the DB, but ignore the error, as the parent error needs to be
		// returned.
		_ = prefixStopDB.Close()
		return fmt.Errorf("couldn't serialize validator key: %w", p.Err)
	}
	stopKey := p.Bytes

	errs := wrappers.Errs{}
	errs.Add(
		prefixStopDB.Delete(stopKey),
		prefixStopDB.Close(),
	)
	return errs.Err
}

// Returns the pending staker that will start staking next
func (vm *VM) nextStakerStart(db database.Database, subnetID ids.ID) (*Tx, error) {
	startIter := prefixdb.NewNested([]byte(fmt.Sprintf("%s%s", subnetID, startDBPrefix)), db).NewIterator()
	defer startIter.Release()

	if !startIter.Next() {
		return nil, errNoValidators
	}
	// Key: [Staker start time] | [Tx ID]
	// Value: Byte repr. of tx that added this validator

	tx := Tx{}
	if _, err := Codec.Unmarshal(startIter.Value(), &tx); err != nil {
		return nil, err
	}
	return &tx, tx.Sign(vm.codec, nil)
}

// Returns the current staker that will stop staking next
func (vm *VM) nextStakerStop(db database.Database, subnetID ids.ID) (*rewardTx, error) {
	stopIter := prefixdb.NewNested([]byte(fmt.Sprintf("%s%s", subnetID, stopDBPrefix)), db).NewIterator()
	defer stopIter.Release()

	if !stopIter.Next() {
		return nil, errNoValidators
	}
	// Key: [Staker stop time] | [Tx ID]
	// Value: Byte repr. of tx that added this validator

	tx := rewardTx{}
	if _, err := Codec.Unmarshal(stopIter.Value(), &tx); err != nil {
		return nil, err
	}
	return &tx, tx.Tx.Sign(vm.codec, nil)
}

// Returns true if [nodeID] is a validator (not a delegator) of subnet [subnetID]
func (vm *VM) isValidator(db database.Database, subnetID ids.ID, nodeID ids.ShortID) (TimedTx, bool, error) {
	stopIter := prefixdb.NewNested([]byte(fmt.Sprintf("%s%s", subnetID, stopDBPrefix)), db).NewIterator()
	defer stopIter.Release()

	for stopIter.Next() {
		txBytes := stopIter.Value()
		tx := rewardTx{}
		if _, err := Codec.Unmarshal(txBytes, &tx); err != nil {
			return nil, false, err
		}

		switch vdr := tx.Tx.UnsignedTx.(type) {
		case *UnsignedAddValidatorTx:
			if subnetID == constants.PrimaryNetworkID && vdr.Validator.NodeID == nodeID {
				if err := tx.Tx.Sign(vm.codec, nil); err != nil {
					return nil, false, err
				}
				return vdr, true, nil
			}
		case *UnsignedAddSubnetValidatorTx:
			if subnetID == vdr.Validator.SubnetID() && vdr.Validator.NodeID == nodeID {
				if err := tx.Tx.Sign(vm.codec, nil); err != nil {
					return nil, false, err
				}
				return vdr, true, nil
			}
		}
	}
	return nil, false, nil
}

// Returns true if [nodeID] will be a validator (not a delegator) of subnet
// [subnetID]
func (vm *VM) willBeValidator(db database.Database, subnetID ids.ID, nodeID ids.ShortID) (TimedTx, bool, error) {
	startIter := prefixdb.NewNested([]byte(fmt.Sprintf("%s%s", subnetID, startDBPrefix)), db).NewIterator()
	defer startIter.Release()

	for startIter.Next() {
		txBytes := startIter.Value()
		tx := Tx{}
		if _, err := Codec.Unmarshal(txBytes, &tx); err != nil {
			return nil, false, err
		}

		switch vdr := tx.UnsignedTx.(type) {
		case *UnsignedAddValidatorTx:
			if subnetID == constants.PrimaryNetworkID && vdr.Validator.NodeID == nodeID {
				if err := tx.Sign(vm.codec, nil); err != nil {
					return nil, false, err
				}
				return vdr, true, nil
			}
		case *UnsignedAddSubnetValidatorTx:
			if subnetID == vdr.Validator.SubnetID() && vdr.Validator.NodeID == nodeID {
				if err := tx.Sign(vm.codec, nil); err != nil {
					return nil, false, err
				}
				return vdr, true, nil
			}
		}
	}
	return nil, false, nil
}

// getUTXO returns the UTXO with the specified ID
func (vm *VM) getUTXO(db database.Database, id ids.ID) (*avax.UTXO, error) {
	utxoIntf, err := vm.State.Get(db, utxoTypeID, id)
	if err != nil {
		return nil, err
	}
	utxo, ok := utxoIntf.(*avax.UTXO)
	if !ok {
		err := fmt.Errorf("expected UTXO from database but got %T", utxoIntf)
		vm.Ctx.Log.Error(err.Error())
		return nil, err
	}
	return utxo, nil
}

// putUTXO persists the given UTXO
func (vm *VM) putUTXO(db database.Database, utxo *avax.UTXO) error {
	utxoID := utxo.InputID()
	if err := vm.State.Put(db, utxoTypeID, utxoID, utxo); err != nil {
		return err
	}

	// If this output lists addresses that it references index it
	if addressable, ok := utxo.Out.(avax.Addressable); ok {
		// For each owner of this UTXO, add to list of UTXOs owned by that addr
		for _, addrBytes := range addressable.Addresses() {
			if err := vm.putReferencingUTXO(db, addrBytes, utxoID); err != nil {
				// We assume that the maximum size of a byte slice that
				// can be stringified is at least the length of an address.
				// If conversion of address to string fails, ignore the error
				addrStr, _ := formatting.Encode(formatting.CB58, addrBytes)
				return fmt.Errorf("couldn't update UTXO set of address %s", addrStr)
			}
		}
	}
	return nil
}

// removeUTXO removes the UTXO with the given ID
// If the utxo doesn't exist, returns nil
func (vm *VM) removeUTXO(db database.Database, utxoID ids.ID) error {
	utxo, err := vm.getUTXO(db, utxoID) // Get the UTXO
	if err != nil {
		return nil
	}
	if err := vm.State.Put(db, utxoTypeID, utxoID, nil); err != nil { // remove the UTXO
		return err
	}
	// If this output lists addresses that it references remove the indices
	if addressable, ok := utxo.Out.(avax.Addressable); ok {
		// For each owner of this UTXO, remove from their list of UTXOs
		for _, addrBytes := range addressable.Addresses() {
			if err := vm.removeReferencingUTXO(db, addrBytes, utxoID); err != nil {
				// We assume that the maximum size of a byte slice that
				// can be stringified is at least the length of an address.
				// If conversion of address to string fails, ignore the error
				addrStr, _ := formatting.Encode(formatting.CB58, addrBytes)
				return fmt.Errorf("couldn't update UTXO set of address %s", addrStr)
			}
		}
	}
	return nil
}

// Return the IDs of UTXOs that reference [addr].
// Only returns UTXOs after [start].
// Returns at most [limit] UTXO IDs.
// Returns nil if no UTXOs reference [addr].

func (vm *VM) getReferencingUTXOs(db database.Database, addr []byte, start ids.ID, limit int) ([]ids.ID, error) {
	idSlice := []ids.ID(nil)

	iter := prefixdb.NewNested(addr, db).NewIteratorWithStart(start[:])
	defer iter.Release()
	numFetched := 0
	for numFetched < limit && iter.Next() {
		if keyID, err := ids.ToID(iter.Key()); err != nil {
			return nil, err
		} else if keyID != start {
			idSlice = append(idSlice, keyID)
			numFetched++

		}
	}
	return idSlice, nil
}

// Persist that the UTXO with ID [utxoID] references [addr]
func (vm *VM) putReferencingUTXO(db database.Database, addrBytes []byte, utxoID ids.ID) error {
	prefixedDB := prefixdb.NewNested(addrBytes, db)
	errs := wrappers.Errs{}
	errs.Add(
		prefixedDB.Put(utxoID[:], nil),
		prefixedDB.Close(),
	)
	return errs.Err
}

// Remove the UTXO with ID [utxoID] from the set of UTXOs that reference [addr]
func (vm *VM) removeReferencingUTXO(db database.Database, addrBytes []byte, utxoID ids.ID) error {
	prefixedDB := prefixdb.NewNested(addrBytes, db)
	return prefixedDB.Delete(utxoID[:])
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
	db database.Database,
	addrs ids.ShortSet,
	startAddr state.Marshaller,
	startUTXOID ids.ID,
	limit int,
	paginate bool,
) ([]*avax.UTXO, ids.ShortID, ids.ID, error) {
	if limit <= 0 || limit > maxUTXOsToFetch {
		limit = maxUTXOsToFetch
	}

	if paginate {
		return vm.getPaginatedUTXOs(db, addrs, startAddr, startUTXOID, limit)
	}
	return vm.getAllUTXOs(db, addrs)
}

func (vm *VM) getPaginatedUTXOs(
	db database.Database,
	addrs ids.ShortSet,
	startAddr state.Marshaller,
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

		utxoIDs, err := vm.getReferencingUTXOs(db, addr.Bytes(), start, searchSize) // Get UTXOs associated with [addr]
		if err != nil {
			return nil, ids.ShortID{}, ids.ID{}, fmt.Errorf("couldn't get UTXOs for address %s: %w", addr, err)
		}
		for _, utxoID := range utxoIDs {
			lastIndex = utxoID // The last searched UTXO - not the last found
			lastAddr = addr    // The last address searched that has UTXOs (even duplicated) - not the last found

			if seen.Contains(utxoID) { // Already have this UTXO in the list
				continue
			}

			utxo, err := vm.getUTXO(db, utxoID)
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

func (vm *VM) getAllUTXOs(
	db database.Database,
	addrs ids.ShortSet,
) ([]*avax.UTXO, ids.ShortID, ids.ID, error) {
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
		lastIndex, err = vm.getAllUniqueAddressUTXOs(db, addr, &seen, &utxos)
		if err != nil {
			return nil, ids.ShortID{}, ids.ID{}, fmt.Errorf("couldn't get UTXOs for address %s: %w", addr, err)
		}

		if lastIndex != ids.Empty {
			lastAddr = addr // The last address searched that has UTXOs (even duplicated) - not the last found
		}
	}
	return utxos, lastAddr, lastIndex, nil
}

func (vm *VM) getAllUniqueAddressUTXOs(db database.Database, addr ids.ShortID, seen *ids.Set, utxos *[]*avax.UTXO) (ids.ID, error) {
	lastIndex := ids.Empty

	for {
		utxoIDs, err := vm.getReferencingUTXOs(db, addr.Bytes(), lastIndex, maxUTXOsToFetch) // Get UTXOs associated with [addr]
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

			utxo, err := vm.getUTXO(db, utxoID)
			if err != nil {
				return ids.ID{}, err
			}
			*utxos = append(*utxos, utxo)
			seen.Add(utxoID)
		}
	}
}

// getBalance returns the balance of [addrs]
func (vm *VM) getBalance(db database.Database, addrs ids.ShortSet) (uint64, error) {
	utxos, _, _, err := vm.GetUTXOs(db, addrs, ids.ShortEmpty, ids.Empty, -1, false)
	if err != nil {
		return 0, fmt.Errorf("couldn't get UTXOs: %w", err)
	}
	balance := uint64(0)
	for _, utxo := range utxos {
		if out, ok := utxo.Out.(avax.Amounter); ok {
			if balance, err = safemath.Add64(out.Amount(), balance); err != nil {
				return 0, err
			}
		}
	}
	return balance, nil
}

// get all the blockchains that exist
func (vm *VM) getChains(db database.Database) ([]*Tx, error) {
	chainsInterface, err := vm.State.Get(db, chainsTypeID, chainsKey)
	if err != nil {
		return nil, err
	}
	chains, ok := chainsInterface.([]*Tx)
	if !ok {
		err := fmt.Errorf("expected to retrieve []*CreateChainTx from database but got type %T", chainsInterface)
		vm.Ctx.Log.Error(err.Error())
		return nil, err
	}
	return chains, nil
}

// get a blockchain by its ID
func (vm *VM) getChain(db database.Database, id ids.ID) (*Tx, error) {
	chains, err := vm.getChains(db)
	if err != nil {
		return nil, err
	}
	for _, chain := range chains {
		if chain.ID() == id {
			return chain, nil
		}
	}
	return nil, fmt.Errorf("blockchain %s doesn't exist", id)
}

// put the list of blockchains that exist to database
func (vm *VM) putChains(db database.Database, chains []*Tx) error {
	return vm.State.Put(db, chainsTypeID, chainsKey, chains)
}

// get the platform chain's timestamp from [db]
func (vm *VM) getTimestamp(db database.Database) (time.Time, error) {
	return vm.State.GetTime(db, timestampKey)
}

// put the platform chain's timestamp in [db]
func (vm *VM) putTimestamp(db database.Database, timestamp time.Time) error {
	return vm.State.PutTime(db, timestampKey, timestamp)
}

// put the subnets that exist to [db]
func (vm *VM) putSubnets(db database.Database, subnets []*Tx) error {
	return vm.State.Put(db, subnetsTypeID, subnetsKey, subnets)
}

// get the subnets that exist in [db]
func (vm *VM) getSubnets(db database.Database) ([]*Tx, error) {
	subnetsIntf, err := vm.State.Get(db, subnetsTypeID, subnetsKey)
	if err != nil {
		return nil, err
	}
	subnets, ok := subnetsIntf.([]*Tx)
	if !ok {
		err := fmt.Errorf("expected to retrieve []*CreateSubnetTx from database but got type %T", subnetsIntf)
		vm.Ctx.Log.Error(err.Error())
		return nil, err
	}
	for _, tx := range subnets {
		if err := tx.Sign(vm.codec, nil); err != nil {
			return nil, err
		}
	}
	return subnets, nil
}

// get the subnet with the specified ID
func (vm *VM) getSubnet(db database.Database, id ids.ID) (*Tx, TxError) {
	subnets, err := vm.getSubnets(db)
	if err != nil {
		return nil, tempError{err}
	}

	for _, subnet := range subnets {
		if subnet.ID() == id {
			return subnet, nil
		}
	}
	return nil, permError{fmt.Errorf("couldn't find subnet with ID %s", id)}
}

// Returns the height of the preferred block
func (vm *VM) preferredHeight() (uint64, error) {
	preferred, err := vm.getBlock(vm.Preferred())
	if err != nil {
		return 0, err
	}
	return preferred.Height(), nil
}

// register each type that we'll be storing in the database
// so that [vm.State] knows how to unmarshal these types from bytes
func (vm *VM) registerDBTypes() {
	marshalValidatorsFunc := func(vdrsIntf interface{}) ([]byte, error) {
		if vdrs, ok := vdrsIntf.(*EventHeap); ok {
			return vdrs.Bytes()
		}
		return nil, fmt.Errorf("expected *EventHeap but got type %T", vdrsIntf)
	}
	unmarshalValidatorsFunc := func(bytes []byte) (interface{}, error) {
		stakers := EventHeap{}
		if _, err := Codec.Unmarshal(bytes, &stakers); err != nil {
			return nil, err
		}
		for _, tx := range stakers.Txs {
			if err := tx.Sign(vm.codec, nil); err != nil {
				return nil, err
			}
		}
		return &stakers, nil
	}
	if err := vm.State.RegisterType(validatorsTypeID, marshalValidatorsFunc, unmarshalValidatorsFunc); err != nil {
		vm.Ctx.Log.Warn(errRegisteringType.Error())
	}

	marshalChainsFunc := func(chainsIntf interface{}) ([]byte, error) {
		if chains, ok := chainsIntf.([]*Tx); ok {
			return GenesisCodec.Marshal(codecVersion, chains)
		}
		return nil, fmt.Errorf("expected []*CreateChainTx but got type %T", chainsIntf)
	}
	unmarshalChainsFunc := func(bytes []byte) (interface{}, error) {
		var chains []*Tx
		if _, err := GenesisCodec.Unmarshal(bytes, &chains); err != nil {
			return nil, err
		}
		for _, tx := range chains {
			if err := tx.Sign(GenesisCodec, nil); err != nil {
				return nil, err
			}
		}
		return chains, nil
	}
	if err := vm.State.RegisterType(chainsTypeID, marshalChainsFunc, unmarshalChainsFunc); err != nil {
		vm.Ctx.Log.Warn(errRegisteringType.Error())
	}

	marshalSubnetsFunc := func(subnetsIntf interface{}) ([]byte, error) {
		if subnets, ok := subnetsIntf.([]*Tx); ok {
			return Codec.Marshal(codecVersion, subnets)
		}
		return nil, fmt.Errorf("expected []*Tx but got type %T", subnetsIntf)
	}
	unmarshalSubnetsFunc := func(bytes []byte) (interface{}, error) {
		var subnets []*Tx
		if _, err := Codec.Unmarshal(bytes, &subnets); err != nil {
			return nil, err
		}
		for _, tx := range subnets {
			if err := tx.Sign(vm.codec, nil); err != nil {
				return nil, err
			}
		}
		return subnets, nil
	}
	if err := vm.State.RegisterType(subnetsTypeID, marshalSubnetsFunc, unmarshalSubnetsFunc); err != nil {
		vm.Ctx.Log.Warn(errRegisteringType.Error())
	}

	marshalUTXOFunc := func(utxoIntf interface{}) ([]byte, error) {
		if utxo, ok := utxoIntf.(*avax.UTXO); ok {
			return Codec.Marshal(codecVersion, utxo)
		} else if utxo, ok := utxoIntf.(avax.UTXO); ok {
			return Codec.Marshal(codecVersion, utxo)
		}
		return nil, fmt.Errorf("expected *avax.UTXO but got type %T", utxoIntf)
	}
	unmarshalUTXOFunc := func(bytes []byte) (interface{}, error) {
		var utxo avax.UTXO
		if _, err := Codec.Unmarshal(bytes, &utxo); err != nil {
			return nil, err
		}
		return &utxo, nil
	}
	if err := vm.State.RegisterType(utxoTypeID, marshalUTXOFunc, unmarshalUTXOFunc); err != nil {
		vm.Ctx.Log.Warn(errRegisteringType.Error())
	}

	marshalTxFunc := func(txIntf interface{}) ([]byte, error) {
		if tx, ok := txIntf.([]byte); ok {
			return tx, nil
		}
		return nil, fmt.Errorf("expected []byte but got type %T", txIntf)
	}
	unmarshalTxFunc := func(bytes []byte) (interface{}, error) {
		return bytes, nil
	}
	if err := vm.State.RegisterType(txTypeID, marshalTxFunc, unmarshalTxFunc); err != nil {
		vm.Ctx.Log.Warn(errRegisteringType.Error())
	}

	marshalStatusFunc := func(statusIntf interface{}) ([]byte, error) {
		if status, ok := statusIntf.(Status); ok {
			return vm.codec.Marshal(codecVersion, status)
		}
		return nil, fmt.Errorf("expected Status but got type %T", statusIntf)
	}
	unmarshalStatusFunc := func(bytes []byte) (interface{}, error) {
		var status Status
		if _, err := Codec.Unmarshal(bytes, &status); err != nil {
			return nil, err
		}
		return status, nil
	}
	if err := vm.State.RegisterType(statusTypeID, marshalStatusFunc, unmarshalStatusFunc); err != nil {
		vm.Ctx.Log.Warn(errRegisteringType.Error())
	}

	marshalCurrentSupplyFunc := func(currentSupplyIntf interface{}) ([]byte, error) {
		if currentSupply, ok := currentSupplyIntf.(uint64); ok {
			return vm.codec.Marshal(codecVersion, currentSupply)
		}
		return nil, fmt.Errorf("expected uint64 but got type %T", currentSupplyIntf)
	}
	unmarshalCurrentSupplyFunc := func(bytes []byte) (interface{}, error) {
		var currentSupply uint64
		if _, err := Codec.Unmarshal(bytes, &currentSupply); err != nil {
			return nil, err
		}
		return currentSupply, nil
	}
	if err := vm.State.RegisterType(currentSupplyTypeID, marshalCurrentSupplyFunc, unmarshalCurrentSupplyFunc); err != nil {
		vm.Ctx.Log.Warn(errRegisteringType.Error())
	}
}

func (vm *VM) getCurrentSupply(db database.Database) (uint64, error) {
	currentSupplyIntf, err := vm.State.Get(db, currentSupplyTypeID, currentSupplyKey)
	if err != nil {
		return 0, err
	}
	if currentSupply, ok := currentSupplyIntf.(uint64); ok {
		return currentSupply, nil
	}
	return 0, fmt.Errorf("expected current supply to be uint64 but is type %T", currentSupplyIntf)
}

func (vm *VM) putCurrentSupply(db database.Database, currentSupply uint64) error {
	return vm.State.Put(db, currentSupplyTypeID, currentSupplyKey, currentSupply)
}

type validatorUptime struct {
	UpDuration  uint64 `serialize:"true"` // In seconds
	LastUpdated uint64 `serialize:"true"` // Unix time in seconds
}

func (vm *VM) uptime(db database.Database, nodeID ids.ShortID) (*validatorUptime, error) {
	uptimeDB := prefixdb.NewNested([]byte(uptimeDBPrefix), db)
	defer uptimeDB.Close()

	uptimeBytes, err := uptimeDB.Get(nodeID.Bytes())
	if err != nil {
		return nil, err
	}

	uptime := validatorUptime{}
	if _, err := Codec.Unmarshal(uptimeBytes, &uptime); err != nil {
		return nil, err
	}
	return &uptime, uptimeDB.Close()
}

func (vm *VM) setUptime(db database.Database, nodeID ids.ShortID, uptime *validatorUptime) error {
	uptimeBytes, err := Codec.Marshal(codecVersion, uptime)
	if err != nil {
		return err
	}

	uptimeDB := prefixdb.NewNested([]byte(uptimeDBPrefix), db)
	errs := wrappers.Errs{}
	errs.Add(
		uptimeDB.Put(nodeID.Bytes(), uptimeBytes),
		uptimeDB.Close(),
	)
	return errs.Err
}

func (vm *VM) deleteUptime(db database.Database, nodeID ids.ShortID) error {
	uptimeDB := prefixdb.NewNested([]byte(uptimeDBPrefix), db)
	errs := wrappers.Errs{}
	errs.Add(
		uptimeDB.Delete(nodeID.Bytes()),
		uptimeDB.Close(),
	)
	return errs.Err
}

// Unmarshal a Block from bytes and initialize it
// The Block being unmarshaled must have had static type Block when it was marshaled
// i.e. don't do:
// block := &Abort{} (or some other type that implements block)
// bytes := codec.Marshal(block)
// instead do:
// var block Block = &Abort{} (or some other type that implements block)
// bytes := codec.Marshal(&block) (need to do &block, not block, because its an interface)
func (vm *VM) unmarshalBlockFunc(bytes []byte) (snowman.Block, error) {
	// Parse the serialized fields from bytes into a new block
	var block Block
	if _, err := Codec.Unmarshal(bytes, &block); err != nil {
		return nil, err
	}
	// Populate the un-serialized fields of the block
	return block, block.initialize(vm, bytes)
}
