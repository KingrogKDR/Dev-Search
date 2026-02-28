package storage

import (
	"context"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

func GetRedisClient() (*redis.Client, error) {

	rdb := redis.NewClient(&redis.Options{
		Addr:         os.Getenv("REDIS_ADDR"),
		Password:     "",
		DB:           0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     20,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return rdb, nil
}
