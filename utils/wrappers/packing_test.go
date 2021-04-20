// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wrappers

import (
	"bytes"
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	ByteSentinal  = 0
	ShortSentinal = 0
	IntSentinal   = 0
	LongSentinal  = 0
	BoolSentinal  = false
)

func TestPackerCheckSpace(t *testing.T) {
	p := Packer{Offset: -1}
	p.CheckSpace(1)
	if !p.Errored() {
		t.Fatal("Expected errNegativeOffset")
	}

	p = Packer{}
	p.CheckSpace(-1)
	if !p.Errored() {
		t.Fatal("Expected errInvalidInput")
	}

	p = Packer{Bytes: []byte{0x01}, Offset: 1}
	p.CheckSpace(1)
	if !p.Errored() {
		t.Fatal("Expected errBadLength")
	}

	p = Packer{Bytes: []byte{0x01}, Offset: 2}
	p.CheckSpace(0)
	if !p.Errored() {
		t.Fatal("Expected errBadLength, due to out of bounds offset")
	}
}

func TestPackerExpand(t *testing.T) {
	p := Packer{Bytes: []byte{0x01}, Offset: 2}
	p.Expand(1)
	if !p.Errored() {
		t.Fatal("packer.Expand didn't notice packer had out of bounds offset")
	}

	p = Packer{Bytes: []byte{0x01, 0x02, 0x03}, Offset: 0}
	p.Expand(1)
	if p.Errored() {
		t.Fatalf("packer.Expand unexpectedly had error %s", p.Err)
	} else if len(p.Bytes) != 3 {
		t.Fatalf("packer.Expand modified byte array, when it didn't need to")
	}
}

func TestPackerPackByte(t *testing.T) {
	p := Packer{MaxSize: 1}

	p.PackByte(0x01)

	if p.Errored() {
		t.Fatal(p.Err)
	}

	if size := len(p.Bytes); size != 1 {
		t.Fatalf("Packer.PackByte wrote %d byte(s) but expected %d byte(s)", size, 1)
	}

	expected := []byte{0x01}
	if !bytes.Equal(p.Bytes, expected) {
		t.Fatalf("Packer.PackByte wrote:\n%v\nExpected:\n%v", p.Bytes, expected)
	}

	p.PackByte(0x02)
	if !p.Errored() {
		t.Fatal("Packer.PackByte did not fail when attempt was beyond p.MaxSize")
	}
}

func TestPackerUnpackByte(t *testing.T) {
	var (
		p                = Packer{Bytes: []byte{0x01}, Offset: 0}
		actual           = p.UnpackByte()
		expected    byte = 1
		expectedLen      = ByteLen
	)
	switch {
	case p.Errored():
		t.Fatalf("Packer.UnpackByte unexpectedly raised %s", p.Err)
	case actual != expected:
		t.Fatalf("Packer.UnpackByte returned %d, but expected %d", actual, expected)
	case p.Offset != expectedLen:
		t.Fatalf("Packer.UnpackByte left Offset %d, expected %d", p.Offset, expectedLen)
	}

	actual = p.UnpackByte()
	if !p.Errored() {
		t.Fatalf("Packer.UnpackByte should have set error, due to attempted out of bounds read")
	} else if actual != ByteSentinal {
		t.Fatalf("Packer.UnpackByte returned %d, expected sentinal value %d", actual, ByteSentinal)
	}
}

func TestPackerPackShort(t *testing.T) {
	p := Packer{MaxSize: 2}

	p.PackShort(0x0102)

	if p.Errored() {
		t.Fatal(p.Err)
	}

	if size := len(p.Bytes); size != 2 {
		t.Fatalf("Packer.PackShort wrote %d byte(s) but expected %d byte(s)", size, 2)
	}

	expected := []byte{0x01, 0x02}
	if !bytes.Equal(p.Bytes, expected) {
		t.Fatalf("Packer.PackShort wrote:\n%v\nExpected:\n%v", p.Bytes, expected)
	}
}

