package middleware

import (
	"context"
	"net/http"

	appctx "github.com/brave-intl/bat-go/libs/context"
)

// NewServiceCtx passes a service into the context
func NewServiceCtx(service interface{}) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), appctx.ServiceKey, service)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
