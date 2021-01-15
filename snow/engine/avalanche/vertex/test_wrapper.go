// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"

	"github.com/ava-labs/avalanchego/ids"
)

var (
	errWrap = errors.New("unexpectedly called Wrap")

	_ Wrapper = &TestWrapper{}
)

type TestWrapper struct {
	T        *testing.T
	CantWrap bool
	WrapF    func(
		epoch uint32,
		tr conflicts.Transition,
		restrictions []ids.ID,
	) (conflicts.Tx, error)
}

func (w *TestWrapper) Default(cant bool) { w.CantWrap = cant }

func (w *TestWrapper) Wrap(
	epoch uint32,
	tr conflicts.Transition,
	restrictions []ids.ID,
) (conflicts.Tx, error) {
	if w.WrapF != nil {
		return w.WrapF(epoch, tr, restrictions)
	}
	if w.CantWrap && w.T != nil {
		w.T.Fatal(errWrap)
	}
	return nil, errWrap
}
