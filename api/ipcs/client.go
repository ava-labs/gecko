// (c) 2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ipcs

import (
	"time"

	"github.com/ava-labs/avalanchego/api/apiargs"

	"github.com/ava-labs/avalanchego/utils/rpc"
)

// Client ...
type Client struct {
	requester rpc.EndpointRequester
}

// NewClient returns a Client for interacting with the IPCS endpoint
func NewClient(uri string, requestTimeout time.Duration) *Client {
	return &Client{
		requester: rpc.NewEndpointRequester(uri, "/ext/ipcs", "ipcs", requestTimeout),
	}
}

// PublishBlockchain requests the node to begin publishing consensus and decision events
func (c *Client) PublishBlockchain(blockchainID string) (*PublishBlockchainReply, error) {
	res := &PublishBlockchainReply{}
	err := c.requester.SendRequest("publishBlockchain", &PublishBlockchainArgs{
		BlockchainID: blockchainID,
	}, res)
	return res, err
}

// UnpublishBlockchain requests the node to stop publishing consensus and decision events
func (c *Client) UnpublishBlockchain(blockchainID string) (bool, error) {
	res := &apiargs.SuccessResponse{}
	err := c.requester.SendRequest("unpublishBlockchain", &UnpublishBlockchainArgs{
		BlockchainID: blockchainID,
	}, res)
	return res.Success, err
}
