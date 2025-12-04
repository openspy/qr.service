package Server

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

type ServerGroupManager struct {
}

func (m *ServerGroupManager) selectGroupRedisDB(context context.Context, redisClient *redis.Client) {
	redisClient.Conn().Select(context, 2)
}

func (m *ServerGroupManager) GetServerKey(context context.Context, redisClient *redis.Client, gameid int, groupid int) string {
	m.selectGroupRedisDB(context, redisClient)
	return ""
}
func (m *ServerGroupManager) IncrNumServers(context context.Context, redisClient *redis.Client, serverKey string) {
	m.selectGroupRedisDB(context, redisClient)
}
func (m *ServerGroupManager) DecrNumServers(context context.Context, redisClient *redis.Client, serverKey string) {
	m.selectGroupRedisDB(context, redisClient)
}
func (m *ServerGroupManager) ResyncAllGroups(context context.Context, redisClient *redis.Client) {
	m.selectGroupRedisDB(context, redisClient)
	log.Printf("group resync\n")

	//do pipelined scan and clear

	//do pipelined scan of all servers, lookup (and cache) game
}
