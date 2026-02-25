package storage

import (
	"github.com/redis/go-redis/v9"
)

func GetRedisClient() *redis.Client {

	rdb := redis.NewClient(&redis.Options{
		Addr:     "redis-14586.crce217.ap-south-1-1.ec2.cloud.redislabs.com:14586",
		Username: "default",
		Password: "ifHf3umMQDZi0wkP45X1WGxJFXMEwJuZ",
		DB:       0,
	})

	return rdb

}
