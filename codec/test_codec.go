// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package codec

import (
	"bytes"
	"math"
	"reflect"
	"testing"

	"github.com/ava-labs/avalanchego/utils/wrappers"
)

var (
	Tests = []func(c GeneralCodec, t testing.TB){
		TestStruct,
		TestRegisterStructTwice,
		TestUInt32,
		TestSlice,
		TestMaxSizeSlice,
		TestBool,
		TestArray,
		TestBigArray,
		TestPointerToStruct,
		TestSliceOfStruct,
		TestInterface,
		TestSliceOfInterface,
		TestArrayOfInterface,
		TestPointerToInterface,
		TestString,
		TestNilSlice,
		TestSerializeUnexportedField,
		TestSerializeOfNoSerializeField,
		TestNilSliceSerialization,
		TestEmptySliceSerialization,
		TestSliceWithEmptySerialization,
		TestRestrictedSlice,
		TestExtraSpace,
		TestSliceLengthOverflow,
	}
)

// The below structs and interfaces exist
// for the sake of testing

type Foo interface {
	Foo() int
}

// *MyInnerStruct implements Foo
type MyInnerStruct struct {
	Str string `serialize:"true"`
}

func (m *MyInnerStruct) Foo() int {
	return 1
}

// *MyInnerStruct2 implements Foo
type MyInnerStruct2 struct {
	Bool bool `serialize:"true"`
}

func (m *MyInnerStruct2) Foo() int {
	return 2
}

// MyInnerStruct3 embeds Foo, an interface,
// so it has to implement TypeID and ConcreteInstance
type MyInnerStruct3 struct {
	Str string        `serialize:"true"`
	M1  MyInnerStruct `serialize:"true"`
	F   Foo           `serialize:"true"`
}

type myStruct struct {
	InnerStruct  MyInnerStruct      `serialize:"true"`
	InnerStruct2 *MyInnerStruct     `serialize:"true"`
	Member1      int64              `serialize:"true"`
	Member2      uint16             `serialize:"true"`
	MyArray2     [5]string          `serialize:"true"`
	MyArray3     [3]MyInnerStruct   `serialize:"true"`
	MyArray4     [2]*MyInnerStruct2 `serialize:"true"`
	MySlice      []byte             `serialize:"true"`
	MySlice2     []string           `serialize:"true"`
	MySlice3     []MyInnerStruct    `serialize:"true"`
	MySlice4     []*MyInnerStruct2  `serialize:"true"`
	MyArray      [4]byte            `serialize:"true"`
	MyInterface  Foo                `serialize:"true"`
	MySlice5     []Foo              `serialize:"true"`
	InnerStruct3 MyInnerStruct3     `serialize:"true"`
	MyPointer    *Foo               `serialize:"true"`
}

