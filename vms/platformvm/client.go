// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"time"

	"github.com/ava-labs/avalanchego/vms/avm/vmargs"

	"github.com/ava-labs/avalanchego/api/apiargs"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/formatting"
	cjson "github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/utils/rpc"
)

// Client ...
type Client struct {
	requester rpc.EndpointRequester
}

// NewClient returns a Client for interacting with the P Chain endpoint
func NewClient(uri string, requestTimeout time.Duration) *Client {
	return &Client{
		requester: rpc.NewEndpointRequester(uri, "/ext/P", "platform", requestTimeout),
	}
}

// GetHeight returns the current block height of the P Chain
func (c *Client) GetHeight() (uint64, error) {
	res := &GetHeightResponse{}
	err := c.requester.SendRequest("getHeight", struct{}{}, res)
	return uint64(res.Height), err
}

// ExportKey returns the private key corresponding to [address] from [user]'s account
func (c *Client) ExportKey(user apiargs.UserPass, address string) (string, error) {
	res := &ExportKeyReply{}
	err := c.requester.SendRequest("exportKey", &ExportKeyArgs{
		UserPass: user,
		Address:  address,
	}, res)
	return res.PrivateKey, err
}

// ImportKey imports the specified [privateKey] to [user]'s keystore
func (c *Client) ImportKey(user apiargs.UserPass, privateKey string) (string, error) {
	res := &apiargs.JSONAddress{}
	err := c.requester.SendRequest("importKey", &ImportKeyArgs{
		UserPass:   user,
		PrivateKey: privateKey,
	}, res)
	return res.Address, err
}

// GetBalance returns the balance of [address] on the P Chain
func (c *Client) GetBalance(address string) (*GetBalanceResponse, error) {
	res := &GetBalanceResponse{}
	err := c.requester.SendRequest("getBalance", &apiargs.JSONAddress{
		Address: address,
	}, res)
	return res, err
}

// CreateAddress creates a new address for [user]
func (c *Client) CreateAddress(user apiargs.UserPass) (string, error) {
	res := &apiargs.JSONAddress{}
	err := c.requester.SendRequest("createAddress", &user, res)
	return res.Address, err
}

// ListAddresses returns an array of platform addresses controlled by [user]
func (c *Client) ListAddresses(user apiargs.UserPass) ([]string, error) {
	res := &apiargs.JSONAddresses{}
	err := c.requester.SendRequest("listAddresses", &user, res)
	return res.Addresses, err
}

// GetUTXOs returns the byte representation of the UTXOs controlled by [addresses]
func (c *Client) GetUTXOs(addresses []string) ([][]byte, vmargs.Index, error) {
	res := &vmargs.GetUTXOsReply{}
	err := c.requester.SendRequest("getUTXOs", &vmargs.GetUTXOsArgs{
		Addresses: addresses,
		Encoding:  formatting.Hex,
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

// GetSubnets returns information about the specified subnets
func (c *Client) GetSubnets(ids []ids.ID) ([]APISubnet, error) {
	res := &GetSubnetsResponse{}
	err := c.requester.SendRequest("getSubnets", &GetSubnetsArgs{
		IDs: ids,
	}, res)
	return res.Subnets, err
}

// GetStakingAssetID returns the assetID of the asset used for staking on
// subnet corresponding to [subnetID]
func (c *Client) GetStakingAssetID(subnetID ids.ID) (ids.ID, error) {
	res := &GetStakingAssetIDResponse{}
	err := c.requester.SendRequest("getStakingAssetID", &GetStakingAssetIDArgs{
		SubnetID: subnetID,
	}, res)
	return res.AssetID, err
}

// GetCurrentValidators returns the list of current validators for subnet with ID [subnetID]
func (c *Client) GetCurrentValidators(subnetID ids.ID) ([]interface{}, error) {
	res := &GetCurrentValidatorsReply{}
	err := c.requester.SendRequest("getCurrentValidators", &GetCurrentValidatorsArgs{
		SubnetID: subnetID,
	}, res)
	return res.Validators, err
}

// GetPendingValidators returns the list of pending validators for subnet with ID [subnetID]
func (c *Client) GetPendingValidators(subnetID ids.ID) ([]interface{}, []interface{}, error) {
	res := &GetPendingValidatorsReply{}
	err := c.requester.SendRequest("getPendingValidators", &GetPendingValidatorsArgs{
		SubnetID: subnetID,
	}, res)
	return res.Validators, res.Delegators, err
}

// GetCurrentSupply returns an upper bound on the supply of AVAX in the system
func (c *Client) GetCurrentSupply() (uint64, error) {
	res := &GetCurrentSupplyReply{}
	err := c.requester.SendRequest("getCurrentSupply", struct{}{}, res)
	return uint64(res.Supply), err
}

// SampleValidators returns the nodeIDs of a sample of [sampleSize] validators from the current validator set for subnet with ID [subnetID]
func (c *Client) SampleValidators(subnetID ids.ID, sampleSize uint16) ([]string, error) {
	res := &SampleValidatorsReply{}
	err := c.requester.SendRequest("sampleValidators", &SampleValidatorsArgs{
		SubnetID: subnetID,
		Size:     cjson.Uint16(sampleSize),
	}, res)
	return res.Validators, err
}

// AddValidator issues a transaction to add a validator to the primary network and returns the txID
func (c *Client) AddValidator(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	rewardAddress,
	nodeID string,
	stakeAmount,
	startTime,
	endTime uint64,
	delegationFeeRate float32,
) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	jsonStakeAmount := cjson.Uint64(stakeAmount)
	err := c.requester.SendRequest("addValidator", &AddValidatorArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass: user,
		},
		APIStaker: APIStaker{
			NodeID:      nodeID,
			StakeAmount: &jsonStakeAmount,
			StartTime:   cjson.Uint64(startTime),
			EndTime:     cjson.Uint64(endTime),
		},
		RewardAddress:     rewardAddress,
		DelegationFeeRate: cjson.Float32(delegationFeeRate),
	}, res)
	return res.TxID, err
}

