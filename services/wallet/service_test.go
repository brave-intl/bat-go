package wallet

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"

	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/services/wallet/model"
	"github.com/brave-intl/bat-go/services/wallet/xslack"
)

func TestService_LinkSolanaAddress(t *testing.T) {
	type tcGiven struct {
		pid             uuid.UUID
		req             linkSolanaAddrRequest
		solAddrsChecker solanaAddrsChecker
		compBotCl       compBotClient
	}

	type tcExpected struct {
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "error_address_not_allowed",
			given: tcGiven{
				solAddrsChecker: &mockSolAddrsChecker{
					fnIsAllowed: func(ctx context.Context, addrs string) error {
						return model.Error("error_address_not_allowed")
					},
				},
			},
			exp: tcExpected{
				err: model.Error("error_address_not_allowed"),
			},
		},

		{
			name: "send_sol_sanctioned_message_error",
			given: tcGiven{
				solAddrsChecker: &mockSolAddrsChecker{
					fnIsAllowed: func(ctx context.Context, addrs string) error {
						return model.ErrSolAddrsNotAllowed
					},
				},
				compBotCl: &xslack.MockClient{
					FnSendMessage: func(ctx context.Context, msg *xslack.Message) error {
						if msg == nil {
							return model.Error("message_should_not_be_nil")
						}

						return model.Error("send_message_error")
					},
				},
			},
			exp: tcExpected{
				err: model.Error("send_message_error"),
			},
		},

		{
			name: "send_sol_sanctioned_message_success",
			given: tcGiven{
				solAddrsChecker: &mockSolAddrsChecker{
					fnIsAllowed: func(ctx context.Context, addrs string) error {
						return model.ErrSolAddrsNotAllowed
					},
				},
				compBotCl: &xslack.MockClient{
					FnSendMessage: func(ctx context.Context, msg *xslack.Message) error {
						if msg == nil {
							return model.Error("message_should_not_be_nil")
						}

						return nil
					},
				},
			},
			exp: tcExpected{
				err: model.ErrSolAddrsNotAllowed,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			s := &Service{
				solAddrsChecker: tc.given.solAddrsChecker,
				compBotCl:       tc.given.compBotCl,
			}

			ctx := context.Background()

			actual := s.LinkSolanaAddress(ctx, tc.given.pid, tc.given.req)
			should.Equal(t, tc.exp.err, actual)
		})
	}
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

func TestSolanaCanJoinWaitlist(t *testing.T) {
	type tcGiven struct {
		id   uuid.UUID
		repo cxLinkRepo
	}

	type tcExpected struct {
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "error",
			given: tcGiven{
				id: uuid.FromStringOrNil("3d24c761-105c-439d-b69a-0e59a0ade368"),
				repo: &mockCxLinkRepo{fnGetCustodianLinkByWalletID: func(ctx context.Context, ID uuid.UUID) (*CustodianLink, error) {
					return nil, model.Error("error cx link")
				}},
			},
			exp: tcExpected{
				err: model.Error("error cx link"),
			},
		},

		{
			name: "no_wallet_custodian",
			given: tcGiven{
				id: uuid.FromStringOrNil("3d24c761-105c-439d-b69a-0e59a0ade368"),
				repo: &mockCxLinkRepo{
					fnGetCustodianLinkByWalletID: func(ctx context.Context, ID uuid.UUID) (*CustodianLink, error) {
						return nil, model.ErrNoWalletCustodian
					},
				},
			},
		},

		{
			name: "error_solana_already_linked",
			given: tcGiven{
				id: uuid.FromStringOrNil("3d24c761-105c-439d-b69a-0e59a0ade368"),
				repo: &mockCxLinkRepo{
					fnGetCustodianLinkByWalletID: func(ctx context.Context, ID uuid.UUID) (*CustodianLink, error) {
						linkID := uuid.FromStringOrNil("2bcf2ed1-a447-457f-99f1-be2bc4c2bcfe")

						return &CustodianLink{
							WalletID:  &ID,
							LinkingID: &linkID,
							Custodian: depositProviderSolana,
							LinkedAt:  time.Date(2025, time.February, 12, 0, 0, 0, 0, time.UTC),
						}, nil
					},
				},
			},
			exp: tcExpected{
				err: model.ErrSolAlreadyLinked,
			},
		},

		{
			name: "error_not_linked_not_solana",
			given: tcGiven{
				id: uuid.FromStringOrNil("3d24c761-105c-439d-b69a-0e59a0ade368"),
				repo: &mockCxLinkRepo{fnGetCustodianLinkByWalletID: func(ctx context.Context, ID uuid.UUID) (*CustodianLink, error) {
					paymentID := uuid.FromStringOrNil("3d24c761-105c-439d-b69a-0e59a0ade368")

					cxLink := &CustodianLink{
						WalletID:  &paymentID,
						LinkingID: ptrFromUUID(uuid.NewV5(ClaimNamespace, "deposit_destination")),
						Custodian: "custodian",
					}

					return cxLink, nil
				}},
			},
		},

		{
			name: "error_linked_not_solana",
			given: tcGiven{
				id: uuid.FromStringOrNil("3d24c761-105c-439d-b69a-0e59a0ade368"),
				repo: &mockCxLinkRepo{fnGetCustodianLinkByWalletID: func(ctx context.Context, ID uuid.UUID) (*CustodianLink, error) {
					paymentID := uuid.FromStringOrNil("3d24c761-105c-439d-b69a-0e59a0ade368")

					cxLink := &CustodianLink{
						WalletID:  &paymentID,
						LinkingID: ptrFromUUID(uuid.NewV5(ClaimNamespace, "deposit_destination")),
						Custodian: "custodian",
						LinkedAt:  time.Date(2025, time.February, 12, 0, 0, 0, 0, time.UTC),
					}

					return cxLink, nil
				}},
			},
		},

		{
			name: "success",
			given: tcGiven{
				id:   uuid.FromStringOrNil("3d24c761-105c-439d-b69a-0e59a0ade368"),
				repo: &mockCxLinkRepo{},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			actual := solanaCanJoinWaitlist(ctx, tc.given.repo, tc.given.id)
			should.Equal(t, tc.exp.err, actual)
		})
	}
}

