// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package common

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
)

// EngineTest is a test engine
type EngineTest struct {
	T *testing.T

	CantIsBootstrapped,
	CantStartup,
	CantGossip,
	CantShutdown,

	CantContext,

	CantNotify,

	CantGetAcceptedFrontier,
	CantGetAcceptedFrontierFailed,
	CantAcceptedFrontier,

	CantGetAccepted,
	CantGetAcceptedFailed,
	CantAccepted,

	CantGet,
	CantGetAncestors,
	CantGetFailed,
	CantGetAncestorsFailed,
	CantPut,
	CantMultiPut,

	CantPushQuery,
	CantPullQuery,
	CantQueryFailed,
	CantChits,

	CantConnected,
	CantDisconnected,

	CantHealth,

	CantGetVtx, CantGetVM bool

	IsBootstrappedF                                    func() bool
	ContextF                                           func() *snow.Context
	StartupF, GossipF, ShutdownF                       func() error
	NotifyF                                            func(Message) error
	GetF, GetAncestorsF, PullQueryF                    func(validatorID ids.ShortID, requestID uint32, containerID ids.ID) error
	PutF, PushQueryF                                   func(validatorID ids.ShortID, requestID uint32, containerID ids.ID, container []byte) error
	MultiPutF                                          func(validatorID ids.ShortID, requestID uint32, containers [][]byte) error
	AcceptedFrontierF, GetAcceptedF, AcceptedF, ChitsF func(validatorID ids.ShortID, requestID uint32, containerIDs []ids.ID) error
	GetAcceptedFrontierF, GetFailedF, GetAncestorsFailedF,
	QueryFailedF, GetAcceptedFrontierFailedF, GetAcceptedFailedF func(validatorID ids.ShortID, requestID uint32) error
	ConnectedF, DisconnectedF func(validatorID ids.ShortID) error
	HealthF                   func() (interface{}, error)
	GetVtxF                   func() (avalanche.Vertex, error)
	GetVMF                    func() VM
}

var _ Engine = &EngineTest{}

// Default ...
func (e *EngineTest) Default(cant bool) {
	e.CantIsBootstrapped = cant

	e.CantStartup = cant
	e.CantGossip = cant
	e.CantShutdown = cant

	e.CantContext = cant

	e.CantNotify = cant

	e.CantGetAcceptedFrontier = cant
	e.CantGetAcceptedFrontierFailed = cant
	e.CantAcceptedFrontier = cant

	e.CantGetAccepted = cant
	e.CantGetAcceptedFailed = cant
	e.CantAccepted = cant

	e.CantGet = cant
	e.CantGetAncestors = cant
	e.CantGetAncestorsFailed = cant
	e.CantGetFailed = cant
	e.CantPut = cant
	e.CantMultiPut = cant

	e.CantPushQuery = cant
	e.CantPullQuery = cant
	e.CantQueryFailed = cant
	e.CantChits = cant

	e.CantConnected = cant
	e.CantDisconnected = cant

	e.CantHealth = cant

	e.CantGetVtx = cant
	e.CantGetVM = cant
}

func (e *EngineTest) Context() *snow.Context {
	if e.ContextF != nil {
		return e.ContextF()
	}
	if e.CantContext && e.T != nil {
		e.T.Fatalf("Unexpectedly called Context")
	}
	return nil
}

func (e *EngineTest) Startup() error {
	if e.StartupF != nil {
		return e.StartupF()
	}
	if !e.CantStartup {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called Startup")
	}
	return errors.New("unexpectedly called Startup")
}

func (e *EngineTest) Gossip() error {
	if e.GossipF != nil {
		return e.GossipF()
	}
	if !e.CantGossip {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called Gossip")
	}
	return errors.New("unexpectedly called Gossip")
}

func (e *EngineTest) Shutdown() error {
	if e.ShutdownF != nil {
		return e.ShutdownF()
	}
	if !e.CantShutdown {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called Shutdown")
	}
	return errors.New("unexpectedly called Shutdown")
}

