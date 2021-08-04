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
	// LookupPublicKey based on the keyID
	LookupPublicKey(ctx context.Context, keyID string) (*httpsignature.Verifier, error)
}

//AddKeyID - Helpful for test cases
func AddKeyID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, httpSignedKeyID{}, id)
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
			s, err := httpsignature.SignatureParamsFromRequest(r)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}

			// Override algorithm and headers to those we want to enforce
			s.Algorithm = httpsignature.ED25519
			s.Headers = []string{"digest", "(request-target)"}

			ctx := context.WithValue(r.Context(), httpSignedKeyID{}, s.KeyID)
			pubKey, err := ks.LookupPublicKey(ctx, s.KeyID)

			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			if pubKey == nil {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			}

			valid, err := s.Verify(*pubKey, crypto.Hash(0), r)

			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			if !valid {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
