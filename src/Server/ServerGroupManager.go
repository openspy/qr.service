package Server

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

const GROUP_SCAN_BATCH_COUNT int64 = 250

type ServerGroupManager struct {
}

func (m *ServerGroupManager) IncrNumServers(context context.Context, redisClient *redis.Client, groupKey string) {
	redisClient.HIncrBy(context, groupKey+"custkeys", "numservers", 1)
}
func (m *ServerGroupManager) DecrNumServers(context context.Context, redisClient *redis.Client, groupKey string) {
	redisClient.HIncrBy(context, groupKey+"custkeys", "numservers", -1)
}
func (m *ServerGroupManager) ResyncAllGroups(context context.Context, redisServerClient *redis.Client, redisGroupClient *redis.Client) {

	var cursor int = 0

	//do pipelined scan and clear
	for {
		deletePipeline := redisGroupClient.Pipeline()
		keys, nextCursor, err := redisGroupClient.Scan(context, uint64(cursor), "*:", GROUP_SCAN_BATCH_COUNT).Result()
		if err != nil {
			log.Printf("ResyncAllGroups scan error: %s\n", err.Error())
			break
		}
		cursor = int(nextCursor)

		for _, key := range keys {
			var custkey = key + "custkeys"
			deletePipeline.HSet(context, custkey, "numservers", "0")
		}
		_, err = deletePipeline.Exec(context)
		if err != nil {
			log.Printf("ResyncAllGroups pipeline error: %s\n", err.Error())
		}

		if cursor == 0 {
			break
		}
	}

	//do pipelined scan of all servers
	for {
		keys, nextCursor, err := redisServerClient.Scan(context, uint64(cursor), "*:", GROUP_SCAN_BATCH_COUNT).Result()
		if err != nil {
			log.Printf("ResyncAllGroups game scan error: %s\n", err.Error())
			break
		}

		cursor = int(nextCursor)
		var groupPipelineCmds []*redis.StringCmd
		var serverKeys []string
		gameGroupPipeline := redisServerClient.Pipeline()
		for _, key := range keys {
			groupPipelineCmds = append(groupPipelineCmds, gameGroupPipeline.HGet(context, key+"custkeys", "gamename")) //XXX: remove cust keys later (it can be incorrect via custkeys as some games modify it)
			groupPipelineCmds = append(groupPipelineCmds, gameGroupPipeline.HGet(context, key+"custkeys", "groupid"))
			groupPipelineCmds = append(groupPipelineCmds, gameGroupPipeline.HGet(context, key, "deleted"))
			serverKeys = append(serverKeys, key)
		}
		_, err = gameGroupPipeline.Exec(context)
		if err != nil {
			log.Printf("ResyncAllGroups pipeline error: %s\n", err.Error())
		}

		//now incr groupids for all servers which have them set
		incrPipeline := redisGroupClient.Pipeline()
		serverGroupSetPipeline := redisServerClient.Pipeline()
		var idx int = 0
		for _, serverKey := range serverKeys {
			gamename, err := groupPipelineCmds[idx].Result()
			if err != nil && len(err.Error()) > 0 {
				log.Printf("ResyncAllGroups pipeline error: %s\n", err.Error())
			}
			groupidStr, err := groupPipelineCmds[idx+1].Result()
			if err != nil && len(err.Error()) > 0 {
				log.Printf("ResyncAllGroups pipeline error: %s\n", err.Error())
			}
			deletedStr, err := groupPipelineCmds[idx+2].Result()
			if err != nil && len(err.Error()) > 0 {
				log.Printf("ResyncAllGroups pipeline error: %s\n", err.Error())
			}
			idx = idx + 3
			if len(groupidStr) == 0 || len(gamename) == 0 || deletedStr != "0" {
				continue
			}
			var groupKey = gamename + ":" + groupidStr + ":custkeys"
			incrPipeline.HIncrBy(context, groupKey, "numservers", 1)
			serverGroupSetPipeline.HSet(context, serverKey, "groupid_set", groupidStr)
		}
		_, err = incrPipeline.Exec(context)
		if err != nil {
			log.Printf("ResyncAllGroups INCR pipeline error: %s\n", err.Error())
		}

		_, err = serverGroupSetPipeline.Exec(context)
		if err != nil {
			log.Printf("ResyncAllGroups INCR server groupid error: %s\n", err.Error())
		}

		if cursor == 0 {
			break
		}
	}

}
