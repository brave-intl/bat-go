package skus

import (
	"context"
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	// What the merchant key length should be.
	keyLength = 24

	errInvalidMerchant  model.Error = "merchant was missing from context"
	errMerchantMismatch model.Error = "Order merchant does not match authentication"
	errLocationMismatch model.Error = "Order location does not match authentication"
	errUnexpectedSKUCvt model.Error = "SKU caveat is not supported on order endpoints"
)

var (
	// EncryptionKey for encrypting secrets.
	EncryptionKey = os.Getenv("ENCRYPTION_KEY")

	byteEncryptionKey [32]byte
)

type caveatsCtxKey struct{}
type merchantCtxKey struct{}

// Key represents a merchant's keys to validate skus
type Key struct {
	ID                 string     `json:"id" db:"id"`
	Name               string     `json:"name" db:"name"`
	Merchant           string     `json:"merchant" db:"merchant_id"`
	EncryptedSecretKey string     `json:"-" db:"encrypted_secret_key"`
	Nonce              string     `json:"-" db:"nonce"`
	CreatedAt          time.Time  `json:"createdAt" db:"created_at"`
	Expiry             *time.Time `json:"expiry" db:"expiry"`
}

// InitEncryptionKeys copies the specified encryption key into memory once
func InitEncryptionKeys() {
	copy(byteEncryptionKey[:], []byte(EncryptionKey))
}

// GetSecretKey decrypts the secret key from the database
func (key *Key) GetSecretKey() (*string, error) {
	encrypted, err := hex.DecodeString(key.EncryptedSecretKey)
	if err != nil {
		return nil, err
	}

	nonce, err := hex.DecodeString(key.Nonce)
	if err != nil {
		return nil, err
	}

	secretKey, err := cryptography.DecryptMessage(byteEncryptionKey, encrypted, nonce)
	if err != nil {
		return nil, err
	}

	return &secretKey, nil
}

// GenerateSecret creates a random key for merchants
func GenerateSecret() (secret string, nonce string, err error) {
	unencryptedSecret, err := randomString(keyLength)
	if err != nil {
		return "", "", err
	}
	unencryptedSecret = cryptography.SecretTokenPrefix + unencryptedSecret

	encryptedBytes, nonceBytes, err := cryptography.EncryptMessage(byteEncryptionKey, []byte(unencryptedSecret))

	return fmt.Sprintf("%x", encryptedBytes), fmt.Sprintf("%x", nonceBytes), err
}

// NewAuthMwr returns a handler that authorises requests via http signature or simple tokens.
func NewAuthMwr(ks httpsignature.Keystore) func(http.Handler) http.Handler {
	merchantVerifier := httpsignature.ParameterizedKeystoreVerifier{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.HS2019,
			Headers: []string{
				"(request-target)",
				"host",
				"date",
				"digest",
				"content-length",
				"content-type",
			},
		},
		Keystore: ks,
		Opts:     crypto.Hash(0),
	}

	// TODO: Keep only VerifyHTTPSignedOnly after migrating Subscriptions to this method.
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Signature") == "" {
				// Assume legacy simple token auth.
				ctx := context.WithValue(r.Context(), merchantCtxKey{}, "brave.com")
				middleware.SimpleTokenAuthorizedOnly(next).ServeHTTP(w, r.WithContext(ctx))
				return
			}

			middleware.VerifyHTTPSignedOnly(merchantVerifier)(next).ServeHTTP(w, r)
		})
	}
}

// LookupVerifier returns the merchant key corresponding to the keyID used for verifying requests
func (s *Service) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {
	rootKeyIDStr, caveats, err := cryptography.DecodeKeyID(keyID)
	if err != nil {
		return nil, nil, err
	}

	rootKeyID, err := uuid.FromString(rootKeyIDStr)
	if err != nil {
		return nil, nil, fmt.Errorf("root key id must be a uuid: %v", err)
	}

	key, err := s.Datastore.GetKey(rootKeyID, false)
	if err != nil {
		return nil, nil, err
	}

	secretKey, err := key.GetSecretKey()
	if err != nil {
		return nil, nil, err
	}
	if secretKey == nil {
		return nil, nil, errors.New("missing secret key")
	}

	secretKeyStr := *secretKey

	if caveats != nil {
		_, secretKeyStr, err = cryptography.Attenuate(rootKeyID.String(), secretKeyStr, caveats)
		if err != nil {
			return nil, nil, err
		}

		ctx = context.WithValue(ctx, caveatsCtxKey{}, caveats)
	}

	ctx = context.WithValue(ctx, merchantCtxKey{}, key.Merchant)

	verifier := httpsignature.Verifier(httpsignature.HMACKey(secretKeyStr))
	return ctx, &verifier, nil
}

// validateOrderMerchantAndCaveats checks that the current authentication of the request has
// permissions to this order by cross-checking the merchant and caveats in context.
func (s *Service) validateOrderMerchantAndCaveats(ctx context.Context, oid uuid.UUID) error {
	merchant, err := merchantFromCtx(ctx)
	if err != nil {
		return err
	}

	order, err := s.orderRepo.Get(ctx, s.Datastore.RawDB(), oid)
	if err != nil {
		return err
	}

	if order.MerchantID != merchant {
		return errMerchantMismatch
	}

	return validateOrderCvt(order, caveatsFromCtx(ctx))
}

func randomString(n int) (string, error) {
	b := make([]byte, n)

	// Note that err == nil only if we read len(b) bytes.
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// merchantFromCtx returns an authorized merchant from ctx.
func merchantFromCtx(ctx context.Context) (string, error) {
	merchant, ok := ctx.Value(merchantCtxKey{}).(string)
	if !ok {
		return "", errInvalidMerchant
	}

	return merchant, nil
}

// caveatsFromCtx returns an authorized caveats from ctx.
func caveatsFromCtx(ctx context.Context) map[string]string {
	caveats, ok := ctx.Value(caveatsCtxKey{}).(map[string]string)
	if !ok {
		return nil
	}

	return caveats
}

func validateOrderCvt(ord *model.Order, cvt map[string]string) error {
	if loc, ok := cvt["location"]; ok && ord.Location.Valid {
		if ord.Location.String != loc {
			return errLocationMismatch
		}
	}

	if _, ok := cvt["sku"]; ok {
		return errUnexpectedSKUCvt
	}

	return nil
}
