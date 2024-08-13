package radom

import (
	"testing"
	"time"

	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

func TestSubscriptionResponse_NextBillingDate(t *testing.T) {
	type tcGiven struct {
		subResp SubscriptionResponse
	}

	type tcExpected struct {
		nxtB    time.Time
		mustErr func(t must.TestingT, err error, i ...interface{})
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "next_billing_date",
			given: tcGiven{
				subResp: SubscriptionResponse{
					NextBillingDateAt: "2024-01-01T00:00:00.000000Z",
				},
			},
			exp: tcExpected{
				nxtB: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},

		{
			name: "next_billing_date_invalid_format",
			given: tcGiven{
				subResp: SubscriptionResponse{
					NextBillingDateAt: "invalid_date_format",
				},
			},
			exp: tcExpected{
				nxtB: time.Time{},
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorContains(t, err, "cannot parse \"invalid_date_format\"")
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.given.subResp.NextBillingDate()
			tc.exp.mustErr(t, err)

			should.Equal(t, tc.exp.nxtB, actual)
		})
	}
}

func TestSubscriptionResponse_LastPaid(t *testing.T) {
	type tcGiven struct {
		subResp SubscriptionResponse
	}

	type tcExpected struct {
		lastPaid time.Time
		mustErr  func(t must.TestingT, err error, i ...interface{})
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "last_paid_empty",
			exp: tcExpected{
				lastPaid: time.Time{},
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, ErrPaymentsEmpty)
				},
			},
		},

		{
			name: "last_paid_invalid_format",
			given: tcGiven{
				subResp: SubscriptionResponse{
					Payments: []Payment{
						{
							Date: "invalid_date_format",
						},
					},
				},
			},
			exp: tcExpected{
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorContains(t, err, "cannot parse \"invalid_date_format\"")
				},
			},
		},

		{
			name: "last_paid",
			given: tcGiven{
				subResp: SubscriptionResponse{
					Payments: []Payment{
						{
							Date: "2024-01-01T00:00:00.000000Z",
						},

						{
							Date: "2024-02-01T00:00:00.000000Z",
						},

						{
							Date: "2024-03-01T00:00:00.000000Z",
						},
					},
				},
			},
			exp: tcExpected{
				lastPaid: time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC),
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.given.subResp.LastPaid()
			tc.exp.mustErr(t, err)

			should.Equal(t, tc.exp.lastPaid, actual)
		})
	}
}
