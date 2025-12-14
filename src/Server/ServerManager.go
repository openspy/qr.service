package Server

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type ServerManager struct {
}

func (m *ServerManager) selectRedisDB(context context.Context, redisClient *redis.Client) {
	redisClient.Conn().Select(context, 0)
}

func (m *ServerManager) GetAddress(context context.Context, redisClient *redis.Client, serverKey string) *netip.AddrPort {
	m.selectRedisDB(context, redisClient)
	results, err := redisClient.HMGet(context, serverKey, "wan_ip", "wan_port").Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetAddress error: %s\n", err.Error())
		return nil
	}
	var wanip = results[0].(string)
	var wanport_str = results[1].(string)

	var addrPort netip.AddrPort
	addrPort, addrErr := netip.ParseAddrPort(wanip + ":" + wanport_str)
	if addrErr != nil {
		fmt.Fprintf(os.Stderr, "GetAddress parse error: %s\n", addrErr.Error())
		return nil
	}
	return &addrPort
}
func (m *ServerManager) GetKey(context context.Context, redisClient *redis.Client, serverKey string, keyName string) string {
	m.selectRedisDB(context, redisClient)
	results, err := redisClient.HGet(context, serverKey, keyName).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetKey error: %s\n", err.Error())
		return ""
	}
	return results
}
func (m *ServerManager) GetKeys(context context.Context, redisClient *redis.Client, serverKey string, keyNames ...string) []string {
	m.selectRedisDB(context, redisClient)
	results, err := redisClient.HMGet(context, serverKey, keyNames...).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetKeys error: %s\n", err.Error())
		return nil
	}

	var strings []string
	for _, str := range results {
		var convStr = str.(string)
		strings = append(strings, convStr)
	}
	return strings
}
func (m *ServerManager) GetKeyInt(context context.Context, redisClient *redis.Client, serverKey string, keyName string) int {
	var key = m.GetKey(context, redisClient, serverKey, keyName)
	intVal, err := strconv.Atoi(key)
	if err != nil {
		//fmt.Fprintf(os.Stderr, "GetKeyInt parse error: %s\n", err.Error())
		return 0
	}
	return intVal
}
func (m *ServerManager) GetCustomKey(context context.Context, redisClient *redis.Client, serverKey string, keyName string) string {
	m.selectRedisDB(context, redisClient)
	var custkey = serverKey + "custkeys"
	results, err := redisClient.HGet(context, custkey, keyName).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetCustomKey error: %s\n", err.Error())
		return ""
	}
	return results
}
func (m *ServerManager) GetCustomKeyInt(context context.Context, redisClient *redis.Client, serverKey string, keyName string) int {
	m.selectRedisDB(context, redisClient)
	var custkey = serverKey + "custkeys"
	var key = m.GetKey(context, redisClient, custkey, keyName)
	intVal, err := strconv.Atoi(key)
	if err != nil {
		//fmt.Fprintf(os.Stderr, "GetCustomKeyInt parse error: %s\n", err.Error())
		return 0
	}
	return intVal
}
func (m *ServerManager) SetKey(context context.Context, redisClient *redis.Client, serverKey string, keyName string, keyValue string) {
	m.selectRedisDB(context, redisClient)
	_, err := redisClient.HSet(context, serverKey, keyName, keyValue).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SetKey error: %s\n", err.Error())
	}
}

func (m *ServerManager) SetKeys(context context.Context, redisClient *redis.Client, serverKey string, keys []string) {
	m.selectRedisDB(context, redisClient)
	_, err := redisClient.HSet(context, serverKey, keys).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SetKeys error: %s\n", err.Error())
	}
}

func (m *ServerManager) GetServerKeyFromAddress(context context.Context, redisClient *redis.Client, addrPort netip.AddrPort) string {
	m.selectRedisDB(context, redisClient)
	portStr := strconv.FormatUint(uint64(addrPort.Port()), 10)
	//ipString := addrPort.Addr().String()
	var key = "IPMAP_" + addrPort.Addr().String() + "-" + portStr
	result, err := redisClient.Get(context, key).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetServerKeyFromAddress error: %s\n", err.Error())
		return ""
	}
	return result
}

func (m *ServerManager) DeleteKey(context context.Context, redisClient *redis.Client, serverKey string, keyName string) {
	m.selectRedisDB(context, redisClient)
	redisClient.HDel(context, serverKey, keyName)
}
