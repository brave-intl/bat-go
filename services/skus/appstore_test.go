package skus

import (
	"testing"

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
