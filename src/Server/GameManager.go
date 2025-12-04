package Server

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type GameManager struct {
}

func (m *GameManager) selectRedisDB(context context.Context, redisClient *redis.Client) {
	redisClient.Conn().Select(context, 2)
}

func (m *GameManager) GetGamename(context context.Context, redisClient *redis.Client, gameid int) string {
	return "gmtest"
}
func (m *GameManager) GetBackendFlags(context context.Context, redisClient *redis.Client, gamename string) int {
	return 0
}
