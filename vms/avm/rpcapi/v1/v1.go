// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package v1

import (
	"net/http"

	"github.com/ava-labs/avalanchego/api/apiargs"

	"github.com/ava-labs/avalanchego/vms/avm/internalavm"

	"github.com/ava-labs/avalanchego/vms/avm/vmargs"
)

// Controller defines the apis available to the AVM
type Controller struct {
	service *internalavm.Service
}

// NewController create a new instance of the Controller
func NewController(vm *internalavm.VM) *Controller {
	return &Controller{service: internalavm.NewService(vm)}
}

// GetUTXOs gets all utxos for passed in addresses
func (c *Controller) GetUTXOs(r *http.Request, args *vmargs.GetUTXOsArgs, reply *vmargs.GetUTXOsReply) error {
	return c.service.GetUTXOs(r, args, reply)
}

// IssueTx attempts to issue a transaction into consensus
func (c *Controller) IssueTx(r *http.Request, args *apiargs.FormattedTx, reply *apiargs.JSONTxID) error {
	return c.service.IssueTx(r, args, reply)
}

// GetTxStatus returns the status of the specified transaction
func (c *Controller) GetTxStatus(r *http.Request, args *apiargs.JSONTxID, reply *vmargs.GetTxStatusReply) error {
	return c.service.GetTxStatus(r, args, reply)
}

// GetTx returns the specified transaction
func (c *Controller) GetTx(r *http.Request, args *apiargs.GetTxArgs, reply *apiargs.FormattedTx) error {
	return c.service.GetTx(r, args, reply)
}

// GetAssetDescription creates an empty account with the name passed in
func (c *Controller) GetAssetDescription(r *http.Request, args *vmargs.GetAssetDescriptionArgs, reply *vmargs.GetAssetDescriptionReply) error {
	return c.service.GetAssetDescription(r, args, reply)
}

// GetBalance returns the amount of an asset that an address at least partially owns
func (c *Controller) GetBalance(r *http.Request, args *vmargs.GetBalanceArgs, reply *vmargs.GetBalanceReply) error {
	return c.service.GetBalance(r, args, reply)
}

// GetAllBalances returns a map where:
//   Key: ID of an asset such that [args.Address] has a non-zero balance of the asset
//   Value: The balance of the asset held by the address
// Note that balances include assets that the address only _partially_ owns
// (ie is one of several addresses specified in a multi-sig)
func (c *Controller) GetAllBalances(r *http.Request, args *apiargs.JSONAddress, reply *vmargs.GetAllBalancesReply) error {
	return c.service.GetAllBalances(r, args, reply)
}

// CreateFixedCapAsset returns ID of the newly created asset
func (c *Controller) CreateFixedCapAsset(r *http.Request, args *vmargs.CreateAssetArgs, reply *vmargs.AssetIDChangeAddr) error {
	return c.service.CreateFixedCapAsset(r, args, reply)
}

// CreateVariableCapAsset returns ID of the newly created asset
func (c *Controller) CreateVariableCapAsset(r *http.Request, args *vmargs.CreateAssetArgs, reply *vmargs.AssetIDChangeAddr) error {
	return c.service.CreateVariableCapAsset(r, args, reply)
}

// CreateNFTAsset returns ID of the newly created asset
func (c *Controller) CreateNFTAsset(r *http.Request, args *vmargs.CreateNFTAssetArgs, reply *vmargs.AssetIDChangeAddr) error {
	return c.service.CreateNFTAsset(r, args, reply)
}

// CreateAddress creates an address for the user [args.Username]
func (c *Controller) CreateAddress(r *http.Request, args *apiargs.UserPass, reply *apiargs.JSONAddress) error {
	return c.service.CreateAddress(r, args, reply)
}

// ListAddresses returns all of the addresses controlled by user [args.Username]
func (c *Controller) ListAddresses(r *http.Request, args *apiargs.UserPass, reply *apiargs.JSONAddresses) error {
	return c.service.ListAddresses(r, args, reply)
}

// ExportKey returns a private key from the provided user
func (c *Controller) ExportKey(r *http.Request, args *vmargs.ExportKeyArgs, reply *vmargs.ExportKeyReply) error {
	return c.service.ExportKey(r, args, reply)
}

// ImportKey adds a private key to the provided user
func (c *Controller) ImportKey(r *http.Request, args *vmargs.ImportKeyArgs, reply *apiargs.JSONAddress) error {
	return c.service.ImportKey(r, args, reply)
}

// Send returns the ID of the newly created transaction
func (c *Controller) Send(r *http.Request, args *vmargs.SendArgs, reply *apiargs.JSONTxIDChangeAddr) error {
	return c.service.Send(r, args, reply)
}

// SendMultiple sends a transaction with multiple outputs.
func (c *Controller) SendMultiple(r *http.Request, args *vmargs.SendMultipleArgs, reply *apiargs.JSONTxIDChangeAddr) error {
	return c.service.SendMultiple(r, args, reply)
}

// Mint issues a transaction that mints more of the asset
func (c *Controller) Mint(r *http.Request, args *vmargs.MintArgs, reply *apiargs.JSONTxIDChangeAddr) error {
	return c.service.Mint(r, args, reply)
}

// SendNFT sends an NFT
func (c *Controller) SendNFT(r *http.Request, args *vmargs.SendNFTArgs, reply *apiargs.JSONTxIDChangeAddr) error {
	return c.service.SendNFT(r, args, reply)
}

// MintNFT issues a MintNFT transaction and returns the ID of the newly created transaction
func (c *Controller) MintNFT(r *http.Request, args *vmargs.MintNFTArgs, reply *apiargs.JSONTxIDChangeAddr) error {
	return c.service.MintNFT(r, args, reply)
}

// ImportAVAX is a deprecated name for Import.
func (c *Controller) ImportAVAX(r *http.Request, args *vmargs.ImportArgs, reply *apiargs.JSONTxID) error {
	return c.service.ImportAVAX(r, args, reply)
}

// Import imports an asset to this chain from the P/C-Chain.
// The AVAX must have already been exported from the P/C-Chain.
// Returns the ID of the newly created atomic transaction
func (c *Controller) Import(r *http.Request, args *vmargs.ImportArgs, reply *apiargs.JSONTxID) error {
	return c.service.Import(r, args, reply)
}

// ExportAVAX sends AVAX from this chain to the P-Chain.
// After this tx is accepted, the AVAX must be imported to the P-chain with an importTx.
// Returns the ID of the newly created atomic transaction
func (c *Controller) ExportAVAX(r *http.Request, args *vmargs.ExportAVAXArgs, reply *apiargs.JSONTxIDChangeAddr) error {
	return c.service.ExportAVAX(r, args, reply)
}

// Export sends an asset from this chain to the P/C-Chain.
// After this tx is accepted, the AVAX must be imported to the P/C-chain with an importTx.
// Returns the ID of the newly created atomic transaction
func (c *Controller) Export(r *http.Request, args *vmargs.ExportArgs, reply *apiargs.JSONTxIDChangeAddr) error {
	return c.service.Export(r, args, reply)
}
