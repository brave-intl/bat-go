package middleware

import (
	"context"
	"net/http"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/redis/go-redis/v9"
	"github.com/throttled/throttled/v2"
	"github.com/throttled/throttled/v2/store/goredisstore.v9"
	"github.com/throttled/throttled/v2/store/memstore"
)

// IPRateLimiterWithStore rate limits based on IP using
// a provided store and a GCRA leaky bucket algorithm.
// This can be a simple memory store, a Redis store, or other stores for
// multi-instance synchronization. See
// https://github.com/throttled/throttled/tree/master/store for details.
func IPRateLimiterWithStore(
	ctx context.Context,
	perMin int,
	burst int,
	store throttled.GCRAStoreCtx,
) func(next http.Handler) http.Handler {
	logger := logging.Logger(ctx, "middleware.IPRateLimiterWithStore")

	return func(next http.Handler) http.Handler {
		quota := throttled.RateQuota{
			MaxRate:  throttled.PerMin(perMin),
			MaxBurst: burst,
		}
		rateLimiter, err := throttled.NewGCRARateLimiterCtx(store, quota)
		if err != nil {
			logger.Fatal().Err(err)
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// override for OPTIONS request methods, as sometimes many cors requests happen quickly??
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			if !isSimpleTokenInContext(r.Context()) {
				httpRateLimiter := throttled.HTTPRateLimiterCtx{
					RateLimiter: rateLimiter,
					VaryBy: &throttled.VaryBy{
						RemoteAddr: true,
						Path:       true,
						Method:     true,
					},
				}
				httpRateLimiter.RateLimit(next).ServeHTTP(w, r)
			} else {
				// override rate limiting for authorized endpoints
				next.ServeHTTP(w, r)
			}
		})
	}
}

// RateLimiter rate limits the number of requests a
// user from a single IP address can make using a simple
// in-memory store that will not synchronize across instances.
func RateLimiter(ctx context.Context, perMin int) func(next http.Handler) http.Handler {
	logger := logging.Logger(ctx, "middleware.RateLimiter")
	store, err := memstore.NewCtx(65536)
	if err != nil {
		logger.Fatal().Err(err)
	}
	// Including burst in the existing function would break the contract so it must
	// be 0 until a point release.
	defaultBurst := 0

	if burst, ok := ctx.Value(appctx.RateLimiterBurstCTXKey).(int); ok {
		defaultBurst = burst
	}

	return IPRateLimiterWithStore(ctx, perMin, defaultBurst, store)
}

// RateLimiterRedisStore rate limits the number of requests a
// user from a single IP address can make and coordinates request counts
// between instances using Redis.
func RateLimiterRedisStore(
	ctx context.Context,
	perMin int,
	burst int,
	redis *redis.Client,
	keyPrefix string,
) func(next http.Handler) http.Handler {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	store, err := goredisstore.NewCtx(redis, keyPrefix)
	if err != nil {
		logger.Fatal().Err(err)
	}
	return IPRateLimiterWithStore(ctx, perMin, burst, store)
}
