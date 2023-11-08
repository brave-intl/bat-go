package middleware

import (
	"context"
	"crypto"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
)

var (
	errMissingSignature = errors.New("missing http signature")
	errInvalidSignature = errors.New("invalid http signature")
	errInvalidHeader    = errors.New("invalid http header")
)

type httpSignedKeyID struct{}

// AddKeyID - Helpful for test cases
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

func SignResponse(p httpsignature.ParameterizedSignator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := logging.Logger(r.Context(), "SignResponse")
			// recorder response writer for debugging
			rec := httptest.NewRecorder()
			for k, v := range w.Header() {
				// add w's headers to response writer for debugging
				rec.Header().Add(k, strings.Join(v, " "))
			}
			// use the signator response writer with a base of recorder
			sw := httpsignature.NewParameterizedSignatorResponseWriter(p, rec)
			next.ServeHTTP(sw, r)
			// write all the recorded headers to real response writer
			for k, v := range rec.Header() {
				w.Header().Add(k, strings.Join(v, " "))
				logger.Info().Str(k, strings.Join(v, " ")).Msg("recorded header")
			}
			// write headers to real response writer
			w.WriteHeader(rec.Code)
			// write body to the real response writer
			w.Write(rec.Body.Bytes())
			logger.Info().Str("body", rec.Body.String()).Msg("recorded body")
		})
	}
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
			logger := logging.Logger(r.Context(), "VerifyHTTPSignedOnly")

			if len(r.Header.Get("Signature")) == 0 {
				logger.Warn().Msg("signature must be present for signed middleware")
				ae := handlers.AppError{
					Cause:   errMissingSignature,
					Message: "signature must be present for signed middleware",
					Code:    http.StatusUnauthorized,
				}
				ae.ServeHTTP(w, r)
				return
			}

			ctx, keyID, err := verifier.VerifyRequest(r)

			if err != nil {
				logger.Error().Err(err).Msg("failed to verify request")
				ae := handlers.AppError{
					Cause:   errInvalidSignature,
					Message: "request signature verification failure",
					Code:    http.StatusForbidden,
				}
				ae.ServeHTTP(w, r)
				return
			}

			ctx = context.WithValue(ctx, httpSignedKeyID{}, keyID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
