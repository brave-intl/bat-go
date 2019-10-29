package middleware

import (
	"log"
	"net/http"

	"github.com/throttled/throttled"
	"github.com/throttled/throttled/store/memstore"
)

// RateLimiter rate limits the number of requests a
// user from a single IP address can make
func RateLimiter() func(http.Handler) http.Handler {
	store, err := memstore.New(65536)
	if err != nil {
		log.Fatal(err)
	}
	quota := throttled.RateQuota{
		MaxRate:  throttled.PerMin(5),
		MaxBurst: 5,
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

	return httpRateLimiter.RateLimit
}
