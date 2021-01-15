// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package secp256k1fx

import (
	"errors"
	"fmt"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/components/verify"
)

const (
	defaultCacheSize = 2048
)

var (
	errWrongVMType                    = errors.New("wrong vm type")
	errWrongTxType                    = errors.New("wrong tx type")
	errWrongOpType                    = errors.New("wrong operation type")
	errWrongInputType                 = errors.New("wrong input type")
	errWrongCredentialType            = errors.New("wrong credential type")
	errWrongOwnerType                 = errors.New("wrong owner type")
	errWrongNumberOfUTXOs             = errors.New("wrong number of utxos for the operation")
	errWrongMintCreated               = errors.New("wrong mint output created from the operation")
	errTimelocked                     = errors.New("output is time locked")
	errTooManySigners                 = errors.New("input has more signers than expected")
	errTooFewSigners                  = errors.New("input has less signers than expected")
	errInputOutputIndexOutOfBounds    = errors.New("input referenced a nonexistent address in the output")
	errInputCredentialSignersMismatch = errors.New("input expected a different number of signers than provided in the credential")
)

// Fx describes the secp256k1 feature extension
type Fx struct {
	VM           VM
	SECPFactory  crypto.FactorySECP256K1R
	bootstrapped bool
}

// Initialize ...
func (fx *Fx) Initialize(vmIntf interface{}) error {
	if err := fx.InitializeVM(vmIntf); err != nil {
		return err
	}

	log := fx.VM.Logger()
	log.Debug("initializing secp561k1 fx")

	fx.SECPFactory = crypto.FactorySECP256K1R{
		Cache: cache.LRU{Size: defaultCacheSize},
	}
	c := fx.VM.CodecRegistry()
	errs := wrappers.Errs{}
	errs.Add(
		c.RegisterType(&TransferInput{}),
		c.RegisterType(&MintOutput{}),
		c.RegisterType(&TransferOutput{}),
		c.RegisterType(&MintOperation{}),
		c.RegisterType(&Credential{}),
		c.RegisterType(&ManagedAssetStatusOutput{}),
		c.RegisterType(&UpdateManagedAssetOperation{}),
	)
	return errs.Err
}

// InitializeVM ...
func (fx *Fx) InitializeVM(vmIntf interface{}) error {
	vm, ok := vmIntf.(VM)
	if !ok {
		return errWrongVMType
	}
	fx.VM = vm
	return nil
}

// Bootstrapping ...
func (fx *Fx) Bootstrapping() error { return nil }

// Bootstrapped ...
func (fx *Fx) Bootstrapped() error { fx.bootstrapped = true; return nil }

// VerifyPermission returns nil iff [credIntf] proves that [ownerIntf] assents to [txIntf]
// [inIntf] must be an *Input or *TransferInput
// [ownerIntf] must be an *OutputOwners or *TransferOutput
func (fx *Fx) VerifyPermission(txIntf, inIntf, credIntf, ownerIntf interface{}) error {
	tx, ok := txIntf.(Tx)
	if !ok {
		return errWrongTxType
	}

	var in *Input
	switch input := inIntf.(type) {
	case *Input:
		in = input
	case *TransferInput:
		in = &input.Input
	default:
		return fmt.Errorf("expected input to be *Input or *TransferInput but is %T", input)
	}

	cred, ok := credIntf.(*Credential)
	if !ok {
		return errWrongCredentialType
	}

	var owner *OutputOwners
	switch o := ownerIntf.(type) {
	case *OutputOwners:
		owner = o
	case *TransferOutput:
		owner = &o.OutputOwners
	default:
		return fmt.Errorf("expected owner to be *OutputOwners or *TransferOutput but got %T", o)
	}

	if err := verify.All(in, cred, owner); err != nil {
		return err
	}
	return fx.VerifyCredentials(tx, in, cred, owner)
}