func (e *EngineTest) Notify(msg Message) error {
	if e.NotifyF != nil {
		return e.NotifyF(msg)
	}
	if !e.CantNotify {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called Notify")
	}
	return errors.New("unexpectedly called Notify")
}

func (e *EngineTest) GetAcceptedFrontier(validatorID ids.ShortID, requestID uint32) error {
	if e.GetAcceptedFrontierF != nil {
		return e.GetAcceptedFrontierF(validatorID, requestID)
	}
	if !e.CantGetAcceptedFrontier {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called GetAcceptedFrontier")
	}
	return errors.New("unexpectedly called GetAcceptedFrontier")
}

func (e *EngineTest) GetAcceptedFrontierFailed(validatorID ids.ShortID, requestID uint32) error {
	if e.GetAcceptedFrontierFailedF != nil {
		return e.GetAcceptedFrontierFailedF(validatorID, requestID)
	}
	if !e.CantGetAcceptedFrontierFailed {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called GetAcceptedFrontierFailed")
	}
	return errors.New("unexpectedly called GetAcceptedFrontierFailed")
}

func (e *EngineTest) AcceptedFrontier(validatorID ids.ShortID, requestID uint32, containerIDs []ids.ID) error {
	if e.AcceptedFrontierF != nil {
		return e.AcceptedFrontierF(validatorID, requestID, containerIDs)
	}
	if !e.CantAcceptedFrontier {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called AcceptedFrontierF")
	}
	return errors.New("unexpectedly called AcceptedFrontierF")
}

func (e *EngineTest) GetAccepted(validatorID ids.ShortID, requestID uint32, containerIDs []ids.ID) error {
	if e.GetAcceptedF != nil {
		return e.GetAcceptedF(validatorID, requestID, containerIDs)
	}
	if !e.CantGetAccepted {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called GetAccepted")
	}
	return errors.New("unexpectedly called GetAccepted")
}

func (e *EngineTest) GetAcceptedFailed(validatorID ids.ShortID, requestID uint32) error {
	if e.GetAcceptedFailedF != nil {
		return e.GetAcceptedFailedF(validatorID, requestID)
	}
	if !e.CantGetAcceptedFailed {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called GetAcceptedFailed")
	}
	return errors.New("unexpectedly called GetAcceptedFailed")
}

func (e *EngineTest) Accepted(validatorID ids.ShortID, requestID uint32, containerIDs []ids.ID) error {
	if e.AcceptedF != nil {
		return e.AcceptedF(validatorID, requestID, containerIDs)
	}
	if !e.CantAccepted {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called Accepted")
	}
	return errors.New("unexpectedly called Accepted")
}

func (e *EngineTest) Get(validatorID ids.ShortID, requestID uint32, containerID ids.ID) error {
	if e.GetF != nil {
		return e.GetF(validatorID, requestID, containerID)
	}
	if !e.CantGet {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called Get")
	}
	return errors.New("unexpectedly called Get")
}

func (e *EngineTest) GetAncestors(validatorID ids.ShortID, requestID uint32, containerID ids.ID) error {
	if e.GetAncestorsF != nil {
		return e.GetAncestorsF(validatorID, requestID, containerID)
	}
	if !e.CantGetAncestors {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called GetAncestors")
	}
	return errors.New("unexpectedly called GetAncestors")

}

func (e *EngineTest) GetFailed(validatorID ids.ShortID, requestID uint32) error {
	if e.GetFailedF != nil {
		return e.GetFailedF(validatorID, requestID)
	}
	if !e.CantGetFailed {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called GetFailed")
	}
	return errors.New("unexpectedly called GetFailed")
}

func (e *EngineTest) GetAncestorsFailed(validatorID ids.ShortID, requestID uint32) error {
	if e.GetAncestorsFailedF != nil {
		return e.GetAncestorsFailedF(validatorID, requestID)
	}
	if e.CantGetAncestorsFailed {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called GetAncestorsFailed")
	}
	return errors.New("unexpectedly called GetAncestorsFailed")
}

