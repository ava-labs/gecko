// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package apiclient

import (
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/vms/avm/vmargs"

	"github.com/ava-labs/avalanchego/api/apiargs"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/rpc"

	cjson "github.com/ava-labs/avalanchego/utils/json"
)

// Client ...
type Client struct {
	v1requester rpc.EndpointRequester
	v2requester rpc.EndpointRequester
}

// NewClient returns an AVM client for interacting with avm [chain]
func NewClient(uri, chain string, requestTimeout time.Duration) *Client {
	return &Client{
		v1requester: rpc.NewEndpointRequester(uri, fmt.Sprintf("/ext/bc/%s", chain), "avm", requestTimeout),
		v2requester: rpc.NewEndpointRequester(uri, fmt.Sprintf("/ext/bc/%s/v2", chain), "avm", requestTimeout),
	}
}

// IssueTx issues a transaction to a node and returns the TxID
func (c *Client) IssueTx(txBytes []byte) (ids.ID, error) {
	txStr, err := formatting.Encode(formatting.Hex, txBytes)
	if err != nil {
		return ids.ID{}, err
	}
	res := &apiargs.JSONTxID{}
	err = c.v1requester.SendRequest("issueTx", &apiargs.FormattedTx{
		Tx:       txStr,
		Encoding: formatting.Hex,
	}, res)
	return res.TxID, err
}

// GetTxStatus returns the status of [txID]
func (c *Client) GetTxStatus(txID ids.ID) (choices.Status, error) {
	res := &vmargs.GetTxStatusReply{}
	err := c.v1requester.SendRequest("getTxStatus", &apiargs.JSONTxID{
		TxID: txID,
	}, res)
	return res.Status, err
}

// GetTx returns the byte representation of [txID]
func (c *Client) GetTx(txID ids.ID) ([]byte, error) {
	res := &apiargs.FormattedTx{}
	err := c.v1requester.SendRequest("getTx", &apiargs.GetTxArgs{
		TxID:     txID,
		Encoding: formatting.Hex,
	}, res)
	if err != nil {
		return nil, err
	}

	txBytes, err := formatting.Decode(res.Encoding, res.Tx)
	if err != nil {
		return nil, err
	}
	return txBytes, nil
}

// GetUTXOs returns the byte representation of the UTXOs controlled by [addrs]
func (c *Client) GetUTXOs(addrs []string, limit uint32, startAddress, startUTXOID string) ([][]byte, vmargs.Index, error) {
	res := &vmargs.GetUTXOsReply{}
	err := c.v1requester.SendRequest("getUTXOs", &vmargs.GetUTXOsArgs{
		Addresses: addrs,
		Limit:     cjson.Uint32(limit),
		StartIndex: vmargs.Index{
			Address: startAddress,
			UTXO:    startUTXOID,
		},
		Encoding: formatting.Hex,
	}, res)
	if err != nil {
		return nil, vmargs.Index{}, err
	}

	utxos := make([][]byte, len(res.UTXOs))
	for i, utxo := range res.UTXOs {
		utxoBytes, err := formatting.Decode(res.Encoding, utxo)
		if err != nil {
			return nil, vmargs.Index{}, err
		}
		utxos[i] = utxoBytes
	}
	return utxos, res.EndIndex, nil
}

// GetAssetDescription returns a description of [assetID]
func (c *Client) GetAssetDescription(assetID string) (*vmargs.GetAssetDescriptionReply, error) {
	res := &vmargs.GetAssetDescriptionReply{}
	err := c.v1requester.SendRequest("getAssetDescription", &vmargs.GetAssetDescriptionArgs{
		AssetID: assetID,
	}, res)
	return res, err
}

// GetBalance returns the balance for [addr] of [assetID]
func (c *Client) GetBalance(addr string, assetID string) (*vmargs.GetBalanceReply, error) {
	res := &vmargs.GetBalanceReply{}
	err := c.v1requester.SendRequest("getBalance", &vmargs.GetBalanceArgs{
		Address: addr,
		AssetID: assetID,
	}, res)
	return res, err
}

