// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/api"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/rpc"

	cjson "github.com/ava-labs/avalanchego/utils/json"
)

// Client ...
type Client struct {
	requester rpc.EndpointRequester
}

// NewClient returns an AVM client for interacting with avm [chain]
func NewClient(uri, chain string, requestTimeout time.Duration) *Client {
	return &Client{
		requester: rpc.NewEndpointRequester(uri, fmt.Sprintf("/ext/bc/%s", chain), "avm", requestTimeout),
	}
}

// IssueTx issues a transaction to a node and returns the TxID
func (c *Client) IssueTx(txBytes []byte) (ids.ID, error) {
	txStr, err := formatting.Encode(formatting.Hex, txBytes)
	if err != nil {
		return ids.ID{}, err
	}
	res := &api.JSONTxID{}
	err = c.requester.SendRequest("issueTx", &api.FormattedTx{
		Tx:       txStr,
		Encoding: formatting.Hex,
	}, res)
	return res.TxID, err
}

// GetTxStatus returns the status of [txID] and if accepted, the [epoch]
// that it was accepted in. If the status is not accepted, then the value
// of the returned [epoch] is not defined.
func (c *Client) GetTxStatus(txID ids.ID) (choices.Status, uint32, error) {
	res := &GetTxStatusReply{}
	err := c.requester.SendRequest("getTxStatus", &api.JSONTxID{
		TxID: txID,
	}, res)
	var epoch uint32
	if res.Epoch != nil {
		epoch = *res.Epoch
	}
	return res.Status, epoch, err
}

// ConfirmTx attempts to confirm [txID] by checking its status [attempts] times
// with a [delay] in between each attempt. If the transaction has not been
// decided by the final attempt, it returns the status of the last attempt.
// If the status is not accepted, the returned epoch is undefined.
// Note: ConfirmTx will block until either the last attempt finishes or the
// client returns a decided status.
func (c *Client) ConfirmTx(txID ids.ID, attempts int, delay time.Duration) (choices.Status, uint32, error) {
	for i := 0; i < attempts-1; i++ {
		status, epoch, err := c.GetTxStatus(txID)
		if err != nil {
			return status, epoch, err
		}

		if status.Decided() {
			return status, epoch, nil
		}
		time.Sleep(delay)
	}

	return c.GetTxStatus(txID)
}

