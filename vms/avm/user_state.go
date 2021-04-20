// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"fmt"

	"github.com/ava-labs/avalanchego/database/encdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

var addresses = ids.Empty

type userState struct{ vm *VM }

// SetAddresses ...
func (s *userState) SetAddresses(db *encdb.Database, addrs []ids.ShortID) error {
	bytes, err := s.vm.codec.Marshal(codecVersion, addrs)
	if err != nil {
		return err
	}
	return db.Put(addresses[:], bytes)
}

// Addresses ...
func (s *userState) Addresses(db *encdb.Database) ([]ids.ShortID, error) {
	bytes, err := db.Get(addresses[:])
	if err != nil {
		return nil, err
	}
	addresses := []ids.ShortID{}
	if _, err := s.vm.codec.Unmarshal(bytes, &addresses); err != nil {
		return nil, err
	}
	return addresses, nil
}

// Keychain returns a Keychain for the user database
// If [addresses] is non-empty it fetches only the keys
// in addresses. If any key is missing, an error is returned.
// If [addresses] is empty, then it will create a keychain using
// every address in [db].
func (s *userState) Keychain(db *encdb.Database, addresses ids.ShortSet) (*secp256k1fx.Keychain, error) {
	kc := secp256k1fx.NewKeychain()

	addrsList := addresses.List()
	if len(addrsList) == 0 {
		// Explicitly drop the error since it may indicate there are no addresses
		addrsList, _ = s.Addresses(db)
	}

	for _, addr := range addrsList {
		sk, err := s.Key(db, addr)
		if err != nil {
			return nil, fmt.Errorf("problem retrieving private key for address %s: %w", addr, err)
		}
		kc.Add(sk)
	}
	return kc, nil
}

// SetKey ...
func (s *userState) SetKey(db *encdb.Database, sk *crypto.PrivateKeySECP256K1R) error {
	return db.Put(sk.PublicKey().Address().Bytes(), sk.Bytes())
}

// Key ...
func (s *userState) Key(db *encdb.Database, address ids.ShortID) (*crypto.PrivateKeySECP256K1R, error) {
	factory := crypto.FactorySECP256K1R{}

	bytes, err := db.Get(address.Bytes())
	if err != nil {
		return nil, err
	}
	sk, err := factory.ToPrivateKey(bytes)
	if err != nil {
		return nil, err
	}
	return sk.(*crypto.PrivateKeySECP256K1R), nil
}
