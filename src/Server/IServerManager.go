package Server

import (
	"context"
	"net/netip"

	"github.com/redis/go-redis/v9"
)

type IServerManager interface {
	GetAddress(context context.Context, redisClient *redis.Client, serverKey string) *netip.AddrPort
	GetKey(context context.Context, redisClient *redis.Client, serverKey string, keyName string) string
	GetKeyInt(context context.Context, redisClient *redis.Client, serverKey string, keyName string) int
	GetCustomKey(context context.Context, redisClient *redis.Client, serverKey string, keyName string) string
	GetCustomKeyInt(context context.Context, redisClient *redis.Client, serverKey string, keyName string) int
	SetKey(context context.Context, redisClient *redis.Client, serverKey string, keyName string, keyValue string)
	GetServerKeyFromAddress(context context.Context, redisClient *redis.Client, addrPort netip.AddrPort) string
	SetKeys(context context.Context, redisClient *redis.Client, serverKey string, keys []string)

	DeleteKey(context context.Context, redisClient *redis.Client, serverKey string, keyName string)
}
