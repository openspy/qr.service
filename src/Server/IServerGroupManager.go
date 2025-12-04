package Server

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type IServerGroupManager interface {
	GetServerKey(context context.Context, redisClient *redis.Client, gameid int, groupid int) string
	IncrNumServers(context context.Context, redisClient *redis.Client, serverKey string)
	DecrNumServers(context context.Context, redisClient *redis.Client, serverKey string)
	ResyncAllGroups(context context.Context, redisClient *redis.Client)
}
