// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/ava-labs/avalanchego/api"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/nftfx"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"

	safemath "github.com/ava-labs/avalanchego/utils/math"
)

const (
	// Max number of addresses that can be passed in as argument to GetUTXOs
	maxGetUTXOsAddrs = 1024

	// Max number of addresses allowed for a single keystore user
	maxKeystoreAddresses = 5000
)

var (
	errUnknownAssetID         = errors.New("unknown asset ID")
	errTxNotCreateAsset       = errors.New("transaction doesn't create an asset")
	errNoMinters              = errors.New("no minters provided")
	errNoHoldersMinters       = errors.New("no minters or initial holders provided")
	errZeroAmount             = errors.New("amount must be positive")
	errNoOutputs              = errors.New("no outputs to send")
	errSpendOverflow          = errors.New("spent amount overflows uint64")
	errInvalidMintAmount      = errors.New("amount minted must be positive")
	errAddressesCantMintAsset = errors.New("provided addresses don't have the authority to mint the provided asset")
	errNilTxID                = errors.New("nil transaction ID")
	errNoAddresses            = errors.New("no addresses provided")
	errNoKeys                 = errors.New("from addresses have no keys or funds")
	errNoFromAddrs            = errors.New("no from addresses given")
)

// Service defines the base service for the asset vm
type Service struct{ vm *VM }

// FormattedAssetID defines a JSON formatted struct containing an assetID as a string
type FormattedAssetID struct {
	AssetID ids.ID `json:"assetID"`
}

// IssueTx attempts to issue a transaction into consensus
func (service *Service) IssueTx(r *http.Request, args *api.FormattedTx, reply *api.JSONTxID) error {
	service.vm.ctx.Log.Info("AVM: IssueTx called with %s", args.Tx)

	txBytes, err := formatting.Decode(args.Encoding, args.Tx)
	if err != nil {
		return fmt.Errorf("problem decoding transaction: %w", err)
	}
	txID, err := service.vm.IssueTx(txBytes)
	if err != nil {
		return err
	}

	reply.TxID = txID
	return nil
}

// GetTxStatusReply defines the GetTxStatus replies returned from the API
type GetTxStatusReply struct {
	Status choices.Status `json:"status"`
	Epoch  *uint32        `json:"epoch,omitempty"`
}

// GetTxStatus returns the status of the specified transaction
func (service *Service) GetTxStatus(r *http.Request, args *api.JSONTxID, reply *GetTxStatusReply) error {
	service.vm.ctx.Log.Info("AVM: GetTxStatus called with %s", args.TxID)

	if args.TxID == ids.Empty {
		return errNilTxID
	}

	tx := newUniqueTx(service.vm, args.TxID, nil)

	reply.Status = tx.Status()
	if reply.Status == choices.Accepted {
		epoch := tx.Epoch()
		reply.Epoch = &epoch
	}
	return nil
}

// GetTx returns the specified transaction
func (service *Service) GetTx(r *http.Request, args *api.GetTxArgs, reply *api.FormattedTx) error {
	service.vm.ctx.Log.Info("AVM: GetTx called with %s", args.TxID)

	if args.TxID == ids.Empty {
		return errNilTxID
	}

	tx := newUniqueTx(service.vm, args.TxID, nil)
	if status := tx.Status(); !status.Fetched() {
		return errUnknownTx
	}

	var err error
	reply.Tx, err = formatting.Encode(args.Encoding, tx.Bytes())
	if err != nil {
		return fmt.Errorf("couldn't encode tx as string: %s", err)
	}
	reply.Encoding = args.Encoding
	return nil
}

// GetUTXOs gets all utxos for passed in addresses
func (service *Service) GetUTXOs(r *http.Request, args *api.GetUTXOsArgs, reply *api.GetUTXOsReply) error {
	service.vm.ctx.Log.Info("AVM: GetUTXOs called for with %s", args.Addresses)

	if len(args.Addresses) == 0 {
		return errNoAddresses
	}
	if len(args.Addresses) > maxGetUTXOsAddrs {
		return fmt.Errorf("number of addresses given, %d, exceeds maximum, %d", len(args.Addresses), maxGetUTXOsAddrs)
	}

	var sourceChain ids.ID
	if args.SourceChain == "" {
		sourceChain = service.vm.ctx.ChainID
	} else {
		chainID, err := service.vm.ctx.BCLookup.Lookup(args.SourceChain)
		if err != nil {
			return fmt.Errorf("problem parsing source chainID %q: %w", args.SourceChain, err)
		}
		sourceChain = chainID
	}

	addrSet := ids.ShortSet{}
	for _, addrStr := range args.Addresses {
		addr, err := service.vm.ParseLocalAddress(addrStr)
		if err != nil {
			return fmt.Errorf("couldn't parse address %q: %w", addrStr, err)
		}
		addrSet.Add(addr)
	}

	startAddr := ids.ShortEmpty
	startUTXO := ids.Empty
	if args.StartIndex.Address != "" || args.StartIndex.UTXO != "" {
		var err error
		startAddr, err = service.vm.ParseLocalAddress(args.StartIndex.Address)
		if err != nil {
			return fmt.Errorf("couldn't parse start index address %q: %w", args.StartIndex.Address, err)
		}
		startUTXO, err = ids.FromString(args.StartIndex.UTXO)
		if err != nil {
			return fmt.Errorf("couldn't parse start index utxo: %w", err)
		}
	}

	var (
		utxos     []*avax.UTXO
		endAddr   ids.ShortID
		endUTXOID ids.ID
		err       error
	)
	if sourceChain == service.vm.ctx.ChainID {
		utxos, endAddr, endUTXOID, err = service.vm.GetUTXOs(
			addrSet,
			startAddr,
			startUTXO,
			int(args.Limit),
			true,
		)
	} else {
		utxos, endAddr, endUTXOID, err = service.vm.GetAtomicUTXOs(
			sourceChain,
			addrSet,
			startAddr,
			startUTXO,
			int(args.Limit),
		)
	}
	if err != nil {
		return fmt.Errorf("problem retrieving UTXOs: %w", err)
	}

	reply.UTXOs = make([]string, len(utxos))
	for i, utxo := range utxos {
		b, err := service.vm.codec.Marshal(service.vm.currentCodecVersion, utxo)
		if err != nil {
			return fmt.Errorf("problem marshalling UTXO: %w", err)
		}
		reply.UTXOs[i], err = formatting.Encode(args.Encoding, b)
		if err != nil {
			return fmt.Errorf("couldn't encode UTXO %s as string: %s", utxo.InputID(), err)
		}
	}

	endAddress, err := service.vm.FormatLocalAddress(endAddr)
	if err != nil {
		return fmt.Errorf("problem formatting address: %w", err)
	}

	reply.EndIndex.Address = endAddress
	reply.EndIndex.UTXO = endUTXOID.String()
	reply.NumFetched = json.Uint64(len(utxos))
	reply.Encoding = args.Encoding
	return nil
}

// GetAssetDescriptionArgs are arguments for passing into GetAssetDescription requests
type GetAssetDescriptionArgs struct {
	AssetID string `json:"assetID"`
}

// GetAssetDescriptionReply defines the GetAssetDescription replies returned from the API
type GetAssetDescriptionReply struct {
	FormattedAssetID
	Name         string     `json:"name"`
	Symbol       string     `json:"symbol"`
	Denomination json.Uint8 `json:"denomination"`
}

// GetAssetDescription creates an empty account with the name passed in
func (service *Service) GetAssetDescription(_ *http.Request, args *GetAssetDescriptionArgs, reply *GetAssetDescriptionReply) error {
	service.vm.ctx.Log.Info("AVM: GetAssetDescription called with %s", args.AssetID)

	assetID, err := service.vm.lookupAssetID(args.AssetID)
	if err != nil {
		return err
	}

	tx := newUniqueTx(service.vm, assetID, nil)
	if status := tx.Status(); !status.Fetched() {
		return errUnknownAssetID
	}
	createAssetTx, ok := tx.UnsignedTx.(*CreateAssetTx)
	if !ok {
		return errTxNotCreateAsset
	}

	reply.AssetID = assetID
	reply.Name = createAssetTx.Name
	reply.Symbol = createAssetTx.Symbol
	reply.Denomination = json.Uint8(createAssetTx.Denomination)

	return nil
}

