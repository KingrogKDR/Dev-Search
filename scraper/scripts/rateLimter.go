package scripts

import "github.com/redis/go-redis/v9"

var RateLimitScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local delay = tonumber(ARGV[2])

local nextAllowed = redis.call("GET", key)

if not nextAllowed then
    nextAllowed = now
else
    nextAllowed = tonumber(nextAllowed)
end

if nextAllowed < now then
    nextAllowed = now
end

local wait = nextAllowed - now
local newNext = nextAllowed + delay

redis.call("SET", key, newNext)

return wait
`)
