package wallet

import (
	"testing"
	"time"

	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
)

func TestParseClaims(t *testing.T) {
	secret := []byte("a jwt secret")

	sig, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.HS256, Key: secret},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	must.Equal(t, nil, err)

	info, err := jwt.Signed(sig).Claims(map[string]interface{}{
		"accountId": "account_id",
		"depositId": "deposit_id",
		"iat":       time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
		"exp":       time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
		"isValid":   true,
	}).CompactSerialize()

	must.Equal(t, nil, err)

	tok, err := jwt.ParseSigned(info)
	must.Equal(t, nil, err)

	actual := &claimsZP{}
	{
		err := tok.Claims(secret, actual)
		must.Equal(t, nil, err)
	}

	expected := &claimsZP{
		Iat:       time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
		Exp:       time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
		DepositID: "deposit_id",
		AccountID: "account_id",
		Valid:     true,
	}

	should.Equal(t, expected, actual)
}

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
					Exp:       time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:     true,
					DepositID: "deposit_id",
					AccountID: "account_id",
				},
			},
			exp: errZPInvalidIat,
		},

		{
			name: "invalid_exp",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:       time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Valid:     true,
					DepositID: "deposit_id",
					AccountID: "account_id",
				},
			},
			exp: errZPInvalidExp,
		},

		{
			name: "invalid_kyc",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:       time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:       time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					DepositID: "deposit_id",
					AccountID: "account_id",
				},
			},
			exp: errZPInvalidKYC,
		},

		{
			name: "invalid_deposit",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:       time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:       time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:     true,
					AccountID: "account_id",
				},
			},
			exp: errZPInvalidDepositID,
		},

		{
			name: "invalid_account",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:       time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:       time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:     true,
					DepositID: "deposit_id",
				},
			},
			exp: errZPInvalidAccountID,
		},

		{
			name: "invalid_before_iat",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:       time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Exp:       time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:     true,
					DepositID: "deposit_id",
					AccountID: "account_id",
				},
			},
			exp: errZPInvalidAfter,
		},

		{
			name: "invalid_after_exp",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 3, 0, time.UTC),
				claims: claimsZP{
					Iat:       time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:       time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:     true,
					DepositID: "deposit_id",
					AccountID: "account_id",
				},
			},
			exp: errZPInvalidBefore,
		},

		{
			name: "valid",
			given: tcGiven{
				now: time.Date(2023, time.August, 16, 1, 1, 1, 0, time.UTC),
				claims: claimsZP{
					Iat:       time.Date(2023, time.August, 16, 1, 1, 0, 0, time.UTC).Unix(),
					Exp:       time.Date(2023, time.August, 16, 1, 1, 2, 0, time.UTC).Unix(),
					Valid:     true,
					DepositID: "deposit_id",
					AccountID: "account_id",
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
