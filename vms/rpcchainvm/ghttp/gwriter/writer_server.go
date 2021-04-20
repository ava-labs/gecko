// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gwriter

import (
	"context"
	"io"

	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gwriter/gwriterproto"
)

var (
	_ gwriterproto.WriterServer = &Server{}
)

// Server is an http.Handler that is managed over RPC.
type Server struct{ writer io.Writer }

// NewServer returns an http.Handler instance managed remotely
func NewServer(writer io.Writer) *Server {
	return &Server{writer: writer}
}

func (s *Server) Write(ctx context.Context, req *gwriterproto.WriteRequest) (*gwriterproto.WriteResponse, error) {
	n, err := s.writer.Write(req.Payload)
	resp := &gwriterproto.WriteResponse{
		Written: int32(n),
	}
	if err != nil {
		resp.Errored = true
		resp.Error = err.Error()
	}
	return resp, nil
}
