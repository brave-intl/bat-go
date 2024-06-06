package skus

import (
	"testing"
	"time"

	should "github.com/stretchr/testify/assert"
)

func TestPlayStoreSubPurchase_hasExpired(t *testing.T) {
	type tcGiven struct {
		sub *playStoreSubPurchase
		now time.Time
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   bool
	}

	tests := []testCase{
		{
			name: "zero_expired",
			given: tcGiven{
				sub: &playStoreSubPurchase{},
				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: true,
		},

		{
			name: "not_expired",
			given: tcGiven{
				sub: &playStoreSubPurchase{
					ExpiryTimeMillis: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC).UnixMilli(),
				},

				now: time.Date(2024, time.January, 2, 0, 0, 1, 0, time.UTC),
			},
			exp: true,
		},

		{
			name: "not_expired_equal",
			given: tcGiven{
				sub: &playStoreSubPurchase{
					ExpiryTimeMillis: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC).UnixMilli(),
				},

				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.sub.hasExpired(tc.given.now)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestPlayStoreSubPurchase_isPending(t *testing.T) {
	type testCase struct {
		name  string
		given *playStoreSubPurchase
		exp   bool
	}

	tests := []testCase{
		{
			name:  "not_pending_no_payment",
			given: &playStoreSubPurchase{},
		},

		{
			name:  "not_pending_paid",
			given: &playStoreSubPurchase{PaymentState: ptrTo(int64(1))},
		},

		{
			name:  "pending",
			given: &playStoreSubPurchase{PaymentState: ptrTo(int64(0))},
			exp:   true,
		},

		{
			name:  "pending_deferred",
			given: &playStoreSubPurchase{PaymentState: ptrTo(int64(3))},
			exp:   true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.isPending()
			should.Equal(t, tc.exp, actual)
		})
	}
}