// GetTx returns the byte representation of [txID]
func (c *Client) GetTx(txID ids.ID) ([]byte, error) {
	res := &api.FormattedTx{}
	err := c.requester.SendRequest("getTx", &api.GetTxArgs{
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
func (c *Client) GetUTXOs(addrs []string, limit uint32, startAddress, startUTXOID string) ([][]byte, api.Index, error) {
	return c.GetAtomicUTXOs(addrs, "", limit, startAddress, startUTXOID)
}

// GetAtomicUTXOs returns the byte representation of the atomic UTXOs controlled by [addresses]
// from [sourceChain]
func (c *Client) GetAtomicUTXOs(addrs []string, sourceChain string, limit uint32, startAddress, startUTXOID string) ([][]byte, api.Index, error) {
	res := &api.GetUTXOsReply{}
	err := c.requester.SendRequest("getUTXOs", &api.GetUTXOsArgs{
		Addresses:   addrs,
		SourceChain: sourceChain,
		Limit:       cjson.Uint32(limit),
		StartIndex: api.Index{
			Address: startAddress,
			UTXO:    startUTXOID,
		},
		Encoding: formatting.Hex,
	}, res)
	if err != nil {
		return nil, api.Index{}, err
	}

	utxos := make([][]byte, len(res.UTXOs))
	for i, utxo := range res.UTXOs {
		utxoBytes, err := formatting.Decode(res.Encoding, utxo)
		if err != nil {
			return nil, api.Index{}, err
		}
		utxos[i] = utxoBytes
	}
	return utxos, res.EndIndex, nil
}

// GetAssetDescription returns a description of [assetID]
func (c *Client) GetAssetDescription(assetID string) (*GetAssetDescriptionReply, error) {
	res := &GetAssetDescriptionReply{}
	err := c.requester.SendRequest("getAssetDescription", &GetAssetDescriptionArgs{
		AssetID: assetID,
	}, res)
	return res, err
}

// GetBalance returns the balance for [addr] of [assetID]
func (c *Client) GetBalance(addr string, assetID string) (*GetBalanceReply, error) {
	res := &GetBalanceReply{}
	err := c.requester.SendRequest("getBalance", &GetBalanceArgs{
		Address: addr,
		AssetID: assetID,
	}, res)
	return res, err
}

// GetAllBalances returns all asset balances for [addr]
func (c *Client) GetAllBalances(addr string) (*GetAllBalancesReply, error) {
	res := &GetAllBalancesReply{}
	err := c.requester.SendRequest("getAllBalances", &api.JSONAddress{
		Address: addr,
	}, res)
	return res, err
}

// CreateAsset creates a new asset and returns its assetID
func (c *Client) CreateAsset(
	user api.UserPass,
	from []string,
	changeAddr,
	name,
	symbol string,
	denomination byte,
	holders []*Holder,
	minters []Minters,
	manager Manager,
) (ids.ID, error) {
	res := &FormattedAssetID{}
	err := c.requester.SendRequest("createAsset", &CreateAssetArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  api.JSONFromAddrs{From: from},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		Name:           name,
		Symbol:         symbol,
		Denomination:   denomination,
		InitialHolders: holders,
		MinterSets:     minters,
		Manager:        manager,
	}, res)
	return res.AssetID, err
}

// CreateFixedCapAsset creates a new fixed cap asset and returns its assetID
func (c *Client) CreateFixedCapAsset(
	user api.UserPass,
	from []string,
	changeAddr,
	name,
	symbol string,
	denomination byte,
	holders []*Holder,
) (ids.ID, error) {
	res := &FormattedAssetID{}
	err := c.requester.SendRequest("createAsset", &CreateAssetArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  api.JSONFromAddrs{From: from},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
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
	user api.UserPass,
	from []string,
	changeAddr,
	name,
	symbol string,
	denomination byte,
	minters []Minters,
) (ids.ID, error) {
	res := &FormattedAssetID{}
	err := c.requester.SendRequest("createAsset", &CreateAssetArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  api.JSONFromAddrs{From: from},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
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
	user api.UserPass,
	from []string,
	changeAddr,
	name,
	symbol string,
	minters []Minters,
) (ids.ID, error) {
	res := &FormattedAssetID{}
	err := c.requester.SendRequest("createNFTAsset", &CreateNFTAssetArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  api.JSONFromAddrs{From: from},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		Name:       name,
		Symbol:     symbol,
		MinterSets: minters,
	}, res)
	return res.AssetID, err
}

// CreateAddress creates a new address controlled by [user]
func (c *Client) CreateAddress(user api.UserPass) (string, error) {
	res := &api.JSONAddress{}
	err := c.requester.SendRequest("createAddress", &user, res)
	return res.Address, err
}

// ListAddresses returns all addresses on this chain controlled by [user]
func (c *Client) ListAddresses(user api.UserPass) ([]string, error) {
	res := &api.JSONAddresses{}
	err := c.requester.SendRequest("listAddresses", &user, res)
	return res.Addresses, err
}

// ExportKey returns the private key corresponding to [addr] controlled by [user]
func (c *Client) ExportKey(user api.UserPass, addr string) (string, error) {
	res := &ExportKeyReply{}
	err := c.requester.SendRequest("exportKey", &ExportKeyArgs{
		UserPass: user,
		Address:  addr,
	}, res)
	return res.PrivateKey, err
}

// ImportKey imports [privateKey] to [user]
func (c *Client) ImportKey(user api.UserPass, privateKey string) (string, error) {
	res := &api.JSONAddress{}
	err := c.requester.SendRequest("importKey", &ImportKeyArgs{
		UserPass:   user,
		PrivateKey: privateKey,
	}, res)
	return res.Address, err
}

// Send [amount] of [assetID] to address [to]
func (c *Client) Send(
	user api.UserPass,
	from []string,
	changeAddr string,
	amount uint64,
	assetID,
	to,
	memo string,
) (ids.ID, error) {
	res := &api.JSONTxID{}
	err := c.requester.SendRequest("send", &SendArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  api.JSONFromAddrs{From: from},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		SendOutput: SendOutput{
			Amount:  cjson.Uint64(amount),
			AssetID: assetID,
			To:      to,
		},
		Memo: memo,
	}, res)
	return res.TxID, err
}

// SendAsManager sends a managed asset using the credential of the asset manager
// rather than the asset holder. The managed asset is sent from [from]. Managed asset
// change is sent to [changeAddr]. Returns the ID of the tx.
func (c *Client) SendAsManager(
	user api.UserPass,
	from []string,
	changeAddr string,
	amount uint64,
	assetID,
	to,
	memo,
	feeChangeAddr string,
	feeFrom []string,
) (ids.ID, error) {
	res := &SendAsManagerResponse{}
	err := c.requester.SendRequest("sendAsManager", &SendAsManagerArgs{
		SendArgs: SendArgs{
			JSONSpendHeader: api.JSONSpendHeader{
				UserPass:       user,
				JSONFromAddrs:  api.JSONFromAddrs{From: from},
				JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
			},
			SendOutput: SendOutput{
				Amount:  cjson.Uint64(amount),
				AssetID: assetID,
				To:      to,
			},
			Memo: memo,
		},
		FeeFrom:       feeFrom,
		FeeChangeAddr: feeChangeAddr,
	}, res)
	return res.TxID, err
}

func (c *Client) UpdateManagedAsset(
	user api.UserPass,
	from []string,
	changeAddr string,
	assetID string,
	frozen bool,
	manager Manager,
) (ids.ID, error) {
	resp := &api.JSONTxIDChangeAddr{}
	err := c.requester.SendRequest("updateManagedAsset", &UpdateManagedAssetArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  api.JSONFromAddrs{From: from},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		AssetID: assetID,
		Frozen:  frozen,
		Manager: manager,
	}, resp)
	return resp.TxID, err
}

// SendMultiple sends a transaction from [user] funding all [outputs]
func (c *Client) SendMultiple(
	user api.UserPass,
	from []string,
	changeAddr string,
	outputs []SendOutput,
	memo string,
) (ids.ID, error) {
	res := &api.JSONTxID{}
	err := c.requester.SendRequest("sendMultiple", &SendMultipleArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  api.JSONFromAddrs{From: from},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		Outputs: outputs,
		Memo:    memo,
	}, res)
	return res.TxID, err
}

