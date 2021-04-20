// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package constants

import "fmt"

// MsgType ...
type MsgType int

const (
	NullMsg MsgType = iota
	GetAcceptedFrontierMsg
	AcceptedFrontierMsg
	GetAcceptedFrontierFailedMsg
	GetAcceptedMsg
	AcceptedMsg
	GetAcceptedFailedMsg
	GetMsg
	PutMsg
	GetFailedMsg
	PushQueryMsg
	PullQueryMsg
	ChitsMsg
	QueryFailedMsg
	ConnectedMsg
	DisconnectedMsg
	NotifyMsg
	GossipMsg
	GetAncestorsMsg
	MultiPutMsg
	GetAncestorsFailedMsg
)

func (t MsgType) String() string {
	switch t {
	case NullMsg:
		return "Null"
	case GetAcceptedFrontierMsg:
		return "Get Accepted Frontier"
	case AcceptedFrontierMsg:
		return "Accepted Frontier"
	case GetAcceptedFrontierFailedMsg:
		return "Get Accepted Frontier Failed"
	case GetAcceptedMsg:
		return "Get Accepted"
	case AcceptedMsg:
		return "Accepted"
	case GetAcceptedFailedMsg:
		return "Get Accepted Failed"
	case GetMsg:
		return "Get"
	case GetAncestorsMsg:
		return "Get Ancestors"
	case GetAncestorsFailedMsg:
		return "Get Ancestors Failed"
	case PutMsg:
		return "Put"
	case MultiPutMsg:
		return "MultiPut"
	case GetFailedMsg:
		return "Get Failed"
	case PushQueryMsg:
		return "Push Query"
	case PullQueryMsg:
		return "Pull Query"
	case ChitsMsg:
		return "Chits"
	case QueryFailedMsg:
		return "Query Failed"
	case ConnectedMsg:
		return "Connected"
	case DisconnectedMsg:
		return "Disconnected"
	case NotifyMsg:
		return "Notify"
	case GossipMsg:
		return "Gossip"
	default:
		return fmt.Sprintf("Unknown Message Type: %d", t)
	}
}
