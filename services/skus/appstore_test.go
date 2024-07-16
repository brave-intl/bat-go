package skus

import (
	"testing"
	"time"

	"github.com/awa/go-iap/appstore"
	should "github.com/stretchr/testify/assert"
)

func TestAppStoreSrvNotification_shouldProcess(t *testing.T) {
	type testCase struct {
		name  string
		given *appStoreSrvNotification
		exp   bool
	}

	tests := []testCase{
		{
			name: "should_renew",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2DidRenew,
				},
			},
			exp: true,
		},

		{
			name: "should_cancel",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2DidChangeRenewalStatus,
					Subtype:          appstore.SubTypeV2AutoRenewDisabled,
				},
			},
			exp: true,
		},

		{
			name: "anything_else",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2PriceIncrease,
					Subtype:          appstore.SubTypeV2Accepted,
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.shouldProcess()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestAppStoreSrvNotification_shouldRenew(t *testing.T) {
	type testCase struct {
		name  string
		given *appStoreSrvNotification
		exp   bool
	}

	tests := []testCase{
		{
			name: "auto_renew",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2DidRenew,
				},
			},
			exp: true,
		},

		{
			name: "billing_recovered",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2DidRenew,
					Subtype:          appstore.SubTypeV2BillingRecovery,
				},
			},
			exp: true,
		},

		{
			name: "resubscribed_after_cancellation",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2DidChangeRenewalStatus,
					Subtype:          appstore.SubTypeV2AutoRenewEnabled,
				},
			},
			exp: true,
		},

		{
			name: "resubscribed_after_expiration",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2Subscribed,
					Subtype:          appstore.SubTypeV2Resubscribe,
				},
			},
			exp: true,
		},

		{
			name: "anything_else",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2PriceIncrease,
					Subtype:          appstore.SubTypeV2Accepted,
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.shouldRenew()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestAppStoreSrvNotification_shouldCancel(t *testing.T) {
	type testCase struct {
		name  string
		given *appStoreSrvNotification
		exp   bool
	}

	tests := []testCase{
		{
			name: "cancellation_or_refund",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2DidChangeRenewalStatus,
					Subtype:          appstore.SubTypeV2AutoRenewDisabled,
				},
			},
			exp: true,
		},

		{
			name: "refund_processed",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2Refund,
				},
			},
			exp: true,
		},

		{
			name: "expired_after_cancellation",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2Expired,
					Subtype:          appstore.SubTypeV2Voluntary,
				},
			},
			exp: true,
		},

		{
			name: "expired_after_billing_retry_ended_no_recovery",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2Expired,
					Subtype:          appstore.SubTypeV2BillingRetry,
				},
			},
			exp: true,
		},

		{
			name: "cancellation_after_price_increase",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2DidChangeRenewalStatus,
				},
			},
			exp: true,
		},

		{
			name: "anything_else",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2PriceIncrease,
					Subtype:          appstore.SubTypeV2Accepted,
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.shouldCancel()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestAppStoreSrvNotification_effect(t *testing.T) {
	type testCase struct {
		name  string
		given *appStoreSrvNotification
		exp   string
	}

	tests := []testCase{
		{
			name: "should_renew",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2DidRenew,
				},
			},
			exp: "renew",
		},

		{
			name: "should_cancel",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2DidChangeRenewalStatus,
					Subtype:          appstore.SubTypeV2AutoRenewDisabled,
				},
			},
			exp: "cancel",
		},

		{
			name: "anything_else",
			given: &appStoreSrvNotification{
				val: &appstore.SubscriptionNotificationV2DecodedPayload{
					NotificationType: appstore.NotificationTypeV2PriceIncrease,
					Subtype:          appstore.SubTypeV2Accepted,
				},
			},
			exp: "skip",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.effect()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestShouldCancelOrderIOS(t *testing.T) {
	type tcGiven struct {
		now  time.Time
		info *appstore.JWSTransactionDecodedPayload
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   bool
	}

	tests := []testCase{
		{
			name: "nil",
			given: tcGiven{
				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
		},

		{
			name: "empty_dates_not_expired",
			given: tcGiven{
				now:  time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				info: &appstore.JWSTransactionDecodedPayload{},
			},
		},

		{
			name: "expires_date_before_no_revocation_date",
			given: tcGiven{
				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				info: &appstore.JWSTransactionDecodedPayload{
					// 2023-12-31 23:59:59.
					ExpiresDate: 1704067199000,
				},
			},
			exp: true,
		},

		{
			name: "expires_date_after_no_revocation_date",
			given: tcGiven{
				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				info: &appstore.JWSTransactionDecodedPayload{
					// 2024-01-01 01:00:01.
					ExpiresDate: 1704070801000,
				},
			},
		},

		{
			name: "expires_date_after_revocation_date_after",
			given: tcGiven{
				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				info: &appstore.JWSTransactionDecodedPayload{
					// 2024-01-01 01:00:01.
					ExpiresDate: 1704070801000,

					// 2024-01-01 00:30:01.
					RevocationDate: 1704069001000,
				},
			},
		},

		{
			name: "expires_date_after_revocation_date_before",
			given: tcGiven{
				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				info: &appstore.JWSTransactionDecodedPayload{
					// 2024-01-01 01:00:01.
					ExpiresDate: 1704070801000,

					// 2023-12-31 23:30:01.
					RevocationDate: 1704065401000,
				},
			},
			exp: true,
		},

		{
			name: "no_expires_date_revocation_date_before",
			given: tcGiven{
				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				info: &appstore.JWSTransactionDecodedPayload{
					// 2023-12-31 23:59:59.
					RevocationDate: 1704067199000,
				},
			},
			exp: true,
		},

		{
			name: "no_expires_date_revocation_date_after",
			given: tcGiven{
				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				info: &appstore.JWSTransactionDecodedPayload{
					// 2024-01-01 01:00:01.
					RevocationDate: 1704070801000,
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := shouldCancelOrderIOS(tc.given.info, tc.given.now)

			should.Equal(t, tc.exp, actual)
		})
	}
}