func TestPackerUnpackShort(t *testing.T) {
	var (
		p                  = Packer{Bytes: []byte{0x01, 0x02}, Offset: 0}
		actual             = p.UnpackShort()
		expected    uint16 = 0x0102
		expectedLen        = ShortLen
	)

	switch {
	case p.Errored():
		t.Fatalf("Packer.UnpackShort unexpectedly raised %s", p.Err)
	case actual != expected:
		t.Fatalf("Packer.UnpackShort returned %d, but expected %d", actual, expected)
	case p.Offset != expectedLen:
		t.Fatalf("Packer.UnpackShort left Offset %d, expected %d", p.Offset, expectedLen)
	}

	actual = p.UnpackShort()
	if !p.Errored() {
		t.Fatalf("Packer.UnpackShort should have set error, due to attempted out of bounds read")
	} else if actual != ShortSentinal {
		t.Fatalf("Packer.UnpackShort returned %d, expected sentinal value %d", actual, ShortSentinal)
	}
}

func TestPackerPackInt(t *testing.T) {
	p := Packer{MaxSize: 4}

	p.PackInt(0x01020304)

	if p.Errored() {
		t.Fatal(p.Err)
	}

	if size := len(p.Bytes); size != 4 {
		t.Fatalf("Packer.PackInt wrote %d byte(s) but expected %d byte(s)", size, 4)
	}

	expected := []byte{0x01, 0x02, 0x03, 0x04}
	if !bytes.Equal(p.Bytes, expected) {
		t.Fatalf("Packer.PackInt wrote:\n%v\nExpected:\n%v", p.Bytes, expected)
	}

	p.PackInt(0x05060708)
	if !p.Errored() {
		t.Fatal("Packer.PackInt did not fail when attempt was beyond p.MaxSize")
	}
}

func TestPackerUnpackInt(t *testing.T) {
	var (
		p                  = Packer{Bytes: []byte{0x01, 0x02, 0x03, 0x04}, Offset: 0}
		actual             = p.UnpackInt()
		expected    uint32 = 0x01020304
		expectedLen        = IntLen
	)

	switch {
	case p.Errored():
		t.Fatalf("Packer.UnpackInt unexpectedly raised %s", p.Err)
	case actual != expected:
		t.Fatalf("Packer.UnpackInt returned %d, but expected %d", actual, expected)
	case p.Offset != expectedLen:
		t.Fatalf("Packer.UnpackInt left Offset %d, expected %d", p.Offset, expectedLen)
	}

	actual = p.UnpackInt()
	if !p.Errored() {
		t.Fatalf("Packer.UnpackInt should have set error, due to attempted out of bounds read")
	} else if actual != IntSentinal {
		t.Fatalf("Packer.UnpackInt returned %d, expected sentinal value %d", actual, IntSentinal)
	}
}

func TestPackerPackLong(t *testing.T) {
	p := Packer{MaxSize: 8}

	p.PackLong(0x0102030405060708)

	if p.Errored() {
		t.Fatal(p.Err)
	}

	if size := len(p.Bytes); size != 8 {
		t.Fatalf("Packer.PackLong wrote %d byte(s) but expected %d byte(s)", size, 8)
	}

	expected := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	if !bytes.Equal(p.Bytes, expected) {
		t.Fatalf("Packer.PackLong wrote:\n%v\nExpected:\n%v", p.Bytes, expected)
	}

	p.PackLong(0x090a0b0c0d0e0f00)
	if !p.Errored() {
		t.Fatal("Packer.PackLong did not fail when attempt was beyond p.MaxSize")
	}
}

func TestPackerUnpackLong(t *testing.T) {
	var (
		p                  = Packer{Bytes: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}, Offset: 0}
		actual             = p.UnpackLong()
		expected    uint64 = 0x0102030405060708
		expectedLen        = LongLen
	)

	switch {
	case p.Errored():
		t.Fatalf("Packer.UnpackLong unexpectedly raised %s", p.Err)
	case actual != expected:
		t.Fatalf("Packer.UnpackLong returned %d, but expected %d", actual, expected)
	case p.Offset != expectedLen:
		t.Fatalf("Packer.UnpackLong left Offset %d, expected %d", p.Offset, expectedLen)
	}

	actual = p.UnpackLong()
	if !p.Errored() {
		t.Fatalf("Packer.UnpackLong should have set error, due to attempted out of bounds read")
	} else if actual != LongSentinal {
		t.Fatalf("Packer.UnpackLong returned %d, expected sentinal value %d", actual, LongSentinal)
	}
}

