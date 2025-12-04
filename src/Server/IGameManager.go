package Server

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// group redis db = 1
type IGameManager interface {
	GetGameKey(context context.Context, redisClient *redis.Client, gameid int) string
	GetBackendFlags(context context.Context, redisClient *redis.Client, gameKey string) int
}
