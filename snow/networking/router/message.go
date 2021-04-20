// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package router

import (
	"fmt"
	"strings"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/constants"
)

type message struct {
	messageType  constants.MsgType
	validatorID  ids.ShortID
	requestID    uint32
	containerID  ids.ID
	container    []byte
	containers   [][]byte
	containerIDs []ids.ID
	notification common.Message
	received     time.Time // Time this message was received
	deadline     time.Time // Time this message must be responded to
}

// IsPeriodic returns true if this message is of a type that is sent on a
// periodic basis.
func (m message) IsPeriodic() bool {
	return m.requestID == constants.GossipMsgRequestID ||
		m.messageType == constants.GossipMsg
}

func (m message) String() string {
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("(%s, ValidatorID: %s, RequestID: %d", m.messageType, m.validatorID, m.requestID))
	if !m.received.IsZero() {
		sb.WriteString(fmt.Sprintf(", Received: %d", m.received.Unix()))
	}
	if !m.deadline.IsZero() {
		sb.WriteString(fmt.Sprintf(", Deadline: %d", m.deadline.Unix()))
	}
	switch m.messageType {
	case constants.GetAcceptedMsg, constants.AcceptedMsg, constants.ChitsMsg, constants.AcceptedFrontierMsg:
		sb.WriteString(fmt.Sprintf(", ContainerIDs: %s)", m.containerIDs))
	case constants.GetMsg, constants.GetAncestorsMsg, constants.PutMsg, constants.PushQueryMsg, constants.PullQueryMsg:
		sb.WriteString(fmt.Sprintf(", ContainerID: %s)", m.containerID))
	case constants.MultiPutMsg:
		sb.WriteString(fmt.Sprintf(", NumContainers: %d)", len(m.containers)))
	case constants.NotifyMsg:
		sb.WriteString(fmt.Sprintf(", Notification: %s)", m.notification))
	default:
		sb.WriteString(")")
	}

	return sb.String()
}
