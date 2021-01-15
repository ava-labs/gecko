// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

// Manager defines all the vertex related functionality that is required by the
// consensus engine.
type Manager interface {
	Builder
	Parser
	Storage
	Wrapper
	ParserTx
}
