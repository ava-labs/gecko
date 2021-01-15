package nftfx

import (
	"bytes"
	"errors"

	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

var (
	errWrongTxType         = errors.New("wrong tx type")
	errWrongUTXOType       = errors.New("wrong utxo type")
	errWrongOperationType  = errors.New("wrong operation type")
	errWrongCredentialType = errors.New("wrong credential type")
	errWrongNumberOfUTXOs  = errors.New("wrong number of UTXOs for the operation")
	errWrongUniqueID       = errors.New("wrong unique ID provided")
	errWrongBytes          = errors.New("wrong bytes provided")
	errCantTransfer        = errors.New("cant transfer with this fx")
)

// Fx ...
type Fx struct{ secp256k1fx.Fx }

// Initialize ...
func (fx *Fx) Initialize(vmIntf interface{}) error {
	if err := fx.InitializeVM(vmIntf); err != nil {
		return err
	}

	log := fx.VM.Logger()
	log.Debug("initializing nft fx")

	c := fx.VM.CodecRegistry()
	errs := wrappers.Errs{}
	errs.Add(
		c.RegisterType(&MintOutput{}),
		c.RegisterType(&TransferOutput{}),
		c.RegisterType(&MintOperation{}),
		c.RegisterType(&TransferOperation{}),
		c.RegisterType(&Credential{}),
	)
	return errs.Err
}

// VerifyOperation ...
func (fx *Fx) VerifyOperation(txIntf, opIntf, credIntf interface{}, utxosIntf []interface{}) error {
	tx, ok := txIntf.(secp256k1fx.Tx)
	switch {
	case !ok:
		return errWrongTxType
	case len(utxosIntf) != 1:
		return errWrongNumberOfUTXOs
	}

	cred, ok := credIntf.(*Credential)
	if !ok {
		return errWrongCredentialType
	}

	switch op := opIntf.(type) {
	case *MintOperation:
		return fx.VerifyMintOperation(tx, op, cred, utxosIntf[0])
	case *TransferOperation:
		return fx.VerifyTransferOperation(tx, op, cred, utxosIntf[0])
	default:
		return errWrongOperationType
	}
}

// VerifyMintOperation ...
func (fx *Fx) VerifyMintOperation(tx secp256k1fx.Tx, op *MintOperation, cred *Credential, utxoIntf interface{}) error {
	out, ok := utxoIntf.(*MintOutput)
	if !ok {
		return errWrongUTXOType
	}

	if err := verify.All(op, cred, out); err != nil {
		return err
	}

	switch {
	case out.GroupID != op.GroupID:
		return errWrongUniqueID
	default:
		return fx.Fx.VerifyCredentials(tx, &op.MintInput, &cred.Credential, &out.OutputOwners)
	}
}

// VerifyTransferOperation ...
func (fx *Fx) VerifyTransferOperation(tx secp256k1fx.Tx, op *TransferOperation, cred *Credential, utxoIntf interface{}) error {
	out, ok := utxoIntf.(*TransferOutput)
	if !ok {
		return errWrongUTXOType
	}

	if err := verify.All(op, cred, out); err != nil {
		return err
	}

	switch {
	case out.GroupID != op.Output.GroupID:
		return errWrongUniqueID
	case !bytes.Equal(out.Payload, op.Output.Payload):
		return errWrongBytes
	default:
		return fx.VerifyCredentials(tx, &op.Input, &cred.Credential, &out.OutputOwners)
	}
}

// VerifyTransfer ...
func (fx *Fx) VerifyTransfer(_, _ interface{}) error { return errCantTransfer }
