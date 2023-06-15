package lua

import "github.com/go-redis/redis/v8"

var (
	//TODO imporve comment
	// Unlock performs an atomic get and del if the value is as expect otherwise return 0.
	Unlock = redis.NewScript(`
		if redis.call("get",KEYS[1]) == ARGV[1]
		then
			return redis.call("del",KEYS[1])
		else
			return 0
		end
	`)
)

//TODO investigate how to achieve this in cluster mode

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