func TestPackerPackFixedBytes(t *testing.T) {
	p := Packer{MaxSize: 4}

	p.PackFixedBytes([]byte("Avax"))

	if p.Errored() {
		t.Fatal(p.Err)
	}

	if size := len(p.Bytes); size != 4 {
		t.Fatalf("Packer.PackFixedBytes wrote %d byte(s) but expected %d byte(s)", size, 4)
	}

	expected := []byte("Avax")
	if !bytes.Equal(p.Bytes, expected) {
		t.Fatalf("Packer.PackFixedBytes wrote:\n%v\nExpected:\n%v", p.Bytes, expected)
	}

	p.PackFixedBytes([]byte("Avax"))
	if !p.Errored() {
		t.Fatal("Packer.PackFixedBytes did not fail when attempt was beyond p.MaxSize")
	}
}

func TestPackerUnpackFixedBytes(t *testing.T) {
	var (
		p           = Packer{Bytes: []byte("Avax")}
		actual      = p.UnpackFixedBytes(4)
		expected    = []byte("Avax")
		expectedLen = 4
	)

	switch {
	case p.Errored():
		t.Fatalf("Packer.UnpackFixedBytes unexpectedly raised %s", p.Err)
	case !bytes.Equal(actual, expected):
		t.Fatalf("Packer.UnpackFixedBytes returned %d, but expected %d", actual, expected)
	case p.Offset != expectedLen:
		t.Fatalf("Packer.UnpackFixedBytes left Offset %d, expected %d", p.Offset, expectedLen)
	}

	actual = p.UnpackFixedBytes(4)
	if !p.Errored() {
		t.Fatalf("Packer.UnpackFixedBytes should have set error, due to attempted out of bounds read")
	} else if actual != nil {
		t.Fatalf("Packer.UnpackFixedBytes returned %v, expected sentinal value %v", actual, nil)
	}
}

func TestPackerPackBytes(t *testing.T) {
	p := Packer{MaxSize: 8}

	p.PackBytes([]byte("Avax"))

	if p.Errored() {
		t.Fatal(p.Err)
	}

	if size := len(p.Bytes); size != 8 {
		t.Fatalf("Packer.PackBytes wrote %d byte(s) but expected %d byte(s)", size, 7)
	}

	expected := []byte("\x00\x00\x00\x04Avax")
	if !bytes.Equal(p.Bytes, expected) {
		t.Fatalf("Packer.PackBytes wrote:\n%v\nExpected:\n%v", p.Bytes, expected)
	}

	p.PackBytes([]byte("Avax"))
	if !p.Errored() {
		t.Fatal("Packer.PackBytes did not fail when attempt was beyond p.MaxSize")
	}
}

func TestPackerUnpackBytes(t *testing.T) {
	var (
		p           = Packer{Bytes: []byte("\x00\x00\x00\x04Avax")}
		actual      = p.UnpackBytes()
		expected    = []byte("Avax")
		expectedLen = 8
	)

	switch {
	case p.Errored():
		t.Fatalf("Packer.UnpackBytes unexpectedly raised %s", p.Err)
	case !bytes.Equal(actual, expected):
		t.Fatalf("Packer.UnpackBytes returned %d, but expected %d", actual, expected)
	case p.Offset != expectedLen:
		t.Fatalf("Packer.UnpackBytes left Offset %d, expected %d", p.Offset, expectedLen)
	}

	actual = p.UnpackBytes()
	if !p.Errored() {
		t.Fatalf("Packer.UnpackBytes should have set error, due to attempted out of bounds read")
	} else if actual != nil {
		t.Fatalf("Packer.UnpackBytes returned %v, expected sentinal value %v", actual, nil)
	}
}

