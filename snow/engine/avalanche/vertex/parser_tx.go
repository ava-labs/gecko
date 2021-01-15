// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/utils/hashing"
)

// ParserTx parses bytes into a transaction.
type ParserTx interface {
	// Parse a transaction from a slice of bytes
	ParseTx(tx []byte) (conflicts.Tx, error)
}

// Parse the provided transaction bytes into a stateless transaction
func ParseTx(transaction []byte) (StatelessTx, error) {
	tx := innerStatelessTx{}
	version, err := Codec.Unmarshal(transaction, &tx)
	tx.Version = version
	return statelessTx{
		innerStatelessTx: tx,
		id:               hashing.ComputeHash256Array(transaction),
		bytes:            transaction,
	}, err
}
