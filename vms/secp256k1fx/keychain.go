// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package secp256k1fx

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/vms/components/verify"
)

var (
	errCantSpend = errors.New("unable to spend this UTXO")
)

// Keychain is a collection of keys that can be used to spend outputs
type Keychain struct {
	factory        *crypto.FactorySECP256K1R
	addrToKeyIndex map[ids.ShortID]int

	// These can be used to iterate over. However, they should not be modified externally.
	Addrs ids.ShortSet
	Keys  []*crypto.PrivateKeySECP256K1R
}

// NewKeychain returns a new, empty, keychain
func NewKeychain() *Keychain {
	return &Keychain{
		factory:        &crypto.FactorySECP256K1R{},
		addrToKeyIndex: make(map[ids.ShortID]int),
	}
}

// Add a new key to the key chain
func (kc *Keychain) Add(key *crypto.PrivateKeySECP256K1R) {
	addr := key.PublicKey().Address()
	if _, ok := kc.addrToKeyIndex[addr]; !ok {
		kc.addrToKeyIndex[addr] = len(kc.Keys)
		kc.Keys = append(kc.Keys, key)
		kc.Addrs.Add(addr)
	}
}

// Get a key from the keychain. If the key is unknown, the
func (kc Keychain) Get(id ids.ShortID) (*crypto.PrivateKeySECP256K1R, bool) {
	if i, ok := kc.addrToKeyIndex[id]; ok {
		return kc.Keys[i], true
	}
	return &crypto.PrivateKeySECP256K1R{}, false
}

// Addresses returns a list of addresses this keychain manages
func (kc Keychain) Addresses() ids.ShortSet { return kc.Addrs }

// New returns a newly generated private key
func (kc *Keychain) New() (*crypto.PrivateKeySECP256K1R, error) {
	skGen, err := kc.factory.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	sk := skGen.(*crypto.PrivateKeySECP256K1R)
	kc.Add(sk)
	return sk, nil
}

// Spend attempts to create an input
func (kc *Keychain) Spend(out verify.Verifiable, time uint64) (verify.Verifiable, []*crypto.PrivateKeySECP256K1R, error) {
	switch out := out.(type) {
	case *ManagedAssetStatusOutput:
		if sigIndices, keys, able := kc.Match(&out.Mgr, time); able {
			return &Input{
				SigIndices: sigIndices,
			}, keys, nil
		}
		return nil, nil, errCantSpend
	case *OutputOwners:
		if sigIndices, keys, able := kc.Match(out, time); able {
			return &Input{
				SigIndices: sigIndices,
			}, keys, nil
		}
		return nil, nil, errCantSpend
	case *MintOutput:
		if sigIndices, keys, able := kc.Match(&out.OutputOwners, time); able {
			return &Input{
				SigIndices: sigIndices,
			}, keys, nil
		}
		return nil, nil, errCantSpend
	case *TransferOutput:
		if sigIndices, keys, able := kc.Match(&out.OutputOwners, time); able {
			return &TransferInput{
				Amt: out.Amt,
				Input: Input{
					SigIndices: sigIndices,
				},
			}, keys, nil
		}
		return nil, nil, errCantSpend
	}
	return nil, nil, fmt.Errorf("can't spend UTXO because it is unexpected type %T", out)
}

// Match attempts to match a list of addresses up to the provided threshold
func (kc *Keychain) Match(owners *OutputOwners, time uint64) ([]uint32, []*crypto.PrivateKeySECP256K1R, bool) {
	if time < owners.Locktime {
		return nil, nil, false
	}
	sigs := make([]uint32, 0, owners.Threshold)
	keys := make([]*crypto.PrivateKeySECP256K1R, 0, owners.Threshold)
	for i := uint32(0); i < uint32(len(owners.Addrs)) && uint32(len(keys)) < owners.Threshold; i++ {
		if key, exists := kc.Get(owners.Addrs[i]); exists {
			sigs = append(sigs, i)
			keys = append(keys, key)
		}
	}
	return sigs, keys, uint32(len(keys)) == owners.Threshold
}

// PrefixedString returns the key chain as a string representation with [prefix]
// added before every line.
func (kc *Keychain) PrefixedString(prefix string) string {
	s := strings.Builder{}
	format := fmt.Sprintf("%%sKey[%s]: Key: %%s Address: %%s\n",
		formatting.IntFormat(len(kc.Keys)-1))
	for i, key := range kc.Keys {
		// We assume that the maximum size of a byte slice that
		// can be stringified is at least the length of a SECP256K1 private key
		keyStr, _ := formatting.Encode(defaultEncoding, key.Bytes())
		s.WriteString(fmt.Sprintf(format,
			prefix,
			i,
			keyStr,
			key.PublicKey().Address()))
	}

	return strings.TrimSuffix(s.String(), "\n")
}

func (kc *Keychain) String() string { return kc.PrefixedString("") }
