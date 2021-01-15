// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"testing"

	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/json"
)

var addrStrArray = []string{
	"A9bTQjfYGBFK3JPRJqF2eh3JYL7cHocvy",
	"6mxBGnjGDCKgkVe7yfrmvMA7xE7qCv3vv",
	"6ncQ19Q2U4MamkCYzshhD8XFjfwAWFzTa",
	"Jz9ayEDt7dx9hDx45aXALujWmL9ZUuqe7",
}

var (
	testHRP = constants.NetworkIDToHRP[networkID]
)

func TestBuildGenesis(t *testing.T) {
	ss := CreateStaticService()
	var addrMap = map[string]string{}
	for _, addrStr := range addrStrArray {
		b, err := formatting.Decode(formatting.CB58, addrStr)
		if err != nil {
			t.Fatal(err)
		}
		addrMap[addrStr], err = formatting.FormatBech32(testHRP, b)
		if err != nil {
			t.Fatal(err)
		}
	}
	args := BuildGenesisArgs{
		Encoding: formatting.Hex,
		GenesisData: map[string]AssetDefinition{
			"asset1": {
				Name:         "myFixedCapAsset",
				Symbol:       "MFCA",
				Denomination: 8,
				InitialState: map[string][]interface{}{
					"fixedCap": {
						Holder{
							Amount:  100000,
							Address: addrMap["A9bTQjfYGBFK3JPRJqF2eh3JYL7cHocvy"],
						},
						Holder{
							Amount:  100000,
							Address: addrMap["6mxBGnjGDCKgkVe7yfrmvMA7xE7qCv3vv"],
						},
						Holder{
							Amount:  json.Uint64(startBalance),
							Address: addrMap["6ncQ19Q2U4MamkCYzshhD8XFjfwAWFzTa"],
						},
						Holder{
							Amount:  json.Uint64(startBalance),
							Address: addrMap["Jz9ayEDt7dx9hDx45aXALujWmL9ZUuqe7"],
						},
					},
				},
			},
			"asset2": {
				Name:   "myVarCapAsset",
				Symbol: "MVCA",
				InitialState: map[string][]interface{}{
					"variableCap": {
						Minters{
							Threshold: 1,
							Minters: []string{
								addrMap["A9bTQjfYGBFK3JPRJqF2eh3JYL7cHocvy"],
								addrMap["6mxBGnjGDCKgkVe7yfrmvMA7xE7qCv3vv"],
							},
						},
						Minters{
							Threshold: 2,
							Minters: []string{
								addrMap["6ncQ19Q2U4MamkCYzshhD8XFjfwAWFzTa"],
								addrMap["Jz9ayEDt7dx9hDx45aXALujWmL9ZUuqe7"],
							},
						},
					},
				},
			},
			"asset3": {
				Name: "myOtherVarCapAsset",
				InitialState: map[string][]interface{}{
					"variableCap": {
						Minters{
							Threshold: 1,
							Minters: []string{
								addrMap["A9bTQjfYGBFK3JPRJqF2eh3JYL7cHocvy"],
							},
						},
					},
				},
			},
		}}
	reply := BuildGenesisReply{}
	err := ss.BuildGenesis(nil, &args, &reply)
	if err != nil {
		t.Fatal(err)
	}
}
