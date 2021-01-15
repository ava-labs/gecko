package secp256k1fx

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/stretchr/testify/require"
)

func TestManagedAssetStatusOutput(t *testing.T) {
	type test struct {
		out              *ManagedAssetStatusOutput
		description      string
		shouldFailVerify bool
	}

	tests := []test{
		{
			description: "valid",
			out: &ManagedAssetStatusOutput{
				IsFrozen: false,
				Mgr: OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
				},
			},
			shouldFailVerify: false,
		},
		{
			description: "threshold > len(addrs)",
			out: &ManagedAssetStatusOutput{
				IsFrozen: false,
				Mgr: OutputOwners{
					Locktime:  0,
					Threshold: 2,
					Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
				},
			},
			shouldFailVerify: true,
		},
		{
			description: "threshold 0, len(addrs) > 0",
			out: &ManagedAssetStatusOutput{
				IsFrozen: false,
				Mgr: OutputOwners{
					Locktime:  0,
					Threshold: 0,
					Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
				},
			},
			shouldFailVerify: true,
		},
		{
			description: "threshold 0, len(addrs) 0",
			out: &ManagedAssetStatusOutput{
				IsFrozen: false,
				Mgr: OutputOwners{
					Locktime:  0,
					Threshold: 0,
					Addrs:     []ids.ShortID{},
				},
			},
			shouldFailVerify: true,
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			err := test.out.Verify()
			if test.shouldFailVerify {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