// GetBalanceArgs are arguments for passing into GetBalance requests
type GetBalanceArgs struct {
	Address string `json:"address"`
	AssetID string `json:"assetID"`
}

// GetBalanceReply defines the GetBalance replies returned from the API
type GetBalanceReply struct {
	Balance json.Uint64   `json:"balance"`
	UTXOIDs []avax.UTXOID `json:"utxoIDs"`
}

// GetBalance returns the amount of an asset that an address at least partially owns
func (service *Service) GetBalance(r *http.Request, args *GetBalanceArgs, reply *GetBalanceReply) error {
	service.vm.ctx.Log.Info("AVM: GetBalance called with address: %s assetID: %s", args.Address, args.AssetID)

	addr, err := service.vm.ParseLocalAddress(args.Address)
	if err != nil {
		return fmt.Errorf("problem parsing address '%s': %w", args.Address, err)
	}

	assetID, err := service.vm.lookupAssetID(args.AssetID)
	if err != nil {
		return err
	}

	addrSet := ids.ShortSet{}
	addrSet.Add(addr)

	utxos, _, _, err := service.vm.GetUTXOs(addrSet, ids.ShortEmpty, ids.Empty, -1, false)
	if err != nil {
		return fmt.Errorf("problem retrieving UTXOs: %w", err)
	}

	reply.UTXOIDs = make([]avax.UTXOID, 0, len(utxos))
	for _, utxo := range utxos {
		if utxo.AssetID() != assetID {
			continue
		}
		transferable, ok := utxo.Out.(avax.TransferableOut)
		if !ok {
			continue
		}
		amt, err := safemath.Add64(transferable.Amount(), uint64(reply.Balance))
		if err != nil {
			return err
		}
		reply.Balance = json.Uint64(amt)
		reply.UTXOIDs = append(reply.UTXOIDs, utxo.UTXOID)
	}

	return nil
}

// Balance ...
type Balance struct {
	AssetID string      `json:"asset"`
	Balance json.Uint64 `json:"balance"`
}

// GetAllBalancesReply is the response from a call to GetAllBalances
type GetAllBalancesReply struct {
	Balances []Balance `json:"balances"`
}

// GetAllBalances returns a map where:
//   Key: ID of an asset such that [args.Address] has a non-zero balance of the asset
//   Value: The balance of the asset held by the address
// Note that balances include assets that the address only _partially_ owns
// (ie is one of several addresses specified in a multi-sig)
func (service *Service) GetAllBalances(r *http.Request, args *api.JSONAddress, reply *GetAllBalancesReply) error {
	service.vm.ctx.Log.Info("AVM: GetAllBalances called with address: %s", args.Address)

	address, err := service.vm.ParseLocalAddress(args.Address)
	if err != nil {
		return fmt.Errorf("problem parsing address '%s': %w", args.Address, err)
	}
	addrSet := ids.ShortSet{}
	addrSet.Add(address)

	utxos, _, _, err := service.vm.GetUTXOs(addrSet, ids.ShortEmpty, ids.Empty, -1, false)
	if err != nil {
		return fmt.Errorf("couldn't get address's UTXOs: %w", err)
	}

	assetIDs := ids.Set{}               // IDs of assets the address has a non-zero balance of
	balances := make(map[ids.ID]uint64) // key: ID (as bytes). value: balance of that asset
	for _, utxo := range utxos {
		transferable, ok := utxo.Out.(avax.TransferableOut)
		if !ok {
			continue
		}
		assetID := utxo.AssetID()
		assetIDs.Add(assetID)
		balance := balances[assetID] // 0 if key doesn't exist
		balance, err := safemath.Add64(transferable.Amount(), balance)
		if err != nil {
			balances[assetID] = math.MaxUint64
		} else {
			balances[assetID] = balance
		}
	}

	reply.Balances = make([]Balance, assetIDs.Len())
	i := 0
	for assetID := range assetIDs {
		if alias, err := service.vm.PrimaryAlias(assetID); err == nil {
			reply.Balances[i] = Balance{
				AssetID: alias,
				Balance: json.Uint64(balances[assetID]),
			}
		} else {
			reply.Balances[i] = Balance{
				AssetID: assetID.String(),
				Balance: json.Uint64(balances[assetID]),
			}
		}
		i++
	}

	return nil
}

// Holder describes how much an address owns of an asset
type Holder struct {
	Amount  json.Uint64 `json:"amount"`
	Address string      `json:"address"`
}

// Minters describes who can mint an asset
type Minters struct {
	Threshold json.Uint32 `json:"threshold"`
	Minters   []string    `json:"minters"`
}

// Manager ...
type Manager struct {
	Threshold json.Uint32 `json:"threshold"`
	Addrs     []string    `json:"addresses"`
}

// CreateAssetArgs are arguments for passing into CreateAsset
type CreateAssetArgs struct {
	api.JSONSpendHeader           // User, password, from addrs, change addr
	Name                string    `json:"name"`
	Symbol              string    `json:"symbol"`
	Denomination        byte      `json:"denomination"`
	InitialHolders      []*Holder `json:"initialHolders"`
	MinterSets          []Minters `json:"minterSets"`
	Manager             Manager   `json:"manager"`
}

// AssetIDChangeAddr is an asset ID and a change address
type AssetIDChangeAddr struct {
	FormattedAssetID
	api.JSONChangeAddr
}

// CreateAsset returns ID of the newly created asset
func (service *Service) CreateAsset(r *http.Request, args *CreateAssetArgs, reply *AssetIDChangeAddr) error {
	service.vm.ctx.Log.Info("AVM: CreateAsset called with name: %s. symbol: %s. number of holders: %d. number of minters: %d. has manager: %t.",
		args.Name,
		args.Symbol,
		len(args.InitialHolders),
		len(args.MinterSets),
		len(args.Manager.Addrs) != 0,
	)

	// Sanity check arguments
	switch {
	case len(args.InitialHolders) == 0 && len(args.MinterSets) == 0:
		return errNoHoldersMinters
	case int(args.Manager.Threshold) > len(args.Manager.Addrs):
		return fmt.Errorf(
			"manager threshold (%d) > number of manager addresses (%d)",
			args.Manager.Threshold,
			len(args.Manager.Addrs),
		)

	}

	// Parse the from addresses
	fromAddrs := ids.ShortSet{}
	for _, addrStr := range args.From {
		addr, err := service.vm.ParseLocalAddress(addrStr)
		if err != nil {
			return fmt.Errorf("couldn't parse 'from' address %s: %w", addrStr, err)
		}
		fromAddrs.Add(addr)
	}

	// Get the UTXOs/keys for the from addresses
	utxos, kc, err := service.vm.LoadUser(args.Username, args.Password, fromAddrs)
	if err != nil {
		return err
	}

	// Parse the change address.
	if len(kc.Keys) == 0 {
		return errNoKeys
	}
	changeAddr, err := service.vm.selectChangeAddr(kc.Keys[0].PublicKey().Address(), args.ChangeAddr)
	if err != nil {
		return err
	}

	amountsSpent, ins, keys, err := service.vm.Spend(
		utxos,
		kc,
		map[ids.ID]uint64{
			service.vm.ctx.AVAXAssetID: service.vm.creationTxFee,
		},
	)
	if err != nil {
		return err
	}

	outs := []*avax.TransferableOutput{}
	if amountSpent := amountsSpent[service.vm.ctx.AVAXAssetID]; amountSpent > service.vm.creationTxFee {
		outs = append(outs, &avax.TransferableOutput{
			Asset: avax.Asset{ID: service.vm.ctx.AVAXAssetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: amountSpent - service.vm.creationTxFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{changeAddr},
				},
			},
		})
	}

	initialState := &InitialState{
		FxID: 0, // TODO: Should lookup secp256k1fx FxID
		Outs: make([]verify.State, 0, len(args.InitialHolders)+len(args.MinterSets)+1),
	}
	for _, holder := range args.InitialHolders {
		addr, err := service.vm.ParseLocalAddress(holder.Address)
		if err != nil {
			return err
		}
		initialState.Outs = append(initialState.Outs, &secp256k1fx.TransferOutput{
			Amt: uint64(holder.Amount),
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{addr},
			},
		})
	}
	for _, owner := range args.MinterSets {
		minter := &secp256k1fx.MintOutput{
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: uint32(owner.Threshold),
				Addrs:     make([]ids.ShortID, 0, len(owner.Minters)),
			},
		}
		for _, address := range owner.Minters {
			addr, err := service.vm.ParseLocalAddress(address)
			if err != nil {
				return err
			}
			minter.Addrs = append(minter.Addrs, addr)
		}
		ids.SortShortIDs(minter.Addrs)
		initialState.Outs = append(initialState.Outs, minter)
	}
	if len(args.Manager.Addrs) != 0 { // This is a managed asset
		status := &secp256k1fx.ManagedAssetStatusOutput{
			IsFrozen: false,
			Mgr: secp256k1fx.OutputOwners{
				Threshold: uint32(args.Manager.Threshold),
				Addrs:     make([]ids.ShortID, 0, len(args.Manager.Addrs)),
			},
		}
		for _, address := range args.Manager.Addrs {
			addr, err := service.vm.ParseLocalAddress(address)
			if err != nil {
				return err
			}
			status.Mgr.Addrs = append(status.Mgr.Addrs, addr)
		}
		ids.SortShortIDs(status.Mgr.Addrs)
		initialState.Outs = append(initialState.Outs, status)
	}
	initialState.Sort(service.vm.codec, service.vm.currentCodecVersion)

	tx := Tx{
		Version: service.vm.currentCodecVersion,
		UnsignedTx: &CreateAssetTx{
			BaseTx: BaseTx{BaseTx: avax.BaseTx{
				NetworkID:    service.vm.ctx.NetworkID,
				BlockchainID: service.vm.ctx.ChainID,
				Outs:         outs,
				Ins:          ins,
			}},
			Name:         args.Name,
			Symbol:       args.Symbol,
			Denomination: args.Denomination,
			States:       []*InitialState{initialState},
		},
	}
	if err := tx.SignSECP256K1Fx(service.vm.codec, keys); err != nil {
		return err
	}

	assetID, err := service.vm.IssueTx(tx.Bytes())
	if err != nil {
		return fmt.Errorf("problem issuing transaction: %w", err)
	}

	reply.AssetID = assetID
	reply.ChangeAddr, err = service.vm.FormatLocalAddress(changeAddr)
	return err
}

