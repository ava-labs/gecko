// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wrappers

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/hashing"
)

const (
	// MaxStringLen ...
	MaxStringLen = math.MaxUint16

	// ByteLen is the number of bytes per byte...
	ByteLen = 1
	// ShortLen is the number of bytes per short
	ShortLen = 2
	// IntLen is the number of bytes per int
	IntLen = 4
	// LongLen is the number of bytes per long
	LongLen = 8
	// BoolLen is the number of bytes per bool
	BoolLen = 1
)

var (
	errBadLength      = errors.New("packer has insufficient length for input")
	errNegativeOffset = errors.New("negative offset")
	errInvalidInput   = errors.New("input does not match expected format")
	errBadType        = errors.New("wrong type passed")
	errBadBool        = errors.New("unexpected value when unpacking bool")
)

// Packer packs and unpacks a byte array from/to standard values
type Packer struct {
	Errs

	// The largest allowed size of expanding the byte array
	MaxSize int
	// The current byte array
	Bytes []byte
	// The offset that is being written to in the byte array
	Offset int
}

// CheckSpace requires that there is at least [bytes] of write space left in the
// byte array. If this is not true, an error is added to the packer
func (p *Packer) CheckSpace(bytes int) {
	switch {
	case p.Offset < 0:
		p.Add(errNegativeOffset)
	case bytes < 0:
		p.Add(errInvalidInput)
	case len(p.Bytes)-p.Offset < bytes:
		p.Add(errBadLength)
	}
}

// Expand ensures that there is [bytes] bytes left of space in the byte slice.
// If this is not allowed due to the maximum size, an error is added to the packer
// In order to understand this code, its important to understand the difference
// between a slice's length and its capacity.
func (p *Packer) Expand(bytes int) {
	neededSize := bytes + p.Offset // Need byte slice's length to be at least [neededSize]
	switch {
	case neededSize <= len(p.Bytes): // Byte slice has sufficient length already
		return
	case neededSize > p.MaxSize: // Lengthening the byte slice would cause it to grow too large
		p.Err = errBadLength
		return
	case neededSize <= cap(p.Bytes): // Byte slice has sufficient capacity to lengthen it without mem alloc
		p.Bytes = p.Bytes[:neededSize]
		return
	default: // Add capacity/length to byte slice
		p.Bytes = append(p.Bytes[:cap(p.Bytes)], make([]byte, neededSize-cap(p.Bytes))...)
	}
}

// PackByte append a byte to the byte array
func (p *Packer) PackByte(val byte) {
	p.Expand(ByteLen)
	if p.Errored() {
		return
	}

	p.Bytes[p.Offset] = val
	p.Offset++
}

// UnpackByte unpack a byte from the byte array
func (p *Packer) UnpackByte() byte {
	p.CheckSpace(ByteLen)
	if p.Errored() {
		return 0
	}

	val := p.Bytes[p.Offset]
	p.Offset++
	return val
}

// PackShort append a short to the byte array
func (p *Packer) PackShort(val uint16) {
	p.Expand(ShortLen)
	if p.Errored() {
		return
	}

	binary.BigEndian.PutUint16(p.Bytes[p.Offset:], val)
	p.Offset += ShortLen
}

// UnpackShort unpack a short from the byte array
func (p *Packer) UnpackShort() uint16 {
	p.CheckSpace(ShortLen)
	if p.Errored() {
		return 0
	}

	val := binary.BigEndian.Uint16(p.Bytes[p.Offset:])
	p.Offset += ShortLen
	return val
}

// PackInt append an int to the byte array
func (p *Packer) PackInt(val uint32) {
	p.Expand(IntLen)
	if p.Errored() {
		return
	}

	binary.BigEndian.PutUint32(p.Bytes[p.Offset:], val)
	p.Offset += IntLen
}

// UnpackInt unpack an int from the byte array
func (p *Packer) UnpackInt() uint32 {
	p.CheckSpace(IntLen)
	if p.Errored() {
		return 0
	}

	val := binary.BigEndian.Uint32(p.Bytes[p.Offset:])
	p.Offset += IntLen
	return val
}

// PackLong append a long to the byte array
func (p *Packer) PackLong(val uint64) {
	p.Expand(LongLen)
	if p.Errored() {
		return
	}

	binary.BigEndian.PutUint64(p.Bytes[p.Offset:], val)
	p.Offset += LongLen
}

// UnpackLong unpack a long from the byte array
func (p *Packer) UnpackLong() uint64 {
	p.CheckSpace(LongLen)
	if p.Errored() {
		return 0
	}

	val := binary.BigEndian.Uint64(p.Bytes[p.Offset:])
	p.Offset += LongLen
	return val
}

