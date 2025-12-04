package Handlers

import (
	"context"
	"fmt"
	"os"
	"os-qr-service/Server"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type ServerGroupUpdater struct {
	serverMgr Server.IServerManager
	groupMgr  Server.IServerGroupManager

	context context.Context

	resyncTicker        *time.Ticker
	newServerNotifyChan chan string
	delServerNotifyChan chan string

	redisOptions *redis.Options
	redisClient  *redis.Client
}

func (h *ServerGroupUpdater) HandleNewServer(serverKey string) {
	h.newServerNotifyChan <- serverKey
}
func (h *ServerGroupUpdater) HandleUpdateServer(serverKey string) {
	h.newServerNotifyChan <- serverKey //bad idea? but we have an exit early check so maybe its fine?
}
func (h *ServerGroupUpdater) HandleDeleteServer(serverKey string) {
	h.delServerNotifyChan <- serverKey
}
func (h *ServerGroupUpdater) getResyncInterval() time.Duration {
	var interval_str = os.Getenv("GROUP_UPDATE_RESYNC_SECS")
	val, err := strconv.Atoi(interval_str)
	if err != nil {
		fmt.Fprintf(os.Stderr, "server group GetResyncInterval env parse error: %s\n", err.Error())
		val = 300
	}
	return time.Duration(val) * time.Second
}
func (h *ServerGroupUpdater) SetManagers(redisOptions *redis.Options, context context.Context, serverMgr Server.IServerManager, serverGroupMgr Server.IServerGroupManager, gameMgr Server.IGameManager) {
	h.context = context
	h.serverMgr = serverMgr
	h.groupMgr = serverGroupMgr

	h.newServerNotifyChan = make(chan string, DEFAULT_CHANNEL_BUFFER_SIZE)
	h.delServerNotifyChan = make(chan string, DEFAULT_CHANNEL_BUFFER_SIZE)

	h.resyncTicker = time.NewTicker(h.getResyncInterval())

	h.redisOptions = redisOptions
	h.redisClient = redis.NewClient(h.redisOptions)

	h.groupMgr.ResyncAllGroups(h.context, h.redisClient)

	go h.syncLoop()
}

/*
Currently because of this resync ticker, this project can only run a single instance...

if we ever need to scale this project, then the syncer should exist as a seperate process which somehow notifies this one to pause
for now it just works by having it on the same thread, so the resync should block the other channel messages from being processed in the mean time
*/
func (h *ServerGroupUpdater) syncLoop() {
	var isRunning bool = true
	for {
		select {
		case serverKey := <-h.newServerNotifyChan:
			h.incrServerGroupid(serverKey)
		case serverKey := <-h.delServerNotifyChan:
			h.decrServerGroupid(serverKey)
		case <-h.resyncTicker.C:
			h.groupMgr.ResyncAllGroups(h.context, h.redisClient)
		case <-h.context.Done():
			isRunning = false
		}
		if !isRunning {
			break
		}
	}
}
func (h *ServerGroupUpdater) incrServerGroupid(serverKey string) {
	if h.serverMgr.GetKeyInt(h.context, h.redisClient, serverKey, "groupid_set") != 0 {
		return
	}

	var groupid = h.serverMgr.GetCustomKeyInt(h.context, h.redisClient, serverKey, "groupid")
	if groupid == 0 {
		return
	}
	var gameid = h.serverMgr.GetKeyInt(h.context, h.redisClient, serverKey, "gameid")

	h.serverMgr.SetKey(h.context, h.redisClient, serverKey, "groupid_set", string(groupid))

	var groupkey = h.groupMgr.GetServerKey(h.context, h.redisClient, gameid, groupid)
	h.groupMgr.IncrNumServers(h.context, h.redisClient, groupkey)
}

func (h *ServerGroupUpdater) decrServerGroupid(serverKey string) {
	if h.serverMgr.GetKeyInt(h.context, h.redisClient, serverKey, "groupid_set") == 0 {
		return
	}
	var groupid = h.serverMgr.GetKeyInt(h.context, h.redisClient, serverKey, "groupid")
	var gameid = h.serverMgr.GetKeyInt(h.context, h.redisClient, serverKey, "gameid")

	var groupkey = h.groupMgr.GetServerKey(h.context, h.redisClient, gameid, groupid)
	h.groupMgr.DecrNumServers(h.context, h.redisClient, groupkey)
}
