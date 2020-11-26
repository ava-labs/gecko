package vmargs

import (
	"github.com/ava-labs/avalanchego/api/apiargs"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/json"
	cjson "github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/vms/components/avax"
)

// Index is an address and an associated UTXO.
// Marks a starting or stopping point when fetching UTXOs. Used for pagination.
type Index struct {
	Address string `json:"address"` // The address as a string
	UTXO    string `json:"utxo"`    // The UTXO ID as a string
}

// GetUTXOsArgs are arguments for passing into GetUTXOs.
// Gets the UTXOs that reference at least one address in [Addresses].
// Returns at most [limit] addresses.
// If specified, [SourceChain] is the chain where the atomic UTXOs were exported from. If empty,
// or the Chain ID of this VM is specified, then GetUTXOs fetches the native UTXOs.
// If [limit] == 0 or > [maxUTXOsToFetch], fetches up to [maxUTXOsToFetch].
// [StartIndex] defines where to start fetching UTXOs (for pagination.)
// UTXOs fetched are from addresses equal to or greater than [StartIndex.Address]
// For address [StartIndex.Address], only UTXOs with IDs greater than [StartIndex.UTXO] will be returned.
// If [StartIndex] is omitted, gets all UTXOs.
// If GetUTXOs is called multiple times, with our without [StartIndex], it is not guaranteed
// that returned UTXOs are unique. That is, the same UTXO may appear in the response of multiple calls.
type GetUTXOsArgs struct {
	Addresses   []string            `json:"addresses"`
	SourceChain string              `json:"sourceChain"`
	Limit       json.Uint32         `json:"limit"`
	StartIndex  Index               `json:"startIndex"`
	Encoding    formatting.Encoding `json:"encoding"`
}

// GetUTXOsReply defines the GetUTXOs replies returned from the API
type GetUTXOsReply struct {
	// Number of UTXOs returned
	NumFetched json.Uint64 `json:"numFetched"`
	// The UTXOs
	UTXOs []string `json:"utxos"`
	// The last UTXO that was returned, and the address it corresponds to.
	// Used for pagination. To get the rest of the UTXOs, call GetUTXOs
	// again and set [StartIndex] to this value.
	EndIndex Index `json:"endIndex"`
	// Encoding specifies the encoding format the UTXOs are returned in
	Encoding formatting.Encoding `json:"encoding"`
}

// BuildGenesisArgs are arguments for BuildGenesis
type BuildGenesisArgs struct {
	NetworkID   cjson.Uint32               `json:"networkID"`
	GenesisData map[string]AssetDefinition `json:"genesisData"`
	Encoding    formatting.Encoding        `json:"encoding"`
}

// AssetDefinition ...
type AssetDefinition struct {
	Name         string                   `json:"name"`
	Symbol       string                   `json:"symbol"`
	Denomination cjson.Uint8              `json:"denomination"`
	InitialState map[string][]interface{} `json:"initialState"`
	Memo         string                   `json:"memo"`
}

// BuildGenesisReply is the reply from BuildGenesis
type BuildGenesisReply struct {
	Bytes    string              `json:"bytes"`
	Encoding formatting.Encoding `json:"encoding"`
}

