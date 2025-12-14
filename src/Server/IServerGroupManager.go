package Server

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type IServerGroupManager interface {
	IncrNumServers(context context.Context, redisClient *redis.Client, serverKey string)
	DecrNumServers(context context.Context, redisClient *redis.Client, serverKey string)
	ResyncAllGroups(context context.Context, redisServerClient *redis.Client, redisGroupClient *redis.Client)
}