// AddDelegator issues a transaction to add a delegator to the primary network and returns the txID
func (c *Client) AddDelegator(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	rewardAddress,
	nodeID string,
	stakeAmount,
	startTime,
	endTime uint64,
) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	jsonStakeAmount := cjson.Uint64(stakeAmount)
	err := c.requester.SendRequest("addDelegator", &AddDelegatorArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		}, APIStaker: APIStaker{
			NodeID:      nodeID,
			StakeAmount: &jsonStakeAmount,
			StartTime:   cjson.Uint64(startTime),
			EndTime:     cjson.Uint64(endTime),
		},
		RewardAddress: rewardAddress,
	}, res)
	return res.TxID, err
}

// AddSubnetValidator issues a transaction to add validator [nodeID] to subnet with ID [subnetID] and returns the txID
func (c *Client) AddSubnetValidator(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	subnetID,
	nodeID string,
	stakeAmount,
	startTime,
	endTime uint64,
) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	jsonStakeAmount := cjson.Uint64(stakeAmount)
	err := c.requester.SendRequest("addSubnetValidator", &AddSubnetValidatorArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		APIStaker: APIStaker{
			NodeID:      nodeID,
			StakeAmount: &jsonStakeAmount,
			StartTime:   cjson.Uint64(startTime),
			EndTime:     cjson.Uint64(endTime),
		},
		SubnetID: subnetID,
	}, res)
	return res.TxID, err
}

// CreateSubnet issues a transaction to create [subnet] and returns the txID
func (c *Client) CreateSubnet(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	controlKeys []string,
	threshold uint32,
) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	err := c.requester.SendRequest("createSubnet", &CreateSubnetArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		APISubnet: APISubnet{
			ControlKeys: controlKeys,
			Threshold:   cjson.Uint32(threshold),
		},
	}, res)
	return res.TxID, err
}

// ExportAVAX issues an ExportAVAX transaction and returns the txID
func (c *Client) ExportAVAX(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	to string,
	amount uint64,
) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	err := c.requester.SendRequest("exportAVAX", &ExportAVAXArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		To:     to,
		Amount: cjson.Uint64(amount),
	}, res)
	return res.TxID, err
}

// ImportAVAX issues an ImportAVAX transaction and returns the txID
func (c *Client) ImportAVAX(
	user apiargs.UserPass,
	from []string,
	changeAddr,
	to,
	sourceChain string,
) (ids.ID, error) {
	res := &apiargs.JSONTxID{}
	err := c.requester.SendRequest("importAVAX", &ImportAVAXArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		To:          to,
		SourceChain: sourceChain,
	}, res)
	return res.TxID, err
}

// CreateBlockchain issues a CreateBlockchain transaction and returns the txID
func (c *Client) CreateBlockchain(
	user apiargs.UserPass,
	from []string,
	changeAddr string,
	subnetID ids.ID,
	vmID string,
	fxIDs []string,
	name string,
	genesisData []byte,
) (ids.ID, error) {
	genesisDataStr, err := formatting.Encode(formatting.Hex, genesisData)
	if err != nil {
		return ids.ID{}, err
	}

	res := &apiargs.JSONTxID{}
	err = c.requester.SendRequest("createBlockchain", &CreateBlockchainArgs{
		JSONSpendHeader: apiargs.JSONSpendHeader{
			UserPass:       user,
			JSONFromAddrs:  apiargs.JSONFromAddrs{From: from},
			JSONChangeAddr: apiargs.JSONChangeAddr{ChangeAddr: changeAddr},
		},
		SubnetID:    subnetID,
		VMID:        vmID,
		FxIDs:       fxIDs,
		Name:        name,
		GenesisData: genesisDataStr,
		Encoding:    formatting.Hex,
	}, res)
	return res.TxID, err
}