// CreateFixedCapAsset returns ID of the newly created asset
func (service *Service) CreateFixedCapAsset(r *http.Request, args *CreateAssetArgs, reply *AssetIDChangeAddr) error {
	service.vm.ctx.Log.Info("AVM: CreateFixedCapAsset called with name: %s symbol: %s number of holders: %d",
		args.Name,
		args.Symbol,
		len(args.InitialHolders),
	)

	return service.CreateAsset(nil, args, reply)
}

// CreateVariableCapAsset returns ID of the newly created asset
func (service *Service) CreateVariableCapAsset(r *http.Request, args *CreateAssetArgs, reply *AssetIDChangeAddr) error {
	service.vm.ctx.Log.Info("AVM: CreateVariableCapAsset called with name: %s symbol: %s number of minters: %d",
		args.Name,
		args.Symbol,
		len(args.MinterSets),
	)

	return service.CreateAsset(nil, args, reply)
}

// CreateNFTAssetArgs are arguments for passing into CreateNFTAsset requests
type CreateNFTAssetArgs struct {
	api.JSONSpendHeader           // User, password, from addrs, change addr
	Name                string    `json:"name"`
	Symbol              string    `json:"symbol"`
	MinterSets          []Minters `json:"minterSets"`
}

// CreateNFTAsset returns ID of the newly created asset
func (service *Service) CreateNFTAsset(r *http.Request, args *CreateNFTAssetArgs, reply *AssetIDChangeAddr) error {
	service.vm.ctx.Log.Info("AVM: CreateNFTAsset called with name: %s symbol: %s number of minters: %d",
		args.Name,
		args.Symbol,
		len(args.MinterSets),
	)

	if len(args.MinterSets) == 0 {
		return errNoMinters
	}

	// Parse the from addresses
	fromAddrs := ids.ShortSet{}
	for _, addrStr := range args.From {
		addr, err := service.vm.ParseLocalAddress(addrStr)
		if err != nil {
			return fmt.Errorf("couldn't parse 'from' address %s: %w", addrStr, err)
		}
		fromAddrs.Add(addr)
	}

	// Get the UTXOs/keys for the from addresses
	utxos, kc, err := service.vm.LoadUser(args.Username, args.Password, fromAddrs)
	if err != nil {
		return err
	}

	// Parse the change address.
	if len(kc.Keys) == 0 {
		return errNoKeys
	}
	changeAddr, err := service.vm.selectChangeAddr(kc.Keys[0].PublicKey().Address(), args.ChangeAddr)
	if err != nil {
		return err
	}

	amountsSpent, ins, keys, err := service.vm.Spend(
		utxos,
		kc,
		map[ids.ID]uint64{
			service.vm.ctx.AVAXAssetID: service.vm.creationTxFee,
		},
	)
	if err != nil {
		return err
	}

	outs := []*avax.TransferableOutput{}
	if amountSpent := amountsSpent[service.vm.ctx.AVAXAssetID]; amountSpent > service.vm.creationTxFee {
		outs = append(outs, &avax.TransferableOutput{
			Asset: avax.Asset{ID: service.vm.ctx.AVAXAssetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: amountSpent - service.vm.creationTxFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{changeAddr},
				},
			},
		})
	}

	initialState := &InitialState{
		FxID: 1, // TODO: Should lookup nftfx FxID
		Outs: make([]verify.State, 0, len(args.MinterSets)),
	}
	for i, owner := range args.MinterSets {
		minter := &nftfx.MintOutput{
			GroupID: uint32(i),
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: uint32(owner.Threshold),
			},
		}
		for _, address := range owner.Minters {
			addr, err := service.vm.ParseLocalAddress(address)
			if err != nil {
				return err
			}
			minter.Addrs = append(minter.Addrs, addr)
		}
		ids.SortShortIDs(minter.Addrs)
		initialState.Outs = append(initialState.Outs, minter)
	}
	initialState.Sort(service.vm.codec, service.vm.currentCodecVersion)

	tx := Tx{
		Version: service.vm.currentCodecVersion,
		UnsignedTx: &CreateAssetTx{
			BaseTx: BaseTx{BaseTx: avax.BaseTx{
				NetworkID:    service.vm.ctx.NetworkID,
				BlockchainID: service.vm.ctx.ChainID,
				Outs:         outs,
				Ins:          ins,
			}},
			Name:         args.Name,
			Symbol:       args.Symbol,
			Denomination: 0, // NFTs are non-fungible
			States:       []*InitialState{initialState},
		},
	}
	if err := tx.SignSECP256K1Fx(service.vm.codec, keys); err != nil {
		return err
	}

	assetID, err := service.vm.IssueTx(tx.Bytes())
	if err != nil {
		return fmt.Errorf("problem issuing transaction: %w", err)
	}

	reply.AssetID = assetID
	reply.ChangeAddr, err = service.vm.FormatLocalAddress(changeAddr)
	return err
}

// CreateAddress creates an address for the user [args.Username]
func (service *Service) CreateAddress(r *http.Request, args *api.UserPass, reply *api.JSONAddress) error {
	service.vm.ctx.Log.Info("AVM: CreateAddress called for user '%s'", args.Username)

	db, err := service.vm.ctx.Keystore.GetDatabase(args.Username, args.Password)
	if err != nil {
		return fmt.Errorf("problem retrieving user %q: %w", args.Username, err)
	}
	defer db.Close()

	user := userState{vm: service.vm}

	addresses, _ := user.Addresses(db)
	if len(addresses) >= maxKeystoreAddresses {
		return fmt.Errorf("keystore user has reached its limit of %d addresses", maxKeystoreAddresses)
	}

	factory := crypto.FactorySECP256K1R{}
	skIntf, err := factory.NewPrivateKey()
	if err != nil {
		return fmt.Errorf("problem generating private key: %w", err)
	}
	sk := skIntf.(*crypto.PrivateKeySECP256K1R)

	if err := user.SetKey(db, sk); err != nil {
		return fmt.Errorf("problem saving private key: %w", err)
	}

	addresses = append(addresses, sk.PublicKey().Address())

	if err := user.SetAddresses(db, addresses); err != nil {
		return fmt.Errorf("problem saving address: %w", err)
	}
	reply.Address, err = service.vm.FormatLocalAddress(sk.PublicKey().Address())
	if err != nil {
		return fmt.Errorf("problem formatting address: %w", err)
	}

	// Return an error if the DB can't close, this will execute before the above
	// db close.
	return db.Close()
}

