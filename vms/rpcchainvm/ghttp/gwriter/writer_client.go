// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gwriter

import (
	"context"
	"errors"
	"io"

	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gwriter/gwriterproto"
)

var (
	_ io.Writer = &Client{}
)

// Client is an io.Writer that talks over RPC.
type Client struct{ client gwriterproto.WriterClient }

// NewClient returns a writer connected to a remote writer
func NewClient(client gwriterproto.WriterClient) *Client {
	return &Client{client: client}
}

func (c *Client) Write(p []byte) (int, error) {
	resp, err := c.client.Write(context.Background(), &gwriterproto.WriteRequest{
		Payload: p,
	})
	if err != nil {
		return 0, err
	}

	if resp.Errored {
		err = errors.New(resp.Error)
	}
	return int(resp.Written), err
}
