package Handlers

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os-qr-service/Server"
	"time"

	"github.com/redis/go-redis/v9"
)

const PREQUERY_IP_VERIFY_FLAG int = 128
const NUM_PROBE_RETRIES int = 3

type ProbeInfo struct {
	Address          *netip.AddrPort
	DoPrequeryVerify bool
	Retries          int
}

type ServerProber struct {
	connection *net.UDPConn
	serverMgr  Server.IServerManager
	gameMgr    Server.IGameManager

	redisOptions *redis.Options
	redisClient  *redis.Client
	context      context.Context

	resyncTicker        *time.Ticker
	newServerNotifyChan chan string

	probes map[string]ProbeInfo
}

func (h *ServerProber) HandleNewServer(serverKey string) {
	h.newServerNotifyChan <- serverKey
}
func (h *ServerProber) HandleUpdateServer(serverKey string) {

}
func (h *ServerProber) HandleDeleteServer(serverKey string) {

}
func (h *ServerProber) SetManagers(redisOptions *redis.Options, context context.Context, serverMgr Server.IServerManager, serverGroupMgr Server.IServerGroupManager, gameMgr Server.IGameManager) {
	h.serverMgr = serverMgr
	h.gameMgr = gameMgr
	h.redisOptions = redisOptions
	h.redisClient = redis.NewClient(h.redisOptions)
	h.context = context
	h.resyncTicker = time.NewTicker(5 * time.Second)
	h.newServerNotifyChan = make(chan string)
	h.probes = make(map[string]ProbeInfo)
	h.Init()
}

func (h *ServerProber) getBindAddr() *net.UDPAddr {
	var bind_str = os.Getenv("SERVER_PROBER_BIND_ADDR")
	addrport, err := netip.ParseAddrPort(bind_str)
	if err != nil {
		v, _ := netip.ParseAddrPort("0.0.0.0:16500")
		addrport = v
	}
	return net.UDPAddrFromAddrPort(addrport)
}

func (h *ServerProber) Init() {
	ser, err := net.ListenUDP("udp", h.getBindAddr())
	if err != nil {
		fmt.Fprintf(os.Stderr, "ServerProber bind failed: %s\n", err.Error())
		return
	}

	h.connection = ser

	go h.listen()
	go h.retryLoop()
}

func (h *ServerProber) Query(destination netip.AddrPort) {
	var addr = net.UDPAddrFromAddrPort(destination)
	log.Printf("QR2 Send query to: %s\n", addr.String())
	writeBuffer := make([]byte, 11)
	writeBuffer[0] = 0xfe
	writeBuffer[1] = 0xfd
	writeBuffer[7] = 0xff

	h.connection.WriteToUDP(writeBuffer, addr)
}

func (h *ServerProber) Query_PrequeryVerify(destination netip.AddrPort) {
	var addr = net.UDPAddrFromAddrPort(destination)
	log.Printf("QR2 Send query to: %s\n", addr.String())
	writeBuffer := make([]byte, 7)
	writeBuffer[0] = 0xfe
	writeBuffer[1] = 0xfd
	writeBuffer[2] = 0x09

	h.connection.WriteToUDP(writeBuffer, addr)
}

func (h *ServerProber) retryLoop() {
	var isRunning bool = true
	for {
		select {
		case serverKey := <-h.newServerNotifyChan:
			h.onNewServer(serverKey)
		case <-h.resyncTicker.C:
			h.doResend()
		case <-h.context.Done():
			isRunning = false
		}
		if !isRunning {
			break
		}
	}
}
func (h *ServerProber) doResend() {
	for serverKey, value := range h.probes {
		if value.Retries >= NUM_PROBE_RETRIES {
			delete(h.probes, serverKey)
			continue
		}
		value.Retries = value.Retries + 1
		h.probes[serverKey] = value
		h.sendProbe(value)

	}
}

func (h *ServerProber) onNewServer(serverKey string) {
	var address = h.serverMgr.GetAddress(h.context, h.redisClient, serverKey)
	if address == nil {
		return
	}

	var gameid = h.serverMgr.GetKeyInt(h.context, h.redisClient, serverKey, "gameid")
	var gamename = h.gameMgr.GetGamename(h.context, h.redisClient, gameid)
	var backendFlags = h.gameMgr.GetBackendFlags(h.context, h.redisClient, gamename)

	var probeInfo ProbeInfo
	probeInfo.Address = address

	if backendFlags&PREQUERY_IP_VERIFY_FLAG != 0 {
		probeInfo.DoPrequeryVerify = true
	}
	h.sendProbe(probeInfo)
	h.probes[serverKey] = probeInfo
}
func (h *ServerProber) sendProbe(probeInfo ProbeInfo) {
	if probeInfo.DoPrequeryVerify {
		h.Query_PrequeryVerify(*probeInfo.Address)
	} else {
		h.Query(*probeInfo.Address)
	}
}
func (h *ServerProber) listen() {
	defer h.connection.Close()

	buf := make([]byte, 1492)
	for {
		bufLen, addr, err := h.connection.ReadFrom(buf)
		if err != nil {
			log.Println("ServerProber Recvfrom failed:", err.Error())
			break
		}

		if bufLen < 6 {
			continue
		}

		var udpAddr *net.UDPAddr = addr.(*net.UDPAddr)
		var addrPort netip.AddrPort = udpAddr.AddrPort()
		var serverKey = h.serverMgr.GetServerKeyFromAddress(h.context, h.redisClient, addrPort)
		if len(serverKey) == 0 {
			continue
		}
		delete(h.probes, serverKey)
		log.Println("got the packet back", serverKey)

		h.serverMgr.SetKeys(h.context, h.redisClient, serverKey, []string{
			"allow_unsolicited_udp", "1",
			"icmp_address", udpAddr.String(),
		})
	}
}
