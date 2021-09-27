package middleware

import (
	"context"
	"crypto"
	"errors"
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/utils/contains"
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

			ctx, keyID, err := verifier.VerifyRequest(r)

			if err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}

			if contains.Str(verifier.SignatureParams.Headers, "date") {
				// Date: Wed, 21 Oct 2015 07:28:00 GMT
				dateStr := r.Header.Get("date")
				date, err := time.Parse(time.RFC1123, dateStr)
				if err != nil {
					http.Error(w, "Invalid date header", http.StatusBadRequest)
					return
				}

				if time.Now().Add(10 * time.Minute).Before(date) {
					http.Error(w, "Request date is invalid", http.StatusTooEarly)
					return
				}
				if time.Now().Add(-10 * time.Minute).After(date) {
					http.Error(w, "Request date is too old", http.StatusRequestTimeout)
					return
				}
			}

			ctx = context.WithValue(ctx, httpSignedKeyID{}, keyID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
