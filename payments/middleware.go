package payments

import (
	"net/http"

	appctx "github.com/brave-intl/bat-go/utils/context"
)

// ConfigurationMiddleware applies the current state of the service's configuration on the ctx
func (service *Service) ConfigurationMiddleware() func(http.Handler) http.Handler {
	// the actual middleware
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// wrap the request context in the baseCtx from the service
			r = r.WithContext(appctx.Wrap(service.baseCtx, r.Context()))
			next.ServeHTTP(w, r)
		})
	}
}
