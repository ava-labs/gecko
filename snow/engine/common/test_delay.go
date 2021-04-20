// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package common

import (
	"testing"
	"time"
)

// DelayTest is a test delay
type DelayTest struct {
	T *testing.T

	CantDelay bool

	DelayF func(time.Duration)
}

// Default set the default callable value to [cant]
func (d *DelayTest) Default(cant bool) {
	d.CantDelay = cant
}

// Delay calls DelayF if it was initialized. If it wasn't initialized and this
// function shouldn't be called and testing was initialized, then testing will
// fail.
func (d *DelayTest) Delay(delay time.Duration) {
	if d.DelayF != nil {
		d.DelayF(delay)
	} else if d.CantDelay && d.T != nil {
		d.T.Fatalf("Unexpectedly called Delay")
	}
}
