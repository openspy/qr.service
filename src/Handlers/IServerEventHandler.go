package Handlers

import (
	"context"
	"os-qr-service/Server"

	"github.com/redis/go-redis/v9"
)

type IServerEventHandler interface {
	HandleNewServer(serverKey string)
	HandleUpdateServer(serverKey string)
	HandleDeleteServer(serverKey string)
	SetManagers(redisOptions *redis.Options, context context.Context, serverMgr Server.IServerManager, serverGroupMgr Server.IServerGroupManager, gameMgr Server.IGameManager)
}
