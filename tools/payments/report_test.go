package payments_test

import (
	"errors"
	"testing"

	should "github.com/stretchr/testify/assert"

	"github.com/brave-intl/bat-go/tools/payments"
)

func TestAttestedReport_EnsureUniqueDest(t *testing.T) {
	type testCase struct {
		name  string
		given []*payments.AttestedTx
		exp   error
	}

	tests := []testCase{
		{
			name: "empty",
		},

		{
			name: "one_item",
			given: []*payments.AttestedTx{
				&payments.AttestedTx{
					Tx: payments.Tx{
						To: "01",
					},
				},
			},
		},

		{
			name: "two_unique",
			given: []*payments.AttestedTx{
				&payments.AttestedTx{
					Tx: payments.Tx{
						To: "01",
					},
				},

				&payments.AttestedTx{
					Tx: payments.Tx{
						To: "02",
					},
				},
			},
		},

		{
			name: "two_unique_one_dupe",
			given: []*payments.AttestedTx{
				&payments.AttestedTx{
					Tx: payments.Tx{
						To: "01",
					},
				},

				&payments.AttestedTx{
					Tx: payments.Tx{
						To: "02",
					},
				},

				&payments.AttestedTx{
					Tx: payments.Tx{
						To: "02",
					},
				},
			},
			exp: payments.ErrDuplicateDepositDestination,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act := payments.AttestedReport(tc.given).EnsureUniqueDest()
			should.Equal(t, true, errors.Is(act, tc.exp))
		})
	}
}

func TestPreparedReport_EnsureUniqueDest(t *testing.T) {
	type testCase struct {
		name  string
		given []*payments.PrepareTx
		exp   error
	}

	tests := []testCase{
		{
			name: "empty",
		},

		{
			name: "one_item",
			given: []*payments.PrepareTx{
				&payments.PrepareTx{
					To: "01",
				},
			},
		},

		{
			name: "two_unique",
			given: []*payments.PrepareTx{
				&payments.PrepareTx{
					To: "01",
				},

				&payments.PrepareTx{
					To: "02",
				},
			},
		},

		{
			name: "two_unique_one_dupe",
			given: []*payments.PrepareTx{
				&payments.PrepareTx{
					To: "01",
				},

				&payments.PrepareTx{
					To: "02",
				},

				&payments.PrepareTx{
					To: "02",
				},
			},
			exp: payments.ErrDuplicateDepositDestination,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act := payments.PreparedReport(tc.given).EnsureUniqueDest()
			should.Equal(t, true, errors.Is(act, tc.exp))
		})
	}
}
