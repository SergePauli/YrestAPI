package db

import (
	"context"

	"YrestAPI/internal/logger"

	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client

// InitRedis принимает адрес явно (а не через os.Getenv)
func InitRedis(addr string) {
	if addr == "" {
		addr = "localhost:6379"
		logger.Warn("redis_default_addr", nil)
	}

	RDB = redis.NewClient(&redis.Options{
		Addr: addr,
	})
}

func PingRedis() error {
	return RDB.Ping(context.Background()).Err()
}
