package radom

import (
	"testing"
	"time"

	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

func TestGetCheckoutSessionResponse_IsSessionExpired(t *testing.T) {
	type tcGiven struct {
		r GetCheckoutSessionResponse
	}

	type tcExpected struct {
		result bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "expired",
			given: tcGiven{
				r: GetCheckoutSessionResponse{
					SessionStatus: "expired",
				},
			},
			exp: tcExpected{
				result: true,
			},
		},

		{
			name: "not_expired",
			given: tcGiven{
				r: GetCheckoutSessionResponse{
					SessionStatus: "session_status",
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.r.IsSessionExpired()
			should.Equal(t, tc.exp.result, actual)
		})
	}
}

func TestGetCheckoutSessionResponse_IsSessionSuccess(t *testing.T) {
	type tcGiven struct {
		r GetCheckoutSessionResponse
	}

	type tcExpected struct {
		result bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "success",
			given: tcGiven{
				r: GetCheckoutSessionResponse{
					SessionStatus: "success",
				},
			},
			exp: tcExpected{
				result: true,
			},
		},

		{
			name: "not_success",
			given: tcGiven{
				r: GetCheckoutSessionResponse{
					SessionStatus: "session_status",
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.r.IsSessionSuccess()
			should.Equal(t, tc.exp.result, actual)
		})
	}
}

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

func TestSubscriptionResponse_SubID(t *testing.T) {
	type tcGiven struct {
		subResp *SubscriptionResponse
	}

	type tcExpected struct {
		sid string
		ok  bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "nil",
		},

		{
			name: "empty_id",
			given: tcGiven{
				subResp: &SubscriptionResponse{},
			},
		},

		{
			name: "id",
			given: tcGiven{
				subResp: &SubscriptionResponse{
					ID: "id",
				},
			},
			exp: tcExpected{
				sid: "id",
				ok:  true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := tc.given.subResp.SubID()
			should.Equal(t, tc.exp.ok, ok)
			should.Equal(t, tc.exp.sid, actual)
		})
	}
}

func TestSubscriptionResponse_IsActive(t *testing.T) {
	type tcGiven struct {
		subResp *SubscriptionResponse
	}

	type tcExpected struct {
		result bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "nil",
		},

		{
			name: "empty_id",
			given: tcGiven{
				subResp: &SubscriptionResponse{},
			},
		},

		{
			name: "not_active",
			given: tcGiven{
				subResp: &SubscriptionResponse{
					Status: "expired",
				},
			},
		},

		{
			name: "active",
			given: tcGiven{
				subResp: &SubscriptionResponse{
					Status: "active",
				},
			},
			exp: tcExpected{
				result: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.subResp.IsActive()
			should.Equal(t, tc.exp.result, actual)
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
					must.ErrorIs(t, err, ErrSubPaymentsEmpty)
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
