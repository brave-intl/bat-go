package datastore

import (
	"context"
	"strconv"

	"github.com/garyburd/redigo/redis"
)

type redisPoolKey struct{}

// WithRedisPool adds a redis pool and the context to use it as a set-like and kv datastore
func WithRedisPool(ctx context.Context, pool *redis.Pool) context.Context {
	ctx = context.WithValue(ctx, setDatastoreKey{}, "redis")
	ctx = context.WithValue(ctx, kvDatastoreKey{}, "redis")
	return context.WithValue(ctx, redisPoolKey{}, pool)
}

// GetRedisConn returns a connection using the redis pool in context
// Remember to defer conn.Close()
func GetRedisConn(ctx context.Context) *redis.Conn {
	pool := ctx.Value(redisPoolKey{}).(*redis.Pool)
	conn := pool.Get()
	return &conn
}

// RedisSet a redis backed implementation of a set-like datastore
type RedisSet struct {
	conn *redis.Conn
	key  string
}

// GetRedisSet returns the redis backed set at key
func GetRedisSet(conn *redis.Conn, key string) RedisSet {
	return RedisSet{conn: conn, key: key}
}

// Cardinality returns the number of elements in the set
func (set *RedisSet) Cardinality() (int, error) {
	return redis.Int((*set.conn).Do("SCARD", set.key))
}

// Contains returns true if the given item is in the set
func (set *RedisSet) Contains(e string) (bool, error) {
	return redis.Bool((*set.conn).Do("SISMEMBER", set.key, e))
}

// Add a single element to the set, return true if newly added
func (set *RedisSet) Add(e string) (bool, error) {
	n, err := redis.Int((*set.conn).Do("SADD", set.key, e))
	if n != 0 {
		return true, err
	}
	return false, err
}

// Close the underlying connection to the datastore
func (set *RedisSet) Close() error {
	return (*set.conn).Close()
}

// RedisKv a redis backed implementation of a key value datastore
type RedisKv struct {
	conn *redis.Conn
}

// GetRedisKv returns the redis backed key value store
func GetRedisKv(conn *redis.Conn) RedisKv {
	return RedisKv{conn: conn}
}

// Set key to hold the string value with ttl in seconds, returns true if updated successfully.
// If ttl is negative, the value will not expire.
// Will only update an existing value if upsert is true.
// Returns an error if the update failed.
func (store *RedisKv) Set(key string, value string, ttl int, upsert bool) (bool, error) {
	var ret string
	var err error

	if ttl < 0 {
		if upsert {
			ret, err = redis.String((*store.conn).Do("SET", key, value))
		} else {
			ret, err = redis.String((*store.conn).Do("SET", key, value, "NX"))
		}
	} else {
		if upsert {
			ret, err = redis.String((*store.conn).Do("SET", key, value, "EX", strconv.Itoa(ttl)))
		} else {
			ret, err = redis.String((*store.conn).Do("SET", key, value, "NX", "EX", strconv.Itoa(ttl)))
		}
	}

	return ret == "OK", err
}

// Get value held at key and returns it, error if the key does not exist
func (store *RedisKv) Get(key string) (string, error) {
	return redis.String((*store.conn).Do("GET", key))
}

// Delete the value held at key, returns true if a value was present
func (store *RedisKv) Delete(key string) (bool, error) {
	n, err := redis.Int((*store.conn).Do("DEL", key))
	if n != 0 {
		return true, err
	}
	return false, err
}

// Count the keys matching pattern
func (store *RedisKv) Count(pattern string) (int, error) {
	return redis.Int((*store.conn).Do("EVAL", "return #redis.call('keys', '"+pattern+"')", 0))
}

// Return the keys matching pattern
func (store *RedisKv) Keys(pattern string) ([]string, error) {
	return redis.Strings((*store.conn).Do("KEYS", pattern))
}

// Close the underlying connection to the datastore
func (store *RedisKv) Close() error {
	return (*store.conn).Close()
}
