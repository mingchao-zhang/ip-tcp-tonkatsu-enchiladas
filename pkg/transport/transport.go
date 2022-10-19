package transport

import (
	"net"
	// "github.com/google/netstack/tcpip/header"
	// "golang.org/x/net/ipv4"
)

type Conn struct {
	conn *net.UDPConn
}

func Recv(listenChan *chan []byte) {

}

func Send(msg []byte, conn net.UDPConn) {

}
