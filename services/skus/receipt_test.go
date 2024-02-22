package skus

import (
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/ptr"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/androidpublisher/v3"
)

func TestIsExpired(t *testing.T) {
	type tcGiven struct {
		expTimeMills int64
		now          time.Time
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   bool
	}

	tests := []testCase{
		{
			name: "expired",
			given: tcGiven{
				now: time.Now(),
			},
			exp: true,
		},
		{
			name: "not_expired_equal",
			given: tcGiven{
				expTimeMills: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC).UnixMilli(),
				now:          time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
		},
		{
			name: "not_expired_future",
			given: tcGiven{
				expTimeMills: time.Now().Add(1 * time.Minute).UnixMilli(),
				now:          time.Now(),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := isSubPurchaseExpired(tc.given.expTimeMills, tc.given.now)
			assert.Equal(t, tc.exp, actual)
		})
	}
}

func TestIsPending(t *testing.T) {
	type tcGiven struct {
		resp *androidpublisher.SubscriptionPurchase
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   bool
	}

	tests := []testCase{
		{
			name:  "not_pending_no_payment_state",
			given: tcGiven{resp: &androidpublisher.SubscriptionPurchase{}},
		},
		{
			name: "not_pending_paid_state",
			given: tcGiven{resp: &androidpublisher.SubscriptionPurchase{
				PaymentState: ptr.To(int64(1)),
			}},
		},
		{
			name: "pending",
			given: tcGiven{resp: &androidpublisher.SubscriptionPurchase{
				PaymentState: ptr.To(int64(0)),
			}},
			exp: true,
		},
		{
			name: "pending_deferred",
			given: tcGiven{resp: &androidpublisher.SubscriptionPurchase{
				PaymentState: ptr.To(int64(3)),
			}},
			exp: true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := isSubPurchasePending(tc.given.resp)
			assert.Equal(t, tc.exp, actual)
		})
	}
}