// ListAddresses returns all of the addresses controlled by user [args.Username]
func (service *Service) ListAddresses(_ *http.Request, args *api.UserPass, response *api.JSONAddresses) error {
	service.vm.ctx.Log.Info("AVM: ListAddresses called for user '%s'", args.Username)

	db, err := service.vm.ctx.Keystore.GetDatabase(args.Username, args.Password)
	if err != nil {
		return fmt.Errorf("problem retrieving user '%s': %w", args.Username, err)
	}

	response.Addresses = []string{}

	user := userState{vm: service.vm}
	addresses, err := user.Addresses(db)
	if err != nil {
		// An error fetching the addresses may just mean that the user has no
		// addresses.
		return db.Close()
	}

	for _, address := range addresses {
		addr, err := service.vm.FormatLocalAddress(address)
		if err != nil {
			// Drop any potential error closing the database to report the
			// original error
			_ = db.Close()
			return fmt.Errorf("problem formatting address: %w", err)
		}
		response.Addresses = append(response.Addresses, addr)
	}
	return db.Close()
}

// ExportKeyArgs are arguments for ExportKey
type ExportKeyArgs struct {
	api.UserPass
	Address string `json:"address"`
}

// ExportKeyReply is the response for ExportKey
type ExportKeyReply struct {
	// The decrypted PrivateKey for the Address provided in the arguments
	PrivateKey string `json:"privateKey"`
}

// ExportKey returns a private key from the provided user
func (service *Service) ExportKey(r *http.Request, args *ExportKeyArgs, reply *ExportKeyReply) error {
	service.vm.ctx.Log.Info("AVM: ExportKey called for user %q", args.Username)

	addr, err := service.vm.ParseLocalAddress(args.Address)
	if err != nil {
		return fmt.Errorf("problem parsing address %q: %w", args.Address, err)
	}

	db, err := service.vm.ctx.Keystore.GetDatabase(args.Username, args.Password)
	if err != nil {
		return fmt.Errorf("problem retrieving user %q: %w", args.Username, err)
	}
	defer db.Close()

	user := userState{vm: service.vm}

	sk, err := user.Key(db, addr)
	if err != nil {
		return fmt.Errorf("problem retrieving private key: %w", err)
	}

	// We assume that the maximum size of a byte slice that
	// can be stringified is at least the length of a SECP256K1 private key
	privKeyStr, _ := formatting.Encode(formatting.CB58, sk.Bytes())
	reply.PrivateKey = constants.SecretKeyPrefix + privKeyStr
	return db.Close()
}

// ImportKeyArgs are arguments for ImportKey
type ImportKeyArgs struct {
	api.UserPass
	PrivateKey string `json:"privateKey"`
}

// ImportKeyReply is the response for ImportKey
type ImportKeyReply struct {
	// The address controlled by the PrivateKey provided in the arguments
	Address string `json:"address"`
}

// ImportKey adds a private key to the provided user
func (service *Service) ImportKey(r *http.Request, args *ImportKeyArgs, reply *api.JSONAddress) error {
	service.vm.ctx.Log.Info("AVM: ImportKey called for user '%s'", args.Username)

	db, err := service.vm.ctx.Keystore.GetDatabase(args.Username, args.Password)
	if err != nil {
		return fmt.Errorf("problem retrieving data: %w", err)
	}
	defer db.Close()

	user := userState{vm: service.vm}

	addresses, _ := user.Addresses(db)
	if len(addresses) >= maxKeystoreAddresses {
		return fmt.Errorf("keystore user has reached its limit of %d addresses", maxKeystoreAddresses)
	}

	if !strings.HasPrefix(args.PrivateKey, constants.SecretKeyPrefix) {
		return fmt.Errorf("private key missing %s prefix", constants.SecretKeyPrefix)
	}
	trimmedPrivateKey := strings.TrimPrefix(args.PrivateKey, constants.SecretKeyPrefix)
	privKeyBytes, err := formatting.Decode(formatting.CB58, trimmedPrivateKey)
	if err != nil {
		return fmt.Errorf("problem parsing private key: %w", err)
	}

	factory := crypto.FactorySECP256K1R{}
	skIntf, err := factory.ToPrivateKey(privKeyBytes)
	if err != nil {
		return fmt.Errorf("problem parsing private key: %w", err)
	}
	sk := skIntf.(*crypto.PrivateKeySECP256K1R)

	if err := user.SetKey(db, sk); err != nil {
		return fmt.Errorf("problem saving key %w", err)
	}

	newAddress := sk.PublicKey().Address()
	reply.Address, err = service.vm.FormatLocalAddress(newAddress)
	if err != nil {
		return fmt.Errorf("problem formatting address: %w", err)
	}
	for _, address := range addresses {
		if newAddress == address {
			return db.Close()
		}
	}

	addresses = append(addresses, newAddress)
	if err := user.SetAddresses(db, addresses); err != nil {
		return fmt.Errorf("problem saving addresses: %w", err)
	}

	return db.Close()
}

// SendOutput specifies that [Amount] of asset [AssetID] be sent to [To]
type SendOutput struct {
	// The amount of funds to send
	Amount json.Uint64 `json:"amount"`

	// ID of the asset being sent
	AssetID string `json:"assetID"`

	// Address of the recipient
	To string `json:"to"`
}

// SendArgs are arguments for passing into Send requests
type SendArgs struct {
	// User, password, from addrs, change addr
	api.JSONSpendHeader

	// The amount, assetID, and destination to send funds to
	SendOutput

	// Memo field
	Memo string `json:"memo"`
}

// SendMultipleArgs are arguments for passing into SendMultiple requests
type SendMultipleArgs struct {
	// User, password, from addrs, change addr
	api.JSONSpendHeader

	// The outputs of the transaction
	Outputs []SendOutput `json:"outputs"`

	// Memo field
	Memo string `json:"memo"`
}

// Send returns the ID of the newly created transaction
func (service *Service) Send(r *http.Request, args *SendArgs, reply *api.JSONTxIDChangeAddr) error {
	return service.SendMultiple(r, &SendMultipleArgs{
		JSONSpendHeader: args.JSONSpendHeader,
		Outputs:         []SendOutput{args.SendOutput},
		Memo:            args.Memo,
	}, reply)
}

