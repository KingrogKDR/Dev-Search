package storage

import (
	"os"

	"github.com/redis/go-redis/v9"
)

func GetRedisClient() *redis.Client {

	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
	})
	return rdb
}
