// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"time"

	"github.com/ava-labs/avalanchego/ids"
)

// PeerID ...
type PeerID struct {
	IP           string    `json:"ip"`
	PublicIP     string    `json:"publicIP"`
	ID           string    `json:"nodeID"`
	Version      string    `json:"version"`
	Up           bool      `json:"up"`
	LastSent     time.Time `json:"lastSent"`
	LastReceived time.Time `json:"lastReceived"`
	Benched      []ids.ID  `json:"benched"`
}
