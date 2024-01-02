package skus

import (
	"context"
	"crypto"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/middleware"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
)

func TestGenerateSecret(t *testing.T) {
	// set up the aes key, typically done with env variable atm
	oldEncryptionKey := EncryptionKey
	defer func() {
		EncryptionKey = oldEncryptionKey
	}()

	EncryptionKey = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0"
	InitEncryptionKeys()

	var byteEncryptionKey [32]byte
	copy(byteEncryptionKey[:], EncryptionKey)

	s, n, err := GenerateSecret()
	if err != nil {
		t.Error("error in generate secret: ", err)
	}

	encrypted, err := hex.DecodeString(s)
	if err != nil {
		t.Error("error while decoding the encrypted string", err)
	}
	nonce, err := hex.DecodeString(n)
	if err != nil {
		t.Error("error while decoding the nonce", err)
	}

	if len(nonce) != 24 {
		t.Error("Nonce does not have correct length", err)
	}

	secretKey, err := cryptography.DecryptMessage(byteEncryptionKey, encrypted, nonce)
	if err != nil {
		t.Error("error in decrypt secret: ", err)
	}

	if !strings.Contains(secretKey, cryptography.SecretTokenPrefix) {
		t.Error("secret key is missing prefix")
	}

	bareSecretKey := strings.TrimPrefix(secretKey, cryptography.SecretTokenPrefix)
	// secretKey is random, so i guess just make sure it is base64?
	k, err := base64.RawURLEncoding.DecodeString(bareSecretKey)
	if err != nil {
		t.Error("error decoding generated secret: ", err)
	}
	if len(bareSecretKey) != 32 {
		t.Error("Secret key does not have correct length", err)
	}
	if len(k) <= 0 {
		t.Error("the key should be bigger than nothing")
	}
}

func TestSecretKey(t *testing.T) {
	// set up the aes key, typically done with env variable atm
	oldEncryptionKey := EncryptionKey
	defer func() {
		EncryptionKey = oldEncryptionKey
		InitEncryptionKeys()
	}()
	EncryptionKey = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0"
	InitEncryptionKeys()

	var (
		sk, err = randomString(20)
		expiry  = time.Now().Add(1 * time.Minute)
		k       = &Key{
			ID:        "test-id",
			Name:      "test-name",
			Merchant:  "test-merchant",
			CreatedAt: time.Now(),
			Expiry:    &expiry,
		}
	)

	if err != nil {
		t.Error("failed to generate a secret key: ", err)
	}

	encryptedBytes, nonceBytes, err := cryptography.EncryptMessage(byteEncryptionKey, []byte(sk))

	k.EncryptedSecretKey = fmt.Sprintf("%x", encryptedBytes)
	k.Nonce = fmt.Sprintf("%x", nonceBytes)
	if err != nil {
		t.Error("failed to encrypt secret key: ", err)
	}

	skResult, err := k.GetSecretKey()
	if err != nil {
		t.Error("failed to get secret key: ", err)
	}

	// the Secret key should now be plaintext in key, check it out
	if skResult == nil || sk != *skResult {
		t.Error("expecting initial plaintext secret key to match decrypted secret key")
	}

}

func TestMerchantSignedMiddleware(t *testing.T) {
	db, mock, _ := sqlmock.New()
	service := &Service{}
	service.Datastore = Datastore(
		&Postgres{
			Postgres: datastore.Postgres{
				DB: sqlx.NewDb(db, "postgres"),
			},
			orderRepo:       repository.NewOrder(),
			orderItemRepo:   repository.NewOrderItem(),
			orderPayHistory: repository.NewOrderPayHistory(),
		},
	)

	// Test that no auth fails
	token := "FOO"
	middleware.TokenList = []string{token}

	fn1 := func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Should not have gotten here")
	}

	authMwr := NewAuthMwr(service)
	handler := middleware.BearerToken(authMwr((http.HandlerFunc(fn1))))

	req, err := http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code, "request without simple or merchant auth should fail")

	// Test that simple auth works and sets caveats / merchant correctly

	fn2 := func(w http.ResponseWriter, r *http.Request) {
		// with simple auth legacy mode there are no caveats
		caveats := caveatsFromCtx(r.Context())
		assert.Nil(t, caveats)

		// and the merchant is always brave.com
		merchant, err := merchantFromCtx(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, merchant, "brave.com")
	}
	handler = middleware.BearerToken(authMwr(http.HandlerFunc(fn2)))

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	req.Header.Set("authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code, "request with simple auth should succeed")

	expectedMerchant := "brave.software"
	expectedCaveats := map[string]string{
		"location": "test.brave.software",
		"sku":      "test-sku",
	}

	// Test that merchant signed works and sets caveats / merchant correctly
	fn3 := func(w http.ResponseWriter, r *http.Request) {
		caveats := caveatsFromCtx(r.Context())
		assert.Equal(t, caveats, expectedCaveats)

		merchant, err := merchantFromCtx(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, merchant, expectedMerchant)
	}
	handler = middleware.BearerToken(authMwr(http.HandlerFunc(fn3)))

	rootID := "a74b1c17-6e29-4bea-a3d7-fc70aebdfc02"
	encSecret, hexNonce, err := GenerateSecret()
	assert.NoError(t, err)

	encrypted, err := hex.DecodeString(encSecret)
	assert.NoError(t, err)

	nonce, err := hex.DecodeString(hexNonce)
	assert.NoError(t, err)

	secretKey, err := cryptography.DecryptMessage(byteEncryptionKey, encrypted, nonce)
	assert.NoError(t, err)

	iD, attenuatedSecret, err := cryptography.Attenuate(rootID, secretKey, expectedCaveats)
	assert.NoError(t, err)

	var sp httpsignature.SignatureParams
	sp.Algorithm = httpsignature.HS2019
	sp.KeyID = iD
	sp.Headers = []string{"(request-target)", "host", "date", "digest", "content-length", "content-type"}

	ps := httpsignature.ParameterizedSignator{
		SignatureParams: sp,
		Signator:        httpsignature.HMACKey(attenuatedSecret),
		Opts:            crypto.Hash(0),
	}
	req, err = http.NewRequest("GET", "/hello-world", nil)
	req.Header.Set("Date", time.Now().Format(time.RFC1123))
	assert.NoError(t, err)
	assert.NoError(t, ps.SignRequest(req))

	rows := sqlmock.NewRows([]string{
		"id",
		"name",
		"merchant_id",
		"created_at",
		"expiry",
		"encrypted_secret_key",
		"nonce",
	})
	rows.AddRow(
		rootID,
		"test key",
		"brave.software",
		time.Now(),
		nil,
		encSecret,
		hexNonce,
	)

	mock.ExpectQuery(`
^select (.+) from api_keys*
`).
		WithArgs(rootID).
		WillReturnRows(rows)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code, "request with merchant auth should succeed")
}

