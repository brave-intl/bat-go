package middleware

import (
	"errors"
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/libs/handlers"
)

var (
	errUpgradeRequired = errors.New("upgrade required, cutoff exceeded")
)

// NewUpgradeRequiredByMiddleware passes a service into the context
func NewUpgradeRequiredByMiddleware(cutoff time.Time) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if time.Now().Before(cutoff) {
				next.ServeHTTP(w, r)
				return
			}
			ae := handlers.AppError{
				Cause:   errUpgradeRequired,
				Message: "upgrade required, cutoff exceeded",
				Code:    http.StatusUpgradeRequired,
			}
			ae.ServeHTTP(w, r)
		})
	}
}