// SendMultiple sends a transaction with multiple outputs.
func (service *Service) SendMultiple(r *http.Request, args *SendMultipleArgs, reply *api.JSONTxIDChangeAddr) error {
	service.vm.ctx.Log.Info("AVM: Send called with username: %s", args.Username)

	// Validate the memo field
	memoBytes := []byte(args.Memo)
	if l := len(memoBytes); l > avax.MaxMemoSize {
		return fmt.Errorf("max memo length is %d but provided memo field is length %d", avax.MaxMemoSize, l)
	} else if len(args.Outputs) == 0 {
		return errNoOutputs
	}

	// Parse the from addresses
	fromAddrs := ids.ShortSet{}
	for _, addrStr := range args.From {
		addr, err := service.vm.ParseLocalAddress(addrStr)
		if err != nil {
			return fmt.Errorf("couldn't parse 'From' address %s: %w", addrStr, err)
		}
		fromAddrs.Add(addr)
	}

	// Load user's UTXOs/keys
	utxos, kc, err := service.vm.LoadUser(args.Username, args.Password, fromAddrs)
	if err != nil {
		return err
	}

	// Parse the change address.
	if len(kc.Keys) == 0 {
		return errNoKeys
	}
	changeAddr, err := service.vm.selectChangeAddr(kc.Keys[0].PublicKey().Address(), args.ChangeAddr)
	if err != nil {
		return err
	}

	// Calculate required input amounts and create the desired outputs
	// String repr. of asset ID --> asset ID
	assetIDs := make(map[string]ids.ID)
	// Asset ID --> amount of that asset being sent
	amounts := make(map[ids.ID]uint64)
	// Outputs of our tx
	outs := []*avax.TransferableOutput{}
	for _, output := range args.Outputs {
		if output.Amount == 0 {
			return errZeroAmount
		}
		assetID, ok := assetIDs[output.AssetID] // Asset ID of next output
		if !ok {
			assetID, err = service.vm.lookupAssetID(output.AssetID)
			if err != nil {
				return fmt.Errorf("couldn't find asset %s", output.AssetID)
			}
			assetIDs[output.AssetID] = assetID
		}
		currentAmount := amounts[assetID]
		newAmount, err := safemath.Add64(currentAmount, uint64(output.Amount))
		if err != nil {
			return fmt.Errorf("problem calculating required spend amount: %w", err)
		}
		amounts[assetID] = newAmount

		// Parse the to address
		to, err := service.vm.ParseLocalAddress(output.To)
		if err != nil {
			return fmt.Errorf("problem parsing to address %q: %w", output.To, err)
		}

		// Create the Output
		outs = append(outs, &avax.TransferableOutput{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: uint64(output.Amount),
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{to},
				},
			},
		})
	}

	amountsWithFee := make(map[ids.ID]uint64, len(amounts)+1)
	for assetID, amount := range amounts {
		amountsWithFee[assetID] = amount
	}

	amountWithFee, err := safemath.Add64(amounts[service.vm.ctx.AVAXAssetID], service.vm.txFee)
	if err != nil {
		return fmt.Errorf("problem calculating required spend amount: %w", err)
	}
	amountsWithFee[service.vm.ctx.AVAXAssetID] = amountWithFee

	amountsSpent, ins, keys, err := service.vm.Spend(
		utxos,
		kc,
		amountsWithFee,
	)
	if err != nil {
		return err
	}

	// Add the required change outputs
	for assetID, amountWithFee := range amountsWithFee {
		amountSpent := amountsSpent[assetID]

		if amountSpent > amountWithFee {
			outs = append(outs, &avax.TransferableOutput{
				Asset: avax.Asset{ID: assetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: amountSpent - amountWithFee,
					OutputOwners: secp256k1fx.OutputOwners{
						Locktime:  0,
						Threshold: 1,
						Addrs:     []ids.ShortID{changeAddr},
					},
				},
			})
		}
	}
	avax.SortTransferableOutputs(outs, service.vm.codec, service.vm.currentCodecVersion)

	tx := Tx{
		Version: service.vm.currentCodecVersion,
		UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    service.vm.ctx.NetworkID,
			BlockchainID: service.vm.ctx.ChainID,
			Outs:         outs,
			Ins:          ins,
			Memo:         memoBytes,
		}},
	}
	if err := tx.SignSECP256K1Fx(service.vm.codec, keys); err != nil {
		return err
	}

	txID, err := service.vm.IssueTx(tx.Bytes())
	if err != nil {
		return fmt.Errorf("problem issuing transaction: %w", err)
	}

	reply.TxID = txID
	reply.ChangeAddr, err = service.vm.FormatLocalAddress(changeAddr)
	return err
}

// MintArgs are arguments for passing into Mint requests
type MintArgs struct {
	api.JSONSpendHeader             // User, password, from addrs, change addr
	Amount              json.Uint64 `json:"amount"`
	AssetID             string      `json:"assetID"`
	To                  string      `json:"to"`
}

// Mint issues a transaction that mints more of the asset
func (service *Service) Mint(r *http.Request, args *MintArgs, reply *api.JSONTxIDChangeAddr) error {
	service.vm.ctx.Log.Info("AVM: Mint called with username: %s", args.Username)

	if args.Amount == 0 {
		return errInvalidMintAmount
	}

	assetID, err := service.vm.lookupAssetID(args.AssetID)
	if err != nil {
		return err
	}

	to, err := service.vm.ParseLocalAddress(args.To)
	if err != nil {
		return fmt.Errorf("problem parsing to address %q: %w", args.To, err)
	}

	// Parse the from addresses
	fromAddrs := ids.ShortSet{}
	for _, addrStr := range args.From {
		addr, err := service.vm.ParseLocalAddress(addrStr)
		if err != nil {
			return fmt.Errorf("couldn't parse 'from' address %s: %w", addrStr, err)
		}
		fromAddrs.Add(addr)
	}

	// Get the UTXOs/keys for the from addresses
	feeUTXOs, feeKc, err := service.vm.LoadUser(args.Username, args.Password, fromAddrs)
	if err != nil {
		return err
	}

	// Parse the change address.
	if len(feeKc.Keys) == 0 {
		return errNoKeys
	}
	changeAddr, err := service.vm.selectChangeAddr(feeKc.Keys[0].PublicKey().Address(), args.ChangeAddr)
	if err != nil {
		return err
	}

	amountsSpent, ins, keys, err := service.vm.Spend(
		feeUTXOs,
		feeKc,
		map[ids.ID]uint64{
			service.vm.ctx.AVAXAssetID: service.vm.txFee,
		},
	)
	if err != nil {
		return err
	}

	outs := []*avax.TransferableOutput{}
	if amountSpent := amountsSpent[service.vm.ctx.AVAXAssetID]; amountSpent > service.vm.txFee {
		outs = append(outs, &avax.TransferableOutput{
			Asset: avax.Asset{ID: service.vm.ctx.AVAXAssetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: amountSpent - service.vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{changeAddr},
				},
			},
		})
	}

	// Get all UTXOs/keys for the user
	utxos, kc, err := service.vm.LoadUser(args.Username, args.Password, nil)
	if err != nil {
		return err
	}

	var opsKeys [][]*crypto.PrivateKeySECP256K1R
	ops, opsKeys, err := service.vm.Mint(
		utxos,
		kc,
		map[ids.ID]uint64{
			assetID: uint64(args.Amount),
		},
		to,
	)
	if err != nil {
		return err
	}
	keys = append(keys, opsKeys...)

	tx := Tx{
		Version: service.vm.currentCodecVersion,
		UnsignedTx: &OperationTx{
			BaseTx: BaseTx{BaseTx: avax.BaseTx{
				NetworkID:    service.vm.ctx.NetworkID,
				BlockchainID: service.vm.ctx.ChainID,
				Outs:         outs,
				Ins:          ins,
			}},
			Ops: ops,
		},
	}
	if err := tx.SignSECP256K1Fx(service.vm.codec, keys); err != nil {
		return err
	}

	txID, err := service.vm.IssueTx(tx.Bytes())
	if err != nil {
		return fmt.Errorf("problem issuing transaction: %w", err)
	}

	reply.TxID = txID
	reply.ChangeAddr, err = service.vm.FormatLocalAddress(changeAddr)
	return err
}

// UpdateManagedAssetArgs are the arguments for UpdateManagedAsset
type UpdateManagedAssetArgs struct {
	// User, password, from addrs, change addr
	api.JSONSpendHeader
	// ID/alias of the asset being frozen
	AssetID string `json:"assetID"`
	// True if the asset is to be frozen
	Frozen bool `json:"frozen"`
	// New manager of the asset
	Manager `json:"manager"`
}

