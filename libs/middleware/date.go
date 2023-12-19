package middleware

import (
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
)

// VerifyDateIsRecent is a middleware that verifies a request has a date
// which is after now()-validFrom and before now()+validTo
func VerifyDateIsRecent(validFrom, validTo time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := logging.Logger(r.Context(), "VerifyDateIsRecent")

			// Date: Wed, 21 Oct 2015 07:28:00 GMT
			dateStr := r.Header.Get("date")
			date, err := time.Parse(time.RFC1123, dateStr)
			if err != nil {
				logger.Error().Err(err).Msg("failed to parse the date header")
				ae := handlers.AppError{
					Cause:   errInvalidHeader,
					Message: "Invalid date header",
					Code:    http.StatusBadRequest,
				}
				ae.ServeHTTP(w, r)
				return
			}

			if !date.After(time.Now().Add(-1 * validFrom)) {
				logger.Error().Err(err).Msg("date is too far in the past")
				ae := handlers.AppError{
					Cause:   errInvalidHeader,
					Message: "date is too far in the past",
					Code:    http.StatusRequestTimeout,
				}
				ae.ServeHTTP(w, r)
				return
			}
			if !date.Before(time.Now().Add(validTo)) {
				logger.Error().Err(err).Msg("date is too far in the future")
				ae := handlers.AppError{
					Cause:   errInvalidHeader,
					Message: "date is too far in the future",
					Code:    http.StatusTooEarly,
				}
				ae.ServeHTTP(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
