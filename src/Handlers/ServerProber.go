package Handlers

/*
	Note: currently this only probes new servers, so its possible a server will be added while this service is offline and not get probed / mess up dedicated server based games
	we should add a startup check that probes all servers without allow_unsolicited_udp
*/
import (
	"context"
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

	redisOptions         *redis.Options
	redisServerMgrClient *redis.Client
	redisGameMgrClient   *redis.Client
	context              context.Context

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
	h.redisOptions.DB = 0
	h.redisServerMgrClient = redis.NewClient(h.redisOptions)

	h.redisOptions.DB = 2
	h.redisGameMgrClient = redis.NewClient(h.redisOptions)
	h.context = context
	h.resyncTicker = time.NewTicker(5 * time.Second)
	h.newServerNotifyChan = make(chan string, DEFAULT_CHANNEL_BUFFER_SIZE)
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
	ser, err := net.ListenUDP("udp4", h.getBindAddr())
	if err != nil {
		log.Printf("ServerProber bind failed: %s\n", err.Error())
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
	log.Printf("QR2 Send prequery to: %s\n", addr.String())
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
	h.connection.Close()
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
	var address = h.serverMgr.GetAddress(h.context, h.redisServerMgrClient, serverKey)
	if address == nil {
		return
	}

	var gameid = h.serverMgr.GetKeyInt(h.context, h.redisServerMgrClient, serverKey, "gameid")
	var gamename = h.gameMgr.GetGameKey(h.context, h.redisGameMgrClient, gameid)
	var backendFlags = h.gameMgr.GetBackendFlags(h.context, h.redisGameMgrClient, gamename)

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
		var serverKey = h.serverMgr.GetServerKeyFromAddress(h.context, h.redisServerMgrClient, addrPort)
		if len(serverKey) == 0 {
			log.Println("ServerProber Recvfrom server not found:", udpAddr.String())
			continue
		}
		log.Println("ServerProber Recvfrom server found:", udpAddr.String())
		delete(h.probes, serverKey)

		h.serverMgr.SetKeys(h.context, h.redisServerMgrClient, serverKey, []string{
			"allow_unsolicited_udp", "1",
			"icmp_address", udpAddr.String(),
		})
	}
}
