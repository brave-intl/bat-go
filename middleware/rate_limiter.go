package middleware

import (
	"log"
	"net/http"

	"github.com/throttled/throttled"
	"github.com/throttled/throttled/store/memstore"
)

// RateLimiter rate limits the number of requests a
// user from a single IP address can make
func RateLimiter(next http.Handler) http.Handler {
	store, err := memstore.New(65536)
	if err != nil {
		log.Fatal(err)
	}
	quota := throttled.RateQuota{
		MaxRate:  throttled.PerMin(60),
		MaxBurst: 60,
	}
	rateLimiter, err := throttled.NewGCRARateLimiter(store, quota)
	if err != nil {
		log.Fatal(err)
	}

	httpRateLimiter := throttled.HTTPRateLimiter{
		RateLimiter: rateLimiter,
		VaryBy: &throttled.VaryBy{
			RemoteAddr: true,
			Path:       true,
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
