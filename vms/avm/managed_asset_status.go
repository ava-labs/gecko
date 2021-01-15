package avm

import "github.com/ava-labs/avalanchego/vms/components/verify"

// ManagedAssetStatus is the status of a managed asset
type ManagedAssetStatus interface {
	// Frozen returns true if the asset is frozen
	// (That is, can't be transferred.)
	Frozen() bool

	// Manager returns the asset's manager, who
	// may send any UTXO containing the asset
	// and who may freeze the asset.
	Manager() verify.State
}
