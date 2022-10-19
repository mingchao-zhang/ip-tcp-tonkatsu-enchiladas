package transport

import (
	"fmt"
	"log"
	"net"

	"golang.org/x/net/ipv4"
)

const (
	MAXMSGSIZE = 1400
)

func Recv(conn net.UDPConn, listenChan *chan []byte) {
	for {
		buffer := make([]byte, MAXMSGSIZE)

		_, sourceAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Panicln("Error reading from the UPD socket: ", err)
		}

		hdr, err := ipv4.ParseHeader(buffer)
		if err != nil {
			fmt.Println("Error parsing the ip header: ", err)
		}

		headerSize := hdr.Len
		msg := buffer[headerSize:]
		//!!! VERIFY CHECKSUM

		fmt.Printf("Received IP packetfrom %s\nHeader:  %v\nMessage:  %s\n",
			sourceAddr.String(), hdr, string(msg))

		*listenChan <- msg
	}
}

func Send(conn net.UDPConn, remoteAddr string, msg []byte) {

}