func TestValidateOrderMerchantAndCaveats(t *testing.T) {
	type tcGiven struct {
		orderID uuid.UUID
		merch   string
		cvt     map[string]string
		repo    *repository.MockOrder
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "invalid_order",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("0fb1d6ba-5d39-4f69-830b-c92c4640c86e")),
				merch:   "brave.com",

				repo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						return nil, model.ErrOrderNotFound
					},
				},
			},
			exp: model.ErrOrderNotFound,
		},

		{
			name: "merchant_no_caveats",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
				merch:   "brave.com",

				repo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:         uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
							Currency:   "BAT",
							MerchantID: "brave.com",
							Location: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "test.brave.com",
								},
							},
							Status: "paid",
						}

						return result, nil
					},
				},
			},
		},

		{
			name: "incorrect_merchant",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
				merch:   "brave.software",

				repo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:         uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
							Currency:   "BAT",
							MerchantID: "brave.com",
							Location: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "test.brave.com",
								},
							},
							Status: "paid",
						}

						return result, nil
					},
				},
			},
			exp: errMerchantMismatch,
		},

		{
			name: "merchant_location",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
				merch:   "brave.com",
				cvt:     map[string]string{"location": "test.brave.com"},

				repo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:         uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
							Currency:   "BAT",
							MerchantID: "brave.com",
							Location: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "test.brave.com",
								},
							},
							Status: "paid",
						}

						return result, nil
					},
				},
			},
		},

		{
			name: "incorrect_location",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
				merch:   "brave.com",
				cvt:     map[string]string{"location": "test.brave.software"},

				repo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:         uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
							Currency:   "BAT",
							MerchantID: "brave.com",
							Location: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "test.brave.com",
								},
							},
							Status: "paid",
						}

						return result, nil
					},
				},
			},
			exp: errLocationMismatch,
		},

		{
			name: "unexpected_sku",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
				merch:   "brave.com",
				cvt:     map[string]string{"location": "test.brave.com", "sku": "some_sku"},

				repo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:         uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
							Currency:   "BAT",
							MerchantID: "brave.com",
							Location: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "test.brave.com",
								},
							},
							Status: "paid",
						}

						return result, nil
					},
				},
			},
			exp: errUnexpectedSKUCvt,
		},

		{
			name: "empty_order_location",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
				merch:   "brave.com",
				cvt:     map[string]string{"location": "test.brave.com"},

				repo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:         uuid.Must(uuid.FromString("056bf179-1c07-4787-bd36-db51a83ad139")),
							Currency:   "BAT",
							MerchantID: "brave.com",
							Status:     "paid",
						}

						return result, nil
					},
				},
			},
		},
	}

	// Need a database instance in Datastore.
	// Not using mocks (as the suppressed return value suggests).
	dbi, _, err := sqlmock.New()
	must.Equal(t, nil, err)

	ds := &Postgres{
		Postgres: datastore.Postgres{DB: sqlx.NewDb(dbi, "postgres")},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), merchantCtxKey{}, tc.given.merch)
			ctx = context.WithValue(ctx, caveatsCtxKey{}, tc.given.cvt)

			svc := &Service{
				Datastore: ds,
				orderRepo: tc.given.repo,
			}

			err := svc.validateOrderMerchantAndCaveats(ctx, tc.given.orderID)
			assert.Equal(t, tc.exp, err)
		})
	}
}
