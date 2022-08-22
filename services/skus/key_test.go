package skus

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
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
	service := Service{}
	service.Datastore = Datastore(
		&Postgres{
			datastore.Postgres{
				DB: sqlx.NewDb(db, "postgres"),
			},
		},
	)

	// Test that no auth fails
	token := "FOO"
	middleware.TokenList = []string{token}

	fn1 := func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Should not have gotten here")
	}
	handler := middleware.BearerToken(service.MerchantSignedMiddleware()(http.HandlerFunc(fn1)))

	req, err := http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code, "request without simple or merchant auth should fail")

	// Test that simple auth works and sets caveats / merchant correctly

	fn2 := func(w http.ResponseWriter, r *http.Request) {
		// with simple auth legacy mode there are no caveats
		caveats := GetCaveats(r.Context())
		assert.Nil(t, caveats)

		// and the merchant is always brave.com
		merchant, err := GetMerchant(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, merchant, "brave.com")
	}
	handler = middleware.BearerToken(service.MerchantSignedMiddleware()(http.HandlerFunc(fn2)))

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
		caveats := GetCaveats(r.Context())
		assert.Equal(t, caveats, expectedCaveats)

		merchant, err := GetMerchant(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, merchant, expectedMerchant)
	}
	handler = middleware.BearerToken(service.MerchantSignedMiddleware()(http.HandlerFunc(fn3)))

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
	db, mock, _ := sqlmock.New()
	service := Service{}
	service.Datastore = Datastore(
		&Postgres{
			datastore.Postgres{
				DB: sqlx.NewDb(db, "postgres"),
			},
		},
	)
	expectedOrderID := uuid.NewV4()

	cases := []validateOrderMerchantAndCaveatsTestCase{
		{"brave.com", nil, uuid.NewV4(), false, "invalid order should fail"},
		{"brave.com", nil, expectedOrderID, true, "correct merchant and no caveats should succeed"},
		{"brave.software", nil, expectedOrderID, false, "incorrect merchant should fail"},
		{"brave.com", map[string]string{"location": "test.brave.com"}, expectedOrderID, true, "correct merchant and location caveat should succeed"},
		{"brave.com", map[string]string{"location": "test.brave.software"}, expectedOrderID, false, "incorrect location caveat should fail"},
		{"brave.com", map[string]string{"sku": "example-sku"}, expectedOrderID, false, "sku caveat is not supported"},
	}

	for _, testCase := range cases {
		itemRows := sqlmock.NewRows([]string{})
		orderRows := sqlmock.NewRows([]string{
			"id",
			"created_at",
			"currency",
			"updated_at",
			"total_price",
			"merchant_id",
			"location",
			"status",
			"allowed_payment_methods",
			"metadata",
			"valid_for",
			"last_paid_at",
			"expires_at",
		})
		orderRows.AddRow(
			expectedOrderID.String(),
			time.Now(),
			"BAT",
			time.Now(),
			"0",
			"brave.com",
			"test.brave.com",
			"paid",
			nil,
			nil,
			nil,
			nil,
			nil,
		)

		mock.ExpectQuery(`
^SELECT (.+) FROM orders*
`).
			WithArgs(expectedOrderID).
			WillReturnRows(orderRows)
		mock.ExpectQuery(`
^SELECT (.+) FROM order_items*
`).
			WithArgs(expectedOrderID).
			WillReturnRows(itemRows)

		ValidateOrderMerchantAndCaveats(t, &service, testCase)
	}
}

type validateOrderMerchantAndCaveatsTestCase struct {
	merchant        string
	caveats         map[string]string
	orderID         uuid.UUID
	expectedSuccess bool
	explanation     string
}

func ValidateOrderMerchantAndCaveats(t *testing.T, service *Service, testCase validateOrderMerchantAndCaveatsTestCase) {
	ctx := context.WithValue(context.Background(), merchantCtxKey{}, testCase.merchant)
	ctx = context.WithValue(ctx, caveatsCtxKey{}, testCase.caveats)

	req, err := http.NewRequestWithContext(ctx, "GET", "/hello-world", nil)
	assert.NoError(t, err)

	err = service.ValidateOrderMerchantAndCaveats(req, testCase.orderID)
	if testCase.expectedSuccess {
		assert.NoError(t, err, testCase.explanation)
	} else {
		assert.Error(t, err, testCase.explanation)
	}
}