// UpdateManagedAsset issues a transaction that updates a managed asset
func (service *Service) UpdateManagedAsset(r *http.Request, args *UpdateManagedAssetArgs, reply *api.JSONTxIDChangeAddr) error {
	service.vm.ctx.Log.Info("AVM: UpdateManagedAsset called with username: %s", args.Username)

	assetID, err := service.vm.lookupAssetID(args.AssetID)
	if err != nil {
		return err
	}

	// Parse the from addresses
	fromAddrs := ids.ShortSet{}
	for _, addrStr := range args.From {
		addr, err := service.vm.ParseLocalAddress(addrStr)
		if err != nil {
			return fmt.Errorf("couldn't parse 'from' address %s: %w", addrStr, err)
		}
		fromAddrs.Add(addr)
	}

	// Get the UTXOs/keys for the from addresses
	feeUTXOs, feeKc, err := service.vm.LoadUser(args.Username, args.Password, fromAddrs)
	if err != nil {
		return err
	}

	// Parse the change address.
	if len(feeKc.Keys) == 0 {
		return errNoKeys
	}
	changeAddr, err := service.vm.selectChangeAddr(feeKc.Keys[0].PublicKey().Address(), args.ChangeAddr)
	if err != nil {
		return err
	}

	amountsSpent, ins, keys, err := service.vm.Spend(
		feeUTXOs,
		feeKc,
		map[ids.ID]uint64{
			service.vm.ctx.AVAXAssetID: service.vm.txFee,
		},
	)
	if err != nil {
		return err
	}

	outs := []*avax.TransferableOutput{}
	if amountSpent := amountsSpent[service.vm.ctx.AVAXAssetID]; amountSpent > service.vm.txFee {
		outs = append(outs, &avax.TransferableOutput{
			Asset: avax.Asset{ID: service.vm.ctx.AVAXAssetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: amountSpent - service.vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{changeAddr},
				},
			},
		})
	}

	// Get all UTXOs/keys for the user
	utxos, kc, err := service.vm.LoadUser(args.Username, args.Password, nil)
	if err != nil {
		return err
	}

	manager := &secp256k1fx.OutputOwners{
		Threshold: uint32(args.Manager.Threshold),
		Addrs:     make([]ids.ShortID, 0, len(args.Manager.Addrs)),
	}
	for _, address := range args.Manager.Addrs {
		addr, err := service.vm.ParseLocalAddress(address)
		if err != nil {
			return err
		}
		manager.Addrs = append(manager.Addrs, addr)
	}

	op, opKeys, err := newUpdateManagedAssetStatusOperation(
		utxos,
		kc,
		assetID,
		service.vm.Clock().Unix(),
		args.Frozen,
		manager,
	)
	if err != nil {
		return err
	}
	keys = append(keys, opKeys)

	tx := Tx{
		Version: service.vm.currentCodecVersion,
		UnsignedTx: &OperationTx{
			BaseTx: BaseTx{BaseTx: avax.BaseTx{
				NetworkID:    service.vm.ctx.NetworkID,
				BlockchainID: service.vm.ctx.ChainID,
				Outs:         outs,
				Ins:          ins,
			}},
			Ops: []*Operation{op},
		},
	}
	if err := tx.SignSECP256K1Fx(service.vm.codec, keys); err != nil {
		return err
	}

	txID, err := service.vm.IssueTx(tx.Bytes())
	if err != nil {
		return fmt.Errorf("problem issuing transaction: %w", err)
	}

	reply.TxID = txID
	reply.ChangeAddr, err = service.vm.FormatLocalAddress(changeAddr)
	return err
}

// SendAsManagerArgs ...
type SendAsManagerArgs struct {
	SendArgs
	// Addresses that tx fee is paid from
	FeeFrom []string `json:"feeFrom"`
	// Address that change from the tx fee is paid to
	FeeChangeAddr string `json:"feeChangeAddr"`
}

type SendAsManagerResponse struct {
	api.JSONTxIDChangeAddr
	FeeChangeAddr string `json:"feeChangeAddr"`
}

// SendAsManager issues a transaction that sends a managed asset
func (service *Service) SendAsManager(_ *http.Request, args *SendAsManagerArgs, reply *SendAsManagerResponse) error {
	service.vm.ctx.Log.Info("AVM: SendAsManager called with username: %s", args.Username)

	// Validate the memo field
	memoBytes := []byte(args.Memo)
	if l := len(memoBytes); l > avax.MaxMemoSize {
		return fmt.Errorf("max memo length is %d but provided memo field is length %d", avax.MaxMemoSize, l)
	}

	// Ensure at least one from address given
	if len(args.From) == 0 {
		return errNoFromAddrs
	}

	// Parse the from addresses
	fromAddrs := ids.ShortSet{}
	for _, addrStr := range args.From {
		addr, err := service.vm.ParseLocalAddress(addrStr)
		if err != nil {
			return fmt.Errorf("couldn't parse 'from' address %s: %w", addrStr, err)
		}
		fromAddrs.Add(addr)
	}

	// Parse the tx fee from addresses
	feeFromAddrs := ids.ShortSet{}
	for _, addrStr := range args.FeeFrom {
		addr, err := service.vm.ParseLocalAddress(addrStr)
		if err != nil {
			return fmt.Errorf("couldn't parse 'feeFrom' address %s: %w", addrStr, err)
		}
		feeFromAddrs.Add(addr)
	}

	// Parse the asset ID
	assetID, err := service.vm.lookupAssetID(args.AssetID)
	if err != nil {
		return err
	}

	// Parse the to address
	to, err := service.vm.ParseLocalAddress(args.To)
	if err != nil {
		return fmt.Errorf("problem parsing 'to' address %s: %w", args.To, err)
	}

	// Load user's keys and UTXOs. We only use the UTXOs to pay the tx fee.
	// The UTXOs containing the managed asset are owned by the from addresses.
	feeUTXOs, kc, err := service.vm.LoadUser(args.Username, args.Password, feeFromAddrs)
	if err != nil {
		return err
	}

	// Parse the managed asset change address.
	if len(kc.Keys) == 0 {
		return errNoKeys
	}
	changeAddr, err := service.vm.selectChangeAddr(kc.Keys[0].PublicKey().Address(), args.ChangeAddr)
	if err != nil {
		return err
	}

	// Parse the fee change address
	feeChangeAddr, err := service.vm.selectChangeAddr(kc.Keys[0].PublicKey().Address(), args.FeeChangeAddr)
	if err != nil {
		return err
	}

	ins := []*avax.TransferableInput{}
	keys := [][]*crypto.PrivateKeySECP256K1R{}
	amountsSpent, feeIns, feeKeys, err := service.vm.Spend(
		feeUTXOs,
		kc,
		map[ids.ID]uint64{
			service.vm.ctx.AVAXAssetID: service.vm.txFee,
		},
	)
	if err != nil {
		return err
	}
	ins = append(ins, feeIns...)
	keys = append(keys, feeKeys...)

	// Create an output for the fee (AVAX) change
	outs := []*avax.TransferableOutput{}
	if amountSpent := amountsSpent[service.vm.ctx.AVAXAssetID]; amountSpent > service.vm.txFee {
		outs = append(outs, &avax.TransferableOutput{
			Asset: avax.Asset{ID: service.vm.ctx.AVAXAssetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: amountSpent - service.vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{feeChangeAddr},
				},
			},
		})
	}

	// Load all the keys held by the user
	_, kc, err = service.vm.LoadUser(args.Username, args.Password, nil)
	if err != nil {
		return err
	}

	// Load UTXOs controlled by from addesses
	utxos, _, _, err := service.vm.GetUTXOs(fromAddrs, ids.ShortEmpty, ids.Empty, -1, false)
	if err != nil {
		return fmt.Errorf("problem retrieving UTXOs: %w", err)
	}

	// Get the manager of this asset
	// TODO: Right now this method assumes that the manager of
	// this asset is [status.Manager] but it could actually be the manager
	// specified in the old status because 2 epochs haven't passed
	// since the last update. Add an argument to this method to specify
	// an epoch to issue this transaction in.
	_, status, _, err := service.vm.state.ManagedAssetStatus(assetID)
	if err == database.ErrNotFound {
		return fmt.Errorf("%s is not a managed asset", assetID)
	} else if err != nil {
		return fmt.Errorf("couldn't get asset manager: %w", err)
	}

	amountSpent, managedAssetIns, managedAssetKeys, err := spendManagedAsset(
		assetID,
		status.Manager(),
		utxos,
		kc,
		uint64(args.Amount),
		service.vm.Clock().Unix(),
	)
	if err != nil {
		return err
	}
	ins = append(ins, managedAssetIns...)
	keys = append(keys, managedAssetKeys...)

	// Create an output for the sent managed asset
	outs = append(outs, &avax.TransferableOutput{
		Asset: avax.Asset{ID: assetID},
		Out: &secp256k1fx.TransferOutput{
			Amt: uint64(args.Amount),
			OutputOwners: secp256k1fx.OutputOwners{
				Locktime:  0,
				Threshold: 1,
				Addrs:     []ids.ShortID{to},
			},
		},
	})

	// Make an output for the managed asset change, if applicable
	if amountSpent > uint64(args.Amount) {
		outs = append(outs, &avax.TransferableOutput{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: amountSpent - uint64(args.Amount),
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{changeAddr},
				},
			},
		})
	}

	avax.SortTransferableInputsWithSigners(ins, keys)
	avax.SortTransferableOutputs(outs, service.vm.codec, service.vm.currentCodecVersion)

	tx := Tx{
		Version: service.vm.currentCodecVersion,
		UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    service.vm.ctx.NetworkID,
			BlockchainID: service.vm.ctx.ChainID,
			Outs:         outs,
			Ins:          ins,
			Memo:         memoBytes,
		}},
	}
	if err := tx.SignSECP256K1Fx(service.vm.codec, keys); err != nil {
		return err
	}

	txID, err := service.vm.IssueTx(tx.Bytes())
	if err != nil {
		return fmt.Errorf("problem issuing transaction: %w", err)
	}

	reply.TxID = txID
	reply.FeeChangeAddr, err = service.vm.FormatLocalAddress(feeChangeAddr)
	if err != nil {
		return err
	}
	reply.ChangeAddr, err = service.vm.FormatLocalAddress(changeAddr)
	return err
}

