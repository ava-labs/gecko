// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

const (
	// maxSize is the maximum allowed vertex size. It is necessary to deter DoS
	maxSize = 1 << 20

	// noEpochTransitionsCodecVersion is the codec version that was used when
	// there were no epoch transitions
	noEpochTransitionsCodecVersion = uint16(0)

	// apricotCodecVersion is the codec version that was used when we added
	// epoch transitions
	apricotCodecVersion = uint16(1)
)

var (
	c codec.Manager
)

func init() {
	codecV0 := linearcodec.New("serializeV0", maxSize)
	codecV1 := linearcodec.New("serializeV1", maxSize)
	c = codec.NewManager(maxSize)

	errs := wrappers.Errs{}
	errs.Add(
		c.RegisterCodec(noEpochTransitionsCodecVersion, codecV0),
		c.RegisterCodec(apricotCodecVersion, codecV1),
	)
	if errs.Errored() {
		panic(errs.Err)
	}
}
