// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/utils/hashing"
)

const (
	defaultStatelessTxCodecVersion = noEpochTransitionsCodecVersion
)

// Wrapper wraps a transition into a transaction with a provided epoch and
// restrictions.
type Wrapper interface {
	// Build a new vertex from the contents of a vertex
	Wrap(
		epoch uint32,
		tr conflicts.Transition,
		restrictions []ids.ID,
	) (conflicts.Tx, error)
}

// Wrap the provided transition bytes into a stateless transaction
func Wrap(
	epoch uint32,
	transition []byte,
	restrictions []ids.ID,
) (StatelessTx, error) {
	tx := innerStatelessTx{
		Version:      defaultStatelessTxCodecVersion,
		Epoch:        epoch,
		Transition:   transition,
		Restrictions: restrictions,
	}
	txBytes, err := Codec.Marshal(defaultStatelessTxCodecVersion, &tx)
	return statelessTx{
		innerStatelessTx: tx,
		id:               hashing.ComputeHash256Array(txBytes),
		bytes:            txBytes,
	}, err
}
