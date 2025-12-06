package Server

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
)

const GROUP_SCAN_BATCH_COUNT int64 = 500

type ServerGroupManager struct {
}

func (m *ServerGroupManager) selectGroupRedisDB(context context.Context, redisClient *redis.Client) {
	redisClient.Conn().Select(context, 1)
}
func (m *ServerGroupManager) selectServerRedisDB(context context.Context, redisClient *redis.Client) {
	redisClient.Conn().Select(context, 0)
}

func (m *ServerGroupManager) GetGroupKey(context context.Context, redisClient *redis.Client, serverKey string) string {
	m.selectServerRedisDB(context, redisClient)
	pipeline := redisClient.Pipeline()
	gamenameCmd := pipeline.HGet(context, serverKey, "gamename")
	groupidCmd := pipeline.HGet(context, serverKey+"custkeys", "groupid")
	_, err := pipeline.Exec(context)
	if err != nil {
		fmt.Fprintf(os.Stderr, "IncrNumServers pipeline error: %s\n", err.Error())
	}
	gamename, _ := gamenameCmd.Result()
	groupid, _ := groupidCmd.Result()
	return gamename + ":" + groupid
}
func (m *ServerGroupManager) IncrNumServers(context context.Context, redisClient *redis.Client, groupKey string) {
	m.selectGroupRedisDB(context, redisClient)
	redisClient.HIncrBy(context, groupKey, "numservers", 1)
}
func (m *ServerGroupManager) DecrNumServers(context context.Context, redisClient *redis.Client, groupKey string) {
	m.selectGroupRedisDB(context, redisClient)
	redisClient.HIncrBy(context, groupKey, "numservers", -1)
}
func (m *ServerGroupManager) ResyncAllGroups(context context.Context, redisClient *redis.Client) {
	m.selectGroupRedisDB(context, redisClient)

	var cursor int = 0

	deletePipeline := redisClient.Pipeline()
	//do pipelined scan and clear
	for {
		keys, nextCursor, err := redisClient.Scan(context, uint64(cursor), "*:*", GROUP_SCAN_BATCH_COUNT).Result()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ResyncAllGroups scan error: %s\n", err.Error())
			break
		}
		cursor = int(nextCursor)

		for _, key := range keys {
			deletePipeline.HSet(context, key, "numservers", "0")
		}
		if cursor == 0 {
			break
		}
	}

	_, err := deletePipeline.Exec(context)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ResyncAllGroups pipeline error: %s\n", err.Error())
	}

	gameGroupPipeline := redisClient.Pipeline()

	//do pipelined scan of all servers, lookup (and cache) game
	m.selectServerRedisDB(context, redisClient)

	var groupPipelineCmds []*redis.StringCmd
	for {
		keys, nextCursor, err := redisClient.Scan(context, uint64(cursor), "*:", GROUP_SCAN_BATCH_COUNT).Result()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ResyncAllGroups game scan error: %s\n", err.Error())
			break
		}
		cursor = int(nextCursor)
		for _, key := range keys {
			groupPipelineCmds = append(groupPipelineCmds, gameGroupPipeline.HGet(context, key, "gamename"))
			groupPipelineCmds = append(groupPipelineCmds, gameGroupPipeline.HGet(context, key+"custkeys", "groupid"))
		}
		if cursor == 0 {
			break
		}
	}

	_, err = gameGroupPipeline.Exec(context)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ResyncAllGroups pipeline error: %s\n", err.Error())
	}

	incrPipeline := redisClient.Pipeline()
	for idx := 0; idx < len(groupPipelineCmds); idx += 2 {
		gamename, _ := groupPipelineCmds[idx].Result()
		groupidStr, _ := groupPipelineCmds[idx+1].Result()
		if len(groupidStr) == 0 || len(gamename) == 0 {
			continue
		}
		var groupKey = gamename + ":" + groupidStr
		incrPipeline.HIncrBy(context, groupKey, "numservers", 1)
	}
	_, err = incrPipeline.Exec(context)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ResyncAllGroups INCR pipeline error: %s\n", err.Error())
	}
}
