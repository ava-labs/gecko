// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ghttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/url"

	"github.com/hashicorp/go-plugin"

	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/ghttpproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/greadcloser"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/greadcloser/greadcloserproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gresponsewriter"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/gresponsewriter/gresponsewriterproto"
)

var (
	_ ghttpproto.HTTPServer = &Server{}
)

// Server is an http.Handler that is managed over RPC.
type Server struct {
	handler http.Handler
	broker  *plugin.GRPCBroker
}

// NewServer returns an http.Handler instance managed remotely
func NewServer(handler http.Handler, broker *plugin.GRPCBroker) *Server {
	return &Server{
		handler: handler,
		broker:  broker,
	}
}

func (s *Server) Handle(ctx context.Context, req *ghttpproto.HTTPRequest) (*ghttpproto.HTTPResponse, error) {
	writerConn, err := s.broker.Dial(req.ResponseWriter.Id)
	if err != nil {
		return nil, err
	}

	readerConn, err := s.broker.Dial(req.Request.Body)
	if err != nil {
		// Drop any error that occurs during closing to return the original
		// error.
		_ = writerConn.Close()
		return nil, err
	}

	writerHeaders := make(http.Header)
	for _, elem := range req.ResponseWriter.Header {
		writerHeaders[elem.Key] = elem.Values
	}

	writer := gresponsewriter.NewClient(writerHeaders, gresponsewriterproto.NewWriterClient(writerConn), s.broker)
	reader := greadcloser.NewClient(greadcloserproto.NewReaderClient(readerConn))

	// create the request with the current context
	request, err := http.NewRequestWithContext(
		ctx,
		req.Request.Method,
		req.Request.RequestURI,
		reader,
	)
	if err != nil {
		return nil, err
	}

	if req.Request.Url != nil {
		request.URL = &url.URL{
			Scheme:     req.Request.Url.Scheme,
			Opaque:     req.Request.Url.Opaque,
			Host:       req.Request.Url.Host,
			Path:       req.Request.Url.Path,
			RawPath:    req.Request.Url.RawPath,
			ForceQuery: req.Request.Url.ForceQuery,
			RawQuery:   req.Request.Url.RawQuery,
			Fragment:   req.Request.Url.Fragment,
		}
		if req.Request.Url.User != nil {
			if req.Request.Url.User.PasswordSet {
				request.URL.User = url.UserPassword(req.Request.Url.User.Username, req.Request.Url.User.Password)
			} else {
				request.URL.User = url.User(req.Request.Url.User.Username)
			}
		}
	}

	request.Proto = req.Request.Proto
	request.ProtoMajor = int(req.Request.ProtoMajor)
	request.ProtoMinor = int(req.Request.ProtoMinor)
	request.Header = make(http.Header, len(req.Request.Header))
	for _, elem := range req.Request.Header {
		request.Header[elem.Key] = elem.Values
	}
	request.ContentLength = req.Request.ContentLength
	request.TransferEncoding = req.Request.TransferEncoding
	request.Host = req.Request.Host
	request.Form = make(url.Values, len(req.Request.Form))
	for _, elem := range req.Request.Form {
		request.Form[elem.Key] = elem.Values
	}
	request.PostForm = make(url.Values, len(req.Request.PostForm))
	for _, elem := range req.Request.PostForm {
		request.PostForm[elem.Key] = elem.Values
	}
	request.Trailer = make(http.Header)
	request.RemoteAddr = req.Request.RemoteAddr
	request.RequestURI = req.Request.RequestURI

	if req.Request.Tls != nil {
		request.TLS = &tls.ConnectionState{
			Version:                     uint16(req.Request.Tls.Version),
			HandshakeComplete:           req.Request.Tls.HandshakeComplete,
			DidResume:                   req.Request.Tls.DidResume,
			CipherSuite:                 uint16(req.Request.Tls.CipherSuite),
			NegotiatedProtocol:          req.Request.Tls.NegotiatedProtocol,
			NegotiatedProtocolIsMutual:  req.Request.Tls.NegotiatedProtocolIsMutual,
			ServerName:                  req.Request.Tls.ServerName,
			PeerCertificates:            make([]*x509.Certificate, len(req.Request.Tls.PeerCertificates.Cert)),
			VerifiedChains:              make([][]*x509.Certificate, len(req.Request.Tls.VerifiedChains)),
			SignedCertificateTimestamps: req.Request.Tls.SignedCertificateTimestamps,
			OCSPResponse:                req.Request.Tls.OcspResponse,
			TLSUnique:                   req.Request.Tls.TlsUnique,
		}
		for i, certBytes := range req.Request.Tls.PeerCertificates.Cert {
			cert, err := x509.ParseCertificate(certBytes)
			if err != nil {
				return nil, err
			}
			request.TLS.PeerCertificates[i] = cert
		}
		for i, chain := range req.Request.Tls.VerifiedChains {
			request.TLS.VerifiedChains[i] = make([]*x509.Certificate, len(chain.Cert))
			for j, certBytes := range chain.Cert {
				cert, err := x509.ParseCertificate(certBytes)
				if err != nil {
					return nil, err
				}
				request.TLS.VerifiedChains[i][j] = cert
			}
		}
	}

	s.handler.ServeHTTP(writer, request)

	errs := wrappers.Errs{}
	errs.Add(
		writerConn.Close(),
		readerConn.Close(),
	)
	return &ghttpproto.HTTPResponse{}, errs.Err
}
