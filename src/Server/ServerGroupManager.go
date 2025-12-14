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

func (m *ServerGroupManager) IncrNumServers(context context.Context, redisClient *redis.Client, groupKey string) {
	redisClient.HIncrBy(context, groupKey, "numservers", 1)
}
func (m *ServerGroupManager) DecrNumServers(context context.Context, redisClient *redis.Client, groupKey string) {
	redisClient.HIncrBy(context, groupKey, "numservers", -1)
}
func (m *ServerGroupManager) ResyncAllGroups(context context.Context, redisServerClient *redis.Client, redisGroupClient *redis.Client) {

	var cursor int = 0

	deletePipeline := redisGroupClient.Pipeline()
	//do pipelined scan and clear
	for {
		keys, nextCursor, err := redisGroupClient.Scan(context, uint64(cursor), "*:*", GROUP_SCAN_BATCH_COUNT).Result()
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

	//do pipelined scan of all servers
	for {
		keys, nextCursor, err := redisServerClient.Scan(context, uint64(cursor), "*:", GROUP_SCAN_BATCH_COUNT).Result()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ResyncAllGroups game scan error: %s\n", err.Error())
			break
		}

		cursor = int(nextCursor)
		var groupPipelineCmds []*redis.StringCmd
		var serverKeys []string
		gameGroupPipeline := redisServerClient.Pipeline()
		for _, key := range keys {
			groupPipelineCmds = append(groupPipelineCmds, gameGroupPipeline.HGet(context, key, "gamename"))
			groupPipelineCmds = append(groupPipelineCmds, gameGroupPipeline.HGet(context, key+"custkeys", "groupid"))
			serverKeys = append(serverKeys, key)
		}
		_, err = gameGroupPipeline.Exec(context)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ResyncAllGroups pipeline error: %s\n", err.Error())
		}

		//now incr groupids for all servers which have them set
		incrPipeline := redisGroupClient.Pipeline()
		serverGroupSetPipeline := redisServerClient.Pipeline()
		for idx := 0; idx < len(groupPipelineCmds); idx += 2 {
			gamename, _ := groupPipelineCmds[idx].Result()
			groupidStr, _ := groupPipelineCmds[idx+1].Result()
			if len(groupidStr) == 0 || len(gamename) == 0 {
				continue
			}
			var groupKey = gamename + ":" + groupidStr
			incrPipeline.HIncrBy(context, groupKey, "numservers", 1)
			serverGroupSetPipeline.HSet(context, serverKeys[idx/2], "groupid_set", groupidStr)
		}
		_, err = incrPipeline.Exec(context)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ResyncAllGroups INCR pipeline error: %s\n", err.Error())
		}

		_, err = serverGroupSetPipeline.Exec(context)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ResyncAllGroups INCR server groupid error: %s\n", err.Error())
		}

		if cursor == 0 {
			break
		}
	}

}
