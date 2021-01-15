// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avmwallet

import (
	"errors"
	"fmt"

	stdmath "math"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/math"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/avm"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

const (
	codecVersion = 1
)

// Wallet is a holder for keys and UTXOs for the Avalanche DAG.
type Wallet struct {
	networkID uint32
	chainID   ids.ID

	clock timer.Clock
	codec codec.Manager
	log   logging.Logger

	keychain *secp256k1fx.Keychain // Mapping from public address to the SigningKeys
	utxoSet  *UTXOSet              // Mapping from utxoIDs to UTXOs

	balance map[ids.ID]uint64
	txFee   uint64

	txs []*avm.Tx
}

// NewWallet returns a new Wallet
func NewWallet(log logging.Logger, networkID uint32, chainID ids.ID, txFee uint64) (*Wallet, error) {
	c := linearcodec.NewDefault()
	m := codec.NewDefaultManager()
	errs := wrappers.Errs{}
	errs.Add(
		c.RegisterType(&avm.BaseTx{}),
		c.RegisterType(&avm.CreateAssetTx{}),
		c.RegisterType(&avm.OperationTx{}),
		c.RegisterType(&avm.ImportTx{}),
		c.RegisterType(&avm.ExportTx{}),
		c.RegisterType(&secp256k1fx.TransferInput{}),
		c.RegisterType(&secp256k1fx.MintOutput{}),
		c.RegisterType(&secp256k1fx.TransferOutput{}),
		c.RegisterType(&secp256k1fx.MintOperation{}),
		c.RegisterType(&secp256k1fx.Credential{}),
		c.RegisterType(&secp256k1fx.ManagedAssetStatusOutput{}),
		c.RegisterType(&secp256k1fx.UpdateManagedAssetOperation{}),
		m.RegisterCodec(codecVersion, c),
	)
	return &Wallet{
		networkID: networkID,
		chainID:   chainID,
		codec:     m,
		log:       log,
		keychain:  secp256k1fx.NewKeychain(),
		utxoSet:   &UTXOSet{},
		balance:   make(map[ids.ID]uint64),
		txFee:     txFee,
	}, errs.Err
}

// Codec returns the codec used for serialization
func (w *Wallet) Codec() codec.Manager { return w.codec }

// GetAddress returns one of the addresses this wallet manages. If no address
// exists, one will be created.
func (w *Wallet) GetAddress() (ids.ShortID, error) {
	if w.keychain.Addrs.Len() == 0 {
		return w.CreateAddress()
	}
	return w.keychain.Addrs.CappedList(1)[0], nil
}

// CreateAddress returns a new address.
// It also saves the address and the private key that controls it
// so the address can be used later
func (w *Wallet) CreateAddress() (ids.ShortID, error) {
	privKey, err := w.keychain.New()
	return privKey.PublicKey().Address(), err
}

// ImportKey imports a private key into this wallet
func (w *Wallet) ImportKey(sk *crypto.PrivateKeySECP256K1R) { w.keychain.Add(sk) }

// AddUTXO adds a new UTXO to this wallet if this wallet may spend it
// The UTXO's output must be an OutputPayment
func (w *Wallet) AddUTXO(utxo *avax.UTXO) {
	out, ok := utxo.Out.(avax.TransferableOut)
	if !ok {
		return
	}

	if _, _, err := w.keychain.Spend(out, stdmath.MaxUint64); err == nil {
		w.utxoSet.Put(utxo)
		w.balance[utxo.AssetID()] += out.Amount()
	}
}

// RemoveUTXO from this wallet
func (w *Wallet) RemoveUTXO(utxoID ids.ID) {
	utxo := w.utxoSet.Get(utxoID)
	if utxo == nil {
		return
	}

	assetID := utxo.AssetID()
	newBalance := w.balance[assetID] - utxo.Out.(avax.TransferableOut).Amount()
	if newBalance == 0 {
		delete(w.balance, assetID)
	} else {
		w.balance[assetID] = newBalance
	}

	w.utxoSet.Remove(utxoID)
}

// Balance returns the amount of the assets in this wallet
func (w *Wallet) Balance(assetID ids.ID) uint64 { return w.balance[assetID] }

// CreateTx returns a tx that sends [amount] of [assetID] to [destAddr]
func (w *Wallet) CreateTx(assetID ids.ID, amount uint64, destAddr ids.ShortID) (*avm.Tx, error) {
	if amount == 0 {
		return nil, errors.New("invalid amount")
	}

	amountSpent := uint64(0)
	time := w.clock.Unix()

	ins := []*avax.TransferableInput{}
	keys := [][]*crypto.PrivateKeySECP256K1R{}
	for _, utxo := range w.utxoSet.UTXOs {
		if utxo.AssetID() != assetID {
			continue
		}
		inputIntf, signers, err := w.keychain.Spend(utxo.Out, time)
		if err != nil {
			continue
		}
		input, ok := inputIntf.(avax.TransferableIn)
		if !ok {
			continue
		}
		spent, err := math.Add64(amountSpent, input.Amount())
		if err != nil {
			return nil, err
		}
		amountSpent = spent

		in := &avax.TransferableInput{
			UTXOID: utxo.UTXOID,
			Asset:  avax.Asset{ID: assetID},
			In:     input,
		}

		ins = append(ins, in)
		keys = append(keys, signers)

		if amountSpent >= amount {
			break
		}
	}

	if amountSpent < amount {
		return nil, errors.New("insufficient funds")
	}

	avax.SortTransferableInputsWithSigners(ins, keys)

	outs := []*avax.TransferableOutput{{
		Asset: avax.Asset{ID: assetID},
		Out: &secp256k1fx.TransferOutput{
			Amt: amount,
			OutputOwners: secp256k1fx.OutputOwners{
				Locktime:  0,
				Threshold: 1,
				Addrs:     []ids.ShortID{destAddr},
			},
		},
	}}

	if amountSpent > amount {
		changeAddr, err := w.GetAddress()
		if err != nil {
			return nil, err
		}
		outs = append(outs, &avax.TransferableOutput{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: amountSpent - amount,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{changeAddr},
				},
			},
		})
	}

	avax.SortTransferableOutputs(outs, w.codec, codecVersion)

	tx := &avm.Tx{
		Version: codecVersion,
		UnsignedTx: &avm.BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    w.networkID,
			BlockchainID: w.chainID,
			Outs:         outs,
			Ins:          ins,
		}}}
	return tx, tx.SignSECP256K1Fx(w.codec, keys)
}

