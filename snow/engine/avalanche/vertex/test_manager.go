// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import "testing"

var (
	_ Manager = &TestManager{}
)

type TestManager struct {
	TestBuilder
	TestParser
	TestStorage
	TestWrapper
	TestParserTx
}

func NewTestManager(t *testing.T) *TestManager {
	return &TestManager{
		TestBuilder:  TestBuilder{T: t},
		TestParser:   TestParser{T: t},
		TestStorage:  TestStorage{T: t},
		TestWrapper:  TestWrapper{T: t},
		TestParserTx: TestParserTx{T: t},
	}
}

func (m *TestManager) Default(cant bool) {
	m.TestBuilder.Default(cant)
	m.TestParser.Default(cant)
	m.TestStorage.Default(cant)
	m.TestWrapper.Default(cant)
	m.TestParserTx.Default(cant)
}
