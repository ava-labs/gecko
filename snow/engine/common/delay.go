// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package common

import (
	"time"
)

// Delay describes the standard interface for specifying a delay
type Delay interface {
	// Delay specifies how much time to delay the next message by
	Delay(time.Duration)
}
