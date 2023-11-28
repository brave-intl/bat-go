package wallet

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
)

func TestClaimsZP(t *testing.T) {
	type tcGiven struct {
		now    time.Time
		claims claimsZP
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "invalid_iat",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Exp:         time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:       true,
					DepositID:   "deposit_id",
					AccountID:   "account_id",
					CountryCode: "IN",
				},
			},
			exp: errZPInvalidIat,
		},

		{
			name: "invalid_exp",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:         time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Valid:       true,
					DepositID:   "deposit_id",
					AccountID:   "account_id",
					CountryCode: "IN",
				},
			},
			exp: errZPInvalidExp,
		},

		{
			name: "invalid_kyc",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:         time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:         time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					DepositID:   "deposit_id",
					AccountID:   "account_id",
					CountryCode: "IN",
				},
			},
			exp: errZPInvalidKYC,
		},

		{
			name: "invalid_deposit",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:         time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:         time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:       true,
					AccountID:   "account_id",
					CountryCode: "IN",
				},
			},
			exp: errZPInvalidDepositID,
		},

		{
			name: "invalid_account",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:         time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:         time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:       true,
					DepositID:   "deposit_id",
					CountryCode: "IN",
				},
			},
			exp: errZPInvalidAccountID,
		},

		{
			name: "invalid_before_iat",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:         time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Exp:         time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:       true,
					DepositID:   "deposit_id",
					AccountID:   "account_id",
					CountryCode: "IN",
				},
			},
			exp: errZPInvalidAfter,
		},

		{
			name: "invalid_after_exp",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 3, 0, time.UTC),
				claims: claimsZP{
					Iat:         time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:         time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:       true,
					DepositID:   "deposit_id",
					AccountID:   "account_id",
					CountryCode: "IN",
				},
			},
			exp: errZPInvalidBefore,
		},
		{
			name: "invalid_country_code",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 3, 0, time.UTC),
				claims: claimsZP{
					Iat:         time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:         time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:       true,
					DepositID:   "deposit_id",
					AccountID:   "account_id",
					CountryCode: "US",
				},
			},
			exp: errorutils.ErrInvalidCountry,
		},
		{
			name: "valid",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:         time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:         time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:       true,
					DepositID:   "deposit_id",
					AccountID:   "account_id",
					CountryCode: "IN",
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act := tc.given.claims.validate(tc.given.now)
			should.Equal(t, tc.exp, act)
		})
	}
}

func Test_parseZebPayClaims(t *testing.T) {
	type tcGiven struct {
		ctxKey       appctx.CTXKey
		secret       string
		sigAlgo      string
		zpLinkingKey string
		claims       map[string]interface{}
	}

	type tcExpected struct {
		claimsZP claimsZP
		appErr   error
	}

	tests := []struct {
		name     string
		given    tcGiven
		expected tcExpected
	}{
		{
			name: "success",
			given: tcGiven{
				ctxKey:       appctx.ZebPayLinkingKeyCTXKey,
				secret:       "test secret",
				zpLinkingKey: base64.StdEncoding.EncodeToString([]byte("test secret")),
				claims: map[string]interface{}{
					"iat":         time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					"exp":         time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					"depositId":   "deposit_id",
					"accountId":   "account_id",
					"isValid":     true,
					"countryCode": "",
				},
				sigAlgo: "HS256",
			},
			expected: tcExpected{
				claimsZP: claimsZP{
					Iat:       time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:       time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					DepositID: "deposit_id",
					AccountID: "account_id",
					Valid:     true,
				},
			},
		},
		{
			name: "bad_config_key",
			given: tcGiven{
				ctxKey:  "bad_key",
				sigAlgo: "HS256",
			},
			expected: tcExpected{
				appErr: &handlers.AppError{
					Cause:   appctx.ErrNotInContext,
					Message: "zebpay linking validation misconfigured",
					Code:    http.StatusInternalServerError,
				},
			},
		},
		{
			name: "bad_config_decode",
			given: tcGiven{
				ctxKey:       appctx.ZebPayLinkingKeyCTXKey,
				secret:       "test secret",
				sigAlgo:      "HS256",
				zpLinkingKey: "!!invalid_64!!",
			},
			expected: tcExpected{
				appErr: &handlers.AppError{
					Cause:   appctx.ErrNotInContext,
					Message: "zebpay linking validation misconfigured",
					Code:    http.StatusInternalServerError,
				},
			},
		},
		{
			name: "wrong_signature_algorithm",
			given: tcGiven{
				ctxKey:       appctx.ZebPayLinkingKeyCTXKey,
				secret:       "test secret",
				sigAlgo:      "HS384",
				zpLinkingKey: base64.StdEncoding.EncodeToString([]byte("test secret")),
			},
			expected: tcExpected{
				appErr: &handlers.AppError{
					Cause:   errZPInvalidToken,
					Message: errZPInvalidToken.Error(),
					Code:    http.StatusBadRequest,
				},
			},
		},
		{
			name: "error_deserializing_claims",
			given: tcGiven{
				ctxKey:       appctx.ZebPayLinkingKeyCTXKey,
				secret:       "test secret",
				sigAlgo:      "HS256",
				zpLinkingKey: base64.StdEncoding.EncodeToString([]byte("test secret")),
				claims: map[string]interface{}{
					"accountId": 1, // invalid account type
				},
			},
			expected: tcExpected{
				appErr: &handlers.AppError{
					Cause:   errZPValidationFailed,
					Message: errZPValidationFailed.Error(),
					Code:    http.StatusBadRequest,
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			signer, err := jose.NewSigner(jose.SigningKey{
				Algorithm: jose.SignatureAlgorithm(tc.given.sigAlgo),
				Key:       []byte(tc.given.secret),
			}, (&jose.SignerOptions{}).WithType("JWT"))
			must.Equal(t, nil, err)

			verificationToken, err := jwt.Signed(signer).Claims(tc.given.claims).CompactSerialize()
			must.Equal(t, nil, err)

			ctx := context.WithValue(context.Background(), tc.given.ctxKey, tc.given.zpLinkingKey)

			actual, err := parseZebPayClaims(ctx, verificationToken)
			should.Equal(t, tc.expected.claimsZP, actual)
			should.Equal(t, tc.expected.appErr, err)
		})
	}
}
