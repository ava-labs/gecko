// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package hierarchycodec

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/reflectcodec"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

const (
	// default max length of a slice being marshalled by Marshal(). Should be <= math.MaxUint32.
	defaultMaxSliceLength = 1 << 18
)

// Codec marshals and unmarshals
type Codec interface {
	codec.Registry
	codec.Codec
	SkipRegistrations(int)
	NextGroup()
}

type typeID struct {
	groupID uint16
	typeID  uint16
}

// Codec handles marshaling and unmarshaling of structs
type hierarchyCodec struct {
	codec.Codec

	lock           sync.RWMutex
	currentGroupID uint16
	nextTypeID     uint16
	typeIDToType   map[typeID]reflect.Type
	typeToTypeID   map[reflect.Type]typeID
}

// New returns a new, concurrency-safe codec
func New(tagName string, maxSliceLen uint32) Codec {
	hCodec := &hierarchyCodec{
		currentGroupID: 0,
		nextTypeID:     0,
		typeIDToType:   map[typeID]reflect.Type{},
		typeToTypeID:   map[reflect.Type]typeID{},
	}
	hCodec.Codec = reflectcodec.New(hCodec, tagName, maxSliceLen)
	return hCodec
}

// NewDefault returns a new codec with reasonable default values
func NewDefault() Codec { return New(reflectcodec.DefaultTagName, defaultMaxSliceLength) }

// SkipRegistrations some number of type IDs
func (c *hierarchyCodec) SkipRegistrations(num int) {
	c.lock.Lock()
	c.nextTypeID += uint16(num)
	c.lock.Unlock()
}

// NextGroup moves to the next group registry
func (c *hierarchyCodec) NextGroup() {
	c.lock.Lock()
	c.currentGroupID++
	c.nextTypeID = 0
	c.lock.Unlock()
}

// RegisterType is used to register types that may be unmarshaled into an interface
// [val] is a value of the type being registered
func (c *hierarchyCodec) RegisterType(val interface{}) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	valType := reflect.TypeOf(val)
	if _, exists := c.typeToTypeID[valType]; exists {
		return fmt.Errorf("type %v has already been registered", valType)
	}

	valTypeID := typeID{
		groupID: c.currentGroupID,
		typeID:  c.nextTypeID,
	}
	c.nextTypeID++

	c.typeIDToType[valTypeID] = valType
	c.typeToTypeID[valType] = valTypeID
	return nil
}

func (c *hierarchyCodec) PackPrefix(p *wrappers.Packer, valueType reflect.Type) error {
	c.lock.RLock()
	defer c.lock.RUnlock()

	typeID, ok := c.typeToTypeID[valueType] // Get the type ID of the value being marshaled
	if !ok {
		return fmt.Errorf("can't marshal unregistered type %q", valueType)
	}
	// Pack type ID so we know what to unmarshal this into
	p.PackShort(typeID.groupID)
	p.PackShort(typeID.typeID)
	return p.Err
}

func (c *hierarchyCodec) UnpackPrefix(p *wrappers.Packer, valueType reflect.Type) (reflect.Value, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	groupID := p.UnpackShort()     // Get the group ID
	typeIDShort := p.UnpackShort() // Get the type ID
	if p.Err != nil {
		return reflect.Value{}, fmt.Errorf("couldn't unmarshal interface: %w", p.Err)
	}
	t := typeID{
		groupID: groupID,
		typeID:  typeIDShort,
	}
	// Get a type that implements the interface
	implementingType, ok := c.typeIDToType[t]
	if !ok {
		return reflect.Value{}, fmt.Errorf("couldn't unmarshal interface: unknown type ID %+v", t)
	}
	// Ensure type actually does implement the interface
	if !implementingType.Implements(valueType) {
		return reflect.Value{}, fmt.Errorf("couldn't unmarshal interface: %s does not implement interface %s", implementingType, valueType)
	}
	return reflect.New(implementingType).Elem(), nil // instance of the proper type
}