// PackBool packs a bool into the byte array
func (p *Packer) PackBool(b bool) {
	if b {
		p.PackByte(1)
	} else {
		p.PackByte(0)
	}
}

// UnpackBool unpacks a bool from the byte array
func (p *Packer) UnpackBool() bool {
	b := p.UnpackByte()
	switch b {
	case 0:
		return false
	case 1:
		return true
	default:
		p.Add(errBadBool)
		return false
	}
}

// PackFixedBytes append a byte slice, with no length descriptor to the byte
// array
func (p *Packer) PackFixedBytes(bytes []byte) {
	p.Expand(len(bytes))
	if p.Errored() {
		return
	}

	copy(p.Bytes[p.Offset:], bytes)
	p.Offset += len(bytes)
}

// UnpackFixedBytes unpack a byte slice, with no length descriptor from the byte
// array
func (p *Packer) UnpackFixedBytes(size int) []byte {
	p.CheckSpace(size)
	if p.Errored() {
		return nil
	}

	bytes := p.Bytes[p.Offset : p.Offset+size]
	p.Offset += size
	return bytes
}

// PackBytes append a byte slice to the byte array
func (p *Packer) PackBytes(bytes []byte) {
	p.PackInt(uint32(len(bytes)))
	p.PackFixedBytes(bytes)
}

// UnpackBytes unpack a byte slice from the byte array
func (p *Packer) UnpackBytes() []byte {
	size := p.UnpackInt()
	return p.UnpackFixedBytes(int(size))
}

// PackFixedByteSlices append a byte slice slice to the byte array
func (p *Packer) PackFixedByteSlices(byteSlices [][]byte) {
	p.PackInt(uint32(len(byteSlices)))
	for _, bytes := range byteSlices {
		p.PackFixedBytes(bytes)
	}
}

// UnpackFixedByteSlices returns a byte slice slice from the byte array.
// Each byte slice has the specified size. The number of byte slices is
// read from the byte array.
func (p *Packer) UnpackFixedByteSlices(size int) [][]byte {
	sliceSize := p.UnpackInt()
	bytes := [][]byte(nil)
	for i := uint32(0); i < sliceSize && !p.Errored(); i++ {
		bytes = append(bytes, p.UnpackFixedBytes(size))
	}
	return bytes
}

// Pack2DByteSlice append a 2D byte slice to the byte array
func (p *Packer) Pack2DByteSlice(byteSlices [][]byte) {
	p.PackInt(uint32(len(byteSlices)))
	for _, bytes := range byteSlices {
		p.PackBytes(bytes)
	}
}

// Unpack2DByteSlice returns a 2D byte slice from the byte array.
func (p *Packer) Unpack2DByteSlice() [][]byte {
	sliceSize := p.UnpackInt()
	bytes := [][]byte(nil)
	for i := uint32(0); i < sliceSize && !p.Errored(); i++ {
		bytes = append(bytes, p.UnpackBytes())
	}
	return bytes
}

// PackStr append a string to the byte array
func (p *Packer) PackStr(str string) {
	strSize := len(str)
	if strSize > MaxStringLen {
		p.Add(errInvalidInput)
	}
	p.PackShort(uint16(strSize))
	p.PackFixedBytes([]byte(str))
}

// UnpackStr unpacks a string from the byte array
func (p *Packer) UnpackStr() string {
	strSize := p.UnpackShort()
	return string(p.UnpackFixedBytes(int(strSize)))
}

// PackIP packs an ip port pair to the byte array
func (p *Packer) PackIP(ip utils.IPDesc) {
	p.PackFixedBytes(ip.IP.To16())
	p.PackShort(ip.Port)
}

// UnpackIP unpacks an ip port pair from the byte array
func (p *Packer) UnpackIP() utils.IPDesc {
	ip := p.UnpackFixedBytes(16)
	port := p.UnpackShort()
	return utils.IPDesc{
		IP:   ip,
		Port: port,
	}
}

// PackIPs unpacks an ip port pair slice from the byte array
func (p *Packer) PackIPs(ips []utils.IPDesc) {
	p.PackInt(uint32(len(ips)))
	for i := 0; i < len(ips) && !p.Errored(); i++ {
		p.PackIP(ips[i])
	}
}

// UnpackIPs unpacks an ip port pair slice from the byte array
func (p *Packer) UnpackIPs() []utils.IPDesc {
	sliceSize := p.UnpackInt()
	ips := []utils.IPDesc(nil)
	for i := uint32(0); i < sliceSize && !p.Errored(); i++ {
		ips = append(ips, p.UnpackIP())
	}
	return ips
}

