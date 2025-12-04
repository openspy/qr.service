package main

import (
	"context"
	"log"
	"os-qr-service/Handlers"
	"os-qr-service/Server"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

type ServerEventListener struct {
	amqpCtx        context.Context
	amqpConnection *amqp.Connection
	amqpChannel    *amqp.Channel
	isRunning      bool
	amqpDelivery   <-chan amqp.Delivery
	redisOptions   *redis.Options
	handlers       []Handlers.IServerEventHandler
	serverManager  Server.IServerManager
	serverGroupMgr Server.IServerGroupManager
	gameManager    Server.IGameManager
}

func (h *ServerEventListener) Init(ctx context.Context, connection *amqp.Connection, serverManager Server.IServerManager, serverGroupMgr Server.IServerGroupManager, gameManager Server.IGameManager, redisOpts *redis.Options) {
	h.amqpCtx = ctx
	h.amqpConnection = connection
	h.isRunning = true

	h.redisOptions = redisOpts

	chListen, err := h.amqpConnection.Channel()
	failOnError(err, "Failed to open a channel")
	h.amqpChannel = chListen

	q, err := h.amqpChannel.QueueDeclare(
		"qr-service", // name
		false,        // durable
		true,         // delete when unused
		true,         // exclusive
		false,        // no-wait
		nil,          // arguments
	)
	failOnError(err, "Failed to declare a queue")

	h.amqpChannel.QueueBind(q.Name, "server.event", "openspy.master", false, nil)

	msgs, err := h.amqpChannel.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		true,   // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Failed to register a consumer")
	h.amqpDelivery = msgs
	h.serverManager = serverManager
	h.serverGroupMgr = serverGroupMgr
	h.gameManager = gameManager

	go EventListenerFunc(h)
}

func EventListenerFunc(h *ServerEventListener) {
	for d := range h.amqpDelivery {
		var msg = string(d.Body)

		var splitMsg = strings.Split(msg, "\\")
		log.Println(splitMsg)

		for _, handler := range h.handlers {
			switch splitMsg[1] {
			case "new":
				handler.HandleNewServer(splitMsg[2])
			case "del":
				handler.HandleDeleteServer(splitMsg[2])
			case "update":
				handler.HandleUpdateServer(splitMsg[2])
			}
		}

	}
}

func (h *ServerEventListener) RegisterHandler(handler Handlers.IServerEventHandler) {
	handler.SetManagers(h.redisOptions, h.amqpCtx, h.serverManager, h.serverGroupMgr, h.gameManager)
	h.handlers = append(h.handlers, handler)
}
