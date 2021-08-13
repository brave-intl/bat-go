package middleware

import (
	"context"
	"crypto"
	"errors"
	"net/http"

	"github.com/brave-intl/bat-go/utils/httpsignature"
)

type httpSignedKeyID struct{}

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
func HTTPSignedOnly(ks httpsignature.Keystore) func(http.Handler) http.Handler {
	verifier := httpsignature.ParameterizedKeystoreVerifier{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			Headers:   []string{"digest", "(request-target)"},
		},
		Keystore: ks,
		Opts:     crypto.Hash(0),
	}

	return VerifyHTTPSignedOnly(verifier)
}

// VerifyHTTPSignedOnly is a middleware that requires an HTTP request to be signed
// which takes a parameterized http signature verifier
func VerifyHTTPSignedOnly(verifier httpsignature.ParameterizedKeystoreVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(r.Header.Get("Signature")) == 0 {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			keyID, err := verifier.VerifyRequest(r)

			if err != nil {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			// FIXME verify known headers, e.g. host, date

			ctx := context.WithValue(r.Context(), httpSignedKeyID{}, keyID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
