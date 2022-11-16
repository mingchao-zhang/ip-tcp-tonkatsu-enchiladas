package tcp

import (
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"
	"sync"
	"time"

	"github.com/google/netstack/tcpip/header"
)

const (
	// Add all the TCP states here?
	READ_WRITE_SLEEP_TIME = time.Millisecond * 10
)

type TcpState struct {
	sockets   map[TcpConn]*TcpSocket
	listeners map[uint16]*TcpListener
	myIP      link.IntIP
	// ports is a set of all the ports for listening purposes
	// We will use this to get a random port for connection
	ports          map[uint16]bool
	fwdTable       *network.FwdTable
	nextUnusedPort uint16
	lock           sync.RWMutex
}

type TcpPacket struct {
	srcIP  link.IntIP
	header header.TCPFields
	data   []byte
}

type TcpListener struct {
	ip   link.IntIP
	port uint16
	ch   chan *TcpPacket
	stop chan bool
}

type TcpConn struct {
	localIP     link.IntIP
	localPort   uint16
	foreignIP   link.IntIP
	foreignPort uint16
}

func (conn TcpConn) String() string {
	res := "\n"
	res += fmt.Sprintf("localIP: %s\n", conn.localIP)
	res += fmt.Sprintf("localPort: %d\n", conn.localPort)
	res += fmt.Sprintf("foreignIP: %s\n", conn.foreignIP)
	res += fmt.Sprintf("foreignIP: %d\n", conn.foreignPort)
	return res
}
