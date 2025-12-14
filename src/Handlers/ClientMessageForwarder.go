package Handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os-qr-service/Helpers"
	"os-qr-service/Server"
	"strconv"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

const (
	QR_MESSAGE_ROUTE_KEY            string = "qr.message"
	QR_CLIENT_MESSAGE_ACK_ROUTE_KEY        = "client-messages.acks"
)

type ClientMessageForwarder struct {
	context        context.Context
	redisOptions   *redis.Options
	redisClient    *redis.Client
	amqpChannel    *amqp.Channel
	amqpQueue      amqp.Queue
	resendTicker   *time.Ticker
	serverManager  Server.IServerManager
	serverGroupMgr Server.IServerGroupManager
	gameManager    Server.IGameManager
}

type QRMessage struct {
	Hostname      string `json:"hostname"`
	DriverAddress string `json:"driver_address"`
	ToAddress     string `json:"to_address"`
	Message       string `json:"message"`
	Version       int    `json:"version"`
	InstanceKey   int    `json:"instance_key"`
	Identifier    int    `json:"identifier"`
	Type          string `json:"type"`
}

func (h *ClientMessageForwarder) SetManagers(redisOptions *redis.Options, context context.Context, serverMgr Server.IServerManager, serverGroupMgr Server.IServerGroupManager, gameMgr Server.IGameManager) {
	h.redisOptions = redisOptions
	h.redisClient = redis.NewClient(h.redisOptions)
	h.context = context
	h.serverManager = serverMgr
}

func (h *ClientMessageForwarder) HandleNewServer(serverKey string) {
}
func (h *ClientMessageForwarder) HandleUpdateServer(serverKey string) {

}
func (h *ClientMessageForwarder) HandleDeleteServer(serverKey string) {

}

func (h *ClientMessageForwarder) SetAMQPConnection(amqpConnection *amqp.Connection) {
	chListen, err := amqpConnection.Channel()
	if err != nil {
		Helpers.FailOnError(err, "Failed to open client message forwarder channel")
	}
	h.amqpChannel = chListen

	q, err := h.amqpChannel.QueueDeclare(
		"qr-service-clientmsg", // name
		false,                  // durable
		true,                   // delete when unused
		true,                   // exclusive
		false,                  // no-wait
		nil,                    // arguments
	)
	Helpers.FailOnError(err, "Failed to declare client message forwarder queue")
	h.amqpChannel.QueueBind(q.Name, QR_MESSAGE_ROUTE_KEY, "openspy.master", false, nil)
	h.amqpChannel.QueueBind(q.Name, QR_CLIENT_MESSAGE_ACK_ROUTE_KEY, "openspy.master", false, nil)
	h.amqpQueue = q
	h.resendTicker = time.NewTicker(1 * time.Second)

	go h.listenLoop()
}

func (h *ClientMessageForwarder) listenLoop() {
	msgs, err := h.amqpChannel.Consume(
		h.amqpQueue.Name, // queue
		"",               // consumer
		true,             // auto-ack
		true,             // exclusive
		false,            // no-local
		false,            // no-wait
		nil,              // args
	)
	Helpers.FailOnError(err, "Failed to consume client message forwarder queue")
	for {
		select {
		case d := <-msgs:
			if d.RoutingKey == QR_MESSAGE_ROUTE_KEY {
				h.OnGotClientMessageFromServerBrowsing(d)
			} else if d.RoutingKey == QR_CLIENT_MESSAGE_ACK_ROUTE_KEY {
				h.OnClientMessageAck(d)
			}
		case <-h.resendTicker.C:
			h.submitResends()
		}
	}

}
func (h *ClientMessageForwarder) OnGotClientMessageFromServerBrowsing(amqpMsg amqp.Delivery) {
	//forward to QR
	var msg = string(amqpMsg.Body)

	var splitMsg = strings.Split(msg, "\\")
	log.Println(splitMsg)

	var toIp = splitMsg[5]
	var toPort = splitMsg[6]
	var message = splitMsg[7]

	addrPort, err := netip.ParseAddrPort(toIp + ":" + toPort)
	if err != nil || len(err.Error()) > 0 {
		fmt.Fprintf(os.Stderr, "OnGotClientMessageFromServerBrowsing ip parse error: %s\n", err.Error())
		return
	}
	h.ForwardMessageToQR(addrPort, message)
}

func (h *ClientMessageForwarder) ForwardMessageToQR(addrPort netip.AddrPort, base64str string) {
	var serverKey = h.serverManager.GetServerKeyFromAddress(h.context, h.redisClient, addrPort)
	if len(serverKey) == 0 {
		return
	}
	var results = h.serverManager.GetKeys(h.context, h.redisClient, serverKey, "instance_key", "driver_hostname", "driver_address")

	instanceKey, parseError := strconv.Atoi(results[0])
	if parseError != nil || len(parseError.Error()) > 0 {
		fmt.Fprintf(os.Stderr, "ForwardMessageToQR instance key parse error: %s\n", parseError.Error())
		return
	}

	var qrMsg QRMessage
	qrMsg.Message = base64str
	qrMsg.Version = 2
	qrMsg.InstanceKey = instanceKey
	qrMsg.ToAddress = addrPort.String()
	qrMsg.Hostname = results[1]
	qrMsg.DriverAddress = results[2]
	qrMsg.Identifier = 111 //need to generate random value, this is the lookup key for client msg ack
	qrMsg.Type = "client_message"

	jsonData, jsonErr := json.Marshal(qrMsg)
	if jsonErr != nil || len(jsonErr.Error()) > 0 {
		fmt.Fprintf(os.Stderr, "ForwardMessageToQR json marshal error: %s\n", parseError.Error())
		return
	}

	err := h.amqpChannel.PublishWithContext(h.context,
		"openspy.master", // exchange
		"qr.message",     // routing key
		false,            // mandatory
		false,            // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        jsonData,
		})
	if err != nil {
		log.Panicf("Failed to publish message: %s\n", err.Error())
		return
	}
}
func (h *ClientMessageForwarder) OnClientMessageAck(amqpMsg amqp.Delivery) {
	//clear retry logic
}

func (h *ClientMessageForwarder) submitResends() {

}
