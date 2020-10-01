package platformvm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/components/avax"
)

func TestBaseTxMarshalJSON(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	blockchainID := ids.NewID([32]byte{1})
	utxoTxID := ids.NewID([32]byte{2})
	assetID := ids.NewID([32]byte{3})
	tx := &BaseTx{BaseTx: avax.BaseTx{
		BlockchainID: blockchainID,
		NetworkID:    4,
		Ins: []*avax.TransferableInput{
			{
				UTXOID: avax.UTXOID{TxID: utxoTxID, OutputIndex: 5},
				Asset:  avax.Asset{ID: assetID},
				In:     &avax.TestTransferable{Val: 100},
			},
		},
		Outs: []*avax.TransferableOutput{
			{
				Asset: avax.Asset{ID: assetID},
				Out:   &avax.TestTransferable{Val: 100},
			},
		},
		Memo: []byte{1, 2, 3},
	}}

	txBytes, err := json.Marshal(tx)
	if err != nil {
		t.Fatal(err)
	}
	asString := string(txBytes)
	if !strings.Contains(asString, `"networkID":4`) {
		t.Fatal("should have network ID")
	} else if !strings.Contains(asString, `"blockchainID":"SYXsAycDPUu4z2ZksJD5fh5nTDcH3vCFHnpcVye5XuJ2jArg"`) {
		t.Fatal("should have blockchainID ID")
	} else if !strings.Contains(asString, `"inputs":[{"txID":"t64jLxDRmxo8y48WjbRALPAZuSDZ6qPVaaeDzxHA4oSojhLt","outputIndex":5,"assetID":"2KdbbWvpeAShCx5hGbtdF15FMMepq9kajsNTqVvvEbhiCRSxU","input":{"Err":null,"Val":100}}]`) {
		t.Fatal("inputs are wrong")
	} else if !strings.Contains(asString, `"outputs":[{"assetID":"2KdbbWvpeAShCx5hGbtdF15FMMepq9kajsNTqVvvEbhiCRSxU","output":{"Err":null,"Val":100}}]`) {
		t.Fatal("outputs are wrong")
	}
}
