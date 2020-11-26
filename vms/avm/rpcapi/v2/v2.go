package v2

import (
	"net/http"

	"github.com/ava-labs/avalanchego/vms/avm/vmargs"

	"github.com/ava-labs/avalanchego/vms/avm/service"
)

// Controller defines the apis available to the AVM
type Controller struct {
	service *service.Service
}

// NewController create a new instance of the Controller
func NewController(service *service.Service) *Controller {
	return &Controller{service: service}
}

// GetTxStatus returns the status of the specified transaction
func (c *Controller) GetUTXOs(_ *http.Request, args *vmargs.GetUTXOsArgs, reply *vmargs.GetUTXOsReply) error {
	c.service.Log().Info("AVM: GetUTXOs called for with %s", args.Addresses)

	// TODO Check for nil args ?
	// TODO check the pointer to pointer assignment
	// Most of these will be validated by the service

	var err error
	reply, err = c.service.GetUTXOs(args) //nolint:ineffassign
	if err != nil {
		return err
	}

	return nil
}
