package datastore

import (
	"github.com/garyburd/redigo/redis"
)

type RedisSet struct {
	conn *redis.Conn
	key  string
}

func GetRedisSet(conn *redis.Conn, key string) RedisSet {
	return RedisSet{conn: conn, key: key}
}

func (set *RedisSet) Cardinality() (int, error) {
	return redis.Int((*set.conn).Do("SCARD", set.key))
}

func (set *RedisSet) Contains(e string) (bool, error) {
	return redis.Bool((*set.conn).Do("SISMEMBER", set.key, e))
}

func (set *RedisSet) Add(e string) (bool, error) {
	n, err := redis.Int((*set.conn).Do("SADD", set.key, e))
	if n != 0 {
		return true, err
	}
	return false, err
}
func (set *RedisSet) Close() error {
	return (*set.conn).Close()
}
