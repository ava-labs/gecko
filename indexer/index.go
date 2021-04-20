package indexer

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/math"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

const (
	// Maximum number of containers IDs that can be fetched at a time
	// in a call to GetContainerRange
	MaxFetchedByRange = 1024
)

var (
	// Maps to the byte representation of the next accepted index
	nextAcceptedIndexKey   []byte = []byte{0x00}
	indexToContainerPrefix []byte = []byte{0x01}
	containerToIDPrefix    []byte = []byte{0x02}
	errNoneAccepted               = errors.New("no containers have been accepted")
	errNumToFetchZero             = fmt.Errorf("numToFetch must be in [1,%d]", MaxFetchedByRange)

	_ Index = &index{}
)

// Index indexes containers in their order of acceptance
// Index implements triggers.Acceptor
// Index is thread-safe.
// Index assumes that Accept is called before the container is committed to the
// database of the VM that the container exists in.
type Index interface {
	Accept(ctx *snow.Context, containerID ids.ID, container []byte) error
	GetContainerByIndex(index uint64) (Container, error)
	GetContainerRange(startIndex uint64, numToFetch uint64) ([]Container, error)
	GetLastAccepted() (Container, error)
	GetIndex(containerID ids.ID) (uint64, error)
	GetContainerByID(containerID ids.ID) (Container, error)
	io.Closer
}

// indexer indexes all accepted transactions by the order in which they were accepted
type index struct {
	codec codec.Manager
	clock timer.Clock
	lock  sync.RWMutex
	// The index of the next accepted transaction
	nextAcceptedIndex uint64
	// When [baseDB] is committed, writes to [baseDB]
	vDB    *versiondb.Database
	baseDB database.Database
	// Both [indexToContainer] and [containerToIndex] have [vDB] underneath
	// Index --> Container
	indexToContainer database.Database
	// Container ID --> Index
	containerToIndex database.Database
	log              logging.Logger
}

// Returns a new, thread-safe Index.
// Closes [baseDB] on close.
func newIndex(
	baseDB database.Database,
	log logging.Logger,
	codec codec.Manager,
	clock timer.Clock,
) (Index, error) {
	vDB := versiondb.New(baseDB)
	indexToContainer := prefixdb.New(indexToContainerPrefix, vDB)
	containerToIndex := prefixdb.New(containerToIDPrefix, vDB)

	i := &index{
		clock:            clock,
		codec:            codec,
		baseDB:           baseDB,
		vDB:              vDB,
		indexToContainer: indexToContainer,
		containerToIndex: containerToIndex,
		log:              log,
	}

	// Get next accepted index from db
	nextAcceptedIndexBytes, err := i.vDB.Get(nextAcceptedIndexKey)
	if err == database.ErrNotFound {
		// Couldn't find it in the database. Must not have accepted any containers in previous runs.
		i.log.Info("next accepted index %d", i.nextAcceptedIndex)
		return i, nil
	}
	if err != nil {
		return nil, fmt.Errorf("couldn't get next accepted index from database: %w", err)
	}
	i.nextAcceptedIndex, err = wrappers.UnpackLong(nextAcceptedIndexBytes)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse next accepted index from bytes: %w", err)
	}
	i.log.Info("next accepted index %d", i.nextAcceptedIndex)
	return i, nil
}

// Close this index
func (i *index) Close() error {
	errs := wrappers.Errs{}
	errs.Add(
		i.indexToContainer.Close(),
		i.containerToIndex.Close(),
		i.vDB.Close(),
		i.baseDB.Close(),
	)
	return errs.Err
}

// Index that the given transaction is accepted
// Returned error should be treated as fatal; the VM should not commit [containerID]
// or any new containers as accepted.
func (i *index) Accept(ctx *snow.Context, containerID ids.ID, containerBytes []byte) error {
	i.lock.Lock()
	defer i.lock.Unlock()

	// It may be the case that in a previous run of this node, this index committed [containerID]
	// as accepted and then the node shut down before the VM committed [containerID] as accepted.
	// In that case, when the node restarts Accept will be called with the same container.
	// Make sure we don't index the same container twice in that event.
	_, err := i.containerToIndex.Get(containerID[:])
	if err == nil {
		ctx.Log.Debug("not indexing already accepted container %s", containerID)
		return nil
	}
	if err != database.ErrNotFound {
		return fmt.Errorf("couldn't get whether %s is accepted: %w", containerID, err)
	}

	ctx.Log.Debug("indexing %d --> container %s", i.nextAcceptedIndex, containerID)
	// Persist index --> Container
	nextAcceptedIndexBytes := wrappers.PackLong(i.nextAcceptedIndex)
	bytes, err := i.codec.Marshal(codecVersion, Container{
		ID:        containerID,
		Bytes:     containerBytes,
		Timestamp: i.clock.Time().UnixNano(),
	})
	if err != nil {
		return fmt.Errorf("couldn't serialize container %s: %w", containerID, err)
	}
	if err := i.indexToContainer.Put(nextAcceptedIndexBytes, bytes); err != nil {
		return fmt.Errorf("couldn't put accepted container %s into index: %w", containerID, err)
	}

	// Persist container ID --> index
	if err := i.containerToIndex.Put(containerID[:], nextAcceptedIndexBytes); err != nil {
		return fmt.Errorf("couldn't map container %s to index: %w", containerID, err)
	}

	// Persist next accepted index
	i.nextAcceptedIndex++
	nextAcceptedIndexBytes = wrappers.PackLong(i.nextAcceptedIndex)
	if err := i.vDB.Put(nextAcceptedIndexKey, nextAcceptedIndexBytes); err != nil {
		return fmt.Errorf("couldn't put accepted container %s into index: %w", containerID, err)
	}

	// Atomically commit [i.vDB], [i.indexToContainer], [i.containerToIndex] to [i.baseDB]
	return i.vDB.Commit()
}

