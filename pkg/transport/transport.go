package transport

import (
	"log"
	"net"
)

const (
	MAXMSGSIZE = 1400
)

func Recv(conn net.UDPConn, listenChan *chan []byte) {
	for {
		buffer := make([]byte, MAXMSGSIZE)

		_, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Panicln("Error reading from the UPD socket: ", err)
		}
		*listenChan <- buffer
	}
}

func Send(conn net.UDPConn, remoteAddr string, msg []byte) {

}
