package indexer

import (
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/stretchr/testify/assert"
)

type mockClient struct {
	f func(reply interface{}) error
}

func (mc *mockClient) SendRequest(_ string, _ interface{}, reply interface{}) error {
	return mc.f(reply)
}

func TestIndexClient(t *testing.T) {
	assert := assert.New(t)
	client := NewClient("http://localhost:9650", "/ext/index/C/block", time.Minute)

	// Test GetIndex
	client.EndpointRequester = &mockClient{
		f: func(reply interface{}) error {
			*(reply.(*GetIndexResponse)) = GetIndexResponse{Index: 5}
			return nil
		},
	}
	index, err := client.GetIndex(&GetIndexArgs{ContainerID: ids.Empty, Encoding: formatting.Hex})
	assert.NoError(err)
	assert.EqualValues(5, index.Index)

	// Test GetLastAccepted
	id := ids.GenerateTestID()
	client.EndpointRequester = &mockClient{
		f: func(reply interface{}) error {
			*(reply.(*FormattedContainer)) = FormattedContainer{ID: id}
			return nil
		},
	}
	container, err := client.GetLastAccepted(&GetLastAcceptedArgs{Encoding: formatting.Hex})
	assert.NoError(err)
	assert.EqualValues(id, container.ID)

	// Test GetContainerRange
	id = ids.GenerateTestID()
	client.EndpointRequester = &mockClient{
		f: func(reply interface{}) error {
			*(reply.(*GetContainerRangeResponse)) = GetContainerRangeResponse{Containers: []FormattedContainer{{ID: id}}}
			return nil
		},
	}
	containers, err := client.GetContainerRange(&GetContainerRangeArgs{StartIndex: 1, NumToFetch: 10, Encoding: formatting.Hex})
	assert.NoError(err)
	assert.Len(containers, 1)
	assert.EqualValues(id, containers[0].ID)

	// Test IsAccepted
	client.EndpointRequester = &mockClient{
		f: func(reply interface{}) error {
			*(reply.(*bool)) = true
			return nil
		},
	}
	isAccepted, err := client.IsAccepted(&GetIndexArgs{ContainerID: ids.Empty, Encoding: formatting.Hex})
	assert.NoError(err)
	assert.True(isAccepted)
}