// Test marshaling/unmarshaling a complicated struct
func TestStruct(codec GeneralCodec, t testing.TB) {
	temp := Foo(&MyInnerStruct{})
	myStructInstance := myStruct{
		InnerStruct:  MyInnerStruct{"hello"},
		InnerStruct2: &MyInnerStruct{"yello"},
		Member1:      1,
		Member2:      2,
		MySlice:      []byte{1, 2, 3, 4},
		MySlice2:     []string{"one", "two", "three"},
		MySlice3:     []MyInnerStruct{{"a"}, {"b"}, {"c"}},
		MySlice4:     []*MyInnerStruct2{{true}, {}},
		MySlice5:     []Foo{&MyInnerStruct2{true}, &MyInnerStruct2{}},
		MyArray:      [4]byte{5, 6, 7, 8},
		MyArray2:     [5]string{"four", "five", "six", "seven"},
		MyArray3:     [3]MyInnerStruct{{"d"}, {"e"}, {"f"}},
		MyArray4:     [2]*MyInnerStruct2{{}, {true}},
		MyInterface:  &MyInnerStruct{"yeet"},
		InnerStruct3: MyInnerStruct3{
			Str: "str",
			M1: MyInnerStruct{
				Str: "other str",
			},
			F: &MyInnerStruct2{},
		},
		MyPointer: &temp,
	}

	manager := NewDefaultManager()
	errs := wrappers.Errs{}
	errs.Add(
		codec.RegisterType(&MyInnerStruct{}), // Register the types that may be unmarshaled into interfaces
		codec.RegisterType(&MyInnerStruct2{}),
		manager.RegisterCodec(0, codec),
	)
	if errs.Errored() {
		t.Fatal(errs.Err)
	}

	myStructBytes, err := manager.Marshal(0, myStructInstance)
	if err != nil {
		t.Fatal(err)
	}

	myStructUnmarshaled := &myStruct{}
	version, err := manager.Unmarshal(myStructBytes, myStructUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(*myStructUnmarshaled, myStructInstance) {
		t.Fatal("should be same")
	}
}

func TestRegisterStructTwice(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	errs := wrappers.Errs{}
	errs.Add(
		codec.RegisterType(&MyInnerStruct{}),
		codec.RegisterType(&MyInnerStruct{}), // Register the same struct twice
	)
	if !errs.Errored() {
		t.Fatal("Registering the same struct twice should have caused an error")
	}
}

func TestUInt32(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	number := uint32(500)

	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	bytes, err := manager.Marshal(0, number)
	if err != nil {
		t.Fatal(err)
	}

	var numberUnmarshaled uint32
	version, err := manager.Unmarshal(bytes, &numberUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if number != numberUnmarshaled {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

func TestSlice(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	mySlice := []bool{true, false, true, true}
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	bytes, err := manager.Marshal(0, mySlice)
	if err != nil {
		t.Fatal(err)
	}

	var sliceUnmarshaled []bool
	version, err := manager.Unmarshal(bytes, &sliceUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(mySlice, sliceUnmarshaled) {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

// Test marshalling/unmarshalling largest possible slice
func TestMaxSizeSlice(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	mySlice := make([]string, math.MaxUint16)
	mySlice[0] = "first!"
	mySlice[math.MaxUint16-1] = "last!"
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}
	bytes, err := manager.Marshal(0, mySlice)
	if err != nil {
		t.Fatal(err)
	}

	var sliceUnmarshaled []string
	version, err := manager.Unmarshal(bytes, &sliceUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(mySlice, sliceUnmarshaled) {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

// Test marshalling a bool
func TestBool(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	myBool := true
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}
	bytes, err := manager.Marshal(0, myBool)
	if err != nil {
		t.Fatal(err)
	}

	var boolUnmarshaled bool
	version, err := manager.Unmarshal(bytes, &boolUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(myBool, boolUnmarshaled) {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

// Test marshalling an array
func TestArray(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	myArr := [5]uint64{5, 6, 7, 8, 9}
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}
	bytes, err := manager.Marshal(0, myArr)
	if err != nil {
		t.Fatal(err)
	}

	var myArrUnmarshaled [5]uint64
	version, err := manager.Unmarshal(bytes, &myArrUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(myArr, myArrUnmarshaled) {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

// Test marshalling a really big array
func TestBigArray(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	myArr := [30000]uint64{5, 6, 7, 8, 9}
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}
	bytes, err := manager.Marshal(0, myArr)
	if err != nil {
		t.Fatal(err)
	}

	var myArrUnmarshaled [30000]uint64
	version, err := manager.Unmarshal(bytes, &myArrUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(myArr, myArrUnmarshaled) {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

// Test marshalling a pointer to a struct
func TestPointerToStruct(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	myPtr := &MyInnerStruct{Str: "Hello!"}
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}
	bytes, err := manager.Marshal(0, myPtr)
	if err != nil {
		t.Fatal(err)
	}

	var myPtrUnmarshaled *MyInnerStruct
	version, err := manager.Unmarshal(bytes, &myPtrUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(myPtr, myPtrUnmarshaled) {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

// Test marshalling a slice of structs
func TestSliceOfStruct(codec GeneralCodec, t testing.TB) {
	mySlice := []MyInnerStruct3{
		{
			Str: "One",
			M1:  MyInnerStruct{"Two"},
			F:   &MyInnerStruct{"Three"},
		},
		{
			Str: "Four",
			M1:  MyInnerStruct{"Five"},
			F:   &MyInnerStruct{"Six"},
		},
	}
	if err := codec.RegisterType(&MyInnerStruct{}); err != nil {
		t.Fatal(err)
	}
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}
	bytes, err := manager.Marshal(0, mySlice)
	if err != nil {
		t.Fatal(err)
	}

	var mySliceUnmarshaled []MyInnerStruct3
	version, err := manager.Unmarshal(bytes, &mySliceUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(mySlice, mySliceUnmarshaled) {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

// Test marshalling an interface
func TestInterface(codec GeneralCodec, t testing.TB) {
	if err := codec.RegisterType(&MyInnerStruct2{}); err != nil {
		t.Fatal(err)
	}
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	var f Foo = &MyInnerStruct2{true}
	bytes, err := manager.Marshal(0, &f)
	if err != nil {
		t.Fatal(err)
	}

	var unmarshaledFoo Foo
	version, err := manager.Unmarshal(bytes, &unmarshaledFoo)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(f, unmarshaledFoo) {
		t.Fatal("expected unmarshaled value to match original")
	}
}

// Test marshalling a slice of interfaces
func TestSliceOfInterface(codec GeneralCodec, t testing.TB) {
	mySlice := []Foo{
		&MyInnerStruct{
			Str: "Hello",
		},
		&MyInnerStruct{
			Str: ", World!",
		},
	}
	if err := codec.RegisterType(&MyInnerStruct{}); err != nil {
		t.Fatal(err)
	}
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}
	bytes, err := manager.Marshal(0, mySlice)
	if err != nil {
		t.Fatal(err)
	}

	var mySliceUnmarshaled []Foo
	version, err := manager.Unmarshal(bytes, &mySliceUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(mySlice, mySliceUnmarshaled) {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

// Test marshalling an array of interfaces
func TestArrayOfInterface(codec GeneralCodec, t testing.TB) {
	myArray := [2]Foo{
		&MyInnerStruct{
			Str: "Hello",
		},
		&MyInnerStruct{
			Str: ", World!",
		},
	}
	if err := codec.RegisterType(&MyInnerStruct{}); err != nil {
		t.Fatal(err)
	}
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}
	bytes, err := manager.Marshal(0, myArray)
	if err != nil {
		t.Fatal(err)
	}

	var myArrayUnmarshaled [2]Foo
	version, err := manager.Unmarshal(bytes, &myArrayUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(myArray, myArrayUnmarshaled) {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

// Test marshalling a pointer to an interface
func TestPointerToInterface(codec GeneralCodec, t testing.TB) {
	var myinnerStruct Foo = &MyInnerStruct{Str: "Hello!"}
	var myPtr *Foo = &myinnerStruct

	if err := codec.RegisterType(&MyInnerStruct{}); err != nil {
		t.Fatal(err)
	}
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	bytes, err := manager.Marshal(0, &myPtr)
	if err != nil {
		t.Fatal(err)
	}

	var myPtrUnmarshaled *Foo
	version, err := manager.Unmarshal(bytes, &myPtrUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(myPtr, myPtrUnmarshaled) {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

// Test marshalling a string
func TestString(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	myString := "Ayy"
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	bytes, err := manager.Marshal(0, myString)
	if err != nil {
		t.Fatal(err)
	}

	var stringUnmarshaled string
	version, err := manager.Unmarshal(bytes, &stringUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(myString, stringUnmarshaled) {
		t.Fatal("expected marshaled and unmarshaled values to match")
	}
}

// Ensure a nil slice is unmarshaled to slice with length 0
func TestNilSlice(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	type structWithSlice struct {
		Slice []byte `serialize:"true"`
	}

	myStruct := structWithSlice{Slice: nil}
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	bytes, err := manager.Marshal(0, myStruct)
	if err != nil {
		t.Fatal(err)
	}

	var structUnmarshaled structWithSlice
	version, err := manager.Unmarshal(bytes, &structUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if len(structUnmarshaled.Slice) != 0 {
		t.Fatal("expected slice to have length 0")
	}
}

// Ensure that trying to serialize a struct with an unexported member
// that has `serialize:"true"` returns error
func TestSerializeUnexportedField(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	type s struct {
		ExportedField   string `serialize:"true"`
		unexportedField string `serialize:"true"`
	}

	myS := s{
		ExportedField:   "Hello, ",
		unexportedField: "world!",
	}

	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Marshal(0, myS); err == nil {
		t.Fatalf("expected err but got none")
	}
}

func TestSerializeOfNoSerializeField(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	type s struct {
		SerializedField   string `serialize:"true"`
		UnserializedField string `serialize:"false"`
		UnmarkedField     string
	}
	myS := s{
		SerializedField:   "Serialize me",
		UnserializedField: "Do not serialize me",
		UnmarkedField:     "No declared serialize",
	}
	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	marshalled, err := manager.Marshal(0, myS)
	if err != nil {
		t.Fatal(err)
	}

	unmarshalled := s{}
	version, err := manager.Unmarshal(marshalled, &unmarshalled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	expectedUnmarshalled := s{SerializedField: "Serialize me"}
	if !reflect.DeepEqual(unmarshalled, expectedUnmarshalled) {
		t.Fatalf("Got %#v, expected %#v", unmarshalled, expectedUnmarshalled)
	}
}

// Test marshalling of nil slice
func TestNilSliceSerialization(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	type simpleSliceStruct struct {
		Arr []uint32 `serialize:"true"`
	}

	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	val := &simpleSliceStruct{}
	expected := []byte{0, 0, 0, 0, 0, 0} // 0 for codec version, then nil slice marshaled as 0 length slice
	result, err := manager.Marshal(0, val)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, result) {
		t.Fatalf("\nExpected: 0x%x\nResult:   0x%x", expected, result)
	}

	valUnmarshaled := &simpleSliceStruct{}
	version, err := manager.Unmarshal(result, &valUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if len(valUnmarshaled.Arr) != 0 {
		t.Fatal("should be 0 length")
	}
}

// Test marshaling a slice that has 0 elements (but isn't nil)
func TestEmptySliceSerialization(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	type simpleSliceStruct struct {
		Arr []uint32 `serialize:"true"`
	}

	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	val := &simpleSliceStruct{Arr: make([]uint32, 0, 1)}
	expected := []byte{0, 0, 0, 0, 0, 0} // 0 for codec version (uint16) and 0 for size (uint32)
	result, err := manager.Marshal(0, val)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(expected, result) {
		t.Fatalf("\nExpected: 0x%x\nResult:   0x%x", expected, result)
	}

	valUnmarshaled := &simpleSliceStruct{}
	version, err := manager.Unmarshal(result, &valUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(valUnmarshaled, val) {
		t.Fatal("should be same")
	}
}

// Test marshaling slice that is not nil and not empty
func TestSliceWithEmptySerialization(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	type emptyStruct struct{}

	type nestedSliceStruct struct {
		Arr []emptyStruct `serialize:"true"`
	}

	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	val := &nestedSliceStruct{
		Arr: make([]emptyStruct, 1000),
	}
	expected := []byte{0x00, 0x00, 0x00, 0x00, 0x03, 0xE8} // codec version (0x00, 0x00) then 1000 for numElts
	result, err := manager.Marshal(0, val)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, result) {
		t.Fatalf("\nExpected: 0x%x\nResult:   0x%x", expected, result)
	}

	unmarshaled := nestedSliceStruct{}
	version, err := manager.Unmarshal(expected, &unmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if len(unmarshaled.Arr) != 1000 {
		t.Fatalf("Should have created a slice of length %d", 1000)
	}
}

func TestSliceWithEmptySerializationOutOfMemory(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	type emptyStruct struct{}

	type nestedSliceStruct struct {
		Arr []emptyStruct `serialize:"true"`
	}

	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	val := &nestedSliceStruct{
		Arr: make([]emptyStruct, math.MaxInt32),
	}
	bytes, err := manager.Marshal(0, val)
	if err == nil {
		t.Fatal("should have failed due to slice length too large")
	}

	unmarshaled := nestedSliceStruct{}
	if _, err := manager.Unmarshal(bytes, &unmarshaled); err == nil {
		t.Fatalf("Should have errored due to excess memory requested")
	}
}

func TestSliceTooLarge(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	val := []struct{}{}
	b := []byte{0x00, 0x00, 0xff, 0xff, 0xff, 0xff}
	if _, err := manager.Unmarshal(b, &val); err == nil {
		t.Fatalf("Should have errored due to memory usage")
	}
}

// Ensure serializing structs with negative number members works
func TestNegativeNumbers(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	type s struct {
		MyInt8  int8  `serialize:"true"`
		MyInt16 int16 `serialize:"true"`
		MyInt32 int32 `serialize:"true"`
		MyInt64 int64 `serialize:"true"`
	}

	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	myS := s{-1, -2, -3, -4}
	bytes, err := manager.Marshal(0, myS)
	if err != nil {
		t.Fatal(err)
	}

	mySUnmarshaled := s{}
	version, err := manager.Unmarshal(bytes, &mySUnmarshaled)
	if err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
	}

	if !reflect.DeepEqual(myS, mySUnmarshaled) {
		t.Log(mySUnmarshaled)
		t.Log(myS)
		t.Fatal("expected marshaled and unmarshaled structs to be the same")
	}
}

// Ensure deserializing structs with too many bytes errors correctly
func TestTooLargeUnmarshal(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	type inner struct {
		B uint16 `serialize:"true"`
	}
	bytes := []byte{0, 0, 0, 0}

	manager := NewManager(3)
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	s := inner{}
	_, err := manager.Unmarshal(bytes, &s)
	if err == nil {
		t.Fatalf("Should have errored due to too many bytes provided")
	}
}

type outerInterface interface {
	ToInt() int
}

type outer struct {
	Interface outerInterface `serialize:"true"`
}

type innerInterface struct{}

func (it *innerInterface) ToInt() int { return 0 }

type innerNoInterface struct{}

// Ensure deserializing structs into the wrong interface errors gracefully
func TestUnmarshalInvalidInterface(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	manager := NewDefaultManager()
	errs := wrappers.Errs{}
	errs.Add(
		codec.RegisterType(&innerInterface{}),
		codec.RegisterType(&innerNoInterface{}),
		manager.RegisterCodec(0, codec),
	)
	if errs.Errored() {
		t.Fatal(errs.Err)
	}

	{
		bytes := []byte{0, 0, 0, 0, 0, 0}
		s := outer{}
		version, err := manager.Unmarshal(bytes, &s)
		if err != nil {
			t.Fatal(err)
		}
		if version != 0 {
			t.Fatalf("wrong version returned. Expected %d ; Returned %d", 0, version)
		}
	}
	{
		bytes := []byte{0, 0, 0, 0, 0, 1}
		s := outer{}
		if _, err := manager.Unmarshal(bytes, &s); err == nil {
			t.Fatalf("should have errored")
		}
	}
}

// Ensure deserializing slices that have been length restricted errors correctly
func TestRestrictedSlice(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	type inner struct {
		Bytes []byte `serialize:"true" len:"2"`
	}
	bytes := []byte{0, 0, 0, 0, 0, 3, 0, 1, 2}

	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	s := inner{}
	if _, err := manager.Unmarshal(bytes, &s); err == nil {
		t.Fatalf("Should have errored due to large of a slice")
	}

	s.Bytes = []byte{0, 1, 2}
	if _, err := manager.Marshal(0, s); err == nil {
		t.Fatalf("Should have errored due to large of a slice")
	}
}

// Test unmarshaling something with extra data
func TestExtraSpace(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	byteSlice := []byte{0x00, 0x00, 0x01, 0x02} // codec version 0x0000 then 0x01 for b then 0x02 as extra data.
	var b byte
	_, err := manager.Unmarshal(byteSlice, &b)
	if err == nil {
		t.Fatalf("Should have errored due to too many bytes being passed in")
	}
}

// Ensure deserializing slices that have been length restricted errors correctly
func TestSliceLengthOverflow(codec GeneralCodec, t testing.TB) {
	var _ GeneralCodec = codec

	type inner struct {
		Vals []uint32 `serialize:"true" len:"2"`
	}
	bytes := []byte{
		// Codec Version:
		0x00, 0x00,
		// Slice Length:
		0xff, 0xff, 0xff, 0xff,
	}

	manager := NewDefaultManager()
	if err := manager.RegisterCodec(0, codec); err != nil {
		t.Fatal(err)
	}

	s := inner{}
	if _, err := manager.Unmarshal(bytes, &s); err == nil {
		t.Fatalf("Should have errored due to large of a slice")
	}
}
