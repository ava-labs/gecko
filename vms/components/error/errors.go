package vmerror

import "errors"

var (
	ErrUnknownAssetID         = errors.New("unknown asset ID")
	ErrTxNotCreateAsset       = errors.New("transaction doesn't create an asset")
	ErrNoHolders              = errors.New("initialHolders must not be empty")
	ErrNoMinters              = errors.New("no minters provided")
	ErrInvalidAmount          = errors.New("amount must be positive")
	ErrNoOutputs              = errors.New("no outputs to send")
	ErrSpendOverflow          = errors.New("spent amount overflows uint64")
	ErrInvalidMintAmount      = errors.New("amount minted must be positive")
	ErrAddressesCantMintAsset = errors.New("provided addresses don't have the authority to mint the provided asset")
	ErrInvalidUTXO            = errors.New("invalid utxo")
	ErrNilTxID                = errors.New("nil transaction ID")
	ErrNoAddresses            = errors.New("no addresses provided")
	ErrNoKeys                 = errors.New("from addresses have no keys or funds")
)