// Mint [amount] of [assetID] to be owned by [to]
func (c *Client) Mint(
	user api.UserPass,
	from []string,
	changeAddr string,
	amount uint64,
	assetID,
	to string,
) (ids.ID, error) {
	res := &api.JSONTxID{}
	err := c.requester.SendRequest("mint", &MintArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  api.JSONFromAddrs{From: from},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		Amount:  cjson.Uint64(amount),
		AssetID: assetID,
		To:      to,
	}, res)
	return res.TxID, err
}

// SendNFT sends an NFT and returns the ID of the newly created transaction
func (c *Client) SendNFT(
	user api.UserPass,
	from []string,
	changeAddr string,
	assetID string,
	groupID uint32,
	to string,
) (ids.ID, error) {
	res := &api.JSONTxID{}
	err := c.requester.SendRequest("sendNFT", &SendNFTArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  api.JSONFromAddrs{From: from},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		AssetID: assetID,
		GroupID: cjson.Uint32(groupID),
		To:      to,
	}, res)
	return res.TxID, err
}

// MintNFT issues a MintNFT transaction and returns the ID of the newly created transaction
func (c *Client) MintNFT(
	user api.UserPass,
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
	res := &api.JSONTxID{}
	err = c.requester.SendRequest("mintNFT", &MintNFTArgs{
		JSONSpendHeader: api.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  api.JSONFromAddrs{From: from},
			JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
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
func (c *Client) ImportAVAX(user api.UserPass, to, sourceChain string) (ids.ID, error) {
	return c.Import(user, to, sourceChain)
}

// Import sends an import transaction to import funds from [sourceChain] and
// returns the ID of the newly created transaction
func (c *Client) Import(user api.UserPass, to, sourceChain string) (ids.ID, error) {
	res := &api.JSONTxID{}
	err := c.requester.SendRequest("import", &ImportArgs{
		UserPass:    user,
		To:          to,
		SourceChain: sourceChain,
	}, res)
	return res.TxID, err
}

// ExportAVAX sends AVAX from this chain to the address specified by [to].
// Returns the ID of the newly created atomic transaction
func (c *Client) ExportAVAX(
	user api.UserPass,
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
	user api.UserPass,
	from []string,
	changeAddr string,
	amount uint64,
	to string,
	assetID string,
) (ids.ID, error) {
	res := &api.JSONTxID{}
	err := c.requester.SendRequest("export", &ExportArgs{
		ExportAVAXArgs: ExportAVAXArgs{
			JSONSpendHeader: api.JSONSpendHeader{
				UserPass:       user,
				JSONFromAddrs:  api.JSONFromAddrs{From: from},
				JSONChangeAddr: api.JSONChangeAddr{ChangeAddr: changeAddr},
			},
			Amount: cjson.Uint64(amount),
			To:     to,
		},
		AssetID: assetID,
	}, res)
	return res.TxID, err
}
