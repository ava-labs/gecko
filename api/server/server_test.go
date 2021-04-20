// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"

	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/logging"
)

type Service struct{ called bool }

type Args struct{}

type Reply struct{}

func (s *Service) Call(_ *http.Request, args *Args, reply *Reply) error {
	s.called = true
	return nil
}

func TestCall(t *testing.T) {
	s := Server{}
	s.Initialize(
		logging.NoLog{},
		logging.NoFactory{},
		"localhost",
		8080,
		[]string{"*"},
	)

	serv := &Service{}
	newServer := rpc.NewServer()
	newServer.RegisterCodec(json2.NewCodec(), "application/json")
	newServer.RegisterCodec(json2.NewCodec(), "application/json;charset=UTF-8")
	if err := newServer.RegisterService(serv, "test"); err != nil {
		t.Fatal(err)
	}

	err := s.AddRoute(
		&common.HTTPHandler{Handler: newServer},
		new(sync.RWMutex),
		"vm/lol",
		"",
		logging.NoLog{},
	)
	if err != nil {
		t.Fatal(err)
	}

	buf, err := json2.EncodeClientRequest("test.Call", &Args{})
	if err != nil {
		t.Fatal(err)
	}

	writer := httptest.NewRecorder()
	body := bytes.NewBuffer(buf)
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	err = s.Call(writer, "POST", "lol", "", body, headers)
	if err != nil {
		t.Fatal(err)
	}

	if !serv.called {
		t.Fatalf("Should have been called")
	}
}