// SendNFTArgs are arguments for passing into SendNFT requests
type SendNFTArgs struct {
	api.JSONSpendHeader             // User, password, from addrs, change addr
	AssetID             string      `json:"assetID"`
	GroupID             json.Uint32 `json:"groupID"`
	To                  string      `json:"to"`
}

// SendNFT sends an NFT
func (service *Service) SendNFT(r *http.Request, args *SendNFTArgs, reply *api.JSONTxIDChangeAddr) error {
	service.vm.ctx.Log.Info("AVM: SendNFT called with username: %s", args.Username)

	// Parse the asset ID
	assetID, err := service.vm.lookupAssetID(args.AssetID)
	if err != nil {
		return err
	}

	// Parse the to address
	to, err := service.vm.ParseLocalAddress(args.To)
	if err != nil {
		return fmt.Errorf("problem parsing to address %q: %w", args.To, err)
	}

	// Parse the from addresses
	fromAddrs := ids.ShortSet{}
	for _, addrStr := range args.From {
		addr, err := service.vm.ParseLocalAddress(addrStr)
		if err != nil {
			return fmt.Errorf("couldn't parse 'from' address %s: %w", addrStr, err)
		}
		fromAddrs.Add(addr)
	}

	// Get the UTXOs/keys for the from addresses
	utxos, kc, err := service.vm.LoadUser(args.Username, args.Password, fromAddrs)
	if err != nil {
		return err
	}

	// Parse the change address.
	if len(kc.Keys) == 0 {
		return errNoKeys
	}
	changeAddr, err := service.vm.selectChangeAddr(kc.Keys[0].PublicKey().Address(), args.ChangeAddr)
	if err != nil {
		return err
	}

	amountsSpent, ins, secpKeys, err := service.vm.Spend(
		utxos,
		kc,
		map[ids.ID]uint64{
			service.vm.ctx.AVAXAssetID: service.vm.txFee,
		},
	)
	if err != nil {
		return err
	}

	outs := []*avax.TransferableOutput{}
	if amountSpent := amountsSpent[service.vm.ctx.AVAXAssetID]; amountSpent > service.vm.txFee {
		outs = append(outs, &avax.TransferableOutput{
			Asset: avax.Asset{ID: service.vm.ctx.AVAXAssetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: amountSpent - service.vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{changeAddr},
				},
			},
		})
	}

	ops, nftKeys, err := service.vm.SpendNFT(
		utxos,
		kc,
		assetID,
		uint32(args.GroupID),
		to,
	)
	if err != nil {
		return err
	}

	tx := Tx{
		Version: service.vm.currentCodecVersion,
		UnsignedTx: &OperationTx{
			BaseTx: BaseTx{BaseTx: avax.BaseTx{
				NetworkID:    service.vm.ctx.NetworkID,
				BlockchainID: service.vm.ctx.ChainID,
				Outs:         outs,
				Ins:          ins,
			}},
			Ops: ops,
		},
	}
	if err := tx.SignSECP256K1Fx(service.vm.codec, secpKeys); err != nil {
		return err
	}
	if err := tx.SignNFTFx(service.vm.codec, nftKeys); err != nil {
		return err
	}

	txID, err := service.vm.IssueTx(tx.Bytes())
	if err != nil {
		return fmt.Errorf("problem issuing transaction: %w", err)
	}

	reply.TxID = txID
	reply.ChangeAddr, err = service.vm.FormatLocalAddress(changeAddr)
	return err
}

// MintNFTArgs are arguments for passing into MintNFT requests
type MintNFTArgs struct {
	api.JSONSpendHeader                     // User, password, from addrs, change addr
	AssetID             string              `json:"assetID"`
	Payload             string              `json:"payload"`
	To                  string              `json:"to"`
	Encoding            formatting.Encoding `json:"encoding"`
}

// MintNFT issues a MintNFT transaction and returns the ID of the newly created transaction
func (service *Service) MintNFT(r *http.Request, args *MintNFTArgs, reply *api.JSONTxIDChangeAddr) error {
	service.vm.ctx.Log.Info("AVM: MintNFT called with username: %s", args.Username)

	assetID, err := service.vm.lookupAssetID(args.AssetID)
	if err != nil {
		return err
	}

	to, err := service.vm.ParseLocalAddress(args.To)
	if err != nil {
		return fmt.Errorf("problem parsing to address %q: %w", args.To, err)
	}

	payloadBytes, err := formatting.Decode(args.Encoding, args.Payload)
	if err != nil {
		return fmt.Errorf("problem decoding payload bytes: %w", err)
	}

	// Parse the from addresses
	fromAddrs := ids.ShortSet{}
	for _, addrStr := range args.From {
		addr, err := service.vm.ParseLocalAddress(addrStr)
		if err != nil {
			return fmt.Errorf("couldn't parse 'from' address %s: %w", addrStr, err)
		}
		fromAddrs.Add(addr)
	}

	// Get the UTXOs/keys for the from addresses
	feeUTXOs, feeKc, err := service.vm.LoadUser(args.Username, args.Password, fromAddrs)
	if err != nil {
		return err
	}

	// Parse the change address.
	if len(feeKc.Keys) == 0 {
		return errNoKeys
	}
	changeAddr, err := service.vm.selectChangeAddr(feeKc.Keys[0].PublicKey().Address(), args.ChangeAddr)
	if err != nil {
		return err
	}

	amountsSpent, ins, secpKeys, err := service.vm.Spend(
		feeUTXOs,
		feeKc,
		map[ids.ID]uint64{
			service.vm.ctx.AVAXAssetID: service.vm.txFee,
		},
	)
	if err != nil {
		return err
	}

	outs := []*avax.TransferableOutput{}
	if amountSpent := amountsSpent[service.vm.ctx.AVAXAssetID]; amountSpent > service.vm.txFee {
		outs = append(outs, &avax.TransferableOutput{
			Asset: avax.Asset{ID: service.vm.ctx.AVAXAssetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: amountSpent - service.vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{changeAddr},
				},
			},
		})
	}

	// Get all UTXOs/keys
	utxos, kc, err := service.vm.LoadUser(args.Username, args.Password, nil)
	if err != nil {
		return err
	}

	ops, nftKeys, err := service.vm.MintNFT(
		utxos,
		kc,
		assetID,
		payloadBytes,
		to,
	)
	if err != nil {
		return err
	}

	tx := Tx{
		Version: service.vm.currentCodecVersion,
		UnsignedTx: &OperationTx{
			BaseTx: BaseTx{BaseTx: avax.BaseTx{
				NetworkID:    service.vm.ctx.NetworkID,
				BlockchainID: service.vm.ctx.ChainID,
				Outs:         outs,
				Ins:          ins,
			}},
			Ops: ops,
		},
	}
	if err := tx.SignSECP256K1Fx(service.vm.codec, secpKeys); err != nil {
		return err
	}
	if err := tx.SignNFTFx(service.vm.codec, nftKeys); err != nil {
		return err
	}

	txID, err := service.vm.IssueTx(tx.Bytes())
	if err != nil {
		return fmt.Errorf("problem issuing transaction: %w", err)
	}

	reply.TxID = txID
	reply.ChangeAddr, err = service.vm.FormatLocalAddress(changeAddr)
	return err
}

// ImportArgs are arguments for passing into Import requests
type ImportArgs struct {
	// User that controls To
	api.UserPass

	// Chain the funds are coming from
	SourceChain string `json:"sourceChain"`

	// Address receiving the imported AVAX
	To string `json:"to"`
}

// ImportAVAX is a deprecated name for Import.
func (service *Service) ImportAVAX(_ *http.Request, args *ImportArgs, reply *api.JSONTxID) error {
	return service.Import(nil, args, reply)
}

