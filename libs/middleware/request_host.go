package middleware

import (
	"net/http"

	"github.com/brave-intl/bat-go/libs/requestutils"
)

// HostTransfer transfers the request id from header to context
func HostTransfer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Header.Get(requestutils.HostHeaderKey)
		if host == "" {
			// use the x-forwarded-host
			host = r.Header.Get(requestutils.XForwardedHostHeaderKey)
		}
		r.Header.Set(requestutils.HostHeaderKey, host)
		next.ServeHTTP(w, r)
	})
}
