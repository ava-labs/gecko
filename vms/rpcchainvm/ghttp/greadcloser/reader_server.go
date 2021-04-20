// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package greadcloser

import (
	"context"
	"io"

	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/greadcloser/greadcloserproto"
)

var (
	_ greadcloserproto.ReaderServer = &Server{}
)

// Server is a io.ReadCloser that is managed over RPC.
type Server struct{ readCloser io.ReadCloser }

// NewServer returns an io.ReadCloser instance managed remotely
func NewServer(readCloser io.ReadCloser) *Server {
	return &Server{readCloser: readCloser}
}

func (s *Server) Read(ctx context.Context, req *greadcloserproto.ReadRequest) (*greadcloserproto.ReadResponse, error) {
	buf := make([]byte, int(req.Length))
	n, err := s.readCloser.Read(buf)
	resp := &greadcloserproto.ReadResponse{
		Read: buf[:n],
	}
	if err != nil {
		resp.Errored = true
		resp.Error = err.Error()
	}
	return resp, nil
}

func (s *Server) Close(ctx context.Context, req *greadcloserproto.CloseRequest) (*greadcloserproto.CloseResponse, error) {
	return &greadcloserproto.CloseResponse{}, s.readCloser.Close()
}