func TestPackerPackFixedByteSlices(t *testing.T) {
	p := Packer{MaxSize: 12}

	p.PackFixedByteSlices([][]byte{[]byte("Avax"), []byte("Evax")})

	if p.Errored() {
		t.Fatal(p.Err)
	}

	if size := len(p.Bytes); size != 12 {
		t.Fatalf("Packer.PackFixedByteSlices wrote %d byte(s) but expected %d byte(s)", size, 12)
	}

	expected := []byte("\x00\x00\x00\x02AvaxEvax")
	if !bytes.Equal(p.Bytes, expected) {
		t.Fatalf("Packer.PackPackFixedByteSlicesBytes wrote:\n%v\nExpected:\n%v", p.Bytes, expected)
	}

	p.PackFixedByteSlices([][]byte{[]byte("Avax"), []byte("Evax")})
	if !p.Errored() {
		t.Fatal("Packer.PackFixedByteSlices did not fail when attempt was beyond p.MaxSize")
	}
}

func TestPackerUnpackFixedByteSlices(t *testing.T) {
	var (
		p           = Packer{Bytes: []byte("\x00\x00\x00\x02AvaxEvax")}
		actual      = p.UnpackFixedByteSlices(4)
		expected    = [][]byte{[]byte("Avax"), []byte("Evax")}
		expectedLen = 12
	)

	switch {
	case p.Errored():
		t.Fatalf("Packer.UnpackFixedByteSlices unexpectedly raised %s", p.Err)
	case !reflect.DeepEqual(actual, expected):
		t.Fatalf("Packer.UnpackFixedByteSlices returned %d, but expected %d", actual, expected)
	case p.Offset != expectedLen:
		t.Fatalf("Packer.UnpackFixedByteSlices left Offset %d, expected %d", p.Offset, expectedLen)
	}

	actual = p.UnpackFixedByteSlices(4)
	if !p.Errored() {
		t.Fatalf("Packer.UnpackFixedByteSlices should have set error, due to attempted out of bounds read")
	} else if actual != nil {
		t.Fatalf("Packer.UnpackFixedByteSlices returned %v, expected sentinal value %v", actual, nil)
	}
}

func TestPackerString(t *testing.T) {
	p := Packer{MaxSize: 6}

	p.PackStr("Avax")

	if p.Errored() {
		t.Fatal(p.Err)
	}

	if size := len(p.Bytes); size != 6 {
		t.Fatalf("Packer.PackStr wrote %d byte(s) but expected %d byte(s)", size, 5)
	}

	expected := []byte{0x00, 0x04, 0x41, 0x76, 0x61, 0x78}
	if !bytes.Equal(p.Bytes, expected) {
		t.Fatalf("Packer.PackStr wrote:\n%v\nExpected:\n%v", p.Bytes, expected)
	}
}

func TestPacker(t *testing.T) {
	packer := Packer{
		MaxSize: 3,
	}

	if packer.Errored() {
		t.Fatalf("Packer has error %s", packer.Err)
	}

	packer.PackShort(17)
	if len(packer.Bytes) != 2 {
		t.Fatalf("Wrong byte length")
	}

	packer.PackShort(1)
	if !packer.Errored() {
		t.Fatalf("Packer should have error")
	}

	newPacker := Packer{
		Bytes: packer.Bytes,
	}

	if newPacker.UnpackShort() != 17 {
		t.Fatalf("Unpacked wrong value")
	}
}

func TestPackBool(t *testing.T) {
	p := Packer{MaxSize: 3}
	p.PackBool(false)
	p.PackBool(true)
	p.PackBool(false)
	if p.Errored() {
		t.Fatal("should have been able to pack 3 bools")
	}

	p2 := Packer{Bytes: p.Bytes}
	bool1, bool2, bool3 := p2.UnpackBool(), p2.UnpackBool(), p2.UnpackBool()

	if p.Errored() {
		t.Fatalf("errors while unpacking bools: %v", p.Errs)
	}

	if bool1 || !bool2 || bool3 {
		t.Fatal("got back wrong values")
	}
}

func TestPackerPackBool(t *testing.T) {
	p := Packer{MaxSize: 1}

	p.PackBool(true)

	if p.Errored() {
		t.Fatal(p.Err)
	}

	if size := len(p.Bytes); size != 1 {
		t.Fatalf("Packer.PackBool wrote %d byte(s) but expected %d byte(s)", size, 1)
	}

	expected := []byte{0x01}
	if !bytes.Equal(p.Bytes, expected) {
		t.Fatalf("Packer.PackBool wrote:\n%v\nExpected:\n%v", p.Bytes, expected)
	}

	p.PackBool(false)
	if !p.Errored() {
		t.Fatal("Packer.PackLong did not fail when attempt was beyond p.MaxSize")
	}
}