// Import imports an asset to this chain from the P/C-Chain.
// The AVAX must have already been exported from the P/C-Chain.
// Returns the ID of the newly created atomic transaction
func (service *Service) Import(_ *http.Request, args *ImportArgs, reply *api.JSONTxID) error {
	service.vm.ctx.Log.Info("AVM: Import called with username: %s", args.Username)

	chainID, err := service.vm.ctx.BCLookup.Lookup(args.SourceChain)
	if err != nil {
		return fmt.Errorf("problem parsing chainID %q: %w", args.SourceChain, err)
	}

	to, err := service.vm.ParseLocalAddress(args.To)
	if err != nil {
		return fmt.Errorf("problem parsing to address %q: %w", args.To, err)
	}

	utxos, kc, err := service.vm.LoadUser(args.Username, args.Password, nil)
	if err != nil {
		return err
	}

	atomicUTXOs, _, _, err := service.vm.GetAtomicUTXOs(chainID, kc.Addrs, ids.ShortEmpty, ids.Empty, -1)
	if err != nil {
		return fmt.Errorf("problem retrieving user's atomic UTXOs: %w", err)
	}

	amountsSpent, importInputs, importKeys, err := service.vm.SpendAll(atomicUTXOs, kc)
	if err != nil {
		return err
	}

	ins := []*avax.TransferableInput{}
	keys := [][]*crypto.PrivateKeySECP256K1R{}

	if amountSpent := amountsSpent[service.vm.ctx.AVAXAssetID]; amountSpent < service.vm.txFee {
		var localAmountsSpent map[ids.ID]uint64
		localAmountsSpent, ins, keys, err = service.vm.Spend(
			utxos,
			kc,
			map[ids.ID]uint64{
				service.vm.ctx.AVAXAssetID: service.vm.txFee - amountSpent,
			},
		)
		if err != nil {
			return err
		}
		for asset, amount := range localAmountsSpent {
			newAmount, err := safemath.Add64(amountsSpent[asset], amount)
			if err != nil {
				return fmt.Errorf("problem calculating required spend amount: %w", err)
			}
			amountsSpent[asset] = newAmount
		}
	}

	// Because we ensured that we had enough inputs for the fee, we can
	// safely just remove it without concern for underflow.
	amountsSpent[service.vm.ctx.AVAXAssetID] -= service.vm.txFee

	keys = append(keys, importKeys...)

	outs := []*avax.TransferableOutput{}
	for assetID, amount := range amountsSpent {
		if amount > 0 {
			outs = append(outs, &avax.TransferableOutput{
				Asset: avax.Asset{ID: assetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: amount,
					OutputOwners: secp256k1fx.OutputOwners{
						Locktime:  0,
						Threshold: 1,
						Addrs:     []ids.ShortID{to},
					},
				},
			})
		}
	}
	avax.SortTransferableOutputs(outs, service.vm.codec, service.vm.currentCodecVersion)

	tx := Tx{
		Version: service.vm.currentCodecVersion,
		UnsignedTx: &ImportTx{
			BaseTx: BaseTx{BaseTx: avax.BaseTx{
				NetworkID:    service.vm.ctx.NetworkID,
				BlockchainID: service.vm.ctx.ChainID,
				Outs:         outs,
				Ins:          ins,
			}},
			SourceChain: chainID,
			ImportedIns: importInputs,
		},
	}
	if err := tx.SignSECP256K1Fx(service.vm.codec, keys); err != nil {
		return err
	}

	txID, err := service.vm.IssueTx(tx.Bytes())
	if err != nil {
		return fmt.Errorf("problem issuing transaction: %w", err)
	}

	reply.TxID = txID
	return nil
}

// ExportAVAXArgs are arguments for passing into ExportAVA requests
type ExportAVAXArgs struct {
	// User, password, from addrs, change addr
	api.JSONSpendHeader
	// Amount of nAVAX to send
	Amount json.Uint64 `json:"amount"`

	// ID of the address that will receive the AVAX. This address includes the
	// chainID, which is used to determine what the destination chain is.
	To string `json:"to"`
}

// ExportAVAX sends AVAX from this chain to the address specified by [to].
// [to] specifies which chain to export funds to.
// After this tx is accepted, the AVAX must be imported to the P/C chain with an importTx.
// Returns the ID of the newly created atomic transaction
func (service *Service) ExportAVAX(_ *http.Request, args *ExportAVAXArgs, reply *api.JSONTxIDChangeAddr) error {
	return service.Export(nil, &ExportArgs{
		ExportAVAXArgs: *args,
		AssetID:        service.vm.ctx.AVAXAssetID.String(),
	}, reply)
}

// ExportArgs are arguments for passing into ExportAVA requests
type ExportArgs struct {
	ExportAVAXArgs
	AssetID string `json:"assetID"`
}

// Export sends an asset from this chain to the P/C-Chain.
// After this tx is accepted, the AVAX must be imported to the P/C-chain with an importTx.
// Returns the ID of the newly created atomic transaction
func (service *Service) Export(_ *http.Request, args *ExportArgs, reply *api.JSONTxIDChangeAddr) error {
	service.vm.ctx.Log.Info("AVM: Export called with username: %s", args.Username)

	// Parse the asset ID
	assetID, err := service.vm.lookupAssetID(args.AssetID)
	if err != nil {
		return err
	}

	chainID, to, err := service.vm.ParseAddress(args.To)
	if err != nil {
		return err
	}

	if args.Amount == 0 {
		return errZeroAmount
	}

	// Parse the from addresses
	fromAddrs := ids.ShortSet{}
	for _, addrStr := range args.From {
		addr, err := service.vm.ParseLocalAddress(addrStr)
		if err != nil {
			return fmt.Errorf("couldn't parse 'from' address %s: %w", addrStr, err)
		}
		fromAddrs.Add(addr)
	}

	// Get the UTXOs/keys for the from addresses
	utxos, kc, err := service.vm.LoadUser(args.Username, args.Password, fromAddrs)
	if err != nil {
		return err
	}

	// Parse the change address.
	if len(kc.Keys) == 0 {
		return errNoKeys
	}
	changeAddr, err := service.vm.selectChangeAddr(kc.Keys[0].PublicKey().Address(), args.ChangeAddr)
	if err != nil {
		return err
	}

	amounts := map[ids.ID]uint64{}
	if assetID == service.vm.ctx.AVAXAssetID {
		amountWithFee, err := safemath.Add64(uint64(args.Amount), service.vm.txFee)
		if err != nil {
			return fmt.Errorf("problem calculating required spend amount: %w", err)
		}
		amounts[service.vm.ctx.AVAXAssetID] = amountWithFee
	} else {
		amounts[service.vm.ctx.AVAXAssetID] = service.vm.txFee
		amounts[assetID] = uint64(args.Amount)
	}

	amountsSpent, ins, keys, err := service.vm.Spend(utxos, kc, amounts)
	if err != nil {
		return err
	}

	exportOuts := []*avax.TransferableOutput{{
		Asset: avax.Asset{ID: assetID},
		Out: &secp256k1fx.TransferOutput{
			Amt: uint64(args.Amount),
			OutputOwners: secp256k1fx.OutputOwners{
				Locktime:  0,
				Threshold: 1,
				Addrs:     []ids.ShortID{to},
			},
		},
	}}

	outs := []*avax.TransferableOutput{}
	for assetID, amountSpent := range amountsSpent {
		amountToSend := amounts[assetID]
		if amountSpent > amountToSend {
			outs = append(outs, &avax.TransferableOutput{
				Asset: avax.Asset{ID: assetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: amountSpent - amountToSend,
					OutputOwners: secp256k1fx.OutputOwners{
						Locktime:  0,
						Threshold: 1,
						Addrs:     []ids.ShortID{changeAddr},
					},
				},
			})
		}
	}
	avax.SortTransferableOutputs(outs, service.vm.codec, service.vm.currentCodecVersion)

	tx := Tx{
		Version: service.vm.currentCodecVersion,
		UnsignedTx: &ExportTx{
			BaseTx: BaseTx{BaseTx: avax.BaseTx{
				NetworkID:    service.vm.ctx.NetworkID,
				BlockchainID: service.vm.ctx.ChainID,
				Outs:         outs,
				Ins:          ins,
			}},
			DestinationChain: chainID,
			ExportedOuts:     exportOuts,
		},
	}
	if err := tx.SignSECP256K1Fx(service.vm.codec, keys); err != nil {
		return err
	}

	txID, err := service.vm.IssueTx(tx.Bytes())
	if err != nil {
		return fmt.Errorf("problem issuing transaction: %w", err)
	}

	reply.TxID = txID
	reply.ChangeAddr, err = service.vm.FormatLocalAddress(changeAddr)
	return err
}
