package transport

import (
	"fmt"
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

func (t *Transport) Send(remoteString string, msg []byte) {
	remoteAddr, err := net.ResolveUDPAddr("udp4", remoteString)
	if err != nil {
		log.Fatal("Cannot resolve udp address: ", err)
	}
	bytesWritten, err := t.Conn.WriteToUDP(msg, remoteAddr)
	if err != nil {
		log.Panicln("Error writing to socket: ", err)
	}
	fmt.Printf("%d bytes are written to the address: %s\n", bytesWritten, remoteString)
}