// TryPackByte attempts to pack the value as a byte
func TryPackByte(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.(uint8); ok {
		packer.PackByte(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackByte attempts to unpack a value as a byte
func TryUnpackByte(packer *Packer) interface{} {
	return packer.UnpackByte()
}

// TryPackShort attempts to pack the value as a short
func TryPackShort(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.(uint16); ok {
		packer.PackShort(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackShort attempts to unpack a value as a short
func TryUnpackShort(packer *Packer) interface{} {
	return packer.UnpackShort()
}

// TryPackInt attempts to pack the value as an int
func TryPackInt(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.(uint32); ok {
		packer.PackInt(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackInt attempts to unpack a value as an int
func TryUnpackInt(packer *Packer) interface{} {
	return packer.UnpackInt()
}

// TryPackLong attempts to pack the value as a long
func TryPackLong(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.(uint64); ok {
		packer.PackLong(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackLong attempts to unpack a value as a long
func TryUnpackLong(packer *Packer) interface{} {
	return packer.UnpackLong()
}

// TryPackHash attempts to pack the value as a 32-byte sequence
func TryPackHash(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.([]byte); ok {
		packer.PackFixedBytes(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackHash attempts to unpack the value as a 32-byte sequence
func TryUnpackHash(packer *Packer) interface{} {
	return packer.UnpackFixedBytes(hashing.HashLen)
}

// TryPackHashes attempts to pack the value as a list of 32-byte sequences
func TryPackHashes(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.([][]byte); ok {
		packer.PackFixedByteSlices(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackHashes attempts to unpack the value as a list of 32-byte sequences
func TryUnpackHashes(packer *Packer) interface{} {
	return packer.UnpackFixedByteSlices(hashing.HashLen)
}

// TryPackAddr attempts to pack the value as a 20-byte sequence
func TryPackAddr(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.([]byte); ok {
		packer.PackFixedBytes(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackAddr attempts to unpack the value as a 20-byte sequence
func TryUnpackAddr(packer *Packer) interface{} {
	return packer.UnpackFixedBytes(hashing.AddrLen)
}

// TryPackAddrList attempts to pack the value as a list of 20-byte sequences
func TryPackAddrList(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.([][]byte); ok {
		packer.PackFixedByteSlices(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackAddrList attempts to unpack the value as a list of 20-byte sequences
func TryUnpackAddrList(packer *Packer) interface{} {
	return packer.UnpackFixedByteSlices(hashing.AddrLen)
}

// TryPackBytes attempts to pack the value as a list of bytes
func TryPackBytes(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.([]byte); ok {
		packer.PackBytes(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackBytes attempts to unpack the value as a list of bytes
func TryUnpackBytes(packer *Packer) interface{} {
	return packer.UnpackBytes()
}

// TryPack2DBytes attempts to pack the value as a 2D byte slice
func TryPack2DBytes(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.([][]byte); ok {
		packer.Pack2DByteSlice(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpack2DBytes attempts to unpack the value as a 2D byte slice
func TryUnpack2DBytes(packer *Packer) interface{} {
	return packer.Unpack2DByteSlice()
}

// TryPackStr attempts to pack the value as a string
func TryPackStr(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.(string); ok {
		packer.PackStr(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackStr attempts to unpack the value as a string
func TryUnpackStr(packer *Packer) interface{} {
	return packer.UnpackStr()
}

// TryPackIP attempts to pack the value as an ip port pair
func TryPackIP(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.(utils.IPDesc); ok {
		packer.PackIP(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackIP attempts to unpack the value as an ip port pair
func TryUnpackIP(packer *Packer) interface{} {
	return packer.UnpackIP()
}

// TryPackIPList attempts to pack the value as an ip port pair list
func TryPackIPList(packer *Packer, valIntf interface{}) {
	if val, ok := valIntf.([]utils.IPDesc); ok {
		packer.PackIPs(val)
	} else {
		packer.Add(errBadType)
	}
}

// TryUnpackIPList attempts to unpack the value as an ip port pair list
func TryUnpackIPList(packer *Packer) interface{} {
	return packer.UnpackIPs()
}

// PackLong returns the byte representation of a uint64
func PackLong(val uint64) []byte {
	bytes := make([]byte, 8)
	binary.BigEndian.PutUint64(bytes, val)
	return bytes
}

// PackLong returns the byte representation of a uint64.
// Returns an error if len(bytes) != 8
func UnpackLong(bytes []byte) (uint64, error) {
	if len(bytes) != 8 {
		return 0, fmt.Errorf("expected len(bytes) to be 8 but is %d", len(bytes))
	}
	return binary.BigEndian.Uint64(bytes), nil
}
