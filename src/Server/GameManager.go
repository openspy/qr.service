package Server

import (
	"context"
	"log"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type GameManager struct {
}

func (m *GameManager) GetGameKey(context context.Context, redisClient *redis.Client, gameid int) string {
	var lookupKey = "gameid_" + strconv.Itoa(gameid)
	result, err := redisClient.Get(context, lookupKey).Result()
	if err != nil && len(err.Error()) > 0 {
		log.Printf("GetGameKey error: %s\n", err.Error())
		return ""
	}
	return result
}
func (m *GameManager) GetBackendFlags(context context.Context, redisClient *redis.Client, gameKey string) int {
	result, err := redisClient.HGet(context, gameKey, "backendflags").Result()
	if err != nil && len(err.Error()) > 0 {
		log.Printf("GetBackendFlags error: %s\n", err.Error())
		return 0
	}
	intVal, err := strconv.Atoi(result)
	if err != nil && len(err.Error()) > 0 {
		log.Printf("GetBackendFlags atoi error: %s\n", err.Error())
		return 0
	}
	return intVal
}
