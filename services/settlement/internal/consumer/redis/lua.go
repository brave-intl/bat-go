package redis

import "github.com/go-redis/redis/v8"

var (
	// Unlock performs an atomic get and del. Unlock compares the provided value with the one stored at key,
	// if they are equal then the key is deleted and a value of 1 is returned otherwise 0 is returned.
	Unlock = redis.NewScript(`
		if redis.call("get",KEYS[1]) == ARGV[1]
		then
			return redis.call("del",KEYS[1])
		else
			return 0
		end
	`)
)
