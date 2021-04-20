// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package greader

import (
	"context"
	"io"

	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/greader/greaderproto"
)

var (
	_ greaderproto.ReaderServer = &Server{}
)

// Server is an io.Reader that is managed over RPC.
type Server struct{ reader io.Reader }

// NewServer returns an io.Reader instance managed remotely
func NewServer(reader io.Reader) *Server {
	return &Server{reader: reader}
}

func (s *Server) Read(ctx context.Context, req *greaderproto.ReadRequest) (*greaderproto.ReadResponse, error) {
	buf := make([]byte, int(req.Length))
	n, err := s.reader.Read(buf)
	resp := &greaderproto.ReadResponse{
		Read: buf[:n],
	}
	if err != nil {
		resp.Errored = true
		resp.Error = err.Error()
	}
	return resp, nil
}
