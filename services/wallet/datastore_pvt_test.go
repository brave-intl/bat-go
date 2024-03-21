package wallet

import (
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/ptr"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

func TestCustodianLink_isLinked(t *testing.T) {
	type tcGiven struct {
		cl *CustodianLink
	}

	type testCase struct {
		name     string
		given    tcGiven
		expected bool
	}

	tests := []testCase{
		{
			name: "is_linked",
			given: tcGiven{
				cl: &CustodianLink{
					LinkedAt: time.Now(),
				}},
			expected: true,
		},
		{
			name:  "is_not_linked_nil",
			given: tcGiven{},
		},
		{
			name: "is_not_linked",
			given: tcGiven{
				cl: &CustodianLink{
					LinkedAt:   time.Now(),
					UnlinkedAt: ptr.To(time.Now()),
				}},
		},
		{
			name: "is_not_linked_disconnected",
			given: tcGiven{
				cl: &CustodianLink{
					LinkedAt:       time.Now(),
					DisconnectedAt: ptr.To(time.Now()),
				}},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.cl.isLinked()
			should.Equal(t, tc.expected, actual)
		})
	}
}

func TestGetEnvMaxCards(t *testing.T) {
	type tcGiven struct {
		custodian string
		key       string
		value     string
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   int
	}

	tests := []testCase{
		{
			name: "solana",
			given: tcGiven{
				custodian: "solana",
				key:       "SOLANA_WALLET_LINKING_LIMIT",
				value:     "10",
			},
			exp: 10,
		},
		{
			name: "non_existent_custodian",
			given: tcGiven{
				key:   "NON_EXISTENT_CUSTODIAN",
				value: "10",
			},
			exp: 4,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {

			t.Cleanup(func() {
				err := os.Unsetenv(tc.given.key)
				must.NoError(t, err)
			})

			err := os.Setenv(tc.given.key, tc.given.value)
			must.NoError(t, err)

			actual := getEnvMaxCards(tc.given.custodian)
			should.Equal(t, tc.exp, actual)
		})
	}
}
