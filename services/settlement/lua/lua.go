package lua

import "github.com/go-redis/redis/v8"

var (
	// Unlock performs an atomic get and del. Unlock compares the provided value argument with the one stored at
	// the key, if they are the equal then the key is deleted and a value of 1 is returned otherwise a 0 is returned.
	Unlock = redis.NewScript(`
		if redis.call("get",KEYS[1]) == ARGV[1]
		then
			return redis.call("del",KEYS[1])
		else
			return 0
		end
	`)
)

//TODO investigate how to achieve this in cluster mode. This is not blocking for first iteration.

// DelConsumer deletes a consumer from the group. A consumer can only be removed if they do not
// have any pending messages.
var (
	DelConsumer = redis.NewScript(`
		local pending = redis.call("XPENDING", KEYS[1], ARGV[1], "-", "+", 1, ARGV[2])
		if #(pending) >= 1 then
			return redis.error_reply("cannot delete consumer it has pending messages")
		end
		return redis.call("XGROUP", "DELCONSUMER", KEYS[2], ARGV[2])
	`)
)
