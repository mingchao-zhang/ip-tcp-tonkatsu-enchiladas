package tcp

import (
	link "ip/pkg/ipinterface"
	"ip/pkg/network"
	"sync"

	"github.com/armon/circbuf"
	"github.com/google/netstack/tcpip/header"
)

type TcpState struct {
	sockets    map[TcpConn]TcpSocket
	listeners  map[uint16]bool
	nextSocket uint32
	myIP       link.IntIP
	// ports is a set of all the ports for listening purposes
	// We will use this to get a random port for connection
	ports          map[uint16]bool
	fwdTable       *network.FwdTable
	nextUnusedPort uint16
	lock           sync.RWMutex
}

type TcpPacket struct {
	header header.TCPFields
	data   []byte
}

type TcpConn struct {
	localIP     link.IntIP
	localPort   uint16
	foreignIP   link.IntIP
	foreignPort uint16
}

type TcpListener struct {
	ip   link.IntIP
	port uint16
	ch   chan *TcpPacket
}

type TcpSocket struct {
	readBuffer  circbuf.Buffer
	writeBuffer circbuf.Buffer
}
