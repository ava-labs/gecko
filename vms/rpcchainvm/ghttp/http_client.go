// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ghttp

import (
	"net/http"

	"google.golang.org/grpc"

	"github.com/hashicorp/go-plugin"

	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/ghttpproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/greadcloser"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/greadcloser/greadcloserproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gresponsewriter"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gresponsewriter/gresponsewriterproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/grpcutils"
)

var (
	_ http.Handler = &Client{}
)

// Client is an http.Handler that talks over RPC.
type Client struct {
	client ghttpproto.HTTPClient
	broker *plugin.GRPCBroker
}

// NewClient returns an HTTP handler database instance connected to a remote
// HTTP handler instance
func NewClient(client ghttpproto.HTTPClient, broker *plugin.GRPCBroker) *Client {
	return &Client{
		client: client,
		broker: broker,
	}
}

func (c *Client) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	closer := grpcutils.ServerCloser{}
	defer closer.GracefulStop()

	readerID := c.broker.NextId()
	go c.broker.AcceptAndServe(readerID, func(opts []grpc.ServerOption) *grpc.Server {
		reader := grpc.NewServer(opts...)
		closer.Add(reader)
		greadcloserproto.RegisterReaderServer(reader, greadcloser.NewServer(r.Body))

		return reader
	})
	writerID := c.broker.NextId()
	go c.broker.AcceptAndServe(writerID, func(opts []grpc.ServerOption) *grpc.Server {
		writer := grpc.NewServer(opts...)
		closer.Add(writer)
		gresponsewriterproto.RegisterWriterServer(writer, gresponsewriter.NewServer(w, c.broker))

		return writer
	})

	req := &ghttpproto.HTTPRequest{
		ResponseWriter: &ghttpproto.ResponseWriter{
			Id:     writerID,
			Header: make([]*ghttpproto.Element, 0, len(r.Header)),
		},
		Request: &ghttpproto.Request{
			Method:           r.Method,
			Proto:            r.Proto,
			ProtoMajor:       int32(r.ProtoMajor),
			ProtoMinor:       int32(r.ProtoMinor),
			Header:           make([]*ghttpproto.Element, 0, len(r.Header)),
			Body:             readerID,
			ContentLength:    r.ContentLength,
			TransferEncoding: r.TransferEncoding,
			Host:             r.Host,
			Form:             make([]*ghttpproto.Element, 0, len(r.Form)),
			PostForm:         make([]*ghttpproto.Element, 0, len(r.PostForm)),
			RemoteAddr:       r.RemoteAddr,
			RequestURI:       r.RequestURI,
		},
	}
	for key, values := range w.Header() {
		req.ResponseWriter.Header = append(req.ResponseWriter.Header, &ghttpproto.Element{
			Key:    key,
			Values: values,
		})
	}
	for key, values := range r.Header {
		req.Request.Header = append(req.Request.Header, &ghttpproto.Element{
			Key:    key,
			Values: values,
		})
	}
	for key, values := range r.Form {
		req.Request.Form = append(req.Request.Form, &ghttpproto.Element{
			Key:    key,
			Values: values,
		})
	}
	for key, values := range r.PostForm {
		req.Request.PostForm = append(req.Request.PostForm, &ghttpproto.Element{
			Key:    key,
			Values: values,
		})
	}

	if r.URL != nil {
		req.Request.Url = &ghttpproto.URL{
			Scheme:     r.URL.Scheme,
			Opaque:     r.URL.Opaque,
			Host:       r.URL.Host,
			Path:       r.URL.Path,
			RawPath:    r.URL.RawPath,
			ForceQuery: r.URL.ForceQuery,
			RawQuery:   r.URL.RawQuery,
			Fragment:   r.URL.Fragment,
		}

		if r.URL.User != nil {
			pwd, set := r.URL.User.Password()
			req.Request.Url.User = &ghttpproto.Userinfo{
				Username:    r.URL.User.Username(),
				Password:    pwd,
				PasswordSet: set,
			}
		}
	}

	if r.TLS != nil {
		req.Request.Tls = &ghttpproto.ConnectionState{
			Version:                    uint32(r.TLS.Version),
			HandshakeComplete:          r.TLS.HandshakeComplete,
			DidResume:                  r.TLS.DidResume,
			CipherSuite:                uint32(r.TLS.CipherSuite),
			NegotiatedProtocol:         r.TLS.NegotiatedProtocol,
			NegotiatedProtocolIsMutual: r.TLS.NegotiatedProtocolIsMutual,
			ServerName:                 r.TLS.ServerName,
			PeerCertificates: &ghttpproto.Certificates{
				Cert: make([][]byte, len(r.TLS.PeerCertificates)),
			},
			VerifiedChains:              make([]*ghttpproto.Certificates, len(r.TLS.VerifiedChains)),
			SignedCertificateTimestamps: r.TLS.SignedCertificateTimestamps,
			OcspResponse:                r.TLS.OCSPResponse,
			TlsUnique:                   r.TLS.TLSUnique,
		}
		for i, cert := range r.TLS.PeerCertificates {
			req.Request.Tls.PeerCertificates.Cert[i] = cert.Raw
		}
		for i, chain := range r.TLS.VerifiedChains {
			req.Request.Tls.VerifiedChains[i] = &ghttpproto.Certificates{
				Cert: make([][]byte, len(chain)),
			}
			for j, cert := range chain {
				req.Request.Tls.VerifiedChains[i].Cert[j] = cert.Raw
			}
		}
	}

	// TODO: is there a better way to handle this error?
	_, _ = c.client.Handle(r.Context(), req)
}
