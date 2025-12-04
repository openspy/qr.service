package Server

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type GameManager struct {
}

func (m *GameManager) selectRedisDB(context context.Context, redisClient *redis.Client) {
	redisClient.Conn().Select(context, 2)
}

func (m *GameManager) GetGameKey(context context.Context, redisClient *redis.Client, gameid int) string {
	m.selectRedisDB(context, redisClient)
	var lookupKey = "gameid_" + string(gameid)
	result, err := redisClient.Get(context, lookupKey).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetGameKey error: %s\n", err.Error())
		return ""
	}
	return result
}
func (m *GameManager) GetBackendFlags(context context.Context, redisClient *redis.Client, gameKey string) int {
	result, err := redisClient.HGet(context, gameKey, "backendflags").Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetBackendFlags error: %s\n", err.Error())
		return 0
	}
	intVal, err := strconv.Atoi(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetBackendFlags atoi error: %s\n", err.Error())
		return 0
	}
	return intVal
}
