// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package admin

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/api/apiargs"

	"github.com/ava-labs/avalanchego/utils/rpc"
)

// SuccessResponseTest defines the expected result of an API call that returns SuccessResponse
type SuccessResponseTest struct {
	Success bool
	Err     error
}

// GetSuccessResponseTests returns a list of possible SuccessResponseTests
func GetSuccessResponseTests() []SuccessResponseTest {
	return []SuccessResponseTest{
		{
			Success: true,
			Err:     nil,
		},
		{
			Success: false,
			Err:     nil,
		},
		{
			Err: errors.New("Non-nil error"),
		},
	}
}

type mockClient struct {
	response interface{}
	err      error
}

// NewMockClient returns a mock client for testing
func NewMockClient(response interface{}, err error) rpc.EndpointRequester {
	return &mockClient{
		response: response,
		err:      err,
	}
}

func (mc *mockClient) SendRequest(method string, params interface{}, reply interface{}) error {
	if mc.err != nil {
		return mc.err
	}

	switch p := reply.(type) {
	case *apiargs.SuccessResponse:
		response := mc.response.(apiargs.SuccessResponse)
		*p = response
	default:
		panic("illegal type")
	}
	return nil
}

func TestStartCPUProfiler(t *testing.T) {
	tests := GetSuccessResponseTests()

	for _, test := range tests {
		mockClient := Client{requester: NewMockClient(apiargs.SuccessResponse{Success: test.Success}, test.Err)}
		success, err := mockClient.StartCPUProfiler()
		// if there is error as expected, the test passes
		if err != nil && test.Err != nil {
			continue
		}
		if err != nil {
			t.Fatalf("Unexepcted error: %s", err)
		}
		if success != test.Success {
			t.Fatalf("Expected success response to be: %v, but found: %v", test.Success, success)
		}
	}
}

func TestStopCPUProfiler(t *testing.T) {
	tests := GetSuccessResponseTests()

	for _, test := range tests {
		mockClient := Client{requester: NewMockClient(apiargs.SuccessResponse{Success: test.Success}, test.Err)}
		success, err := mockClient.StopCPUProfiler()
		// if there is error as expected, the test passes
		if err != nil && test.Err != nil {
			continue
		}
		if err != nil {
			t.Fatalf("Unexepcted error: %s", err)
		}
		if success != test.Success {
			t.Fatalf("Expected success response to be: %v, but found: %v", test.Success, success)
		}
	}
}

func TestMemoryProfile(t *testing.T) {
	tests := GetSuccessResponseTests()

	for _, test := range tests {
		mockClient := Client{requester: NewMockClient(apiargs.SuccessResponse{Success: test.Success}, test.Err)}
		success, err := mockClient.MemoryProfile()
		// if there is error as expected, the test passes
		if err != nil && test.Err != nil {
			continue
		}
		if err != nil {
			t.Fatalf("Unexepcted error: %s", err)
		}
		if success != test.Success {
			t.Fatalf("Expected success response to be: %v, but found: %v", test.Success, success)
		}
	}
}

func TestLockProfile(t *testing.T) {
	tests := GetSuccessResponseTests()

	for _, test := range tests {
		mockClient := Client{requester: NewMockClient(apiargs.SuccessResponse{Success: test.Success}, test.Err)}
		success, err := mockClient.LockProfile()
		// if there is error as expected, the test passes
		if err != nil && test.Err != nil {
			continue
		}
		if err != nil {
			t.Fatalf("Unexepcted error: %s", err)
		}
		if success != test.Success {
			t.Fatalf("Expected success response to be: %v, but found: %v", test.Success, success)
		}
	}
}

func TestAlias(t *testing.T) {
	tests := GetSuccessResponseTests()

	for _, test := range tests {
		mockClient := Client{requester: NewMockClient(apiargs.SuccessResponse{Success: test.Success}, test.Err)}
		success, err := mockClient.Alias("alias", "alias2")
		// if there is error as expected, the test passes
		if err != nil && test.Err != nil {
			continue
		}
		if err != nil {
			t.Fatalf("Unexepcted error: %s", err)
		}
		if success != test.Success {
			t.Fatalf("Expected success response to be: %v, but found: %v", test.Success, success)
		}
	}
}

func TestAliasChain(t *testing.T) {
	tests := GetSuccessResponseTests()

	for _, test := range tests {
		mockClient := Client{requester: NewMockClient(apiargs.SuccessResponse{Success: test.Success}, test.Err)}
		success, err := mockClient.AliasChain("chain", "chain-alias")
		// if there is error as expected, the test passes
		if err != nil && test.Err != nil {
			continue
		}
		if err != nil {
			t.Fatalf("Unexepcted error: %s", err)
		}
		if success != test.Success {
			t.Fatalf("Expected success response to be: %v, but found: %v", test.Success, success)
		}
	}
}

func TestStacktrace(t *testing.T) {
	tests := GetSuccessResponseTests()

	for _, test := range tests {
		mockClient := Client{requester: NewMockClient(apiargs.SuccessResponse{Success: test.Success}, test.Err)}
		success, err := mockClient.Stacktrace()
		// if there is error as expected, the test passes
		if err != nil && test.Err != nil {
			continue
		}
		if err != nil {
			t.Fatalf("Unexepcted error: %s", err)
		}
		if success != test.Success {
			t.Fatalf("Expected success response to be: %v, but found: %v", test.Success, success)
		}
	}
}
