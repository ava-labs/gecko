// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package genesis

import (
	"time"

	"github.com/ava-labs/avalanchego/utils/units"
)

var (
	fujiGenesisConfigJSON = `{
		"networkID": 5,
		"allocations": [
			{
				"ethAddr": "0xb3d82b1367d362de99ab59a658165aff520cbd4d",
				"avaxAddr": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"initialAmount": 0,
				"unlockSchedule": [
					{
						"amount": 40000000000000000
					}
				]
			},
			{
				"ethAddr": "0xb3d82b1367d362de99ab59a658165aff520cbd4d",
				"avaxAddr": "X-fuji1xpmx0ljrpvqexrvrj26fnggvr0ax9wm32gaxmx",
				"initialAmount": 32000000000000000,
				"unlockSchedule": []
			},
			{
				"ethAddr": "0xb3d82b1367d362de99ab59a658165aff520cbd4d",
				"avaxAddr": "X-fuji1wrv92qg5x3dsqrtukdc8qxnpqust3qdakxgm4s",
				"initialAmount": 32000000000000000,
				"unlockSchedule": []
			},
			{
				"ethAddr": "0xb3d82b1367d362de99ab59a658165aff520cbd4d",
				"avaxAddr": "X-fuji1qrmj7u9pquyy3mahzxeq0nnlnj2aceedjfqqrq",
				"initialAmount": 32000000000000000,
				"unlockSchedule": []
			},
			{
				"ethAddr": "0xb3d82b1367d362de99ab59a658165aff520cbd4d",
				"avaxAddr": "X-fuji1cap3ru2ghc3jtdnuyey738ru8u5ekdadcvrtyk",
				"initialAmount": 32000000000000000,
				"unlockSchedule": []
			},
			{
				"ethAddr": "0xb3d82b1367d362de99ab59a658165aff520cbd4d",
				"avaxAddr": "X-fuji18g2m7483k6swe46cpfmq96t09sp63pgv7judr4",
				"initialAmount": 32000000000000000,
				"unlockSchedule": []
			},
			{
				"ethAddr": "0xb3d82b1367d362de99ab59a658165aff520cbd4d",
				"avaxAddr": "X-fuji1zwe0kxhg73x3ehgtkkz24k9czlfgztc45hgrg3",
				"initialAmount": 32000000000000000,
				"unlockSchedule": []
			},
			{
				"ethAddr": "0xb3d82b1367d362de99ab59a658165aff520cbd4d",
				"avaxAddr": "X-fuji1fqcs4m9p8gdp7gckk30n8u68d55jk0hdumx30f",
				"initialAmount": 32000000000000000,
				"unlockSchedule": []
			},
			{
				"ethAddr": "0xb3d82b1367d362de99ab59a658165aff520cbd4d",
				"avaxAddr": "X-fuji18lany6fjlzxc7vuqfd9x4k9wqp0yhk074p283d",
				"initialAmount": 32000000000000000,
				"unlockSchedule": []
			},
			{
				"ethAddr": "0xb3d82b1367d362de99ab59a658165aff520cbd4d",
				"avaxAddr": "X-fuji18lany6fjlzxc7vuqfd9x4k9wqp0yhk074p283d",
				"initialAmount": 32000000000000000,
				"unlockSchedule": []
			},
			{
				"ethAddr": "0xb3d82b1367d362de99ab59a658165aff520cbd4d",
				"avaxAddr": "X-fuji10d2fqjfl3ghl73z2ez65ufanxwwhccxugq8z2t",
				"initialAmount": 32000000000000000,
				"unlockSchedule": []
			}
		],
		"startTime": 1599696000,
		"initialStakeDuration": 31536000,
		"initialStakeDurationOffset": 54000,
		"initialStakedFunds": [
			"X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw"
		],
		"initialStakers": [
			{
				"nodeID": "NodeID-NpagUxt6KQiwPch9Sd4osv8kD1TZnkjdk",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 1000000
			},
			{
				"nodeID": "NodeID-2m38qc95mhHXtrhjyGbe7r2NhniqHHJRB",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 500000
			},
			{
				"nodeID": "NodeID-LQwRLm4cbJ7T2kxcxp4uXCU5XD8DFrE1C",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 250000
			},
			{
				"nodeID": "NodeID-hArafGhY2HFTbwaaVh1CSCUCUCiJ2Vfb",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 125000
			},
			{
				"nodeID": "NodeID-4QBwET5o8kUhvt9xArhir4d3R25CtmZho",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 62500
			},
			{
				"nodeID": "NodeID-HGZ8ae74J3odT8ESreAdCtdnvWG1J4X5n",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 31250
			},
			{
				"nodeID": "NodeID-4KXitMCoE9p2BHA6VzXtaTxLoEjNDo2Pt",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-JyE4P8f4cTryNV8DCz2M81bMtGhFFHexG",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-EzGaipqomyK9UKx9DBHV6Ky3y68hoknrF",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-CYKruAjwH1BmV3m37sXNuprbr7dGQuJwG",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-LegbVf6qaMKcsXPnLStkdc1JVktmmiDxy",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-FesGqwKq7z5nPFHa5iwZctHE5EZV9Lpdq",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-BFa1padLXBj7VHa2JYvYGzcTBPQGjPhUy",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-4B4rc5vdD1758JSBYL1xyvE5NHGzz6xzH",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-EDESh4DfZFC15i613pMtWniQ9arbBZRnL",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-CZmZ9xpCzkWqjAyS7L4htzh5Lg6kf1k18",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-CTtkcXvVdhpNp6f97LEUXPwsRD3A2ZHqP",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-84KbQHSDnojroCVY7vQ7u9Tx7pUonPaS",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-JjvzhxnLHLUQ5HjVRkvG827ivbLXPwA9u",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			},
			{
				"nodeID": "NodeID-4CWTbdvgXHY1CLXqQNAp22nJDo5nAmts6",
				"rewardAddress": "X-fuji1wycv8n7d2fg9aq6unp23pnj4q0arv03ysya8jw",
				"delegationFee": 20000
			}
		],
		"cChainGenesis": "{\"config\":{\"chainId\":43113,\"homesteadBlock\":0,\"daoForkBlock\":0,\"daoForkSupport\":true,\"eip150Block\":0,\"eip150Hash\":\"0x2086799aeebeae135c246c65021c82b4e15a2c451340993aacfd2751886514f0\",\"eip155Block\":0,\"eip158Block\":0,\"byzantiumBlock\":0,\"constantinopleBlock\":0,\"petersburgBlock\":0,\"istanbulBlock\":0,\"muirGlacierBlock\":0},\"nonce\":\"0x0\",\"timestamp\":\"0x0\",\"extraData\":\"0x00\",\"gasLimit\":\"0x5f5e100\",\"difficulty\":\"0x0\",\"mixHash\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"coinbase\":\"0x0000000000000000000000000000000000000000\",\"alloc\":{\"0100000000000000000000000000000000000000\":{\"code\":\"0x7300000000000000000000000000000000000000003014608060405260043610603d5760003560e01c80631e010439146042578063b6510bb314606e575b600080fd5b605c60048036036020811015605657600080fd5b503560b1565b60408051918252519081900360200190f35b818015607957600080fd5b5060af60048036036080811015608e57600080fd5b506001600160a01b03813516906020810135906040810135906060013560b6565b005b30cd90565b836001600160a01b031681836108fc8690811502906040516000604051808303818888878c8acf9550505050505015801560f4573d6000803e3d6000fd5b505050505056fea26469706673582212201eebce970fe3f5cb96bf8ac6ba5f5c133fc2908ae3dcd51082cfee8f583429d064736f6c634300060a0033\",\"balance\":\"0x0\"}},\"number\":\"0x0\",\"gasUsed\":\"0x0\",\"parentHash\":\"0x0000000000000000000000000000000000000000000000000000000000000000\"}",
		"message": "hi mom"
	}`

	// FujiParams are the params used for the fuji testnet
	FujiParams = Params{
		TxFee:                units.MilliAvax,
		CreationTxFee:        10 * units.MilliAvax,
		UptimeRequirement:    .6, // 60%
		MinValidatorStake:    1 * units.Avax,
		MaxValidatorStake:    3 * units.MegaAvax,
		MinDelegatorStake:    1 * units.Avax,
		MinDelegationFee:     20000, // 2%
		MinStakeDuration:     24 * time.Hour,
		MaxStakeDuration:     365 * 24 * time.Hour,
		StakeMintingPeriod:   365 * 24 * time.Hour,
		EpochFirstTransition: time.Unix(1607626800, 0),
		EpochDuration:        6 * time.Hour,
	}
)
