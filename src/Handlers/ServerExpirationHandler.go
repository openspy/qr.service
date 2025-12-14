package Handlers

import (
	"context"
	"fmt"
	"log"
	"os"
	"os-qr-service/Server"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

const SERVER_EXPIRE_SCAN_BATCH_COUNT int64 = 500

type ServerExpirationHandler struct {
	context      context.Context
	serverMgr    Server.IServerManager
	redisOptions *redis.Options
	redisClient  *redis.Client

	amqpChannel  *amqp.Channel
	resyncTicker *time.Ticker
}

func (h *ServerExpirationHandler) getResyncInterval() time.Duration {
	var interval_str = os.Getenv("SERVER_EXPIRE_SCAN_SECS")
	val, err := strconv.Atoi(interval_str)
	if err != nil {
		fmt.Fprintf(os.Stderr, "server expire GetResyncInterval env parse error: %s\n", err.Error())
		val = 60
	}
	return time.Duration(val) * time.Second
}
func (h *ServerExpirationHandler) getMinTTL() time.Duration {
	var interval_str = os.Getenv("SERVER_EXPIRE_MIN_TTL_SECS")
	val, err := strconv.Atoi(interval_str)
	if err != nil {
		fmt.Fprintf(os.Stderr, "server expire getMinTTL env parse error: %s\n", err.Error())
		val = 180
	}
	return time.Duration(val) * time.Second
}

func (h *ServerExpirationHandler) HandleNewServer(serverKey string) {

}
func (h *ServerExpirationHandler) HandleUpdateServer(serverKey string) {

}
func (h *ServerExpirationHandler) HandleDeleteServer(serverKey string) {

}
func (h *ServerExpirationHandler) SetManagers(redisOptions *redis.Options, context context.Context, serverMgr Server.IServerManager, serverGroupMgr Server.IServerGroupManager, gameMgr Server.IGameManager) {
	h.context = context
	h.serverMgr = serverMgr
	h.redisOptions = redisOptions
	h.redisOptions.DB = 0
	h.redisClient = redis.NewClient(h.redisOptions)

	h.resyncTicker = time.NewTicker(h.getResyncInterval())
}
func (h *ServerExpirationHandler) SetAMQPConnection(amqpConnection *amqp.Connection) {
	chListen, err := amqpConnection.Channel()
	if err != nil {
		log.Panicf("Failed to open channel: %s", err)
		return
	}
	h.amqpChannel = chListen
}
func (h *ServerExpirationHandler) sendServerExpireMessage(serverKey string) {
	var body = "\\del\\" + serverKey
	message := []byte(body)

	err := h.amqpChannel.PublishWithContext(h.context,
		"openspy.master", // exchange
		"server.event",   // routing key
		false,            // mandatory
		false,            // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        message,
		})
	if err != nil {
		log.Panicf("Failed to publish message: %s\n", err.Error())
		return
	}
}

func (h *ServerExpirationHandler) syncLoop() {
	var isRunning bool = true
	for {
		select {
		case <-h.resyncTicker.C:
			h.doExpirationScan()
		case <-h.context.Done():
			isRunning = false
		}
		if !isRunning {
			break
		}
	}
}

func (h *ServerExpirationHandler) doExpirationScan() {
	//do scan and pipelined TTL
	//if TTL < min ttl, append to delete list
	//then set deletes (pipelined) and send event

	var cursor int = 0
	for {
		keys, nextCursor, err := h.redisClient.Scan(h.context, uint64(cursor), "*:", SERVER_EXPIRE_SCAN_BATCH_COUNT).Result()
		if err != nil {
			fmt.Fprintf(os.Stderr, "doExpirationScan server scan error: %s\n", err.Error())
			break
		}
		cursor = int(nextCursor)

		ttlCheckPipeline := h.redisClient.Pipeline()
		var ttlCmds []*redis.DurationCmd
		var ttlGetDel []*redis.StringCmd
		for _, key := range keys {
			ttlCmds = append(ttlCmds, ttlCheckPipeline.TTL(h.context, key))
			ttlGetDel = append(ttlGetDel, ttlCheckPipeline.HGet(h.context, key, "deleted"))
		}

		_, ttlCheckErr := ttlCheckPipeline.Exec(h.context)
		if ttlCheckErr != nil {
			fmt.Fprintf(os.Stderr, "doExpirationScan ttl check error: %s\n", ttlCheckErr.Error())
			break
		}

		deletePipeline := h.redisClient.Pipeline()
		var deletedKeys []string
		for idx, key := range keys {
			if ttlCmds[idx].Val() > h.getMinTTL() {
				continue
			}
			var deleted = ttlGetDel[idx].Val()
			deletedVal, _ := strconv.Atoi(deleted)
			if deletedVal != 0 {
				continue
			}
			deletePipeline.HSet(h.context, key, "deleted", "1")
			deletedKeys = append(deletedKeys, key)
		}
		_, deleteErr := deletePipeline.Exec(h.context)
		if deleteErr != nil {
			fmt.Fprintf(os.Stderr, "doExpirationScan delete error: %s\n", deleteErr.Error())
			break
		}
		for _, key := range deletedKeys {
			h.sendServerExpireMessage(key)
		}

		if cursor == 0 {
			break
		}
	}
}