// GetBlockchainStatus returns the current status of blockchain with ID: [blockchainID]
func (c *Client) GetBlockchainStatus(blockchainID string) (Status, error) {
	res := &GetBlockchainStatusReply{}
	err := c.requester.SendRequest("getBlockchainStatus", &GetBlockchainStatusArgs{
		BlockchainID: blockchainID,
	}, res)
	return res.Status, err
}

// ValidatedBy returns the ID of the Subnet that validates [blockchainID]
func (c *Client) ValidatedBy(blockchainID ids.ID) (ids.ID, error) {
	res := &ValidatedByResponse{}
	err := c.requester.SendRequest("validatedBy", &ValidatedByArgs{
		BlockchainID: blockchainID,
	}, res)
	return res.SubnetID, err
}

// Validates returns the list of blockchains that are validated by the subnet with ID [subnetID]
func (c *Client) Validates(subnetID ids.ID) ([]ids.ID, error) {
	res := &ValidatesResponse{}
	err := c.requester.SendRequest("validates", &ValidatesArgs{
		SubnetID: subnetID,
	}, res)
	return res.BlockchainIDs, err
}

// GetBlockchains returns the list of blockchains on the platform
func (c *Client) GetBlockchains() ([]APIBlockchain, error) {
	res := &GetBlockchainsResponse{}
	err := c.requester.SendRequest("getBlockchains", struct{}{}, res)
	return res.Blockchains, err
}

// IssueTx issues the transaction and returns its transaction ID
func (c *Client) IssueTx(txBytes []byte) (ids.ID, error) {
	txStr, err := formatting.Encode(formatting.Hex, txBytes)
	if err != nil {
		return ids.ID{}, err
	}

	res := &apiargs.JSONTxID{}
	err = c.requester.SendRequest("issueTx", &apiargs.FormattedTx{
		Tx:       txStr,
		Encoding: formatting.Hex,
	}, res)
	return res.TxID, err
}

// GetTx returns the byte representation of the transaction corresponding to [txID]
func (c *Client) GetTx(txID ids.ID) ([]byte, error) {
	res := &apiargs.FormattedTx{}
	err := c.requester.SendRequest("getTx", &apiargs.GetTxArgs{
		TxID:     txID,
		Encoding: formatting.Hex,
	}, res)
	if err != nil {
		return nil, err
	}
	return formatting.Decode(res.Encoding, res.Tx)
}

// GetTxStatus returns the status of the transaction corresponding to [txID]
func (c *Client) GetTxStatus(txID ids.ID, includeReason bool) (*GetTxStatusResponse, error) {
	res := new(GetTxStatusResponse)
	err := c.requester.SendRequest("getTxStatus", &GetTxStatusArgs{
		TxID:          txID,
		IncludeReason: includeReason,
	}, res)
	return res, err
}

// GetStake returns the amount of nAVAX that [addresses] have cumulatively
// staked on the Primary Network.
func (c *Client) GetStake(addrs []string) (uint64, error) {
	res := new(GetStakeReply)
	err := c.requester.SendRequest("getStake", &apiargs.JSONAddresses{
		Addresses: addrs,
	}, res)
	return uint64(res.Stake), err
}

// GetMinStake returns the minimum staking amount in nAVAX for validators
// and delegators respectively
func (c *Client) GetMinStake() (uint64, uint64, error) {
	res := new(GetMinStakeReply)
	err := c.requester.SendRequest("getMinStake", struct{}{}, res)
	return uint64(res.MinValidatorStake), uint64(res.MinDelegatorStake), err
}

// GetTotalStake returns the total amount (in nAVAX) staked on the network
func (c *Client) GetTotalStake() (uint64, error) {
	res := new(GetStakeReply)
	err := c.requester.SendRequest("getTotalStake", struct{}{}, res)
	return uint64(res.Stake), err
}

// GetMaxStakeAmount returns the maximum amount of nAVAX staking to the named
// node during the time period.
func (c *Client) GetMaxStakeAmount(subnetID ids.ID, nodeID string, startTime, endTime uint64) (uint64, error) {
	res := new(GetMaxStakeAmountReply)
	err := c.requester.SendRequest("getMaxStakeAmount", &GetMaxStakeAmountArgs{
		SubnetID:  subnetID,
		NodeID:    nodeID,
		StartTime: cjson.Uint64(startTime),
		EndTime:   cjson.Uint64(endTime),
	}, res)
	return uint64(res.Amount), err
}
