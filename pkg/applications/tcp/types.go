package tcp

import (
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"
	"sync"

	"github.com/google/netstack/tcpip/header"
	"github.com/smallnest/ringbuffer"
)

const (
// Add all the TCP states here?
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
	stop chan bool
}

type TcpSocket struct {
	readBuffer  *ringbuffer.RingBuffer
	writeBuffer *ringbuffer.RingBuffer
	ch          chan *TcpPacket
	stop        chan bool
	state       byte
	initSeqNum  uint32
}

func MakeTcpSocket(state byte) (*TcpSocket, error) {
	// replace with random later
	initSeqNum := 0

	return &TcpSocket{
		readBuffer:  ringbuffer.New(BufferSize),
		writeBuffer: ringbuffer.New(BufferSize),
		ch:          make(chan *TcpPacket),
		stop:        make(chan bool),
		state:       state,
		initSeqNum:  uint32(initSeqNum),
	}, nil
}

func (conn TcpConn) String() string {
	res := "\n"
	res += fmt.Sprintf("localIP: %s\n", conn.localIP)
	res += fmt.Sprintf("localPort: %d\n", conn.localPort)
	res += fmt.Sprintf("foreignIP: %s\n", conn.foreignIP)
	res += fmt.Sprintf("foreignIP: %d\n", conn.foreignPort)
	return res
}
