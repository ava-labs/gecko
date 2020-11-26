// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package apiargs

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/formatting"
)

// This file contains structs used in arguments and responses in services

// SuccessResponse indicates success of an API call
type SuccessResponse struct {
	Success bool `json:"success"`
}

// JSONTxID contains the ID of a transaction
type JSONTxID struct {
	TxID ids.ID `json:"txID"`
}

// UserPass contains a username and a password
type UserPass struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// JSONAddress contains an address
type JSONAddress struct {
	Address string `json:"address"`
}

// JSONAddresses contains a list of address
type JSONAddresses struct {
	Addresses []string `json:"addresses"`
}

// JSONChangeAddr is the address change is sent to, if any
type JSONChangeAddr struct {
	ChangeAddr string `json:"changeAddr"`
}

// JSONTxIDChangeAddr is a tx ID and change address
type JSONTxIDChangeAddr struct {
	JSONTxID
	JSONChangeAddr
}

// JSONFromAddrs is a list of addresses to send funds from
type JSONFromAddrs struct {
	From []string `json:"from"`
}

// JSONSpendHeader is 3 arguments to a method that spends (including those with tx fees)
// 1) The username/password
// 2) The addresses used in the method
// 3) The address to send change to
type JSONSpendHeader struct {
	UserPass
	JSONFromAddrs
	JSONChangeAddr
}

// GetTxArgs ...
type GetTxArgs struct {
	TxID     ids.ID              `json:"txID"`
	Encoding formatting.Encoding `json:"encoding"`
}

// FormattedTx defines a JSON formatted struct containing a Tx in CB58 format
type FormattedTx struct {
	Tx       string              `json:"tx"`
	Encoding formatting.Encoding `json:"encoding"`
}

// Index is an address and an associated UTXO.
// Marks a starting or stopping point when fetching UTXOs. Used for pagination.
type Index struct {
	Address string `json:"address"` // The address as a string
	UTXO    string `json:"utxo"`    // The UTXO ID as a string
}
