package payments

import (
	"net/http"
	"time"

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

// AuthorizerSignedMiddleware requires that requests are signed by valid payment authorizers. If `authorizer` is nil, use paymentAuthorizers.
func AuthorizerSignedMiddleware(authorizers *Authorizers) func(http.Handler) http.Handler {
	if authorizers == nil {
		authorizers = &paymentAuthorizers
	}
	authorizerVerifier := httpsignature.ParameterizedKeystoreVerifier{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			Headers:   httpsignature.RequestSigningHeaders,
		},
		Keystore: authorizers,
	}
	// the actual middleware
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// allow which are requests up to 30 days old and 1 minute in the future
			next = middleware.VerifyDateIsRecent(30*24*time.Hour, 1*time.Minute)(next)
			middleware.VerifyHTTPSignedOnly(authorizerVerifier)(next).ServeHTTP(w, r)
		})
	}
}
