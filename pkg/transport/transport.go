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
	conn       *net.UDPConn
	listenChan *chan []byte
}

func (t *Transport) Init(udpPort string, listenChan *chan []byte) {
	// resolve udp4 address
	listenString := fmt.Sprintf(":%s", udpPort)
	listenAddr, err := net.ResolveUDPAddr("udp4", listenString)
	if err != nil {
		log.Fatal("Error resolving udp address: ", err)
	}
	// create connections
	t.conn, err = net.ListenUDP("udp4", listenAddr)
	if err != nil {
		log.Fatal("Cannot create the udp connection: ", err)
	}

	t.listenChan = listenChan
}

func (t *Transport) Close() {
	t.conn.Close()
}

func (t *Transport) SendToLocalHost(buffer []byte) {
	*t.listenChan <- buffer
}

func (t *Transport) Recv() {
	for {
		buffer := make([]byte, MAXMSGSIZE)

		_, _, err := t.conn.ReadFromUDP(buffer)
		if err != nil {
			log.Panicln("Error reading from the UPD socket: ", err)
		}
		*t.listenChan <- buffer
	}
}

func (t *Transport) Send(remoteString string, msg []byte) {
	remoteAddr, err := net.ResolveUDPAddr("udp4", remoteString)
	if err != nil {
		log.Fatal("Cannot resolve udp address: ", err)
	}
	_, err = t.conn.WriteToUDP(msg, remoteAddr)
	if err != nil {
		fmt.Println("transport.send: ", msg)
		log.Panicln("Error writing to socket: ", err)
	}
}
