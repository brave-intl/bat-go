package middleware

import (
	"context"
	"net/http"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/throttled/throttled"
	"github.com/throttled/throttled/store/memstore"
)

// RateLimiter rate limits the number of requests a
// user from a single IP address can make
func RateLimiter(ctx context.Context) func(next http.Handler) http.Handler {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	return func(next http.Handler) http.Handler {
		store, err := memstore.New(65536)
		if err != nil {
			logger.Fatal().Err(err)
		}
		quota := throttled.RateQuota{
			MaxRate: throttled.PerMin(180),
		}
		rateLimiter, err := throttled.NewGCRARateLimiter(store, quota)
		if err != nil {
			logger.Fatal().Err(err)
		}

		httpRateLimiter := throttled.HTTPRateLimiter{
			RateLimiter: rateLimiter,
			VaryBy: &throttled.VaryBy{
				RemoteAddr: true,
				Path:       true,
				Method:     true,
			},
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isSimpleTokenInContext(r.Context()) {
				httpRateLimiter.RateLimit(next).ServeHTTP(w, r)
			} else {
				// override rate limiting for authorized endpoints
				next.ServeHTTP(w, r)
			}
		})
	}
}
