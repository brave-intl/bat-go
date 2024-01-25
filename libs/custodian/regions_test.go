package custodian

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerdictAllowList(t *testing.T) {
	gabm := GeoAllowBlockMap{
		Allow: []string{"US", "FR"},
	}
	if gabm.Verdict("CA") {
		t.Error("should have failed, CA not in allow list")
	}

	if !gabm.Verdict("US") {
		t.Error("should have passed, US in allow list")
	}
}

func TestVerdictBlockList(t *testing.T) {
	gabm := GeoAllowBlockMap{
		Block: []string{"US", "FR"},
	}
	if !gabm.Verdict("CA") {
		t.Error("should have been true, CA not in block list")
	}

	if gabm.Verdict("US") {
		t.Error("should have been false, US in block list")
	}
}

func TestRegions_Decode(t *testing.T) {
	type tcGiven struct {
		input []byte
	}

	type exp struct {
		allow []string
		block []string
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   exp
	}

	testCases := []testCase{
		{
			name: "solana",
			given: tcGiven{
				input: []byte(`{"solana":{"allow":["AA"],"block":["AB"]}}`),
			},
			exp: exp{
				allow: []string{"AA"},
				block: []string{"AB"},
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			regions := Regions{}
			err := regions.Decode(context.Background(), tc.given.input)
			require.NoError(t, err)

			assert.Equal(t, tc.exp.allow, regions.Solana.Allow)
			assert.Equal(t, tc.exp.block, regions.Solana.Block)
		})
	}
}
