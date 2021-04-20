// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package rpc

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	rpc "github.com/gorilla/rpc/v2/json2"
)

// Requester ...
type Requester interface {
	SendJSONRPCRequest(endpoint string, method string, params interface{}, reply interface{}) error
}

type jsonRPCRequester struct {
	uri    string
	client http.Client
}

// NewRPCRequester ...
func NewRPCRequester(uri string, requestTimeout time.Duration) Requester {
	return &jsonRPCRequester{
		uri: uri,
		client: http.Client{
			Timeout: requestTimeout,
		},
	}
}

// SendJSONRPCRequest ...
func (requester jsonRPCRequester) SendJSONRPCRequest(endpoint string, method string, params interface{}, reply interface{}) error {
	// Golang has a nasty & subtle behaviour where duplicated '//' in the URL is treated as GET, even if it's POST
	// https://stackoverflow.com/questions/23463601/why-golang-treats-my-post-request-as-a-get-one
	endpoint = strings.TrimLeft(endpoint, "/")

	requestBodyBytes, err := rpc.EncodeClientRequest(method, params)
	if err != nil {
		return fmt.Errorf("problem marshaling request to endpoint '%v' with method '%v' and params '%v': %w", endpoint, method, params, err)
	}

	url := fmt.Sprintf("%v/%v", requester.uri, endpoint)
	resp, err := requester.client.Post(url, "application/json", bytes.NewBuffer(requestBodyBytes))
	if err != nil {
		return fmt.Errorf("problem while making JSON RPC POST request to %s: %s", url, err)
	}
	statusCode := resp.StatusCode

	// Return an error for any non successful status code
	if statusCode < 200 || statusCode > 299 {
		// Drop any error during close to report the original error
		_ = resp.Body.Close()
		return fmt.Errorf("received status code '%v'", statusCode)
	}

	if err := rpc.DecodeClientResponse(resp.Body, reply); err != nil {
		return err
	}
	return resp.Body.Close()
}

// EndpointRequester ...
type EndpointRequester interface {
	SendRequest(method string, params interface{}, reply interface{}) error
}

type avalancheEndpointRequester struct {
	requester      Requester
	endpoint, base string
}

// NewEndpointRequester ...
func NewEndpointRequester(uri, endpoint, base string, requestTimeout time.Duration) EndpointRequester {
	return &avalancheEndpointRequester{
		requester: NewRPCRequester(uri, requestTimeout),
		endpoint:  endpoint,
		base:      base,
	}
}

func (e *avalancheEndpointRequester) SendRequest(method string, params interface{}, reply interface{}) error {
	return e.requester.SendJSONRPCRequest(
		e.endpoint,
		fmt.Sprintf("%s.%s", e.base, method),
		params,
		reply,
	)
}
