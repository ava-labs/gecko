package avm

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
)

// ID that this VM uses when labeled
var (
	ID = ids.ID{'a', 'v', 'm'}
)

// Factory ...
type Factory struct {
	CreationFee uint64
	Fee         uint64
}

// TODO review this
// New ...
func (f *Factory) New(*snow.Context) (interface{}, error) {
	return NewVM(f.CreationFee, f.Fee), nil
}