// GetAllBalances returns all asset balances for [addr]
func (c *Client) GetAllBalances(addr string) (*vmargs.GetAllBalancesReply, error) {
	res := &vmargs.GetAllBalancesReply{}
	err := c.v1requester.SendRequest("getAllBalances", &apiargs.JSONAddress{
		Address: addr,
	}, res)
	return res, err
}

// CreateAsset creates a new asset and returns its assetID
func (c *Client) CreateAsset(
	user apiargs.UserPass,
	from []string,
	changeAddr,
	name,
	symbol string,
	denomination byte,
	holders []*vmargs.Holder,
	minters []vmargs.Owners,
) (ids.ID, error) {
	res := &vmargs.FormattedAssetID{}
	err := c.v1requester.SendRequest("createAsset", &vmargs.CreateAssetArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		Name:           name,
		Symbol:         symbol,
		Denomination:   denomination,
		InitialHolders: holders,
		MinterSets:     minters,
	}, res)
	return res.AssetID, err
}

// CreateFixedCapAsset creates a new fixed cap asset and returns its assetID
func (c *Client) CreateFixedCapAsset(
	user apiargs.UserPass,
	from []string,
	changeAddr,
	name,
	symbol string,
	denomination byte,
	holders []*vmargs.Holder,
) (ids.ID, error) {
	res := &vmargs.FormattedAssetID{}
	err := c.v1requester.SendRequest("createAsset", &vmargs.CreateAssetArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		Name:           name,
		Symbol:         symbol,
		Denomination:   denomination,
		InitialHolders: holders,
	}, res)
	return res.AssetID, err
}

// CreateVariableCapAsset creates a new variable cap asset and returns its assetID
func (c *Client) CreateVariableCapAsset(
	user apiargs.UserPass,
	from []string,
	changeAddr,
	name,
	symbol string,
	denomination byte,
	minters []vmargs.Owners,
) (ids.ID, error) {
	res := &vmargs.FormattedAssetID{}
	err := c.v1requester.SendRequest("createAsset", &vmargs.CreateAssetArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		Name:         name,
		Symbol:       symbol,
		Denomination: denomination,
		MinterSets:   minters,
	}, res)
	return res.AssetID, err
}

// CreateNFTAsset creates a new NFT asset and returns its assetID
func (c *Client) CreateNFTAsset(
	user apiargs.UserPass,
	from []string,
	changeAddr,
	name,
	symbol string,
	minters []vmargs.Owners,
) (ids.ID, error) {
	res := &vmargs.FormattedAssetID{}
	err := c.v1requester.SendRequest("createNFTAsset", &vmargs.CreateNFTAssetArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		Name:       name,
		Symbol:     symbol,
		MinterSets: minters,
	}, res)
	return res.AssetID, err
}

// CreateAddress creates a new address controlled by [user]
func (c *Client) CreateAddress(user apiargs.UserPass) (string, error) {
	res := &apiargs.JSONAddress{}
	err := c.v1requester.SendRequest("createAddress", &user, res)
	return res.Address, err
}

// ListAddresses returns all addresses on this chain controlled by [user]
func (c *Client) ListAddresses(user apiargs.UserPass) ([]string, error) {
	res := &apiargs.JSONAddresses{}
	err := c.v1requester.SendRequest("listAddresses", &user, res)
	return res.Addresses, err
}

// ExportKey returns the private key corresponding to [addr] controlled by [user]
func (c *Client) ExportKey(user apiargs.UserPass, addr string) (string, error) {
	res := &vmargs.ExportKeyReply{}
	err := c.v1requester.SendRequest("exportKey", &vmargs.ExportKeyArgs{
		UserPass: user,
		Address:  addr,
	}, res)
	return res.PrivateKey, err
}

// ImportKey imports [privateKey] to [user]
func (c *Client) ImportKey(user apiargs.UserPass, privateKey string) (string, error) {
	res := &apiargs.JSONAddress{}
	err := c.v1requester.SendRequest("importKey", &vmargs.ImportKeyArgs{
		UserPass:   user,
		PrivateKey: privateKey,
	}, res)
	return res.Address, err
}

