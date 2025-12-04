package Handlers

import "net"

type ISBClientMessageHandler interface {
	OnClientMessage(toAddress net.Addr)
	OnClientMessageAck()
}
