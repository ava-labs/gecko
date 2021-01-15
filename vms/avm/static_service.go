// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/codec/reflectcodec"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"

	cjson "github.com/ava-labs/avalanchego/utils/json"
)

var (
	errUnknownAssetType = errors.New("unknown asset type")
)

// StaticService defines the base service for the asset vm
type StaticService struct{}

// CreateStaticService ...
func CreateStaticService() *StaticService {
	return &StaticService{}
}

// BuildGenesisArgs are arguments for BuildGenesis
type BuildGenesisArgs struct {
	NetworkID   cjson.Uint32               `json:"networkID"`
	GenesisData map[string]AssetDefinition `json:"genesisData"`
	Encoding    formatting.Encoding        `json:"encoding"`
}

// AssetDefinition ...
type AssetDefinition struct {
	Name         string                   `json:"name"`
	Symbol       string                   `json:"symbol"`
	Denomination cjson.Uint8              `json:"denomination"`
	InitialState map[string][]interface{} `json:"initialState"`
	Memo         string                   `json:"memo"`
}

// BuildGenesisReply is the reply from BuildGenesis
type BuildGenesisReply struct {
	Bytes    string              `json:"bytes"`
	Encoding formatting.Encoding `json:"encoding"`
}

// BuildGenesis returns the UTXOs such that at least one address in [args.Addresses] is
// referenced in the UTXO.
func (ss *StaticService) BuildGenesis(_ *http.Request, args *BuildGenesisArgs, reply *BuildGenesisReply) error {
	errs := wrappers.Errs{}

	// TODO use genesisCodec() here instead
	c := linearcodec.New(reflectcodec.DefaultTagName, 1<<20)
	manager := codec.NewManager(math.MaxUint32)
	errs.Add(
		c.RegisterType(&BaseTx{}),
		c.RegisterType(&CreateAssetTx{}),
		c.RegisterType(&OperationTx{}),
		c.RegisterType(&ImportTx{}),
		c.RegisterType(&ExportTx{}),
		c.RegisterType(&secp256k1fx.TransferInput{}),
		c.RegisterType(&secp256k1fx.MintOutput{}),
		c.RegisterType(&secp256k1fx.TransferOutput{}),
		c.RegisterType(&secp256k1fx.MintOperation{}),
		c.RegisterType(&secp256k1fx.Credential{}),
		manager.RegisterCodec(preApricotCodecVersion, c),
	)
	if errs.Errored() {
		return errs.Err
	}

	g := Genesis{}
	for assetAlias, assetDefinition := range args.GenesisData {
		assetMemo, err := formatting.Decode(args.Encoding, assetDefinition.Memo)

		if err != nil {
			return fmt.Errorf("problem formatting asset definition memo due to: %w", err)
		}
		asset := GenesisAsset{
			Alias: assetAlias,
			CreateAssetTx: CreateAssetTx{
				BaseTx: BaseTx{BaseTx: avax.BaseTx{
					NetworkID:    uint32(args.NetworkID),
					BlockchainID: ids.Empty,
					Memo:         assetMemo,
				}},
				Name:         assetDefinition.Name,
				Symbol:       assetDefinition.Symbol,
				Denomination: byte(assetDefinition.Denomination),
			},
		}
		if len(assetDefinition.InitialState) > 0 {
			initialState := &InitialState{
				FxID: 0, // TODO: Should lookup secp256k1fx FxID
			}
			for assetType, initialStates := range assetDefinition.InitialState {
				switch assetType {
				case "fixedCap":
					for _, state := range initialStates {
						b, err := json.Marshal(state)
						if err != nil {
							return fmt.Errorf("problem marshaling state: %w", err)
						}
						holder := Holder{}
						if err := json.Unmarshal(b, &holder); err != nil {
							return fmt.Errorf("problem unmarshaling holder: %w", err)
						}
						_, addrbuff, err := formatting.ParseBech32(holder.Address)
						if err != nil {
							return fmt.Errorf("problem parsing holder address: %w", err)
						}
						addr, err := ids.ToShortID(addrbuff)
						if err != nil {
							return fmt.Errorf("problem parsing holder address: %w", err)
						}
						initialState.Outs = append(initialState.Outs, &secp256k1fx.TransferOutput{
							Amt: uint64(holder.Amount),
							OutputOwners: secp256k1fx.OutputOwners{
								Threshold: 1,
								Addrs:     []ids.ShortID{addr},
							},
						})
					}
				case "variableCap":
					for _, state := range initialStates {
						b, err := json.Marshal(state)
						if err != nil {
							return fmt.Errorf("problem marshaling state: %w", err)
						}
						owners := Minters{}
						if err := json.Unmarshal(b, &owners); err != nil {
							return fmt.Errorf("problem unmarshaling Owners: %w", err)
						}

						out := &secp256k1fx.MintOutput{
							OutputOwners: secp256k1fx.OutputOwners{
								Threshold: 1,
							},
						}
						for _, address := range owners.Minters {
							_, addrbuff, err := formatting.ParseBech32(address)
							if err != nil {
								return fmt.Errorf("problem parsing minters address: %w", err)
							}
							addr, err := ids.ToShortID(addrbuff)
							if err != nil {
								return fmt.Errorf("problem parsing minters address: %w", err)
							}
							out.Addrs = append(out.Addrs, addr)
						}
						out.Sort()

						initialState.Outs = append(initialState.Outs, out)
					}
				default:
					return errUnknownAssetType
				}
			}
			initialState.Sort(manager, preApricotCodecVersion)
			asset.States = append(asset.States, initialState)
		}
		asset.Sort()
		g.Txs = append(g.Txs, &asset)
	}
	g.Sort()

	b, err := manager.Marshal(preApricotCodecVersion, &g)
	if err != nil {
		return fmt.Errorf("problem marshaling genesis: %w", err)
	}

	reply.Bytes, err = formatting.Encode(args.Encoding, b)
	if err != nil {
		return fmt.Errorf("couldn't encode genesis as string: %s", err)
	}
	reply.Encoding = args.Encoding
	return nil
}