// Returns the ID of the [index]th accepted container and the container itself.
// For example, if [index] == 0, returns the first accepted container.
// If [index] == 1, returns the second accepted container, etc.
// Returns an error if there is no container at the given index.
func (i *index) GetContainerByIndex(index uint64) (Container, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	return i.getContainerByIndex(index)
}

// Assumes [i.lock] is held
func (i *index) getContainerByIndex(index uint64) (Container, error) {
	lastAcceptedIndex, ok := i.lastAcceptedIndex()
	if !ok || index > lastAcceptedIndex {
		return Container{}, fmt.Errorf("no container at index %d", index)
	}
	indexBytes := wrappers.PackLong(index)
	return i.getContainerByIndexBytes(indexBytes)
}

// [indexBytes] is the byte representation of the index to fetch.
// Assumes [i.lock] is held
func (i *index) getContainerByIndexBytes(indexBytes []byte) (Container, error) {
	containerBytes, err := i.indexToContainer.Get(indexBytes)
	if err != nil {
		i.log.Error("couldn't read container from database: %w", err)
		return Container{}, fmt.Errorf("couldn't read from database: %w", err)
	}
	var container Container
	if _, err = i.codec.Unmarshal(containerBytes, &container); err != nil {
		return Container{}, fmt.Errorf("couldn't unmarshal container: %w", err)
	}
	return container, nil
}

// GetContainerRange returns the IDs of containers at indices
// [startIndex], [startIndex+1], ..., [startIndex+numToFetch-1].
// [startIndex] should be <= i.lastAcceptedIndex().
// [numToFetch] should be in [0, MaxFetchedByRange]
func (i *index) GetContainerRange(startIndex, numToFetch uint64) ([]Container, error) {
	// Check arguments for validity
	if numToFetch == 0 {
		return nil, errNumToFetchZero
	} else if numToFetch > MaxFetchedByRange {
		return nil, fmt.Errorf("requested %d but maximum page size is %d", numToFetch, MaxFetchedByRange)
	}

	i.lock.RLock()
	defer i.lock.RUnlock()

	lastAcceptedIndex, ok := i.lastAcceptedIndex()
	if !ok {
		return nil, errNoneAccepted
	} else if startIndex > lastAcceptedIndex {
		return nil, fmt.Errorf("start index (%d) > last accepted index (%d)", startIndex, lastAcceptedIndex)
	}

	// Calculate the last index we will fetch
	lastIndex := math.Min64(startIndex+numToFetch-1, lastAcceptedIndex)
	// [lastIndex] is always >= [startIndex] so this is safe.
	// [numToFetch] is limited to [MaxFetchedByRange] so [containers] is bounded in size.
	containers := make([]Container, int(lastIndex)-int(startIndex)+1)

	n := 0
	var err error
	for j := startIndex; j <= lastIndex; j++ {
		containers[n], err = i.getContainerByIndex(j)
		if err != nil {
			return nil, fmt.Errorf("couldn't get container at index %d: %w", j, err)
		}
		n++
	}
	return containers, nil
}

// Returns database.ErrNotFound if the container is not indexed as accepted
func (i *index) GetIndex(containerID ids.ID) (uint64, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	indexBytes, err := i.containerToIndex.Get(containerID[:])
	if err != nil {
		return 0, err
	}
	index, err := wrappers.UnpackLong(indexBytes)
	if err != nil {
		// Should never happen
		i.log.Error("couldn't unpack index: %w", err)
		return 0, err
	}
	return index, nil
}

func (i *index) GetContainerByID(containerID ids.ID) (Container, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	// Read index from database
	indexBytes, err := i.containerToIndex.Get(containerID[:])
	if err != nil {
		return Container{}, err
	}
	return i.getContainerByIndexBytes(indexBytes)
}

// GetLastAccepted returns the last accepted container.
// Returns an error if no containers have been accepted.
func (i *index) GetLastAccepted() (Container, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	lastAcceptedIndex, exists := i.lastAcceptedIndex()
	if !exists {
		return Container{}, errNoneAccepted
	}
	return i.getContainerByIndex(lastAcceptedIndex)
}

// Assumes i.lock is held
// Returns:
// 1) The index of the most recently accepted transaction,
//    or 0 if no transactions have been accepted
// 2) Whether at least 1 transaction has been accepted
func (i *index) lastAcceptedIndex() (uint64, bool) {
	return i.nextAcceptedIndex - 1, i.nextAcceptedIndex != 0
}