// GetTxStatusReply defines the GetTxStatus replies returned from the API
type GetTxStatusReply struct {
	Status choices.Status `json:"status"`
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

// FormattedAssetID defines a JSON formatted struct containing an assetID as a string
type FormattedAssetID struct {
	AssetID ids.ID `json:"assetID"`
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

// CreateFixedCapAssetArgs are arguments for passing into CreateFixedCapAsset requests
type CreateFixedCapAssetArgs struct {
	apiargs.JSONSpendHeader           // User, password, from addrs, change addr
	Name                    string    `json:"name"`
	Symbol                  string    `json:"symbol"`
	Denomination            byte      `json:"denomination"`
	InitialHolders          []*Holder `json:"initialHolders"`
}

// Holder describes how much an address owns of an asset
type Holder struct {
	Amount  json.Uint64 `json:"amount"`
	Address string      `json:"address"`
}

// AssetIDChangeAddr is an asset ID and a change address
type AssetIDChangeAddr struct {
	FormattedAssetID
	apiargs.JSONChangeAddr
}

// CreateVariableCapAssetArgs are arguments for passing into CreateVariableCapAsset requests
type CreateVariableCapAssetArgs struct {
	apiargs.JSONSpendHeader          // User, password, from addrs, change addr
	Name                    string   `json:"name"`
	Symbol                  string   `json:"symbol"`
	Denomination            byte     `json:"denomination"`
	MinterSets              []Owners `json:"minterSets"`
}

// Owners describes who can perform an action
type Owners struct {
	Threshold json.Uint32 `json:"threshold"`
	Minters   []string    `json:"minters"`
}

// CreateNFTAssetArgs are arguments for passing into CreateNFTAsset requests
type CreateNFTAssetArgs struct {
	apiargs.JSONSpendHeader          // User, password, from addrs, change addr
	Name                    string   `json:"name"`
	Symbol                  string   `json:"symbol"`
	MinterSets              []Owners `json:"minterSets"`
}

// ExportKeyArgs are arguments for ExportKey
type ExportKeyArgs struct {
	apiargs.UserPass
	Address string `json:"address"`
}

// ExportKeyReply is the response for ExportKey
type ExportKeyReply struct {
	// The decrypted PrivateKey for the Address provided in the arguments
	PrivateKey string `json:"privateKey"`
}

// ImportKeyArgs are arguments for ImportKey
type ImportKeyArgs struct {
	apiargs.UserPass
	PrivateKey string `json:"privateKey"`
}

// ImportKeyReply is the response for ImportKey
type ImportKeyReply struct {
	// The address controlled by the PrivateKey provided in the arguments
	Address string `json:"address"`
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

// SendMultipleArgs are arguments for passing into SendMultiple requests
type SendMultipleArgs struct {
	// User, password, from addrs, change addr
	apiargs.JSONSpendHeader

	// The outputs of the transaction
	Outputs []SendOutput `json:"outputs"`

	// The addresses to send funds from
	// If empty, will send from any addresses
	// controlled by the given user
	From []string `json:"from"`

	// Memo field
	Memo string `json:"memo"`
}

// MintArgs are arguments for passing into Mint requests
type MintArgs struct {
	apiargs.JSONSpendHeader             // User, password, from addrs, change addr
	Amount                  json.Uint64 `json:"amount"`
	AssetID                 string      `json:"assetID"`
	To                      string      `json:"to"`
}

// SendNFTArgs are arguments for passing into SendNFT requests
type SendNFTArgs struct {
	apiargs.JSONSpendHeader             // User, password, from addrs, change addr
	AssetID                 string      `json:"assetID"`
	GroupID                 json.Uint32 `json:"groupID"`
	To                      string      `json:"to"`
}

// MintNFTArgs are arguments for passing into MintNFT requests
type MintNFTArgs struct {
	apiargs.JSONSpendHeader                     // User, password, from addrs, change addr
	AssetID                 string              `json:"assetID"`
	Payload                 string              `json:"payload"`
	To                      string              `json:"to"`
	Encoding                formatting.Encoding `json:"encoding"`
}

// ImportArgs are arguments for passing into Import requests
type ImportArgs struct {
	// User that controls To
	apiargs.UserPass

	// Chain the funds are coming from
	SourceChain string `json:"sourceChain"`

	// Address receiving the imported AVAX
	To string `json:"to"`
}

// ExportAVAXArgs are arguments for passing into ExportAVA requests
type ExportAVAXArgs struct {
	// User, password, from addrs, change addr
	apiargs.JSONSpendHeader
	// Amount of nAVAX to send
	Amount json.Uint64 `json:"amount"`

	// ID of the address that will receive the AVAX. This address includes the
	// chainID, which is used to determine what the destination chain is.
	To string `json:"to"`
}

// ExportArgs are arguments for passing into ExportAVA requests
type ExportArgs struct {
	ExportAVAXArgs
	AssetID string `json:"assetID"`
}

// CreateAssetArgs are arguments for passing into CreateAsset
type CreateAssetArgs struct {
	apiargs.JSONSpendHeader           // User, password, from addrs, change addr
	Name                    string    `json:"name"`
	Symbol                  string    `json:"symbol"`
	Denomination            byte      `json:"denomination"`
	InitialHolders          []*Holder `json:"initialHolders"`
	MinterSets              []Owners  `json:"minterSets"`
}

// SendArgs are arguments for passing into Send requests
type SendArgs struct {
	// User, password, from addrs, change addr
	apiargs.JSONSpendHeader

	// The amount, assetID, and destination to send funds to
	SendOutput

	// The addresses to send funds from
	// If empty, will send from any addresses
	// controlled by the given user
	From []string `json:"from"`

	// Memo field
	Memo string `json:"memo"`
}
