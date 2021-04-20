// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gconn

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gconn/gconnproto"
)

var (
	_ net.Conn = &Client{}
)

// Client is an implementation of a connection that talks over RPC.
type Client struct {
	client  gconnproto.ConnClient
	local   net.Addr
	remote  net.Addr
	toClose []io.Closer
}

// NewClient returns a connection connected to a remote connection
func NewClient(client gconnproto.ConnClient, local, remote net.Addr, toClose ...io.Closer) *Client {
	return &Client{
		client:  client,
		local:   local,
		remote:  remote,
		toClose: toClose,
	}
}

func (c *Client) Read(p []byte) (int, error) {
	resp, err := c.client.Read(context.Background(), &gconnproto.ReadRequest{
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

func (c *Client) Write(b []byte) (int, error) {
	resp, err := c.client.Write(context.Background(), &gconnproto.WriteRequest{
		Payload: b,
	})
	if err != nil {
		return 0, err
	}

	if resp.Errored {
		err = errors.New(resp.Error)
	}
	return int(resp.Length), err
}

func (c *Client) Close() error {
	_, err := c.client.Close(context.Background(), &gconnproto.CloseRequest{})
	errs := wrappers.Errs{}
	errs.Add(err)
	for _, toClose := range c.toClose {
		errs.Add(toClose.Close())
	}
	return errs.Err
}

func (c *Client) LocalAddr() net.Addr  { return c.local }
func (c *Client) RemoteAddr() net.Addr { return c.remote }

func (c *Client) SetDeadline(t time.Time) error {
	bytes, err := t.MarshalBinary()
	if err != nil {
		return err
	}
	_, err = c.client.SetDeadline(context.Background(), &gconnproto.SetDeadlineRequest{
		Time: bytes,
	})
	return err
}

func (c *Client) SetReadDeadline(t time.Time) error {
	bytes, err := t.MarshalBinary()
	if err != nil {
		return err
	}
	_, err = c.client.SetReadDeadline(context.Background(), &gconnproto.SetReadDeadlineRequest{
		Time: bytes,
	})
	return err
}

func (c *Client) SetWriteDeadline(t time.Time) error {
	bytes, err := t.MarshalBinary()
	if err != nil {
		return err
	}
	_, err = c.client.SetWriteDeadline(context.Background(), &gconnproto.SetWriteDeadlineRequest{
		Time: bytes,
	})
	return err
}
