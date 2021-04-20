// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package sender

import (
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
)

// ExternalSenderTest is a test sender
type ExternalSenderTest struct {
	T *testing.T
	B *testing.B

	CantGetAcceptedFrontier, CantAcceptedFrontier,
	CantGetAccepted, CantAccepted,
	CantGetAncestors, CantMultiPut,
	CantGet, CantPut,
	CantPullQuery, CantPushQuery, CantChits,
	CantGossip bool

	GetAcceptedFrontierF func(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration) []ids.ShortID
	AcceptedFrontierF    func(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID)

	GetAcceptedF func(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, containerIDs []ids.ID) []ids.ShortID
	AcceptedF    func(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID)

	GetAncestorsF func(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID) bool
	MultiPutF     func(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containers [][]byte)

	GetF func(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID) bool
	PutF func(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerID ids.ID, container []byte)

	PushQueryF func(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID, container []byte) []ids.ShortID
	PullQueryF func(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID) []ids.ShortID
	ChitsF     func(validatorID ids.ShortID, chainID ids.ID, requestID uint32, votes []ids.ID)

	GossipF func(chainID ids.ID, containerID ids.ID, container []byte)
}

// Default set the default callable value to [cant]
func (s *ExternalSenderTest) Default(cant bool) {
	s.CantGetAcceptedFrontier = cant
	s.CantAcceptedFrontier = cant

	s.CantGetAccepted = cant
	s.CantAccepted = cant

	s.CantGetAncestors = cant
	s.CantMultiPut = cant

	s.CantGet = cant
	s.CantPut = cant

	s.CantPullQuery = cant
	s.CantPushQuery = cant
	s.CantChits = cant

	s.CantGossip = cant
}

// GetAcceptedFrontier calls GetAcceptedFrontierF if it was initialized. If it
// wasn't initialized and this function shouldn't be called and testing was
// initialized, then testing will fail.
func (s *ExternalSenderTest) GetAcceptedFrontier(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration) []ids.ShortID {
	switch {
	case s.GetAcceptedFrontierF != nil:
		return s.GetAcceptedFrontierF(validatorIDs, chainID, requestID, deadline)
	case s.CantGetAcceptedFrontier && s.T != nil:
		s.T.Fatalf("Unexpectedly called GetAcceptedFrontier")
	case s.CantGetAcceptedFrontier && s.B != nil:
		s.B.Fatalf("Unexpectedly called GetAcceptedFrontier")
	}
	return nil
}

// AcceptedFrontier calls AcceptedFrontierF if it was initialized. If it wasn't
// initialized and this function shouldn't be called and testing was
// initialized, then testing will fail.
func (s *ExternalSenderTest) AcceptedFrontier(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID) {
	switch {
	case s.AcceptedFrontierF != nil:
		s.AcceptedFrontierF(validatorID, chainID, requestID, containerIDs)
	case s.CantAcceptedFrontier && s.T != nil:
		s.T.Fatalf("Unexpectedly called AcceptedFrontier")
	case s.CantAcceptedFrontier && s.B != nil:
		s.B.Fatalf("Unexpectedly called AcceptedFrontier")
	}
}

// GetAccepted calls GetAcceptedF if it was initialized. If it wasn't
// initialized and this function shouldn't be called and testing was
// initialized, then testing will fail.
func (s *ExternalSenderTest) GetAccepted(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, containerIDs []ids.ID) []ids.ShortID {
	switch {
	case s.GetAcceptedF != nil:
		return s.GetAcceptedF(validatorIDs, chainID, requestID, deadline, containerIDs)
	case s.CantGetAccepted && s.T != nil:
		s.T.Fatalf("Unexpectedly called GetAccepted")
	case s.CantGetAccepted && s.B != nil:
		s.B.Fatalf("Unexpectedly called GetAccepted")
	}
	return nil
}

// Accepted calls AcceptedF if it was initialized. If it wasn't initialized and
// this function shouldn't be called and testing was initialized, then testing
// will fail.
func (s *ExternalSenderTest) Accepted(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID) {
	switch {
	case s.AcceptedF != nil:
		s.AcceptedF(validatorID, chainID, requestID, containerIDs)
	case s.CantAccepted && s.T != nil:
		s.T.Fatalf("Unexpectedly called Accepted")
	case s.CantAccepted && s.B != nil:
		s.B.Fatalf("Unexpectedly called Accepted")
	}
}

