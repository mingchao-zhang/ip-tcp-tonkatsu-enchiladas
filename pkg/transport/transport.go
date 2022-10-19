package transport

import (
	"log"
	"net"
)

const (
	MAXMSGSIZE = 1400
)

type Transport struct {
	Conn *net.UDPConn
}

func (t *Transport) Recv(listenChan *chan []byte) {
	for {
		buffer := make([]byte, MAXMSGSIZE)

		_, _, err := t.Conn.ReadFromUDP(buffer)
		if err != nil {
			log.Panicln("Error reading from the UPD socket: ", err)
		}
		*listenChan <- buffer
	}
}

func (t *Transport) Send(remoteAddr string, msg []byte) {

}
