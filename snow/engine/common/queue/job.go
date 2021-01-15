// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package queue

import (
	"github.com/ava-labs/avalanchego/ids"
)

// Job ...
type Job interface {
	ID() ids.ID

	MissingDependencies() (ids.Set, error)
	Execute() error

	Bytes() []byte

	DispatchID() ids.ID
	DispatchBytes() []byte
}
