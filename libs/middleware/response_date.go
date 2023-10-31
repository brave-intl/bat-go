package middleware

import (
	"net/http"
	"time"
)

// SetResponseDate sets a header date in the response
func SetResponseDate() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("date", time.Now().Format(time.RFC1123))
			next.ServeHTTP(w, r)
		})
	}
}
