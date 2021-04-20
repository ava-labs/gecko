// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gconn

import (
	"context"
	"net"
	"time"

	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gconn/gconnproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/grpcutils"
)

var (
	_ gconnproto.ConnServer = &Server{}
)

// Server is an http.Conn that is managed over RPC.
type Server struct {
	conn   net.Conn
	closer *grpcutils.ServerCloser
}

// NewServer returns an http.Conn managed remotely
func NewServer(conn net.Conn, closer *grpcutils.ServerCloser) *Server {
	return &Server{
		conn:   conn,
		closer: closer,
	}
}

func (s *Server) Read(ctx context.Context, req *gconnproto.ReadRequest) (*gconnproto.ReadResponse, error) {
	buf := make([]byte, int(req.Length))
	n, err := s.conn.Read(buf)
	resp := &gconnproto.ReadResponse{
		Read: buf[:n],
	}
	if err != nil {
		resp.Errored = true
		resp.Error = err.Error()
	}
	return resp, nil
}

func (s *Server) Write(ctx context.Context, req *gconnproto.WriteRequest) (*gconnproto.WriteResponse, error) {
	n, err := s.conn.Write(req.Payload)
	if err != nil {
		return nil, err
	}
	return &gconnproto.WriteResponse{
		Length: int32(n),
	}, nil
}

func (s *Server) Close(ctx context.Context, req *gconnproto.CloseRequest) (*gconnproto.CloseResponse, error) {
	err := s.conn.Close()
	s.closer.Stop()
	return &gconnproto.CloseResponse{}, err
}

func (s *Server) SetDeadline(ctx context.Context, req *gconnproto.SetDeadlineRequest) (*gconnproto.SetDeadlineResponse, error) {
	deadline := time.Time{}
	err := deadline.UnmarshalBinary(req.Time)
	if err != nil {
		return nil, err
	}
	return &gconnproto.SetDeadlineResponse{}, s.conn.SetDeadline(deadline)
}

func (s *Server) SetReadDeadline(ctx context.Context, req *gconnproto.SetReadDeadlineRequest) (*gconnproto.SetReadDeadlineResponse, error) {
	deadline := time.Time{}
	err := deadline.UnmarshalBinary(req.Time)
	if err != nil {
		return nil, err
	}
	return &gconnproto.SetReadDeadlineResponse{}, s.conn.SetReadDeadline(deadline)
}

func (s *Server) SetWriteDeadline(ctx context.Context, req *gconnproto.SetWriteDeadlineRequest) (*gconnproto.SetWriteDeadlineResponse, error) {
	deadline := time.Time{}
	err := deadline.UnmarshalBinary(req.Time)
	if err != nil {
		return nil, err
	}
	return &gconnproto.SetWriteDeadlineResponse{}, s.conn.SetWriteDeadline(deadline)
}
