package skus

import (
	"context"
	"errors"
	"testing"
	"time"

	should "github.com/stretchr/testify/assert"
	"google.golang.org/api/idtoken"

	"github.com/brave-intl/bat-go/services/skus/model"
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

type mockGPSTokenValidator struct {
	fnValidate func(ctx context.Context, token, aud string) (*idtoken.Payload, error)
}

func (m *mockGPSTokenValidator) Validate(ctx context.Context, token, aud string) (*idtoken.Payload, error) {
	if m.fnValidate == nil {
		return &idtoken.Payload{Audience: aud}, nil
	}

	return m.fnValidate(ctx, token, aud)
}