// Send [amount] of [assetID] to address [to]
func (c *Client) Send(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	amount uint64,
	assetID,
	to,
	memo string,
) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	err := c.v1requester.SendRequest("send", &vmargs.SendArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		SendOutput: vmargs.SendOutput{
			Amount:  cjson.Uint64(amount),
			AssetID: assetID,
			To:      to,
		},
		Memo: memo,
	}, res)
	return res.TxID, err
}

// SendMultiple sends a transaction from [user] funding all [outputs]
func (c *Client) SendMultiple(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	outputs []vmargs.SendOutput,
	memo string,
) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	err := c.v1requester.SendRequest("sendMultiple", &vmargs.SendMultipleArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		Outputs: outputs,
		Memo:    memo,
	}, res)
	return res.TxID, err
}

// Mint [amount] of [assetID] to be owned by [to]
func (c *Client) Mint(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	amount uint64,
	assetID,
	to string,
) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	err := c.v1requester.SendRequest("mint", &vmargs.MintArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		Amount:  cjson.Uint64(amount),
		AssetID: assetID,
		To:      to,
	}, res)
	return res.TxID, err
}

// SendNFT sends an NFT and returns the ID of the newly created transaction
func (c *Client) SendNFT(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	assetID string,
	groupID uint32,
	to string,
) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	err := c.v1requester.SendRequest("sendNFT", &vmargs.SendNFTArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		AssetID: assetID,
		GroupID: cjson.Uint32(groupID),
		To:      to,
	}, res)
	return res.TxID, err
}

// MintNFT issues a MintNFT transaction and returns the ID of the newly created transaction
func (c *Client) MintNFT(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	assetID string,
	payload []byte,
	to string,
) (ids.ID, error) {
	payloadStr, err := formatting.Encode(formatting.Hex, payload)
	if err != nil {
		return ids.ID{}, err
	}
	res := &apiargs.JSONTxID{}
	err = c.v1requester.SendRequest("mintNFT", &vmargs.MintNFTArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		AssetID:  assetID,
		Payload:  payloadStr,
		To:       to,
		Encoding: formatting.Hex,
	}, res)
	return res.TxID, err
}

// ImportAVAX sends an import transaction to import funds from [sourceChain] and
// returns the ID of the newly created transaction
// This is a deprecated name for Import
func (c *Client) ImportAVAX(user apiargs.UserPass, to, sourceChain string) (ids.ID, error) {
	return c.Import(user, to, sourceChain)
}

// Import sends an import transaction to import funds from [sourceChain] and
// returns the ID of the newly created transaction
func (c *Client) Import(user apiargs.UserPass, to, sourceChain string) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	err := c.v1requester.SendRequest("import", &vmargs.ImportArgs{
		UserPass:    user,
		To:          to,
		SourceChain: sourceChain,
	}, res)
	return res.TxID, err
}

// ExportAVAX sends AVAX from this chain to the address specified by [to].
// Returns the ID of the newly created atomic transaction
func (c *Client) ExportAVAX(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	amount uint64,
	to string,
) (ids.ID, error) {
	return c.Export(user, from, changeAddr, amount, to, "AVAX")
}

// Export sends an asset from this chain to the P/C-Chain.
// After this tx is accepted, the AVAX must be imported to the P/C-chain with an importTx.
// Returns the ID of the newly created atomic transaction
func (c *Client) Export(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	amount uint64,
	to string,
	assetID string,
) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	err := c.v1requester.SendRequest("export", &vmargs.ExportArgs{
		ExportAVAXArgs: vmargs.ExportAVAXArgs{
			JSONSpendHeader: apiargs.JSONSpendHeader{
				UserPass:       user,
				JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
				JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
			},
			Amount: cjson.Uint64(amount),
			To:     to,
		},
		AssetID: assetID,
	}, res)
	return res.TxID, err
}
