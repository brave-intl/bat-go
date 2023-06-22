package payments

import (
	"net/http"

	"crypto"

	appctx "github.com/brave-intl/bat-go/libs/context"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/middleware"
)

// ConfigurationMiddleware applies the current state of the service's configuration on the ctx.
func (s *Service) ConfigurationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// wrap the request context in the baseCtx from the service
		r = r.WithContext(appctx.Wrap(s.baseCtx, r.Context()))
		next.ServeHTTP(w, r)
	})
}

// AuthorizerSignedMiddleware requires that requests are signed by valid payment authorizers.
func (s *Service) AuthorizerSignedMiddleware() func(http.Handler) http.Handler {
	authorizerVerifier := httpsignature.ParameterizedKeystoreVerifier{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			Headers: []string{
				"(request-target)", "host", "date", "digest", "content-length", "content-type",
			},
		},
		Keystore: s,
		Opts:     crypto.Hash(0),
	}
	// the actual middleware
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			middleware.VerifyHTTPSignedOnly(authorizerVerifier)(next).ServeHTTP(w, r)
		})
	}
}
