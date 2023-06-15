package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
)

func TestRateLimiterMemoryMiddleware(t *testing.T) {
	limit := 60
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wrappedHandler := RateLimiter(ctx, limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server := httptest.NewServer(wrappedHandler)
	defer server.Close()

	for a := 1; a < limit+2; a++ {
		resp, _ := http.Get(server.URL)
		// The GCRA in throttled divides each minute into equal segments and
		// only allows 1 call per segment. This means we can't hit the API 60
		// times in 1 second. In order to verify expected behavior, start querying
		// more than 1 second apart and move closer, hitting the first limit on
		// iteration 4
		if a > 3 {
			assert.Equal(t, resp.StatusCode, 429, "Limiter should trigger immediately after limit is exceeded")
		} else {
			assert.NotEqual(t, resp.StatusCode, 429, "Limiter should not trigger early")
		}
		// Sleep to allow the bucket to fill up. Sleep less each iteration so
		// that we eventually hit the limit.
		// Iteration 1: sleeps 2 sec
		// Iteration 2: sleeps 1 sec
		// Iteration 3: sleeps 0.66 sec
		time.Sleep(time.Duration(2/a) * time.Second)
	}
}

func TestRateLimiterRedisMiddleware(t *testing.T) {
	limit := 60
	burst := 2
	mr, _ := miniredis.Run()
	pool := &redis.Pool{
		MaxIdle:     1,
		IdleTimeout: 5000,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", mr.Addr())
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wrappedHandler := RateLimiterRedisStore(ctx, limit, burst, pool, "", 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server := httptest.NewServer(wrappedHandler)
	defer server.Close()

	for a := 1; a < limit+2; a++ {
		resp, _ := http.Get(server.URL)
		// The GCRA in throttled divides each minute into equal segments and
		// only allows 1 call per segment plus bursts. In order to verify
		// expected behavior, start querying more than 1 second apart and move
		//closer. Accounting for the burst setting we should this at iteration
		// 5
		if a > 5 {
			assert.Equal(t, resp.StatusCode, 429, "Limiter should trigger immediately after limit is exceeded")
		} else {
			assert.NotEqual(t, resp.StatusCode, 429, "Limiter should not trigger early")
		}
		// Sleep to allow the bucket to fill up. Sleep less each iteration so
		// that we eventually hit the limit.
		// Iteration 1: sleeps 2 sec
		// Iteration 2: sleeps 1 sec
		// Iteration 3: sleeps 0.66 sec
		time.Sleep(time.Duration(2/a) * time.Second)
	}
}