// GenerateTxs generates the transactions that will be sent
// during the test
// Generate them all on test initialization so tx generation is not bottleneck
// in testing
func (w *Wallet) GenerateTxs(numTxs int, assetID ids.ID) error {
	w.log.Info("Generating %d transactions", numTxs)

	ctx := snow.DefaultContextTest()
	ctx.NetworkID = w.networkID
	ctx.ChainID = w.chainID

	frequency := numTxs / 50
	if frequency > 1000 {
		frequency = 1000
	}

	w.txs = make([]*avm.Tx, numTxs)
	for i := 0; i < numTxs; i++ {
		addr, err := w.CreateAddress()
		if err != nil {
			return err
		}
		tx, err := w.CreateTx(assetID, 1, addr)
		if err != nil {
			return err
		}

		for _, utxoID := range tx.InputUTXOs() {
			w.RemoveUTXO(utxoID.InputID())
		}
		for _, utxo := range tx.UTXOs() {
			w.AddUTXO(utxo)
		}

		if numGenerated := i + 1; numGenerated%frequency == 0 {
			w.log.Info("Generated %d out of %d transactions", numGenerated, numTxs)
		}

		w.txs[i] = tx
	}

	w.log.Info("Finished generating %d transactions", numTxs)
	return nil
}

// NextTx returns the next tx to be sent as part of xput test
func (w *Wallet) NextTx() *avm.Tx {
	if len(w.txs) == 0 {
		return nil
	}
	tx := w.txs[0]
	w.txs = w.txs[1:]
	return tx
}

func (w *Wallet) String() string {
	return fmt.Sprintf(
		"Keychain:\n"+
			"%s\n"+
			"%s",
		w.keychain.PrefixedString("    "),
		w.utxoSet.PrefixedString("    "),
	)
}
