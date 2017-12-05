package utils

import (
	"context"

	"github.com/garyburd/redigo/redis"
)

func WithRedisPool(ctx context.Context, pool *redis.Pool) context.Context {
	ctx = context.WithValue(ctx, "datastore.set", "redis")
	ctx = context.WithValue(ctx, "datastore.kv", "redis")
	return context.WithValue(ctx, "redis.pool", pool)
}

// Remember to defer conn.Close()
func GetRedisConn(ctx context.Context) *redis.Conn {
	pool := ctx.Value("redis.pool").(*redis.Pool)
	conn := pool.Get()
	return &conn
}
