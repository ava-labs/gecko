// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
)

var (
	errParseTx = errors.New("unexpectedly called ParseTx")

	_ ParserTx = &TestParserTx{}
)

type TestParserTx struct {
	T           *testing.T
	CantParseTx bool
	ParseTxF    func([]byte) (conflicts.Tx, error)
}

func (p *TestParserTx) Default(cant bool) { p.CantParseTx = cant }

func (p *TestParserTx) ParseTx(b []byte) (conflicts.Tx, error) {
	if p.ParseTxF != nil {
		return p.ParseTxF(b)
	}
	if p.CantParseTx && p.T != nil {
		p.T.Fatal(errParseTx)
	}
	return nil, errParseTx
}
