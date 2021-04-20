package indexer

import (
	"testing"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/stretchr/testify/assert"
)

func TestIndex(t *testing.T) {
	// Setup
	pageSize := uint64(64)
	assert := assert.New(t)
	codec := codec.NewDefaultManager()
	err := codec.RegisterCodec(codecVersion, linearcodec.NewDefault())
	assert.NoError(err)
	baseDB := memdb.New()
	db := versiondb.New(baseDB)
	ctx := snow.DefaultContextTest()

	indexIntf, err := newIndex(db, logging.NoLog{}, codec, timer.Clock{})
	assert.NoError(err)
	idx := indexIntf.(*index)

	// Populate "containers" with random IDs/bytes
	containers := map[ids.ID][]byte{}
	for i := uint64(0); i < 2*pageSize; i++ {
		containers[ids.GenerateTestID()] = utils.RandomBytes(32)
	}

	// Accept each container and after each, make assertions
	i := uint64(0)
	for containerID, containerBytes := range containers {
		err = idx.Accept(ctx, containerID, containerBytes)
		assert.NoError(err)

		lastAcceptedIndex, ok := idx.lastAcceptedIndex()
		assert.True(ok)
		assert.EqualValues(i, lastAcceptedIndex)
		assert.EqualValues(i+1, idx.nextAcceptedIndex)

		gotContainer, err := idx.GetContainerByID(containerID)
		assert.NoError(err)
		assert.Equal(containerBytes, gotContainer.Bytes)

		gotIndex, err := idx.GetIndex(containerID)
		assert.NoError(err)
		assert.EqualValues(i, gotIndex)

		gotContainer, err = idx.GetContainerByIndex(i)
		assert.NoError(err)
		assert.Equal(containerBytes, gotContainer.Bytes)

		gotContainer, err = idx.GetLastAccepted()
		assert.NoError(err)
		assert.Equal(containerBytes, gotContainer.Bytes)

		containers, err := idx.GetContainerRange(i, 1)
		assert.NoError(err)
		assert.Len(containers, 1)
		assert.Equal(containerBytes, containers[0].Bytes)

		containers, err = idx.GetContainerRange(i, 2)
		assert.NoError(err)
		assert.Len(containers, 1)
		assert.Equal(containerBytes, containers[0].Bytes)

		i++
	}

	// Create a new index with the same database and ensure contents still there
	assert.NoError(db.Commit())
	assert.NoError(idx.Close())
	db = versiondb.New(baseDB)
	indexIntf, err = newIndex(db, logging.NoLog{}, codec, timer.Clock{})
	assert.NoError(err)
	idx = indexIntf.(*index)

	// Get all of the containers
	containersList, err := idx.GetContainerRange(0, pageSize)
	assert.NoError(err)
	assert.Len(containersList, int(pageSize))
	containersList2, err := idx.GetContainerRange(pageSize, pageSize)
	assert.NoError(err)
	assert.Len(containersList2, int(pageSize))
	containersList = append(containersList, containersList2...)

	// Ensure that the data is correct
	lastTimestamp := int64(0)
	sawContainers := ids.Set{}
	for _, container := range containersList {
		assert.False(sawContainers.Contains(container.ID)) // Should only see this container once
		assert.Contains(containers, container.ID)
		assert.EqualValues(containers[container.ID], container.Bytes)
		// Timestamps should be non-decreasing
		assert.True(container.Timestamp >= lastTimestamp)
		lastTimestamp = container.Timestamp
		sawContainers.Add(container.ID)
	}
}

func TestIndexGetContainerByRangeMaxPageSize(t *testing.T) {
	// Setup
	assert := assert.New(t)
	codec := codec.NewDefaultManager()
	err := codec.RegisterCodec(codecVersion, linearcodec.NewDefault())
	assert.NoError(err)
	db := memdb.New()
	ctx := snow.DefaultContextTest()
	indexIntf, err := newIndex(db, logging.NoLog{}, codec, timer.Clock{})
	assert.NoError(err)
	idx := indexIntf.(*index)

	// Insert [MaxFetchedByRange] + 1 containers
	for i := uint64(0); i < MaxFetchedByRange+1; i++ {
		err = idx.Accept(ctx, ids.GenerateTestID(), utils.RandomBytes(32))
		assert.NoError(err)
	}

	// Page size too large
	_, err = idx.GetContainerRange(0, MaxFetchedByRange+1)
	assert.Error(err)

	// Make sure data is right
	containers, err := idx.GetContainerRange(0, MaxFetchedByRange)
	assert.NoError(err)
	assert.Len(containers, MaxFetchedByRange)

	containers2, err := idx.GetContainerRange(1, MaxFetchedByRange)
	assert.NoError(err)
	assert.Len(containers2, MaxFetchedByRange)

	assert.Equal(containers[1], containers2[0])
	assert.Equal(containers[MaxFetchedByRange-1], containers2[MaxFetchedByRange-2])

	// Should have last 2 elements
	containers, err = idx.GetContainerRange(MaxFetchedByRange-1, MaxFetchedByRange)
	assert.NoError(err)
	assert.Len(containers, 2)
	assert.EqualValues(containers[1], containers2[MaxFetchedByRange-1])
	assert.EqualValues(containers[0], containers2[MaxFetchedByRange-2])
}

func TestDontIndexSameContainerTwice(t *testing.T) {
	// Setup
	assert := assert.New(t)
	codec := codec.NewDefaultManager()
	err := codec.RegisterCodec(codecVersion, linearcodec.NewDefault())
	assert.NoError(err)
	db := memdb.New()
	ctx := snow.DefaultContextTest()
	idx, err := newIndex(db, logging.NoLog{}, codec, timer.Clock{})
	assert.NoError(err)

	// Accept the same container twice
	containerID := ids.GenerateTestID()
	assert.NoError(idx.Accept(ctx, containerID, []byte{1, 2, 3}))
	assert.NoError(idx.Accept(ctx, containerID, []byte{4, 5, 6}))
	_, err = idx.GetContainerByIndex(1)
	assert.Error(err, "should not have accepted same container twice")
	gotContainer, err := idx.GetContainerByID(containerID)
	assert.NoError(err)
	assert.EqualValues(gotContainer.Bytes, []byte{1, 2, 3}, "should not have accepted same container twice")

}
