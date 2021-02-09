// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package conflicts

import (
	"github.com/ava-labs/avalanchego/ids"
)

// rejector implements Blockable
type rejector struct {
	c        *Conflicts
	deps     ids.Set
	rejected bool // true if the tx has been rejected
	tx       Tx
}

func (r *rejector) Dependencies() ids.Set { return r.deps }

func (r *rejector) Fulfill(ids.ID) {
	if r.rejected {
		return
	}
	r.rejected = true
	r.c.rejectable = append(r.c.rejectable, r.tx)
}

func (*rejector) Abandon(ids.ID) {}
func (*rejector) Update()        {}