func TestPackerUnpackBool(t *testing.T) {
	var (
		p                = Packer{Bytes: []byte{0x01}, Offset: 0}
		actual           = p.UnpackBool()
		expected    bool = true
		expectedLen      = BoolLen
	)

	switch {
	case p.Errored():
		t.Fatalf("Packer.UnpackBool unexpectedly raised %s", p.Err)
	case actual != expected:
		t.Fatalf("Packer.UnpackBool returned %t, but expected %t", actual, expected)
	case p.Offset != expectedLen:
		t.Fatalf("Packer.UnpackBool left Offset %d, expected %d", p.Offset, expectedLen)
	}

	actual = p.UnpackBool()
	if !p.Errored() {
		t.Fatalf("Packer.UnpackBool should have set error, due to attempted out of bounds read")
	} else if actual != BoolSentinal {
		t.Fatalf("Packer.UnpackBool returned %t, expected sentinal value %t", actual, BoolSentinal)
	}

	p = Packer{Bytes: []byte{0x42}, Offset: 0}
	expected = false
	actual = p.UnpackBool()
	if !p.Errored() {
		t.Fatalf("Packer.UnpackBool id not raise error for invalid boolean value %v", p.Bytes)
	} else if actual != expected {
		t.Fatalf("Packer.UnpackBool returned %t, expected sentinal value %t", actual, BoolSentinal)
	}
}

func TestPacker2DByteSlice(t *testing.T) {
	// Case: empty array
	p := Packer{MaxSize: 1024}
	arr := [][]byte{}
	p.Pack2DByteSlice(arr)
	if p.Errored() {
		t.Fatal(p.Err)
	}
	arrUnpacked := p.Unpack2DByteSlice()
	if len(arrUnpacked) != 0 {
		t.Fatal("should be empty")
	}

	// Case: Array has one element
	p = Packer{MaxSize: 1024}
	arr = [][]byte{
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}
	p.Pack2DByteSlice(arr)
	if p.Errored() {
		t.Fatal(p.Err)
	}
	p = Packer{MaxSize: 1024, Bytes: p.Bytes}
	arrUnpacked = p.Unpack2DByteSlice()
	if p.Errored() {
		t.Fatal(p.Err)
	}
	if l := len(arrUnpacked); l != 1 {
		t.Fatalf("should be length 1 but is length %d", l)
	}
	if !bytes.Equal(arrUnpacked[0], arr[0]) {
		t.Fatal("should match")
	}

	// Case: Array has multiple elements
	p = Packer{MaxSize: 1024}
	arr = [][]byte{
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		{11, 12, 3, 4, 5, 6, 7, 8, 9, 10},
	}
	p.Pack2DByteSlice(arr)
	if p.Errored() {
		t.Fatal(p.Err)
	}
	p = Packer{MaxSize: 1024, Bytes: p.Bytes}
	arrUnpacked = p.Unpack2DByteSlice()
	if p.Errored() {
		t.Fatal(p.Err)
	}
	if l := len(arrUnpacked); l != 2 {
		t.Fatalf("should be length 1 but is length %d", l)
	}
	if !bytes.Equal(arrUnpacked[0], arr[0]) {
		t.Fatal("should match")
	}
	if !bytes.Equal(arrUnpacked[1], arr[1]) {
		t.Fatal("should match")
	}
}

func TestPackLong(t *testing.T) {
	for _, n := range []uint64{0, 10000, math.MaxUint64} {
		bytes := PackLong(n)
		got, err := UnpackLong(bytes)
		assert.NoError(t, err)
		assert.Equal(t, n, got)
	}
}

func TestUnpackLong(t *testing.T) {
	// Too few bytes
	bytes := make([]byte, 7)
	_, err := UnpackLong(bytes)
	assert.Error(t, err)

	// Too many bytes
	bytes = make([]byte, 9)
	_, err = UnpackLong(bytes)
	assert.Error(t, err)

	// Right number of bytes
	bytes = make([]byte, 8)
	n, err := UnpackLong(bytes)
	assert.NoError(t, err)
	assert.EqualValues(t, 0, n)
}
