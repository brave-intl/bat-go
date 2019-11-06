package middleware

import (
	"context"
	"crypto"
	"errors"
	"net/http"

	"github.com/brave-intl/bat-go/utils/httpsignature"
)

type httpSignedKeyID struct{}

// Keystore provides a way to lookup a public key based on the keyID a request was signed with
type Keystore interface {
	// GetPublicKey based on the keyID
	GetPublicKey(ctx context.Context, keyID string) (*httpsignature.Verifier, error)
}

// GetKeyID retrieves the http signing keyID from the context
func GetKeyID(ctx context.Context) (string, error) {
	keyID, ok := ctx.Value(httpSignedKeyID{}).(string)
	if !ok {
		return "", errors.New("keyID was missing from context")
	}
	return keyID, nil
}

// HTTPSignedOnly is a middleware that requires an HTTP request to be signed
func HTTPSignedOnly(ks Keystore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var s httpsignature.Signature
			err := s.UnmarshalText([]byte(r.Header.Get("Signature")))
			if err != nil {
				http.Error(w, http.StatusText(400), 400)
				return
			}

			// Override algorithm and headers to those we want to enforce
			s.Algorithm = httpsignature.ED25519
			s.Headers = []string{"digest", "(request-target)"}

			ctx := context.WithValue(r.Context(), httpSignedKeyID{}, s.KeyID)
			pubKey, err := ks.GetPublicKey(ctx, s.KeyID)

			if err != nil {
				http.Error(w, http.StatusText(500), 500)
				return
			}
			if pubKey == nil {
				http.Error(w, http.StatusText(404), 404)
				return
			}

			valid, err := s.Verify(*pubKey, crypto.Hash(0), r)

			if err != nil {
				http.Error(w, http.StatusText(500), 500)
				return
			}
			if !valid {
				http.Error(w, http.StatusText(403), 403)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
