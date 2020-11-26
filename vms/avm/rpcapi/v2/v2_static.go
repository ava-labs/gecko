package v2

import (
	"net/http"

	"github.com/ava-labs/avalanchego/vms/avm/static"
	"github.com/ava-labs/avalanchego/vms/avm/vmargs"
)

// StaticController defines the apis available to the AVM
type StaticController struct {
}

// NewStaticController create a new instance of the Static Controller
func NewStaticController() *StaticController {
	return &StaticController{}
}

// BuildGenesis returns the UTXOs such that at least one address in [args.Addresses] is
// referenced in the UTXO.
func (c *StaticController) BuildGenesis(_ *http.Request, args *vmargs.BuildGenesisArgs, reply *vmargs.BuildGenesisReply) error {
	return static.BuildGenesis(args, reply)
}
