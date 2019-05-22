// +build integration

package grant

import (
	"os"
	"strings"
	"testing"

	"github.com/garyburd/redigo/redis"
)

var (
	redisURL = os.Getenv("REDIS_URL")
)

func TestBreaker(t *testing.T) {
	breakerCountThreshold = 3

	redisAddress := "localhost:6379"
	if len(redisURL) > 0 {
		redisAddress = strings.TrimPrefix(redisURL, "redis://")
	}

	c, err := redis.Dial("tcp", redisAddress)
	if err != nil {
		t.Error(err)
	}

	c.Do("SELECT", "42")
	c.Do("FLUSHDB")

	b := GetBreaker(&c)
	b.Increment()
	if breakerTripped {
		t.Error("Breaker tripped early")
	}
	b.Increment()
	if breakerTripped {
		t.Error("Breaker tripped early")
	}
	b.Increment()
	if !breakerTripped {
		t.Error("Breaker didn't trip!")
	}

}
