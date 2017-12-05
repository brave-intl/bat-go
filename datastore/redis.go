package datastore

import (
	"strconv"

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

type RedisKv struct {
	conn *redis.Conn
}

func GetRedisKv(conn *redis.Conn) RedisKv {
	return RedisKv{conn: conn}
}

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

func (store *RedisKv) Get(key string) (string, error) {
	return redis.String((*store.conn).Do("GET", key))
}

func (store *RedisKv) Delete(key string) (bool, error) {
	n, err := redis.Int((*store.conn).Do("DEL", key))
	if n != 0 {
		return true, err
	}
	return false, err
}

func (store *RedisKv) Close() error {
	return (*store.conn).Close()
}
