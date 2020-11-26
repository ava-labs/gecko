// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package internalavm

import (
	"sort"
	"strings"

	"github.com/ava-labs/avalanchego/utils"
)

// Genesis ...
type Genesis struct {
	Txs []*GenesisAsset `serialize:"true"`
}

// Less ...
func (g *Genesis) Less(i, j int) bool { return strings.Compare(g.Txs[i].Alias, g.Txs[j].Alias) == -1 }

// Len ...
func (g *Genesis) Len() int { return len(g.Txs) }

// Swap ...
func (g *Genesis) Swap(i, j int) { g.Txs[j], g.Txs[i] = g.Txs[i], g.Txs[j] }

// Sort ...
func (g *Genesis) Sort() { sort.Sort(g) }

// IsSortedAndUnique ...
func (g *Genesis) IsSortedAndUnique() bool { return utils.IsSortedAndUnique(g) }

// GenesisAsset ...
type GenesisAsset struct {
	Alias         string `serialize:"true"`
	CreateAssetTx `serialize:"true"`
}