// GetAncestors calls GetAncestorsF if it was initialized. If it wasn't initialized and this
// function shouldn't be called and testing was initialized, then testing will
// fail.
func (s *ExternalSenderTest) GetAncestors(vdr ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Duration, vtxID ids.ID) bool {
	switch {
	case s.GetAncestorsF != nil:
		return s.GetAncestorsF(vdr, chainID, requestID, deadline, vtxID)
	case s.CantGetAncestors && s.T != nil:
		s.T.Fatalf("Unexpectedly called GetAncestors")
	case s.CantGetAncestors && s.B != nil:
		s.B.Fatalf("Unexpectedly called GetAncestors")
	}
	return false
}

// MultiPut calls MultiPutF if it was initialized. If it wasn't initialized and this
// function shouldn't be called and testing was initialized, then testing will
// fail.
func (s *ExternalSenderTest) MultiPut(vdr ids.ShortID, chainID ids.ID, requestID uint32, vtxs [][]byte) {
	switch {
	case s.MultiPutF != nil:
		s.MultiPutF(vdr, chainID, requestID, vtxs)
	case s.CantMultiPut && s.T != nil:
		s.T.Fatalf("Unexpectedly called MultiPut")
	case s.CantMultiPut && s.B != nil:
		s.B.Fatalf("Unexpectedly called MultiPut")
	}
}

// Get calls GetF if it was initialized. If it wasn't initialized and this
// function shouldn't be called and testing was initialized, then testing will
// fail.
func (s *ExternalSenderTest) Get(vdr ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Duration, vtxID ids.ID) bool {
	switch {
	case s.GetF != nil:
		return s.GetF(vdr, chainID, requestID, deadline, vtxID)
	case s.CantGet && s.T != nil:
		s.T.Fatalf("Unexpectedly called Get")
	case s.CantGet && s.B != nil:
		s.B.Fatalf("Unexpectedly called Get")
	}
	return false
}

// Put calls PutF if it was initialized. If it wasn't initialized and this
// function shouldn't be called and testing was initialized, then testing will
// fail.
func (s *ExternalSenderTest) Put(vdr ids.ShortID, chainID ids.ID, requestID uint32, vtxID ids.ID, vtx []byte) {
	switch {
	case s.PutF != nil:
		s.PutF(vdr, chainID, requestID, vtxID, vtx)
	case s.CantPut && s.T != nil:
		s.T.Fatalf("Unexpectedly called Put")
	case s.CantPut && s.B != nil:
		s.B.Fatalf("Unexpectedly called Put")
	}
}

// PushQuery calls PushQueryF if it was initialized. If it wasn't initialized
// and this function shouldn't be called and testing was initialized, then
// testing will fail.
func (s *ExternalSenderTest) PushQuery(vdrs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, vtxID ids.ID, vtx []byte) []ids.ShortID {
	switch {
	case s.PushQueryF != nil:
		return s.PushQueryF(vdrs, chainID, requestID, deadline, vtxID, vtx)
	case s.CantPushQuery && s.T != nil:
		s.T.Fatalf("Unexpectedly called PushQuery")
	case s.CantPushQuery && s.B != nil:
		s.B.Fatalf("Unexpectedly called PushQuery")
	}
	return nil
}

// PullQuery calls PullQueryF if it was initialized. If it wasn't initialized
// and this function shouldn't be called and testing was initialized, then
// testing will fail.
func (s *ExternalSenderTest) PullQuery(vdrs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, vtxID ids.ID) []ids.ShortID {
	switch {
	case s.PullQueryF != nil:
		return s.PullQueryF(vdrs, chainID, requestID, deadline, vtxID)
	case s.CantPullQuery && s.T != nil:
		s.T.Fatalf("Unexpectedly called PullQuery")
	case s.CantPullQuery && s.B != nil:
		s.B.Fatalf("Unexpectedly called PullQuery")
	}
	return nil
}

// Chits calls ChitsF if it was initialized. If it wasn't initialized and this
// function shouldn't be called and testing was initialized, then testing will
// fail.
func (s *ExternalSenderTest) Chits(vdr ids.ShortID, chainID ids.ID, requestID uint32, votes []ids.ID) {
	switch {
	case s.ChitsF != nil:
		s.ChitsF(vdr, chainID, requestID, votes)
	case s.CantChits && s.T != nil:
		s.T.Fatalf("Unexpectedly called Chits")
	case s.CantChits && s.B != nil:
		s.B.Fatalf("Unexpectedly called Chits")
	}
}

// Gossip calls GossipF if it was initialized. If it wasn't initialized and this
// function shouldn't be called and testing was initialized, then testing will
// fail.
func (s *ExternalSenderTest) Gossip(chainID ids.ID, containerID ids.ID, container []byte) {
	switch {
	case s.GossipF != nil:
		s.GossipF(chainID, containerID, container)
	case s.CantGossip && s.T != nil:
		s.T.Fatalf("Unexpectedly called Gossip")
	case s.CantGossip && s.B != nil:
		s.B.Fatalf("Unexpectedly called Gossip")
	}
}
