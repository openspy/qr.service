package Handlers

import (
	"context"
	"encoding/json"
	"log"
	"math/rand/v2"
	"net/netip"
	"os-qr-service/Helpers"
	"os-qr-service/Server"
	"strconv"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

const (
	QR_MESSAGE_ROUTE_KEY            string = "client.message"
	QR_CLIENT_MESSAGE_ACK_ROUTE_KEY        = "client-messages.acks"
)

const NUM_CLIENT_MESSAGE_RETRIES int = 3

type QRMessage struct {
	Hostname      string `json:"hostname"`
	DriverAddress string `json:"driver_address"`
	ToAddress     string `json:"to_address"`
	FromAddress   string `json:"from_address"` //this is inverted in the ack..
	Message       string `json:"message"`
	Version       int    `json:"version"`
	InstanceKey   int    `json:"instance_key"`
	Identifier    int32  `json:"identifier"`
	Type          string `json:"type"`
	Retries       int
	LastResend    time.Time
}

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

	forwardedMessages map[string]QRMessage
}

func (h *ClientMessageForwarder) SetManagers(redisOptions *redis.Options, context context.Context, serverMgr Server.IServerManager, serverGroupMgr Server.IServerGroupManager, gameMgr Server.IGameManager) {
	h.redisOptions = redisOptions
	h.redisOptions.DB = 0
	h.redisClient = redis.NewClient(h.redisOptions)
	h.context = context
	h.serverManager = serverMgr
	h.forwardedMessages = make(map[string]QRMessage)
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
	if err != nil && len(err.Error()) > 0 {
		log.Printf("OnGotClientMessageFromServerBrowsing ip parse error: %s\n", err.Error())
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
	if parseError != nil && len(parseError.Error()) > 0 {
		log.Printf("ForwardMessageToQR instance key parse error: %s\n", parseError.Error())
		return
	}

	var qrMsg QRMessage
	qrMsg.Message = base64str
	qrMsg.Version = 2
	qrMsg.InstanceKey = instanceKey
	qrMsg.ToAddress = addrPort.String()
	qrMsg.Hostname = results[1]
	qrMsg.DriverAddress = results[2]
	qrMsg.Identifier = int32(rand.Int())
	qrMsg.Type = "client_message"
	qrMsg.Retries = 0
	qrMsg.LastResend = time.Now()

	log.Printf("ForwardMessageToQR: %s - %d\n", qrMsg.ToAddress, qrMsg.InstanceKey)

	var qrMsgKey = h.getClientMessageUniqueKey(qrMsg)
	h.forwardedMessages[qrMsgKey] = qrMsg
	h.publishQRMessage(qrMsg)

}
func (h *ClientMessageForwarder) publishQRMessage(msg QRMessage) {
	jsonData, jsonErr := json.Marshal(msg)
	if jsonErr != nil && len(jsonErr.Error()) > 0 {
		log.Printf("publishQRMessage json marshal error: %s\n", jsonErr.Error())
		return
	}

	log.Printf("publishQRMessage: %s - %d\n", msg.ToAddress, msg.InstanceKey)

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
	var qrMsg QRMessage
	jsonErr := json.Unmarshal(amqpMsg.Body, &qrMsg)
	if jsonErr != nil && len(jsonErr.Error()) > 0 {
		log.Printf("OnClientMessageAck json marshal error: %s\n", jsonErr.Error())
		return
	}

	qrMsg.ToAddress = qrMsg.FromAddress //handle inversion from qr server

	log.Printf("OnClientMessageAck: %s - %d\n", qrMsg.ToAddress, qrMsg.InstanceKey)

	var qrMsgKey = h.getClientMessageUniqueKey(qrMsg)
	delete(h.forwardedMessages, qrMsgKey)
}

func (h *ClientMessageForwarder) submitResends() {
	for msgKey, value := range h.forwardedMessages {
		if value.Retries >= NUM_PROBE_RETRIES {
			delete(h.forwardedMessages, msgKey)
			continue
		}
		if time.Since(value.LastResend) < time.Duration(2*time.Second) {
			continue
		}
		value.LastResend = time.Now()
		value.Retries = value.Retries + 1
		h.forwardedMessages[msgKey] = value
		h.publishQRMessage(value)

	}
}

func (h *ClientMessageForwarder) getClientMessageUniqueKey(msg QRMessage) string {
	return msg.Hostname + ":" + msg.DriverAddress + ":" + msg.ToAddress + ":" + strconv.Itoa(msg.InstanceKey) + ":" + strconv.Itoa(int(msg.Identifier))
}
