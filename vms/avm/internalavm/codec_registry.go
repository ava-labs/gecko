// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package internalavm

import (
	"reflect"

	"github.com/ava-labs/avalanchego/utils/codec"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

var (
	_ codec.Registry = &codecRegistry{}
)

type codecRegistry struct {
	codecs      []codec.Codec
	index       int
	typeToIndex map[reflect.Type]int
}

func (cr *codecRegistry) Skip(amount int) {
	for _, c := range cr.codecs {
		c.Skip(amount)
	}
}

func (cr *codecRegistry) RegisterType(val interface{}) error {
	valType := reflect.TypeOf(val)
	cr.typeToIndex[valType] = cr.index

	errs := wrappers.Errs{}
	for _, c := range cr.codecs {
		errs.Add(c.RegisterType(val))
	}
	return errs.Err
}
