package secp256k1fx

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/stretchr/testify/require"
)

func TestUpdateManagedAssetVerify(t *testing.T) {
	type test struct {
		op               *UpdateManagedAssetOperation
		description      string
		shouldFailVerify bool
	}

	tests := []test{
		{
			description:      "nil",
			op:               nil,
			shouldFailVerify: true,
		},
		{
			description: "valid",
			op: &UpdateManagedAssetOperation{
				Input: Input{
					SigIndices: []uint32{0},
				},
				ManagedAssetStatusOutput: ManagedAssetStatusOutput{
					IsFrozen: false,
					Mgr: OutputOwners{
						Locktime:  0,
						Threshold: 1,
						Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
					},
				},
			},
			shouldFailVerify: false,
		},
		{
			description: "empty input",
			op: &UpdateManagedAssetOperation{
				Input: Input{},
				ManagedAssetStatusOutput: ManagedAssetStatusOutput{
					IsFrozen: false,
					Mgr: OutputOwners{
						Locktime:  0,
						Threshold: 1,
						Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
					},
				},
			},
			shouldFailVerify: false,
		},
		{
			description: "empty ManagedAssetStatusOutput",
			op: &UpdateManagedAssetOperation{
				Input: Input{
					SigIndices: []uint32{0},
				},
				ManagedAssetStatusOutput: ManagedAssetStatusOutput{},
			},
			shouldFailVerify: true,
		},
		{
			description: "manager threshold == 0",
			op: &UpdateManagedAssetOperation{
				Input: Input{
					SigIndices: []uint32{0},
				},
				ManagedAssetStatusOutput: ManagedAssetStatusOutput{
					IsFrozen: false,
					Mgr: OutputOwners{
						Locktime:  0,
						Threshold: 0,
						Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
					},
				},
			},
			shouldFailVerify: true,
		},
		{
			description: "manager no addresses",
			op: &UpdateManagedAssetOperation{
				Input: Input{
					SigIndices: []uint32{0},
				},
				ManagedAssetStatusOutput: ManagedAssetStatusOutput{
					IsFrozen: false,
					Mgr: OutputOwners{
						Locktime:  0,
						Threshold: 0,
						Addrs:     []ids.ShortID{},
					},
				},
			},
			shouldFailVerify: true,
		},
		{
			description: "manager no addresses 2",
			op: &UpdateManagedAssetOperation{
				Input: Input{
					SigIndices: []uint32{0},
				},
				ManagedAssetStatusOutput: ManagedAssetStatusOutput{
					IsFrozen: false,
					Mgr: OutputOwners{
						Locktime:  0,
						Threshold: 1,
						Addrs:     []ids.ShortID{},
					},
				},
			},
			shouldFailVerify: true,
		},
		{
			description: "manager threshold > len(addrs)",
			op: &UpdateManagedAssetOperation{
				Input: Input{
					SigIndices: []uint32{0},
				},
				ManagedAssetStatusOutput: ManagedAssetStatusOutput{
					IsFrozen: false,
					Mgr: OutputOwners{
						Locktime:  0,
						Threshold: 2,
						Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
					},
				},
			},
			shouldFailVerify: true,
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			err := test.op.Verify()
			if test.shouldFailVerify {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
