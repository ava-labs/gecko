// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package keystore

import (
	"time"

	"github.com/ava-labs/avalanchego/api/apiargs"

	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/rpc"
)

// Client ...
type Client struct {
	requester rpc.EndpointRequester
}

// NewClient ...
func NewClient(uri string, requestTimeout time.Duration) *Client {
	return &Client{
		requester: rpc.NewEndpointRequester(uri, "/ext/keystore", "keystore", requestTimeout),
	}
}

// CreateUser ...
func (c *Client) CreateUser(user apiargs.UserPass) (bool, error) {
	res := &apiargs.SuccessResponse{}
	err := c.requester.SendRequest("createUser", &user, res)
	return res.Success, err
}

// ListUsers lists the usernames of all keystore users on the node
func (c *Client) ListUsers() ([]string, error) {
	res := &ListUsersReply{}
	err := c.requester.SendRequest("listUsers", struct{}{}, res)
	return res.Users, err
}

// ExportUser returns the byte representation of the requested [user]
func (c *Client) ExportUser(user apiargs.UserPass) ([]byte, error) {
	res := &ExportUserReply{
		Encoding: formatting.Hex,
	}
	err := c.requester.SendRequest("exportUser", &user, res)
	if err != nil {
		return nil, err
	}
	return formatting.Decode(res.Encoding, res.User)
}

// ImportUser imports the keystore user in [account] under [user]
func (c *Client) ImportUser(user apiargs.UserPass, account []byte) (bool, error) {
	accountStr, err := formatting.Encode(formatting.Hex, account)
	if err != nil {
		return false, err
	}

	res := &apiargs.SuccessResponse{}
	err = c.requester.SendRequest("importUser", &ImportUserArgs{
		UserPass: user,
		User:     accountStr,
		Encoding: formatting.Hex,
	}, res)
	return res.Success, err
}

// DeleteUser removes [user] from the node's keystore users
func (c *Client) DeleteUser(user apiargs.UserPass) (bool, error) {
	res := &apiargs.SuccessResponse{}
	err := c.requester.SendRequest("deleteUser", &user, res)
	return res.Success, err
}
