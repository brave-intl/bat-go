package payments

import (
	"crypto"
	"net/http"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/httpsignature"
)

// AuthorizerSignedMiddleware requires that requests are signed by valid payment authorizers
func (service *Service) AuthorizerSignedMiddleware() func(http.Handler) http.Handler {
	authorizerVerifier := httpsignature.ParameterizedKeystoreVerifier{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			Headers: []string{
				"(request-target)", "host", "date", "digest", "content-length", "content-type",
			},
		},
		Keystore: service,
		Opts:     crypto.Hash(0),
	}
	// the actual middleware
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			middleware.VerifyHTTPSignedOnly(authorizerVerifier)(next).ServeHTTP(w, r)
		})
	}
}
