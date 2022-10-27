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

	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/middleware"
	uuid "github.com/satori/go.uuid"
)

// EncryptionKey for encrypting secrets
var EncryptionKey = os.Getenv("ENCRYPTION_KEY")
var byteEncryptionKey [32]byte

// What the merchant key length should be
var keyLength = 24

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

func randomString(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// Note that err == nil only if we read len(b) bytes.
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
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

// MerchantSignedMiddleware requires that requests are signed by valid merchant keys
func (s *Service) MerchantSignedMiddleware() func(http.Handler) http.Handler {
	merchantVerifier := httpsignature.ParameterizedKeystoreVerifier{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.HS2019,
			Headers: []string{
				"(request-target)", "host", "date", "digest", "content-length", "content-type",
			},
		},
		Keystore: s,
		Opts:     crypto.Hash(0),
	}

	// TODO replace with returning VerifyHTTPSignedOnly once we've migrated
	// subscriptions server auth off simple token
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(r.Header.Get("Signature")) == 0 {
				// Assume legacy simple token auth

				ctx := context.WithValue(r.Context(), merchantCtxKey{}, "brave.com")
				middleware.SimpleTokenAuthorizedOnly(next).ServeHTTP(w, r.WithContext(ctx))
				return
			}

			middleware.VerifyHTTPSignedOnly(merchantVerifier)(next).ServeHTTP(w, r)
		})
	}
}

// GetCaveats returns any authorized caveats that have been stored in the context
func GetCaveats(ctx context.Context) map[string]string {
	caveats, ok := ctx.Value(caveatsCtxKey{}).(map[string]string)
	if !ok {
		return nil
	}
	return caveats
}

// GetMerchant returns any authorized merchant that has been stored in the context
func GetMerchant(ctx context.Context) (string, error) {
	merchant, ok := ctx.Value(merchantCtxKey{}).(string)
	if !ok {
		return "", errors.New("merchant was missing from context")
	}
	return merchant, nil
}

// ValidateOrderMerchantAndCaveats checks that the current authentication of the request has
// permissions to this order by cross-checking the merchant and caveats in context
func (s *Service) ValidateOrderMerchantAndCaveats(r *http.Request, orderID uuid.UUID) error {
	merchant, err := GetMerchant(r.Context())
	if err != nil {
		return err
	}
	caveats := GetCaveats(r.Context())

	order, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return err
	}

	if order.MerchantID != merchant {
		return errors.New("Order merchant does not match authentication")
	}

	if caveats != nil {
		if location, ok := caveats["location"]; ok {
			if order.Location.Valid && order.Location.String != location {
				return errors.New("Order location does not match authentication")
			}
		}

		if _, ok := caveats["sku"]; ok {
			return errors.New("SKU caveat is not supported on order endpoints")
		}
	}
	return nil
}
