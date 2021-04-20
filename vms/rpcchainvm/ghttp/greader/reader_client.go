// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package greader

import (
	"context"
	"errors"
	"io"

	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/greader/greaderproto"
)

var (
	_ io.Reader = &Client{}
)

// Client is a reader that talks over RPC.
type Client struct{ client greaderproto.ReaderClient }

// NewClient returns a reader connected to a remote reader
func NewClient(client greaderproto.ReaderClient) *Client {
	return &Client{client: client}
}

func (c *Client) Read(p []byte) (int, error) {
	resp, err := c.client.Read(context.Background(), &greaderproto.ReadRequest{
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
