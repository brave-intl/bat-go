package payments_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"

	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"github.com/brave-intl/payments-service/tools/payments"
)

func TestAttestedReport_EnsureUniqueDest(t *testing.T) {
	type testCase struct {
		name  string
		given payments.AttestedReport
		exp   error
	}

	tests := []testCase{
		{
			name: "empty",
		},

		{
			name: "one_item",
			given: payments.AttestedReport{
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
						To: "01",
					},
				},
			},
		},

		{
			name: "two_unique",
			given: payments.AttestedReport{
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
						To: "01",
					},
				},
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
						To: "02",
					},
				},
			},
		},

		{
			name: "two_unique_one_dupe",
			given: payments.AttestedReport{
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
						To: "01",
					},
				},
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
						To: "02",
					},
				},
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
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
		given payments.PreparedReport
		exp   error
	}

	tests := []testCase{
		{
			name: "empty",
		},

		{
			name: "one_item",
			given: payments.PreparedReport{
				&paymentLib.PaymentDetails{
					To: "01",
				},
			},
		},

		{
			name: "two_unique",
			given: payments.PreparedReport{
				&paymentLib.PaymentDetails{
					To: "01",
				},
				&paymentLib.PaymentDetails{
					To: "02",
				},
			},
		},

		{
			name: "two_unique_one_dupe",
			given: payments.PreparedReport{
				&paymentLib.PaymentDetails{
					To: "01",
				},
				&paymentLib.PaymentDetails{
					To: "02",
				},
				&paymentLib.PaymentDetails{
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

func TestAttestedReport_EnsureTransactionAmountsMatch(t *testing.T) {
	type attestedTestCase struct {
		name  string
		given payments.AttestedReport
		exp   error
	}
	type preparedTestCase struct {
		given payments.PreparedReport
	}

	attestedTests := []attestedTestCase{
		{
			name: "empty",
		},

		{
			name: "one_item",
			given: payments.AttestedReport{
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
						To:     "01",
						Amount: decimal.NewFromFloat(1.234),
					},
				},
			},
		},

		{
			name: "two_unique",
			given: payments.AttestedReport{
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
						To:     "01",
						Amount: decimal.NewFromFloat(1.234),
					},
				},
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
						To:     "02",
						Amount: decimal.NewFromFloat(1.234),
					},
				},
			},
		},

		{
			name: "two_unique_one_dupe",
			given: payments.AttestedReport{
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
						To:     "01",
						Amount: decimal.NewFromFloat(1.234),
					},
				},
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
						To:     "02",
						Amount: decimal.NewFromFloat(1.234),
					},
				},
				paymentLib.PrepareResponse{
					PaymentDetails: paymentLib.PaymentDetails{
						To:     "03",
						Amount: decimal.NewFromFloat(1.235),
					},
				},
			},
			exp: payments.ErrMismatchedDepositAmounts,
		},
	}
	preparedTests := []preparedTestCase{
		{},

		{
			given: payments.PreparedReport{
				&paymentLib.PaymentDetails{
					To:     "01",
					Amount: decimal.NewFromFloat(1.234),
				},
			},
		},

		{
			given: payments.PreparedReport{
				&paymentLib.PaymentDetails{
					To:     "01",
					Amount: decimal.NewFromFloat(1.234),
				},
				&paymentLib.PaymentDetails{
					To:     "02",
					Amount: decimal.NewFromFloat(1.234),
				},
			},
		},

		{
			given: payments.PreparedReport{
				&paymentLib.PaymentDetails{
					To:     "01",
					Amount: decimal.NewFromFloat(1.234),
				},
				&paymentLib.PaymentDetails{
					To:     "02",
					Amount: decimal.NewFromFloat(1.234),
				},
				&paymentLib.PaymentDetails{
					To:     "03",
					Amount: decimal.NewFromFloat(1.234),
				},
			},
		},
	}

	for i := range attestedTests {
		ac := attestedTests[i]
		pc := preparedTests[i]

		t.Run(ac.name, func(t *testing.T) {
			act := payments.AttestedReport(ac.given).EnsureTransactionAmountsMatch(pc.given)
			should.Equal(t, true, errors.Is(act, ac.exp))
		})
	}
}
