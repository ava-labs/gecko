// (c) 2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package health

import (
	"time"

	"github.com/ava-labs/avalanchego/utils/rpc"
)

// Client for Avalanche Health API Endpoint
type Client struct {
	requester rpc.EndpointRequester
}

// NewClient returns a client to interact with Health API endpoint
func NewClient(uri string, requestTimeout time.Duration) *Client {
	return &Client{
		requester: rpc.NewEndpointRequester(uri, "/ext/health", "health", requestTimeout),
	}
}

// GetLiveness returns a health check on the Avalanche node
func (c *Client) GetLiveness() (*APIHealthReply, error) {
	res := &APIHealthReply{}
	err := c.requester.SendRequest("getLiveness", struct{}{}, res)
	return res, err
}

// Health returns a health check on the Avalanche node
func (c *Client) Health() (*APIHealthReply, error) {
	res := &APIHealthReply{}
	err := c.requester.SendRequest("health", struct{}{}, res)
	return res, err
}

// AwaitHealthy queries the GetLiveness endpoint [checks] times, with a pause of [interval]
// in between checks and returns early if GetLiveness returns healthy
func (c *Client) AwaitHealthy(checks int, interval time.Duration) (bool, error) {
	var err error
	for i := 0; i < checks; i++ {
		var res *APIHealthReply
		res, err = c.GetLiveness()
		if err == nil && res.Healthy {
			return true, nil
		}
		time.Sleep(interval)
	}
	return false, err
}