func (e *EngineTest) Put(validatorID ids.ShortID, requestID uint32, containerID ids.ID, container []byte) error {
	if e.PutF != nil {
		return e.PutF(validatorID, requestID, containerID, container)
	}
	if !e.CantPut {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called Put")
	}
	return errors.New("unexpectedly called Put")
}

func (e *EngineTest) MultiPut(validatorID ids.ShortID, requestID uint32, containers [][]byte) error {
	if e.MultiPutF != nil {
		return e.MultiPutF(validatorID, requestID, containers)
	}
	if !e.CantMultiPut {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called MultiPut")
	}
	return errors.New("unexpectedly called MultiPut")
}

func (e *EngineTest) PushQuery(validatorID ids.ShortID, requestID uint32, containerID ids.ID, container []byte) error {
	if e.PushQueryF != nil {
		return e.PushQueryF(validatorID, requestID, containerID, container)
	}
	if !e.CantPushQuery {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called PushQuery")
	}
	return errors.New("unexpectedly called PushQuery")
}

func (e *EngineTest) PullQuery(validatorID ids.ShortID, requestID uint32, containerID ids.ID) error {
	if e.PullQueryF != nil {
		return e.PullQueryF(validatorID, requestID, containerID)
	}
	if !e.CantPullQuery {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called PullQuery")
	}
	return errors.New("unexpectedly called PullQuery")
}

func (e *EngineTest) QueryFailed(validatorID ids.ShortID, requestID uint32) error {
	if e.QueryFailedF != nil {
		return e.QueryFailedF(validatorID, requestID)
	}
	if !e.CantQueryFailed {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called QueryFailed")
	}
	return errors.New("unexpectedly called QueryFailed")
}

func (e *EngineTest) Chits(validatorID ids.ShortID, requestID uint32, containerIDs []ids.ID) error {
	if e.ChitsF != nil {
		return e.ChitsF(validatorID, requestID, containerIDs)
	}
	if !e.CantChits {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called Chits")
	}
	return errors.New("unexpectedly called Chits")
}

func (e *EngineTest) Connected(validatorID ids.ShortID) error {
	if e.ConnectedF != nil {
		return e.ConnectedF(validatorID)
	}
	if !e.CantConnected {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called Connected")
	}
	return errors.New("unexpectedly called Connected")
}

func (e *EngineTest) Disconnected(validatorID ids.ShortID) error {
	if e.DisconnectedF != nil {
		return e.DisconnectedF(validatorID)
	}
	if !e.CantDisconnected {
		return nil
	}
	if e.T != nil {
		e.T.Fatalf("Unexpectedly called Disconnected")
	}
	return errors.New("unexpectedly called Disconnected")
}

func (e *EngineTest) IsBootstrapped() bool {
	if e.IsBootstrappedF != nil {
		return e.IsBootstrappedF()
	}
	if e.CantIsBootstrapped && e.T != nil {
		e.T.Fatalf("Unexpectedly called IsBootstrapped")
	}
	return false
}

func (e *EngineTest) HealthCheck() (interface{}, error) {
	if e.HealthF != nil {
		return e.HealthF()
	}
	if e.CantHealth && e.T != nil {
		e.T.Fatalf("Unexpectedly called Health")
	}
	return nil, errors.New("unexpectedly called Health")
}

func (e *EngineTest) GetVtx() (avalanche.Vertex, error) {
	if e.GetVtxF != nil {
		return e.GetVtxF()
	}
	if e.CantGetVtx && e.T != nil {
		e.T.Fatalf("Unexpectedly called GetVtx")
	}
	return nil, errors.New("unexpectedly called GetVtx")
}

func (e *EngineTest) GetVM() VM {
	if e.GetVMF != nil {
		return e.GetVMF()
	}
	if e.CantGetVM && e.T != nil {
		e.T.Fatalf("Unexpectedly called GetVM")
	}
	return nil
}
