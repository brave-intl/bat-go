package skus

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
	"google.golang.org/api/idtoken"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type mockGPSTokenValidator struct {
	fnValidate func(ctx context.Context, token, aud string) (*idtoken.Payload, error)
}

func (m *mockGPSTokenValidator) Validate(ctx context.Context, token, aud string) (*idtoken.Payload, error) {
	if m.fnValidate == nil {
		return &idtoken.Payload{Audience: aud}, nil
	}

	return m.fnValidate(ctx, token, aud)
}

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

func TestGPSNtfAuthenticator_authenticate(t *testing.T) {
	type tcGiven struct {
		hdr   string
		cfg   gpsValidatorConfig
		valid *mockGPSTokenValidator
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "disabled",
			given: tcGiven{
				cfg:   gpsValidatorConfig{disabled: true},
				valid: &mockGPSTokenValidator{},
			},
			exp: errGPSDisabled,
		},

		{
			name: "invalid_auth_header_empty",
			given: tcGiven{
				valid: &mockGPSTokenValidator{},
			},
			exp: errGPSAuthHeaderEmpty,
		},

		{
			name: "invalid_auth_header_fmt",
			given: tcGiven{
				hdr:   "some-random-header-value",
				valid: &mockGPSTokenValidator{},
			},
			exp: errGPSAuthHeaderFmt,
		},

		{
			name: "invalid_auth_token",
			given: tcGiven{
				hdr: "Bearer: some_token",
				valid: &mockGPSTokenValidator{
					func(ctx context.Context, token, aud string) (*idtoken.Payload, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "invalid_issuer_empty",
			given: tcGiven{
				hdr: "Bearer: some_token",
				valid: &mockGPSTokenValidator{
					func(ctx context.Context, token, aud string) (*idtoken.Payload, error) {
						return &idtoken.Payload{}, nil
					},
				},
			},
			exp: errGPSInvalidIssuer,
		},

		{
			name: "invalid_issuer_diff",
			given: tcGiven{
				hdr: "Bearer: some_token",
				cfg: gpsValidatorConfig{iss: "issuer_01"},
				valid: &mockGPSTokenValidator{
					func(ctx context.Context, token, aud string) (*idtoken.Payload, error) {
						return &idtoken.Payload{Issuer: "issuer_02"}, nil
					},
				},
			},
			exp: errGPSInvalidIssuer,
		},

		{
			name: "invalid_invalid_email",
			given: tcGiven{
				hdr: "Bearer: some_token",
				cfg: gpsValidatorConfig{
					iss:     "issuer_01",
					svcAcct: "account-01@appspot.gserviceaccount.com",
				},
				valid: &mockGPSTokenValidator{
					func(ctx context.Context, token, aud string) (*idtoken.Payload, error) {
						result := &idtoken.Payload{
							Issuer: "issuer_01",
							Claims: map[string]interface{}{"email": "account-02@appspot.gserviceaccount.com"},
						}

						return result, nil
					},
				},
			},
			exp: errGPSInvalidEmail,
		},

		{
			name: "invalid_email_not_verified",
			given: tcGiven{
				hdr: "Bearer: some_token",
				cfg: gpsValidatorConfig{
					iss:     "issuer_01",
					svcAcct: "account-01@appspot.gserviceaccount.com",
				},
				valid: &mockGPSTokenValidator{
					func(ctx context.Context, token, aud string) (*idtoken.Payload, error) {
						result := &idtoken.Payload{
							Issuer: "issuer_01",
							Claims: map[string]interface{}{"email": "account-01@appspot.gserviceaccount.com"},
						}

						return result, nil
					},
				},
			},
			exp: errGPSEmailNotVerified,
		},

		{
			name: "valid",
			given: tcGiven{
				hdr: "Bearer: some_token",
				cfg: gpsValidatorConfig{
					iss:     "issuer_01",
					svcAcct: "account-01@appspot.gserviceaccount.com",
				},
				valid: &mockGPSTokenValidator{
					func(ctx context.Context, token, aud string) (*idtoken.Payload, error) {
						result := &idtoken.Payload{
							Issuer: "issuer_01",
							Claims: map[string]interface{}{
								"email":          "account-01@appspot.gserviceaccount.com",
								"email_verified": true,
							},
						}

						return result, nil
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			auth := newGPSNtfAuthenticator(tc.given.cfg, tc.given.valid)

			actual := auth.authenticate(context.Background(), tc.given.hdr)
			should.Equal(t, true, errors.Is(actual, tc.exp))
		})
	}
}

func TestParsePlayStoreDevNotification(t *testing.T) {
	type tcExpected struct {
		val   *playStoreDevNotification
		fnErr must.ErrorAssertionFunc
	}

	type testCase struct {
		name  string
		given []byte
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "invalid_input",
			exp: tcExpected{
				fnErr: func(tt must.TestingT, err error, i ...interface{}) {
					must.ErrorContains(tt, err, "failed to unmarshal message:")
				},
			},
		},

		{
			name:  "invalid_input_base64",
			given: []byte(`{"message": {"data": "not-base64"}}`),
			exp: tcExpected{
				fnErr: func(tt must.TestingT, err error, i ...interface{}) {
					must.ErrorContains(tt, err, "failed to decode message data:")
				},
			},
		},

		{
			name:  "invalid_input_inner_data",
			given: []byte(`{"message": {"data": "dGVzdA=="}}`),
			exp: tcExpected{
				fnErr: func(tt must.TestingT, err error, i ...interface{}) {
					must.ErrorContains(tt, err, "failed to unmarshal notification:")
				},
			},
		},

		{
			name:  "valid_subscription",
			given: []byte(`{"subscription": "projects/myproject/subscriptions/mysubscription", "message": { "data": "ewogICAgInZlcnNpb24iOiAiMS4wIiwKICAgICJwYWNrYWdlTmFtZSI6ICJjb20uc29tZS50aGluZyIsCiAgICAiZXZlbnRUaW1lTWlsbGlzIjogIjE1MDMzNDk1NjYxNjgiLAogICAgInN1YnNjcmlwdGlvbk5vdGlmaWNhdGlvbiI6IHsKICAgICAgICAidmVyc2lvbiI6ICIxLjAiLAogICAgICAgICJub3RpZmljYXRpb25UeXBlIjogNCwKICAgICAgICAicHVyY2hhc2VUb2tlbiI6ICJQVVJDSEFTRV9UT0tFTiIsCiAgICAgICAgInN1YnNjcmlwdGlvbklkIjogIm1vbnRobHkwMDEiCiAgICB9Cn0=", "messageId": "136969346945"}}`),
			exp: tcExpected{
				val: &playStoreDevNotification{
					PackageName:    "com.some.thing",
					EventTimeMilli: json.Number("1503349566168"),
					SubscriptionNtf: &playStoreSubscriptionNtf{
						Type:          4,
						PurchaseToken: "PURCHASE_TOKEN",
						SubID:         "monthly001",
					},
				},
			},
		},

		{
			name:  "valid_one_time_product",
			given: []byte(`{"subscription": "projects/myproject/subscriptions/mysubscription", "message": { "data": "ewogICAgInZlcnNpb24iOiIxLjAiLAogICAgInBhY2thZ2VOYW1lIjoiY29tLnNvbWUudGhpbmciLAogICAgImV2ZW50VGltZU1pbGxpcyI6IjE1MDMzNDk1NjYxNjgiLAogICAgIm9uZVRpbWVQcm9kdWN0Tm90aWZpY2F0aW9uIjogewogICAgICAgICJ2ZXJzaW9uIjoiMS4wIiwKICAgICAgICAibm90aWZpY2F0aW9uVHlwZSI6MSwKICAgICAgICAicHVyY2hhc2VUb2tlbiI6IlBVUkNIQVNFX1RPS0VOIiwKICAgICAgICAic2t1IjoibXkuc2t1IgogICAgfQp9", "messageId": "136969346945"}}`),
			exp: tcExpected{
				val: &playStoreDevNotification{
					PackageName:       "com.some.thing",
					EventTimeMilli:    json.Number("1503349566168"),
					OneTimeProductNtf: &struct{}{},
				},
			},
		},

		{
			name:  "valid_voided_purchase",
			given: []byte(`{"subscription": "projects/myproject/subscriptions/mysubscription", "message": { "data": "ewogICAgInZlcnNpb24iOiIxLjAiLAogICAgInBhY2thZ2VOYW1lIjoiY29tLnNvbWUudGhpbmciLAogICAgImV2ZW50VGltZU1pbGxpcyI6IjE1MDMzNDk1NjYxNjgiLAogICAgInZvaWRlZFB1cmNoYXNlTm90aWZpY2F0aW9uIjogewogICAgICAgICJwdXJjaGFzZVRva2VuIjoiUFVSQ0hBU0VfVE9LRU4iLAogICAgICAgICJvcmRlcklkIjoiR1MuMDAwMC0wMDAwLTAwMDAiLAogICAgICAgICJwcm9kdWN0VHlwZSI6MSwKICAgICAgICAicmVmdW5kVHlwZSI6MQogICAgfQp9", "messageId": "136969346945"}}`),
			exp: tcExpected{
				val: &playStoreDevNotification{
					PackageName:    "com.some.thing",
					EventTimeMilli: json.Number("1503349566168"),
					VoidedPurchaseNtf: &playStoreVoidedPurchaseNtf{
						ProductType:   1,
						RefundType:    1,
						PurchaseToken: "PURCHASE_TOKEN",
					},
				},
			},
		},

		{
			name:  "valid_test",
			given: []byte(`{"subscription": "projects/myproject/subscriptions/mysubscription", "message": { "data": "ewogICAgInZlcnNpb24iOiIxLjAiLAogICAgInBhY2thZ2VOYW1lIjoiY29tLnNvbWUudGhpbmciLAogICAgImV2ZW50VGltZU1pbGxpcyI6IjE1MDMzNDk1NjYxNjgiLAogICAgInRlc3ROb3RpZmljYXRpb24iOiB7CiAgICAgICAgInZlcnNpb24iOiIxLjAiCiAgICB9Cn0=", "messageId": "136969346945"}}`),
			exp: tcExpected{
				val: &playStoreDevNotification{
					PackageName:    "com.some.thing",
					EventTimeMilli: json.Number("1503349566168"),
					TestNtf:        &struct{}{},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := parsePlayStoreDevNotification(tc.given)
			if tc.exp.fnErr != nil {
				tc.exp.fnErr(t, err)

				return
			}

			must.Equal(t, nil, err)

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestPlayStoreDevNotification_shouldProcess(t *testing.T) {
	type testCase struct {
		name  string
		given *playStoreDevNotification
		exp   bool
	}

	tests := []testCase{
		{
			name: "sub_true",
			given: &playStoreDevNotification{
				SubscriptionNtf: &playStoreSubscriptionNtf{Type: 1},
			},
			exp: true,
		},

		{
			name: "sub_false",
			given: &playStoreDevNotification{
				SubscriptionNtf: &playStoreSubscriptionNtf{Type: 20},
			},
		},

		{
			name: "voided_purchase_true",
			given: &playStoreDevNotification{
				VoidedPurchaseNtf: &playStoreVoidedPurchaseNtf{ProductType: 1},
			},
			exp: true,
		},

		{
			name: "voided_purchase_false",
			given: &playStoreDevNotification{
				VoidedPurchaseNtf: &playStoreVoidedPurchaseNtf{ProductType: 2},
			},
		},

		{
			name: "one_time_product_false",
			given: &playStoreDevNotification{
				OneTimeProductNtf: &struct{}{},
			},
		},

		{
			name: "test_false",
			given: &playStoreDevNotification{
				TestNtf: &struct{}{},
			},
		},

		{
			name:  "invalid_false",
			given: &playStoreDevNotification{},
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

func TestPlayStoreDevNotification_ntfType(t *testing.T) {
	type testCase struct {
		name  string
		given *playStoreDevNotification
		exp   string
	}

	tests := []testCase{
		{
			name: "subscription",
			given: &playStoreDevNotification{
				SubscriptionNtf: &playStoreSubscriptionNtf{Type: 1},
			},
			exp: "subscription",
		},

		{
			name: "voided_purchase",
			given: &playStoreDevNotification{
				VoidedPurchaseNtf: &playStoreVoidedPurchaseNtf{ProductType: 1},
			},
			exp: "voided_purchase",
		},

		{
			name: "one_time_product",
			given: &playStoreDevNotification{
				OneTimeProductNtf: &struct{}{},
			},
			exp: "one_time_product",
		},

		{
			name: "test",
			given: &playStoreDevNotification{
				TestNtf: &struct{}{},
			},
			exp: "test",
		},

		{
			name:  "invalid",
			given: &playStoreDevNotification{},
			exp:   "unknown",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.ntfType()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestPlayStoreDevNotification_ntfSubType(t *testing.T) {
	type testCase struct {
		name  string
		given *playStoreDevNotification
		exp   int
	}

	tests := []testCase{
		{
			name: "subscription_1",
			given: &playStoreDevNotification{
				SubscriptionNtf: &playStoreSubscriptionNtf{Type: 1},
			},
			exp: 1,
		},

		{
			name: "voided_purchase_1",
			given: &playStoreDevNotification{
				VoidedPurchaseNtf: &playStoreVoidedPurchaseNtf{ProductType: 1},
			},
			exp: 1,
		},

		{
			name: "one_time_product_0",
			given: &playStoreDevNotification{
				OneTimeProductNtf: &struct{}{},
			},
		},

		{
			name: "test_0",
			given: &playStoreDevNotification{
				TestNtf: &struct{}{},
			},
		},

		{
			name:  "invalid",
			given: &playStoreDevNotification{},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.ntfSubType()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestPlayStoreDevNotification_effect(t *testing.T) {
	type testCase struct {
		name  string
		given *playStoreDevNotification
		exp   string
	}

	tests := []testCase{
		{
			name: "subscription_renew",
			given: &playStoreDevNotification{
				SubscriptionNtf: &playStoreSubscriptionNtf{Type: 1},
			},
			exp: "renew",
		},

		{
			name: "subscription_cancel",
			given: &playStoreDevNotification{
				SubscriptionNtf: &playStoreSubscriptionNtf{Type: 3},
			},
			exp: "cancel",
		},

		{
			name: "subscription_skip",
			given: &playStoreDevNotification{
				SubscriptionNtf: &playStoreSubscriptionNtf{Type: 20},
			},
			exp: "skip",
		},

		{
			name: "voided_purchase_cancel",
			given: &playStoreDevNotification{
				VoidedPurchaseNtf: &playStoreVoidedPurchaseNtf{ProductType: 1},
			},
			exp: "cancel",
		},

		{
			name: "voided_purchase_skip",
			given: &playStoreDevNotification{
				VoidedPurchaseNtf: &playStoreVoidedPurchaseNtf{ProductType: 2},
			},
			exp: "skip",
		},

		{
			name: "one_time_product_skip",
			given: &playStoreDevNotification{
				OneTimeProductNtf: &struct{}{},
			},
			exp: "skip",
		},

		{
			name: "test",
			given: &playStoreDevNotification{
				TestNtf: &struct{}{},
			},
			exp: "skip",
		},

		{
			name:  "invalid",
			given: &playStoreDevNotification{},
			exp:   "skip",
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

func TestPlayStoreDevNotification_isBeforeCutoff(t *testing.T) {
	type testCase struct {
		name  string
		given *playStoreDevNotification
		exp   bool
	}

	tests := []testCase{
		{
			name:  "invalid_time",
			given: &playStoreDevNotification{EventTimeMilli: "garbage"},
			exp:   true,
		},

		{
			name:  "before",
			given: &playStoreDevNotification{EventTimeMilli: "1719791940000"},
			exp:   true,
		},

		{
			name:  "exact",
			given: &playStoreDevNotification{EventTimeMilli: "1719792000000"},
		},

		{
			name:  "after",
			given: &playStoreDevNotification{EventTimeMilli: "1722470400000"},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			acutal := tc.given.isBeforeCutoff()
			should.Equal(t, tc.exp, acutal)
		})
	}
}

func TestPlayStoreDevNotification_purchaseToken(t *testing.T) {
	type tcExpected struct {
		val string
		ok  bool
	}

	type testCase struct {
		name  string
		given *playStoreDevNotification
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "subscription",
			given: &playStoreDevNotification{
				SubscriptionNtf: &playStoreSubscriptionNtf{PurchaseToken: "PURCHASE_TOKEN"},
			},
			exp: tcExpected{
				val: "PURCHASE_TOKEN",
				ok:  true,
			},
		},

		{
			name: "voided_purchase",
			given: &playStoreDevNotification{
				VoidedPurchaseNtf: &playStoreVoidedPurchaseNtf{PurchaseToken: "PURCHASE_TOKEN"},
			},
			exp: tcExpected{
				val: "PURCHASE_TOKEN",
				ok:  true,
			},
		},

		{
			name: "one_time_product",
			given: &playStoreDevNotification{
				OneTimeProductNtf: &struct{}{},
			},
		},

		{
			name: "test",
			given: &playStoreDevNotification{
				TestNtf: &struct{}{},
			},
		},

		{
			name:  "invalid",
			given: &playStoreDevNotification{},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := tc.given.purchaseToken()
			should.Equal(t, tc.exp.ok, ok)

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestPlayStoreSubscriptionNtf_shouldProcess(t *testing.T) {
	type testCase struct {
		name  string
		given *playStoreSubscriptionNtf
		exp   bool
	}

	tests := []testCase{
		{
			name:  "renew",
			given: &playStoreSubscriptionNtf{Type: 1},
			exp:   true,
		},

		{
			name:  "cancel",
			given: &playStoreSubscriptionNtf{Type: 3},
			exp:   true,
		},

		{
			name:  "skip",
			given: &playStoreSubscriptionNtf{Type: 20},
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

func TestPlayStoreSubscriptionNtf_shouldRenew(t *testing.T) {
	type testCase struct {
		name  string
		given *playStoreSubscriptionNtf
		exp   bool
	}

	tests := []testCase{
		{
			name:  "recovered",
			given: &playStoreSubscriptionNtf{Type: 1},
			exp:   true,
		},

		{
			name:  "renewed",
			given: &playStoreSubscriptionNtf{Type: 2},
			exp:   true,
		},

		{
			name:  "restarted",
			given: &playStoreSubscriptionNtf{Type: 7},
			exp:   true,
		},

		{
			name:  "cancelled",
			given: &playStoreSubscriptionNtf{Type: 3},
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

func TestPlayStoreSubscriptionNtf_shouldCancel(t *testing.T) {
	type testCase struct {
		name  string
		given *playStoreSubscriptionNtf
		exp   bool
	}

	tests := []testCase{
		{
			name:  "cancelled",
			given: &playStoreSubscriptionNtf{Type: 3},
			exp:   true,
		},

		{
			name:  "revoked",
			given: &playStoreSubscriptionNtf{Type: 12},
			exp:   true,
		},

		{
			name:  "expired",
			given: &playStoreSubscriptionNtf{Type: 13},
			exp:   true,
		},

		{
			name:  "recovered",
			given: &playStoreSubscriptionNtf{Type: 1},
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

func TestPlayStoreVoidedPurchaseNtf_shouldProcess(t *testing.T) {
	type testCase struct {
		name  string
		given *playStoreVoidedPurchaseNtf
		exp   bool
	}

	tests := []testCase{
		{
			name:  "subscription",
			given: &playStoreVoidedPurchaseNtf{ProductType: 1},
			exp:   true,
		},

		{
			name:  "one_time",
			given: &playStoreVoidedPurchaseNtf{ProductType: 2},
		},

		{
			name:  "unknown",
			given: &playStoreVoidedPurchaseNtf{},
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

func TestNewReceiptDataGoogle(t *testing.T) {
	type tcGiven struct {
		req  model.ReceiptRequest
		item *playStoreSubPurchase
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   model.ReceiptData
	}

	tests := []testCase{
		{
			name: "valid",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "sub_id",
				},
				item: &playStoreSubPurchase{
					ExpiryTimeMillis: 1719792001000,
				},
			},
			exp: model.ReceiptData{
				Type:      model.VendorGoogle,
				ProductID: "sub_id",
				ExtID:     "blob",
				ExpiresAt: time.Date(2024, time.July, 1, 0, 0, 1, 0, time.UTC),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := newReceiptDataGoogle(tc.given.req, tc.given.item)

			should.Equal(t, tc.exp, actual)
		})
	}
}
