// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package greadcloser

import (
	"context"
	"errors"
	"io"

	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/greadcloser/greadcloserproto"
)

var (
	_ io.ReadCloser = &Client{}
)

// Client is a read closer that talks over RPC.
type Client struct{ client greadcloserproto.ReaderClient }

// NewClient returns a read closer connected to a remote read closer
func NewClient(client greadcloserproto.ReaderClient) *Client {
	return &Client{client: client}
}

func (c *Client) Read(p []byte) (int, error) {
	resp, err := c.client.Read(context.Background(), &greadcloserproto.ReadRequest{
		Length: int32(len(p)),
	})
	if err != nil {
		return 0, err
	}

	copy(p, resp.Read)

	if resp.Errored {
		err = errors.New(resp.Error)
	}
	return len(resp.Read), err
}

func (c *Client) Close() error {
	_, err := c.client.Close(context.Background(), &greadcloserproto.CloseRequest{})
	return err
}
