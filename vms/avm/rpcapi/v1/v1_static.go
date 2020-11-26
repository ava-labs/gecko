package v1

import (
	"net/http"

	"github.com/ava-labs/avalanchego/vms/avm/internalavm"
	"github.com/ava-labs/avalanchego/vms/avm/vmargs"
)

// Controller defines the apis available to the AVM
type StaticController struct {
}

// NewStaticController create a new instance of the Controller
func NewStaticController() *StaticController {
	return &StaticController{}
}

// BuildGenesis returns the UTXOs such that at least one address in [args.Addresses] is
// referenced in the UTXO.
func (c *StaticController) BuildGenesis(_ *http.Request, args *vmargs.BuildGenesisArgs, reply *vmargs.BuildGenesisReply) error {
	return internalavm.CreateStaticService().BuildGenesis(nil, args, reply)
}
