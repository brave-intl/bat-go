// +build integration

package grant

import (
	"testing"

	"github.com/garyburd/redigo/redis"
)

func TestBreaker(t *testing.T) {
	breakerCountThreshold = 3

	c, err := redis.Dial("tcp", ":6379")
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
