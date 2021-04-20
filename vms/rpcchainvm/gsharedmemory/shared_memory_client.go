// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gsharedmemory

import (
	"context"

	"github.com/ava-labs/avalanchego/chains/atomic"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/gsharedmemory/gsharedmemoryproto"
)

var (
	_ atomic.SharedMemory = &Client{}
)

// Client is atomic.SharedMemory that talks over RPC.
type Client struct {
	client gsharedmemoryproto.SharedMemoryClient
}

// NewClient returns shared memory connected to remote shared memory
func NewClient(client gsharedmemoryproto.SharedMemoryClient) *Client {
	return &Client{client: client}
}

func (c *Client) Put(peerChainID ids.ID, elems []*atomic.Element, batches ...database.Batch) error {
	req := gsharedmemoryproto.PutRequest{
		PeerChainID: peerChainID[:],
		Elems:       make([]*gsharedmemoryproto.Element, len(elems)),
		Batches:     make([]*gsharedmemoryproto.Batch, len(batches)),
	}

	for i, elem := range elems {
		req.Elems[i] = &gsharedmemoryproto.Element{
			Key:    elem.Key,
			Value:  elem.Value,
			Traits: elem.Traits,
		}
	}

	for i, batch := range batches {
		batch := batch.Inner()
		fb := filteredBatch{
			writes:  make(map[string][]byte),
			deletes: make(map[string]struct{}),
		}
		if err := batch.Replay(&fb); err != nil {
			return err
		}
		req.Batches[i] = &gsharedmemoryproto.Batch{
			Puts:    fb.PutRequests(),
			Deletes: fb.DeleteRequests(),
		}
	}

	_, err := c.client.Put(context.Background(), &req)
	return err
}

func (c *Client) Get(peerChainID ids.ID, keys [][]byte) (values [][]byte, err error) {
	resp, err := c.client.Get(context.Background(), &gsharedmemoryproto.GetRequest{
		PeerChainID: peerChainID[:],
		Keys:        keys,
	})
	if err != nil {
		return nil, err
	}
	return resp.Values, nil
}

func (c *Client) Indexed(
	peerChainID ids.ID,
	traits [][]byte,
	startTrait,
	startKey []byte,
	limit int,
) (
	values [][]byte,
	lastTrait,
	lastKey []byte,
	err error,
) {
	resp, err := c.client.Indexed(context.Background(), &gsharedmemoryproto.IndexedRequest{
		PeerChainID: peerChainID[:],
		Traits:      traits,
		StartTrait:  startTrait,
		StartKey:    startKey,
		Limit:       int32(limit),
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return resp.Values, resp.LastTrait, resp.LastKey, nil
}

func (c *Client) Remove(peerChainID ids.ID, keys [][]byte, batches ...database.Batch) error {
	req := gsharedmemoryproto.RemoveRequest{
		PeerChainID: peerChainID[:],
		Keys:        keys,
		Batches:     make([]*gsharedmemoryproto.Batch, len(batches)),
	}

	for i, batch := range batches {
		batch := batch.Inner()
		fb := filteredBatch{
			writes:  make(map[string][]byte),
			deletes: make(map[string]struct{}),
		}
		if err := batch.Replay(&fb); err != nil {
			return err
		}
		req.Batches[i] = &gsharedmemoryproto.Batch{
			Puts:    fb.PutRequests(),
			Deletes: fb.DeleteRequests(),
		}
	}

	_, err := c.client.Remove(context.Background(), &req)
	return err
}

type filteredBatch struct {
	writes  map[string][]byte
	deletes map[string]struct{}
}

func (b *filteredBatch) Put(key []byte, value []byte) error {
	keyStr := string(key)
	delete(b.deletes, keyStr)
	b.writes[keyStr] = value
	return nil
}

func (b *filteredBatch) Delete(key []byte) error {
	keyStr := string(key)
	delete(b.writes, keyStr)
	b.deletes[keyStr] = struct{}{}
	return nil
}

func (b *filteredBatch) PutRequests() []*gsharedmemoryproto.BatchPut {
	reqs := make([]*gsharedmemoryproto.BatchPut, 0, len(b.writes))
	for keyStr, value := range b.writes {
		reqs = append(reqs, &gsharedmemoryproto.BatchPut{
			Key:   []byte(keyStr),
			Value: value,
		})
	}
	return reqs
}

func (b *filteredBatch) DeleteRequests() []*gsharedmemoryproto.BatchDelete {
	reqs := make([]*gsharedmemoryproto.BatchDelete, 0, len(b.deletes))
	for keyStr := range b.deletes {
		reqs = append(reqs, &gsharedmemoryproto.BatchDelete{
			Key: []byte(keyStr),
		})
	}
	return reqs
}
