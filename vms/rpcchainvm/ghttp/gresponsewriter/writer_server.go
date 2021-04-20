// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gresponsewriter

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/grpc"

	"github.com/hashicorp/go-plugin"

	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gconn"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gconn/gconnproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/greader"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/greader/greaderproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gresponsewriter/gresponsewriterproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gwriter"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gwriter/gwriterproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/grpcutils"
)

var (
	_ gresponsewriterproto.WriterServer = &Server{}
)

// Server is an http.ResponseWriter that is managed over RPC.
type Server struct {
	writer http.ResponseWriter
	broker *plugin.GRPCBroker
}

// NewServer returns an http.ResponseWriter instance managed remotely
func NewServer(writer http.ResponseWriter, broker *plugin.GRPCBroker) *Server {
	return &Server{
		writer: writer,
		broker: broker,
	}
}

func (s *Server) Write(ctx context.Context, req *gresponsewriterproto.WriteRequest) (*gresponsewriterproto.WriteResponse, error) {
	headers := s.writer.Header()
	for key := range headers {
		delete(headers, key)
	}
	for _, header := range req.Headers {
		headers[header.Key] = header.Values
	}

	n, err := s.writer.Write(req.Payload)
	if err != nil {
		return nil, err
	}
	return &gresponsewriterproto.WriteResponse{
		Written: int32(n),
	}, nil
}

func (s *Server) WriteHeader(ctx context.Context, req *gresponsewriterproto.WriteHeaderRequest) (*gresponsewriterproto.WriteHeaderResponse, error) {
	headers := s.writer.Header()
	for key := range headers {
		delete(headers, key)
	}
	for _, header := range req.Headers {
		headers[header.Key] = header.Values
	}
	s.writer.WriteHeader(int(req.StatusCode))
	return &gresponsewriterproto.WriteHeaderResponse{}, nil
}

func (s *Server) Flush(ctx context.Context, req *gresponsewriterproto.FlushRequest) (*gresponsewriterproto.FlushResponse, error) {
	flusher, ok := s.writer.(http.Flusher)
	if !ok {
		return nil, errors.New("response writer doesn't support flushing")
	}
	flusher.Flush()
	return &gresponsewriterproto.FlushResponse{}, nil
}

func (s *Server) Hijack(ctx context.Context, req *gresponsewriterproto.HijackRequest) (*gresponsewriterproto.HijackResponse, error) {
	hijacker, ok := s.writer.(http.Hijacker)
	if !ok {
		return nil, errors.New("response writer doesn't support hijacking")
	}
	conn, readWriter, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}

	connID := s.broker.NextId()
	readerID := s.broker.NextId()
	writerID := s.broker.NextId()
	closer := grpcutils.ServerCloser{}

	go s.broker.AcceptAndServe(connID, func(opts []grpc.ServerOption) *grpc.Server {
		server := grpc.NewServer(opts...)
		closer.Add(server)
		gconnproto.RegisterConnServer(server, gconn.NewServer(conn, &closer))
		return server
	})
	go s.broker.AcceptAndServe(readerID, func(opts []grpc.ServerOption) *grpc.Server {
		server := grpc.NewServer(opts...)
		closer.Add(server)
		greaderproto.RegisterReaderServer(server, greader.NewServer(readWriter))
		return server
	})
	go s.broker.AcceptAndServe(writerID, func(opts []grpc.ServerOption) *grpc.Server {
		server := grpc.NewServer(opts...)
		closer.Add(server)
		gwriterproto.RegisterWriterServer(server, gwriter.NewServer(readWriter))
		return server
	})

	local := conn.LocalAddr()
	remote := conn.RemoteAddr()

	return &gresponsewriterproto.HijackResponse{
		ConnServer:    connID,
		LocalNetwork:  local.Network(),
		LocalString:   local.String(),
		RemoteNetwork: remote.Network(),
		RemoteString:  remote.String(),
		ReaderServer:  readerID,
		WriterServer:  writerID,
	}, nil
}
