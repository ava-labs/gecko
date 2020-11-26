// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package admin

import (
	"time"

	"github.com/ava-labs/avalanchego/api/apiargs"

	"github.com/ava-labs/avalanchego/utils/rpc"
)

// Client for the Avalanche Platform Info API Endpoint
type Client struct {
	requester rpc.EndpointRequester
}

// NewClient returns a new Info API Client
func NewClient(uri string, requestTimeout time.Duration) *Client {
	return &Client{
		requester: rpc.NewEndpointRequester(uri, "/ext/admin", "admin", requestTimeout),
	}
}

// StartCPUProfiler ...
func (c *Client) StartCPUProfiler() (bool, error) {
	res := &apiargs.SuccessResponse{}
	err := c.requester.SendRequest("startCPUProfiler", struct{}{}, res)
	return res.Success, err
}

// StopCPUProfiler ...
func (c *Client) StopCPUProfiler() (bool, error) {
	res := &apiargs.SuccessResponse{}
	err := c.requester.SendRequest("stopCPUProfiler", struct{}{}, res)
	return res.Success, err
}

// MemoryProfile ...
func (c *Client) MemoryProfile() (bool, error) {
	res := &apiargs.SuccessResponse{}
	err := c.requester.SendRequest("memoryProfile", struct{}{}, res)
	return res.Success, err
}

// LockProfile ...
func (c *Client) LockProfile() (bool, error) {
	res := &apiargs.SuccessResponse{}
	err := c.requester.SendRequest("memoryProfile", struct{}{}, res)
	return res.Success, err
}

// Alias ...
func (c *Client) Alias(endpoint, alias string) (bool, error) {
	res := &apiargs.SuccessResponse{}
	err := c.requester.SendRequest("alias", &AliasArgs{
		Endpoint: endpoint,
		Alias:    alias,
	}, res)
	return res.Success, err
}

// AliasChain ...
func (c *Client) AliasChain(chain, alias string) (bool, error) {
	res := &apiargs.SuccessResponse{}
	err := c.requester.SendRequest("aliasChain", &AliasChainArgs{
		Chain: chain,
		Alias: alias,
	}, res)
	return res.Success, err
}

// Stacktrace ...
func (c *Client) Stacktrace() (bool, error) {
	res := &apiargs.SuccessResponse{}
	err := c.requester.SendRequest("stacktrace", struct{}{}, res)
	return res.Success, err
}
