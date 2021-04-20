package indexer

import "github.com/ava-labs/avalanchego/ids"

// Container is something that gets accepted
// (a block, transaction or vertex)
type Container struct {
	// ID of this container
	ID ids.ID `serialize:"true"`
	// Byte representation of this container
	Bytes []byte `serialize:"true"`
	// Unix time, in nanoseconds, at which this container was accepted by this node
	Timestamp int64 `serialize:"true"`
}
