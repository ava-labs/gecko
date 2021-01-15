package propertyfx

import (
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
	errWrongMintOutput     = errors.New("wrong mint output provided")
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
		c.RegisterType(&OwnedOutput{}),
		c.RegisterType(&MintOperation{}),
		c.RegisterType(&BurnOperation{}),
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
	case *BurnOperation:
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
	case !out.OutputOwners.Equals(&op.MintOutput.OutputOwners):
		return errWrongMintOutput
	default:
		return fx.Fx.VerifyCredentials(tx, &op.MintInput, &cred.Credential, &out.OutputOwners)
	}
}

// VerifyTransferOperation ...
func (fx *Fx) VerifyTransferOperation(tx secp256k1fx.Tx, op *BurnOperation, cred *Credential, utxoIntf interface{}) error {
	out, ok := utxoIntf.(*OwnedOutput)
	if !ok {
		return errWrongUTXOType
	}

	if err := verify.All(op, cred, out); err != nil {
		return err
	}

	return fx.VerifyCredentials(tx, &op.Input, &cred.Credential, &out.OutputOwners)
}

// VerifyTransfer ...
func (fx *Fx) VerifyTransfer(_, _ interface{}) error { return errCantTransfer }
