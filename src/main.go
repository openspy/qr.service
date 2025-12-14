package main

import (
	"context"
	"crypto/tls"
	"log"
	"os"
	"os-qr-service/Handlers"
	"os-qr-service/Server"
	"os/signal"
	"strconv"

	"os-qr-service/Helpers"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

func GetRedisOptions() *redis.Options {
	redisOptions := &redis.Options{
		Addr: os.Getenv("REDIS_SERVER"),
	}
	redisUsername := os.Getenv("REDIS_USERNAME")
	redisPassword := os.Getenv("REDIS_PASSWORD")

	if len(redisUsername) > 0 {
		redisOptions.Username = redisUsername
	}
	if len(redisPassword) > 0 {
		redisOptions.Password = redisPassword
	}

	redisUseTLS := os.Getenv("REDIS_USE_TLS")

	if len(redisUseTLS) > 0 {
		useTLSInt, _ := strconv.Atoi(redisUseTLS)

		if useTLSInt == 1 {
			tlsConfig := &tls.Config{
				MinVersion: tls.VersionTLS12,
				//InsecureSkipVerify: true,
				//Certificates: []tls.Certificate{cert}
			}

			redisSkipSSLVerify := os.Getenv("REDIS_INSECURE_TLS")
			if len(redisSkipSSLVerify) > 0 {
				useInsecureTLS, _ := strconv.Atoi(redisSkipSSLVerify)

				if useInsecureTLS == 1 {
					tlsConfig.InsecureSkipVerify = true
				}
			}
			redisOptions.TLSConfig = tlsConfig
		}
	}

	redisOptions.DB = 0
	return redisOptions
}
func main() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		signal.Stop(signalChan)
		cancel()
	}()

	var amqpAddress string = os.Getenv("RABBITMQ_URL")

	//make listener connection, etc
	listenConn, err := amqp.Dial(amqpAddress)
	Helpers.FailOnError(err, "Failed to connect to RabbitMQ")
	defer listenConn.Close()

	var serverMgr = Server.ServerManager{}

	var serverGroupMgr = Server.ServerGroupManager{}

	var gameMgr = Server.GameManager{}

	var serverEventListener *ServerEventListener = &ServerEventListener{}
	serverEventListener.Init(ctx, listenConn, &serverMgr, &serverGroupMgr, &gameMgr, GetRedisOptions())

	serverEventListener.RegisterHandler(&Handlers.ServerProber{})
	serverEventListener.RegisterHandler(&Handlers.CountryCodeAssigner{})
	serverEventListener.RegisterHandler(&Handlers.ServerGroupUpdater{})

	var expireHandler = &Handlers.ServerExpirationHandler{}
	expireHandler.SetAMQPConnection(listenConn)
	serverEventListener.RegisterHandler(expireHandler)

	var clientMsgForwarder = &Handlers.ClientMessageForwarder{}
	clientMsgForwarder.SetAMQPConnection(listenConn)
	serverEventListener.RegisterHandler(clientMsgForwarder)

	var isRunning bool = true
	for {
		select {
		case <-signalChan:
			cancel()
		case <-ctx.Done():
			log.Println("Shutdown event", ctx.Err())
			isRunning = false
		}
		if !isRunning {
			break
		}
	}

}