// VerifyOperation ...
func (fx *Fx) VerifyOperation(txIntf, opIntf, credIntf interface{}, outsIntf []interface{}) error {
	if len(outsIntf) != 1 {
		return errWrongNumberOfUTXOs
	}

	tx, ok := txIntf.(Tx)
	if !ok {
		return errWrongTxType
	}

	cred, ok := credIntf.(*Credential)
	if !ok {
		return errWrongCredentialType
	}

	switch op := opIntf.(type) {
	case *MintOperation:
		out, ok := outsIntf[0].(*MintOutput)
		if !ok {
			return fmt.Errorf("expected output to be *MintOutput but got %T", outsIntf[0])
		}
		return fx.verifyMintOperation(tx, op, cred, out)
	case *UpdateManagedAssetOperation:
		out, ok := outsIntf[0].(*ManagedAssetStatusOutput)
		if !ok {
			return fmt.Errorf("expected output to be *ManagedAssetStatusOutput but got %T", outsIntf[0])
		}
		return fx.verifyUpdateManagedAssetOperation(tx, op, cred, out)
	default:
		return errWrongOpType
	}
}

func (fx *Fx) verifyMintOperation(tx Tx, op *MintOperation, cred *Credential, out *MintOutput) error {
	if err := verify.All(op, cred, out); err != nil {
		return err
	}
	if !out.Equals(&op.MintOutput.OutputOwners) {
		return errWrongMintCreated
	}
	return fx.VerifyCredentials(tx, &op.MintInput, cred, &out.OutputOwners)
}

func (fx *Fx) verifyUpdateManagedAssetOperation(
	tx Tx,
	op *UpdateManagedAssetOperation,
	cred *Credential,
	out *ManagedAssetStatusOutput,
) error {
	if err := verify.All(op, cred, out); err != nil {
		return err
	}
	return fx.VerifyCredentials(tx, &op.Input, cred, &out.Mgr)
}

// VerifyTransfer ...
func (fx *Fx) VerifyTransfer(inIntf, outIntf interface{}) error {
	in, ok := inIntf.(*TransferInput)
	if !ok {
		return errWrongInputType
	}
	out, ok := outIntf.(*TransferOutput)
	if !ok {
		return fmt.Errorf("expected output to be *TransferOutput but got %T", outIntf)
	}
	if err := verify.All(in, out); err != nil {
		return err
	}
	if out.Amt != in.Amt {
		return fmt.Errorf("output amount (%d) != input amount (%d)", out.Amt, in.Amt)
	}
	return nil
}

// VerifyCredentials ensures that the output can be spent by the input with the
// credential. A nil return values means the output can be spent.
func (fx *Fx) VerifyCredentials(tx Tx, in *Input, cred *Credential, out *OutputOwners) error {
	numSigs := len(in.SigIndices)
	switch {
	case out.Locktime > fx.VM.Clock().Unix():
		return errTimelocked
	case out.Threshold < uint32(numSigs):
		return errTooManySigners
	case out.Threshold > uint32(numSigs):
		return errTooFewSigners
	case numSigs != len(cred.Sigs):
		return errInputCredentialSignersMismatch
	case !fx.bootstrapped: // disable signature verification during bootstrapping
		return nil
	}

	txHash := hashing.ComputeHash256(tx.UnsignedBytes())
	for i, index := range in.SigIndices {
		// Make sure the input references an address that exists
		if index >= uint32(len(out.Addrs)) {
			return errInputOutputIndexOutOfBounds
		}
		// Make sure each signature in the signature list is from an owner of
		// the output being consumed
		sig := cred.Sigs[i]
		pk, err := fx.SECPFactory.RecoverHashPublicKey(txHash, sig[:])
		if err != nil {
			return err
		}
		if expectedAddress := out.Addrs[index]; expectedAddress != pk.Address() {
			return fmt.Errorf("expected signature from %s but got from %s",
				expectedAddress,
				pk.Address())
		}
	}

	return nil
}

// CreateOutput creates a new output with the provided control group worth
// the specified amount
func (fx *Fx) CreateOutput(amount uint64, ownerIntf interface{}) (interface{}, error) {
	owner, ok := ownerIntf.(*OutputOwners)
	if !ok {
		return nil, errWrongOwnerType
	}
	if err := owner.Verify(); err != nil {
		return nil, err
	}
	return &TransferOutput{
		Amt:          amount,
		OutputOwners: *owner,
	}, nil
}
