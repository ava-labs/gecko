package chains

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/networking/router"
)

// MockManager implements Manager but does nothing. Always returns nil error.
// To be used only in tests
type MockManager struct{}

func (mm MockManager) Router() router.Router            { return nil }
func (mm MockManager) CreateChain(ChainParameters)      {}
func (mm MockManager) ForceCreateChain(ChainParameters) {}
func (mm MockManager) AddRegistrant(Registrant)         {}
func (mm MockManager) Aliases(ids.ID) []string          { return nil }
func (mm MockManager) Alias(ids.ID, string) error       { return nil }
func (mm MockManager) Shutdown()                        {}
func (mm MockManager) SubnetID(ids.ID) (ids.ID, error)  { return ids.ID{}, nil }
func (mm MockManager) IsBootstrapped(ids.ID) bool       { return false }

func (mm MockManager) Lookup(s string) (ids.ID, error) {
	id, err := ids.FromString(s)
	if err == nil {
		return id, nil
	}
	return ids.ID{}, nil
}

func (mm MockManager) LookupVM(s string) (ids.ID, error) {
	id, err := ids.FromString(s)
	if err == nil {
		return id, nil
	}
	return ids.ID{}, nil
}
