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
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			verifier := httpsignature.ParameterizedKeystoreVerifier{
				SignatureParams: httpsignature.SignatureParams{
					Algorithm: httpsignature.ED25519,
					Headers:   []string{"digest", "(request-target)"},
				},
				Keystore: ks,
				Opts:     crypto.Hash(0),
			}

			_, err := verifier.VerifyRequest(r)

			if err != nil {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r.WithContext(r.Context()))
		})
	}
}
