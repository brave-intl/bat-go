package middleware

import (
	"net/http"
)

func RequestCorrelationMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if corrID := r.Header.Get("X-Correlation-ID"); corrID != "" {
				w.Header().Set("X-Correlation-ID", corrID)
			}
			next.ServeHTTP(w, r)
		})
	}
}
