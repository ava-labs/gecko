// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package reflectcodec

import (
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

const (
	// DefaultTagName that enables serialization.
	DefaultTagName = "serialize"
)

var (
	errMarshalNil   = errors.New("can't marshal nil pointer or interface")
	errUnmarshalNil = errors.New("can't unmarshal nil")
	errNeedPointer  = errors.New("argument to unmarshal must be a pointer")
	errExtraSpace   = errors.New("trailing buffer space")
)

type TypeCodec interface {
	// UnpackPrefix unpacks the prefix of an interface from the given packer.
	// The prefix specifies the concrete type that the interface should be
	// deserialized into. This function returns a new instance of that concrete
	// type. The concrete type must implement the given type.
	UnpackPrefix(*wrappers.Packer, reflect.Type) (reflect.Value, error)

	// PackPrefix packs the prefix for the given type into the given packer.
	// This identifies the bytes that follow, which are the byte representation
	// of an interface, as having the given concrete type.
	// When deserializing the bytes, the prefix specifies which concrete type
	// to deserialize into.
	PackPrefix(*wrappers.Packer, reflect.Type) error
}

// genericCodec handles marshaling and unmarshaling of structs with a generic
// implementation for interface encoding.
//
// A few notes:
// 1) We use "marshal" and "serialize" interchangeably, and "unmarshal" and "deserialize" interchangeably
// 2) To include a field of a struct in the serialized form, add the tag `{tagName}:"true"` to it. `{tagName}` defaults to `serialize`.
// 3) These typed members of a struct may be serialized:
//    bool, string, uint[8,16,32,64], int[8,16,32,64],
//	  structs, slices, arrays, interface.
//	  structs, slices and arrays can only be serialized if their constituent values can be.
// 4) To marshal an interface, you must pass a pointer to the value
// 5) To unmarshal an interface,  you must call codec.RegisterType([instance of the type that fulfills the interface]).
// 6) Serialized fields must be exported
// 7) nil slices are marshaled as empty slices
type genericCodec struct {
	typer       TypeCodec
	maxSliceLen uint32
	fielder     StructFielder
}

// New returns a new, concurrency-safe codec
func New(typer TypeCodec, tagName string, maxSliceLen uint32) codec.Codec {
	return &genericCodec{
		typer:       typer,
		maxSliceLen: maxSliceLen,
		fielder:     NewStructFielder(tagName, maxSliceLen),
	}
}

// To marshal an interface, [value] must be a pointer to the interface
func (c *genericCodec) MarshalInto(value interface{}, p *wrappers.Packer) error {
	if value == nil {
		return errMarshalNil // can't marshal nil
	}

	return c.marshal(reflect.ValueOf(value), p, c.maxSliceLen)
}

// marshal writes the byte representation of [value] to [p]
// [value]'s underlying value must not be a nil pointer or interface
// c.lock should be held for the duration of this function
func (c *genericCodec) marshal(value reflect.Value, p *wrappers.Packer, maxSliceLen uint32) error {
	valueKind := value.Kind()
	switch valueKind {
	case reflect.Interface, reflect.Ptr, reflect.Invalid:
		if value.IsNil() { // Can't marshal nil (except nil slices)
			return errMarshalNil
		}
	}

	switch valueKind {
	case reflect.Uint8:
		p.PackByte(uint8(value.Uint()))
		return p.Err
	case reflect.Int8:
		p.PackByte(uint8(value.Int()))
		return p.Err
	case reflect.Uint16:
		p.PackShort(uint16(value.Uint()))
		return p.Err
	case reflect.Int16:
		p.PackShort(uint16(value.Int()))
		return p.Err
	case reflect.Uint32:
		p.PackInt(uint32(value.Uint()))
		return p.Err
	case reflect.Int32:
		p.PackInt(uint32(value.Int()))
		return p.Err
	case reflect.Uint64:
		p.PackLong(value.Uint())
		return p.Err
	case reflect.Int64:
		p.PackLong(uint64(value.Int()))
		return p.Err
	case reflect.String:
		p.PackStr(value.String())
		return p.Err
	case reflect.Bool:
		p.PackBool(value.Bool())
		return p.Err
	case reflect.Uintptr, reflect.Ptr:
		return c.marshal(value.Elem(), p, c.maxSliceLen)
	case reflect.Interface:
		underlyingValue := value.Interface()
		underlyingType := reflect.TypeOf(underlyingValue)
		if err := c.typer.PackPrefix(p, underlyingType); err != nil {
			return err
		}
		if err := c.marshal(value.Elem(), p, c.maxSliceLen); err != nil {
			return err
		}
		return p.Err
	case reflect.Slice:
		numElts := value.Len() // # elements in the slice/array. 0 if this slice is nil.
		if uint32(numElts) > maxSliceLen {
			return fmt.Errorf("slice length, %d, exceeds maximum length, %d",
				numElts,
				maxSliceLen)
		}
		p.PackInt(uint32(numElts)) // pack # elements
		if p.Err != nil {
			return p.Err
		}
		// If this is a slice of bytes, manually pack the bytes rather
		// than calling marshal on each byte. This improves performance.
		if elemKind := value.Type().Elem().Kind(); elemKind == reflect.Uint8 {
			p.PackFixedBytes(value.Bytes())
			return p.Err
		}
		for i := 0; i < numElts; i++ { // Process each element in the slice
			if err := c.marshal(value.Index(i), p, c.maxSliceLen); err != nil {
				return err
			}
		}
		return nil
	case reflect.Array:
		numElts := value.Len()
		if elemKind := value.Type().Kind(); elemKind == reflect.Uint8 {
			sliceVal := value.Convert(reflect.TypeOf([]byte{}))
			p.PackFixedBytes(sliceVal.Bytes())
			return p.Err
		}
		if uint32(numElts) > c.maxSliceLen {
			return fmt.Errorf("array length, %d, exceeds maximum length, %d", numElts, c.maxSliceLen)
		}
		for i := 0; i < numElts; i++ { // Process each element in the array
			if err := c.marshal(value.Index(i), p, c.maxSliceLen); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		serializedFields, err := c.fielder.GetSerializedFields(value.Type())
		if err != nil {
			return err
		}
		for _, fieldDesc := range serializedFields { // Go through all fields of this struct that are serialized
			if err := c.marshal(value.Field(fieldDesc.Index), p, fieldDesc.MaxSliceLen); err != nil { // Serialize the field and write to byte array
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("can't marshal unknown kind %s", valueKind)
	}
}

// Unmarshal unmarshals [bytes] into [dest], where
// [dest] must be a pointer or interface
func (c *genericCodec) Unmarshal(bytes []byte, dest interface{}) error {
	if dest == nil {
		return errUnmarshalNil
	}

	p := wrappers.Packer{
		Bytes: bytes,
	}
	destPtr := reflect.ValueOf(dest)
	if destPtr.Kind() != reflect.Ptr {
		return errNeedPointer
	}
	if err := c.unmarshal(&p, destPtr.Elem(), c.maxSliceLen); err != nil {
		return err
	}
	if p.Offset != len(bytes) {
		return errExtraSpace
	}
	return nil
}

// Unmarshal from p.Bytes into [value]. [value] must be addressable.
// c.lock should be held for the duration of this function
func (c *genericCodec) unmarshal(p *wrappers.Packer, value reflect.Value, maxSliceLen uint32) error {
	switch value.Kind() {
	case reflect.Uint8:
		value.SetUint(uint64(p.UnpackByte()))
		if p.Err != nil {
			return fmt.Errorf("couldn't unmarshal uint8: %w", p.Err)
		}
		return nil
	case reflect.Int8:
		value.SetInt(int64(p.UnpackByte()))
		if p.Err != nil {
			return fmt.Errorf("couldn't unmarshal int8: %w", p.Err)
		}
		return nil
	case reflect.Uint16:
		value.SetUint(uint64(p.UnpackShort()))
		if p.Err != nil {
			return fmt.Errorf("couldn't unmarshal uint16: %w", p.Err)
		}
		return nil
	case reflect.Int16:
		value.SetInt(int64(p.UnpackShort()))
		if p.Err != nil {
			return fmt.Errorf("couldn't unmarshal int16: %w", p.Err)
		}
		return nil
	case reflect.Uint32:
		value.SetUint(uint64(p.UnpackInt()))
		if p.Err != nil {
			return fmt.Errorf("couldn't unmarshal uint32: %w", p.Err)
		}
		return nil
	case reflect.Int32:
		value.SetInt(int64(p.UnpackInt()))
		if p.Err != nil {
			return fmt.Errorf("couldn't unmarshal int32: %w", p.Err)
		}
		return nil
	case reflect.Uint64:
		value.SetUint(p.UnpackLong())
		if p.Err != nil {
			return fmt.Errorf("couldn't unmarshal uint64: %w", p.Err)
		}
		return nil
	case reflect.Int64:
		value.SetInt(int64(p.UnpackLong()))
		if p.Err != nil {
			return fmt.Errorf("couldn't unmarshal int64: %w", p.Err)
		}
		return nil
	case reflect.Bool:
		value.SetBool(p.UnpackBool())
		if p.Err != nil {
			return fmt.Errorf("couldn't unmarshal bool: %w", p.Err)
		}
		return nil
	case reflect.Slice:
		numElts32 := p.UnpackInt()
		if p.Err != nil {
			return fmt.Errorf("couldn't unmarshal slice: %w", p.Err)
		}
		if numElts32 > maxSliceLen {
			return fmt.Errorf("array length, %d, exceeds maximum length, %d",
				numElts32,
				maxSliceLen)
		}
		if numElts32 > math.MaxInt32 {
			return fmt.Errorf("array length, %d, exceeds maximum length, %d",
				numElts32,
				math.MaxInt32)
		}
		numElts := int(numElts32)

		// If this is a slice of bytes, manually unpack the bytes rather
		// than calling unmarshal on each byte. This improves performance.
		if elemKind := value.Type().Elem().Kind(); elemKind == reflect.Uint8 {
			value.SetBytes(p.UnpackFixedBytes(numElts))
			return p.Err
		}
		// set [value] to be a slice of the appropriate type/capacity (right now it is nil)
		value.Set(reflect.MakeSlice(value.Type(), numElts, numElts))
		// Unmarshal each element into the appropriate index of the slice
		for i := 0; i < numElts; i++ {
			if err := c.unmarshal(p, value.Index(i), c.maxSliceLen); err != nil {
				return fmt.Errorf("couldn't unmarshal slice element: %w", err)
			}
		}
		return nil
	case reflect.Array:
		numElts := value.Len()
		if elemKind := value.Type().Elem().Kind(); elemKind == reflect.Uint8 {
			unpackedBytes := p.UnpackFixedBytes(numElts)
			if p.Errored() {
				return p.Err
			}
			// Get a slice to the underlying array value
			underlyingSlice := value.Slice(0, numElts).Interface().([]byte)
			copy(underlyingSlice, unpackedBytes)
			return nil
		}
		for i := 0; i < numElts; i++ {
			if err := c.unmarshal(p, value.Index(i), c.maxSliceLen); err != nil {
				return fmt.Errorf("couldn't unmarshal array element: %w", err)
			}
		}
		return nil
	case reflect.String:
		value.SetString(p.UnpackStr())
		if p.Err != nil {
			return fmt.Errorf("couldn't unmarshal string: %w", p.Err)
		}
		return nil
	case reflect.Interface:
		intfImplementor, err := c.typer.UnpackPrefix(p, value.Type())
		if err != nil {
			return err
		}
		// Unmarshal into the struct
		if err := c.unmarshal(p, intfImplementor, c.maxSliceLen); err != nil {
			return fmt.Errorf("couldn't unmarshal interface: %w", err)
		}
		// And assign the filled struct to the value
		value.Set(intfImplementor)
		return nil
	case reflect.Struct:
		// Get indices of fields that will be unmarshaled into
		serializedFieldIndices, err := c.fielder.GetSerializedFields(value.Type())
		if err != nil {
			return fmt.Errorf("couldn't unmarshal struct: %w", err)
		}
		// Go through the fields and umarshal into them
		for _, fieldDesc := range serializedFieldIndices {
			if err := c.unmarshal(p, value.Field(fieldDesc.Index), fieldDesc.MaxSliceLen); err != nil {
				return fmt.Errorf("couldn't unmarshal struct: %w", err)
			}
		}
		return nil
	case reflect.Ptr:
		// Get the type this pointer points to
		t := value.Type().Elem()
		// Create a new pointer to a new value of the underlying type
		v := reflect.New(t)
		// Fill the value
		if err := c.unmarshal(p, v.Elem(), c.maxSliceLen); err != nil {
			return fmt.Errorf("couldn't unmarshal pointer: %w", err)
		}
		// Assign to the top-level struct's member
		value.Set(v)
		return nil
	case reflect.Invalid:
		return errUnmarshalNil
	default:
		return fmt.Errorf("can't unmarshal unknown type %s", value.Kind().String())
	}
}
