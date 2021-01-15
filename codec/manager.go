// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package codec

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ava-labs/avalanchego/utils/wrappers"
)

const (
	// default max size, in bytes, of something being marshalled by Marshal()
	defaultMaxSize = 1 << 18

	// initial capacity of byte slice that values are marshaled into.
	// Larger value --> need less memory allocations but possibly have allocated but unused memory
	// Smaller value --> need more memory allocations but more efficient use of allocated memory
	initialSliceCap = 128
)

var (
	errMarshalNil        = errors.New("can't marshal nil pointer or interface")
	errUnmarshalNil      = errors.New("can't unmarshal nil")
	errCantPackVersion   = errors.New("couldn't pack codec version")
	errCantUnpackVersion = errors.New("couldn't unpack codec version")
	errDuplicatedVersion = errors.New("duplicated codec version")
)

// Manager describes the functionality for managing codec versions.
type Manager interface {
	// Associate the given codec with the given version ID
	RegisterCodec(version uint16, codec Codec) error

	// Define the maximum size, in bytes, of something serialized/deserialized
	// by this codec manager
	SetMaxSize(int)

	// Marshal the given value using the codec with the given version.
	// RegisterCodec must have been called with that version.
	Marshal(version uint16, source interface{}) (destination []byte, err error)

	// Unmarshal the given bytes into the given destination. [destination] must
	// be a pointer or an interface. Returns the version of the codec that
	// produces the given bytes.
	Unmarshal(source []byte, destination interface{}) (version uint16, err error)
}

// NewManager returns a new codec manager.
func NewManager(maxSize int) Manager {
	return &manager{
		maxSize: maxSize,
		codecs:  map[uint16]Codec{},
	}
}

// NewDefaultManager returns a new codec manager.
func NewDefaultManager() Manager { return NewManager(defaultMaxSize) }

type manager struct {
	lock    sync.RWMutex
	maxSize int
	codecs  map[uint16]Codec
}

// RegisterCodec is used to register a new codec version that can be used to
// (un)marshal with.
func (m *manager) RegisterCodec(version uint16, codec Codec) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if _, exists := m.codecs[version]; exists {
		return errDuplicatedVersion
	}
	m.codecs[version] = codec
	return nil
}

// SetMaxSize of bytes allowed
func (m *manager) SetMaxSize(size int) {
	m.lock.Lock()
	m.maxSize = size
	m.lock.Unlock()
}

// To marshal an interface, [value] must be a pointer to the interface.
func (m *manager) Marshal(version uint16, value interface{}) ([]byte, error) {
	if value == nil {
		return nil, errMarshalNil // can't marshal nil
	}

	m.lock.RLock()
	c, exists := m.codecs[version]
	m.lock.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown codec version %d", version)
	}

	p := wrappers.Packer{
		MaxSize: m.maxSize,
		Bytes:   make([]byte, 0, initialSliceCap),
	}
	p.PackShort(version)
	if p.Errored() {
		return nil, errCantPackVersion // Should never happen
	}
	return p.Bytes, c.MarshalInto(value, &p)
}

// Unmarshal unmarshals [bytes] into [dest], where [dest] must be a pointer or
// interface.
func (m *manager) Unmarshal(bytes []byte, dest interface{}) (uint16, error) {
	if dest == nil {
		return 0, errUnmarshalNil
	}

	m.lock.RLock()
	if len(bytes) > m.maxSize {
		m.lock.RUnlock()
		return 0, fmt.Errorf("byte array exceeds maximum length, %d", m.maxSize)
	}

	p := wrappers.Packer{
		Bytes: bytes,
	}

	version := p.UnpackShort()
	if p.Errored() { // Make sure the codec version is correct
		m.lock.RUnlock()
		return 0, errCantUnpackVersion
	}

	c, exists := m.codecs[version]
	m.lock.RUnlock()
	if !exists {
		return version, fmt.Errorf("unknown codec version %d", version)
	}
	return version, c.Unmarshal(p.Bytes[p.Offset:], dest)
}
