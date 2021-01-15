package secp256k1fx

import (
	"errors"

	"github.com/ava-labs/avalanchego/vms/components/verify"
)

var (
	errManagerThresholdZero = errors.New("ManagedAssetStatusOutput.Manager.Threshold can't be 0")
)

// ManagedAssetStatusOutput represents the status of a managed asset
// [Frozen] is true iff the asset is frozen
// [OutputOwners] may move any UTXOs with this asset, and may freeze/unfreeze
// all UTXOs of the asset.
// Meets the avm's ManagedAssetStatus interface
type ManagedAssetStatusOutput struct {
	IsFrozen bool         `serialize:"true" json:"frozen"`
	Mgr      OutputOwners `serialize:"true" json:"manager"`
}

// Frozen returns true iff this asset is frozen.
// Meets the avm's ManagedAssetStatus interface
func (s *ManagedAssetStatusOutput) Frozen() bool {
	return s.IsFrozen
}

// Frozen returns the manager of this asset.
// Meets the avm's ManagedAssetStatus interface
func (s *ManagedAssetStatusOutput) Manager() verify.State {
	return &s.Mgr
}

// Verify ...
func (s *ManagedAssetStatusOutput) Verify() error {
	if err := s.Mgr.Verify(); err != nil {
		return err
	}
	// Make sure the manager is not empty
	if s.Mgr.Threshold == 0 {
		return errManagerThresholdZero
	}
	return nil
}

// VerifyState ...
func (s *ManagedAssetStatusOutput) VerifyState() error {
	return s.Verify()
}

// Verify ...
func (s *ManagedAssetStatusOutput) Addresses() [][]byte {
	return s.Mgr.Addresses()
}
