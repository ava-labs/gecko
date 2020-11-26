package apiclient

import (
	"github.com/ava-labs/avalanchego/utils/formatting"
	cjson "github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/utils/rpc"
	"github.com/ava-labs/avalanchego/vms/avm/vmargs"
)

// ClientV2 ...
type ClientV2 struct {
	v2requester rpc.EndpointRequester
}

// V2 returns the V2 version of the client
func (c *Client) V2() *ClientV2 {
	return &ClientV2{v2requester: c.v2requester}
}

// GetUTXOs returns the byte representation of the UTXOs controlled by [addrs]
func (c *ClientV2) GetUTXOs(addrs []string, limit uint32, startAddress, startUTXOID string) ([][]byte, vmargs.Index, error) {
	res := &vmargs.GetUTXOsReply{}
	err := c.v2requester.SendRequest("getUTXOs", &vmargs.GetUTXOsArgs{
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
