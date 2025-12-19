package Handlers

import (
	"context"
	"log"
	"os"
	"os-qr-service/Server"

	"github.com/oschwald/geoip2-golang/v2"
	"github.com/redis/go-redis/v9"
)

type CountryCodeAssigner struct {
	serverMgr Server.IServerManager
	geoip     *geoip2.Reader

	context             context.Context
	redisOptions        *redis.Options
	redisClient         *redis.Client
	newServerNotifyChan chan string
}

func (h *CountryCodeAssigner) ResolveCountryCode(serverKey string) string {
	var addr = h.serverMgr.GetAddress(h.context, h.redisClient, serverKey)
	if addr == nil {
		return ""
	}
	record, err := h.geoip.Country(addr.Addr())
	if err != nil {
		log.Fatal(err)
		return ""
	}
	if !record.HasData() {
		return ""
	}
	return record.Country.ISOCode
}
func (h *CountryCodeAssigner) handleNewServerEvent(serverKey string) {
	var countryCode = h.ResolveCountryCode(serverKey)
	if len(countryCode) == 0 {
		return
	}
	h.serverMgr.SetKey(h.context, h.redisClient, serverKey, "country", countryCode)
	log.Printf("CountryCodeAssigner new server: %s - %s - %s\n",
		h.serverMgr.GetAddress(h.context, h.redisClient, serverKey).Addr().String(),
		h.serverMgr.GetCustomKey(h.context, h.redisClient, serverKey, "hostname"),
		h.serverMgr.GetKey(h.context, h.redisClient, serverKey, "challenge"))
}
func (h *CountryCodeAssigner) HandleNewServer(serverKey string) {
	h.newServerNotifyChan <- serverKey
}
func (h *CountryCodeAssigner) HandleUpdateServer(serverKey string) {

}
func (h *CountryCodeAssigner) HandleDeleteServer(serverKey string) {

}

func (h *CountryCodeAssigner) syncLoop() {
	var isRunning bool = true
	for {
		select {
		case serverKey := <-h.newServerNotifyChan:
			h.handleNewServerEvent(serverKey)
		case <-h.context.Done():
			isRunning = false
		}
		if !isRunning {
			break
		}
	}
}
func (h *CountryCodeAssigner) SetManagers(redisOptions *redis.Options, context context.Context, serverMgr Server.IServerManager, serverGroupMgr Server.IServerGroupManager, gameMgr Server.IGameManager) {
	h.redisOptions = redisOptions
	h.redisOptions.DB = 0
	h.redisClient = redis.NewClient(h.redisOptions)
	h.context = context
	h.newServerNotifyChan = make(chan string, DEFAULT_CHANNEL_BUFFER_SIZE)

	h.serverMgr = serverMgr

	var dbPath string = os.Getenv("GEOIP_DB_PATH")

	db, err := geoip2.Open(dbPath)

	if err != nil {
		return
	}
	h.geoip = db

	go h.syncLoop()
}
