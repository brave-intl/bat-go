package grant

import (
	"context"
	"strconv"

	"github.com/brave-intl/bat-go/datastore"
	"github.com/garyburd/redigo/redis"
	raven "github.com/getsentry/raven-go"
)

const (
	breakerCountKey = "breaker:grant_redeem_error:count"
	breakerCountTTL = 60 // 1 minute
)

var (
	breakerTripped        = false
	breakerCountThreshold = 10
)

// Breaker helps implement a basic circuit-breaker pattern
type Breaker struct {
	conn *redis.Conn
}

// GetBreakerFromContext by getting a redis connection from the context
func GetBreakerFromContext(ctx context.Context) Breaker {
	conn := datastore.GetRedisConn(ctx)
	return GetBreaker(conn)
}

// GetBreaker from redis connection
func GetBreaker(conn *redis.Conn) Breaker {
	return Breaker{conn}
}

// Increment breaker failure count, if over threshold trip the breaker
func (b *Breaker) Increment() error {
	currentValue, err := b.incr()
	if err != nil {
		return err
	}

	_, err = b.expire(breakerCountTTL)
	if err != nil {
		return err
	}

	// Breaker is configured to trip if 10 error events occur each separated by 1 minute or less
	if currentValue >= breakerCountThreshold {
		breakerTripped = true
		raven.CaptureMessage("Circuit breaker tripped!!!", map[string]string{"breaker": "true"})
	}
	return nil
}

func (b *Breaker) incr() (int, error) {
	return redis.Int((*b.conn).Do("INCR", breakerCountKey))
}

func (b *Breaker) expire(ttl int) (bool, error) {
	n, err := redis.Int((*b.conn).Do("EXPIRE", breakerCountKey, strconv.Itoa(ttl)))
	if n != 0 {
		return true, err
	}
	return false, err
}