type mockCxLinkRepo struct {
	fnGetCustodianLinkByWalletID func(ctx context.Context, ID uuid.UUID) (*CustodianLink, error)
}

func (m *mockCxLinkRepo) GetCustodianLinkByWalletID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error) {
	if m.fnGetCustodianLinkByWalletID == nil {
		return nil, nil
	}

	return m.fnGetCustodianLinkByWalletID(ctx, ID)
}

func TestDoesSolAddrsHaveATAForMint(t *testing.T) {
	type tcGiven struct {
		solCl     solanaClient
		solAddrs  string
		mintAddrs string
	}

	type tcExpected struct {
		assertErr should.ErrorAssertionFunc
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "solana_address_invalid",
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.NotNil(t, err)
				},
			},
		},

		{
			name: "mint_address_invalid",
			given: tcGiven{
				solAddrs: "Ei71196o8MpDsVdXyZEewEdxP4A8n3KMZKTFE7KE2xTR",
			},
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.NotNil(t, err)
				},
			},
		},

		{
			name: "sol_client_error",
			given: tcGiven{
				solAddrs:  "Ei71196o8MpDsVdXyZEewEdxP4A8n3KMZKTFE7KE2xTR",
				mintAddrs: "EPeUFDgHRxs9xxEPVaL6kfGQvCon7jmAWKVUHuux1Tpz",
				solCl: &mockSolClient{
					fnGetTokenAccountsByOwner: func(ctx context.Context, owner solana.PublicKey, mint solana.PublicKey) (*rpc.GetTokenAccountsResult, error) {
						return nil, model.Error("solana_client_error")
					},
				},
			},
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.ErrorIs(t, err, model.Error("solana_client_error"))
				},
			},
		},

		{
			name: "no_ata_nil",
			given: tcGiven{
				solAddrs:  "Ei71196o8MpDsVdXyZEewEdxP4A8n3KMZKTFE7KE2xTR",
				mintAddrs: "EPeUFDgHRxs9xxEPVaL6kfGQvCon7jmAWKVUHuux1Tpz",
				solCl: &mockSolClient{
					fnGetTokenAccountsByOwner: func(ctx context.Context, owner solana.PublicKey, mint solana.PublicKey) (*rpc.GetTokenAccountsResult, error) {
						return nil, nil
					},
				},
			},
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.ErrorIs(t, err, model.ErrSolAddrsHasNoATAForMint)
				},
			},
		},

		{
			name: "no_ata_empty",
			given: tcGiven{
				solAddrs:  "Ei71196o8MpDsVdXyZEewEdxP4A8n3KMZKTFE7KE2xTR",
				mintAddrs: "EPeUFDgHRxs9xxEPVaL6kfGQvCon7jmAWKVUHuux1Tpz",
				solCl: &mockSolClient{
					fnGetTokenAccountsByOwner: func(ctx context.Context, owner solana.PublicKey, mint solana.PublicKey) (*rpc.GetTokenAccountsResult, error) {
						return &rpc.GetTokenAccountsResult{}, nil
					},
				},
			},
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.ErrorIs(t, err, model.ErrSolAddrsHasNoATAForMint)
				},
			},
		},

		{
			name: "success",
			given: tcGiven{
				solAddrs:  "Ei71196o8MpDsVdXyZEewEdxP4A8n3KMZKTFE7KE2xTR",
				mintAddrs: "EPeUFDgHRxs9xxEPVaL6kfGQvCon7jmAWKVUHuux1Tpz",
				solCl: &mockSolClient{
					fnGetTokenAccountsByOwner: func(ctx context.Context, owner solana.PublicKey, mint solana.PublicKey) (*rpc.GetTokenAccountsResult, error) {
						return &rpc.GetTokenAccountsResult{Value: []*rpc.TokenAccount{{}}}, nil
					},
				},
			},
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.Nil(t, err)
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			actual := doesSolAddrsHaveATAForMint(ctx, tc.given.solCl, tc.given.solAddrs, tc.given.mintAddrs)
			tc.exp.assertErr(t, actual)
		})
	}
}

type mockSolClient struct {
	fnGetTokenAccountsByOwner func(ctx context.Context, owner solana.PublicKey, mint solana.PublicKey) (*rpc.GetTokenAccountsResult, error)
}

func (c *mockSolClient) GetTokenAccountsByOwner(ctx context.Context, owner solana.PublicKey, mint solana.PublicKey) (*rpc.GetTokenAccountsResult, error) {
	if c.fnGetTokenAccountsByOwner == nil {
		return &rpc.GetTokenAccountsResult{}, nil
	}

	return c.fnGetTokenAccountsByOwner(ctx, owner, mint)
}
