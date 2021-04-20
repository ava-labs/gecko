// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chains

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/stretchr/testify/assert"
)

func TestSubnet(t *testing.T) {
	assert := assert.New(t)
	s := subnet{}

	chainID0 := ids.GenerateTestID()
	chainID1 := ids.GenerateTestID()
	chainID2 := ids.GenerateTestID()

	assert.True(s.IsBootstrapped(), "An empty subnet should be considered bootstrapped")

	s.addChain(chainID0)
	assert.False(s.IsBootstrapped(), "A subnet with one chain in bootstrapping shouldn't be considered bootstrapped")

	s.Bootstrapped(chainID0)
	assert.True(s.IsBootstrapped(), "A subnet with only bootstrapped chains should be considered bootstrapped")

	s.addChain(chainID1)
	assert.False(s.IsBootstrapped(), "A subnet with one chain in bootstrapping shouldn't be considered bootstrapped")

	s.addChain(chainID2)
	assert.False(s.IsBootstrapped(), "A subnet with one chain in bootstrapping shouldn't be considered bootstrapped")

	s.Bootstrapped(chainID1)
	assert.False(s.IsBootstrapped(), "A subnet with one chain in bootstrapping shouldn't be considered bootstrapped")

	s.Bootstrapped(chainID2)
	assert.True(s.IsBootstrapped(), "A subnet with only bootstrapped chains should be considered bootstrapped")
}
