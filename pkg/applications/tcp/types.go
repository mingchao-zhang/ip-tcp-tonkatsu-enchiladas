package tcp

import (
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"
	"ip/pkg/transport"
	"sync"
	"time"

	"github.com/google/netstack/tcpip/header"
	"go.uber.org/atomic"
)

const (
	READ_WRITE_SLEEP_TIME = time.Second * 2
	// READ_WRITE_SLEEP_TIME = time.Millisecond * 10
	TcpProtocolNum    = uint8(header.TCPProtocolNumber)
	TcpHeaderLen      = header.TCPMinimumSize
	BufferSize        = 1<<16 - 1
	TcpMaxSegmentSize = transport.MAXMSGSIZE - TcpHeaderLen - header.IPv4MinimumSize

	Alpha = 0.85
	Beta  = 1.65

	RTOMin = 50 * time.Microsecond
	RTOMax = 200 * time.Microsecond

	SYN_RECEIVED = "SYN_RECEIVED"
	SYN_SENT     = "SYN_SENT"
	ESTABLISHED  = "ESTAB"
	FIN_WAIT_1   = "FIN_WAIT_1"
	FIN_WAIT_2   = "FIN_WAIT_2"
	CLOSE_WAIT   = "CLOSE_WAIT"
	LAST_ACK     = "LAST_ACK"

	SHUTDOWN_READ  = 1
	SHUTDOWN_WRITE = 2
)

var nextSockId = atomic.NewInt32(-1)

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

// An Item is something we manage in a Priority queue.
type TcpPacketItem struct {
	Value    *TcpPacket // The Value of the item; arbitrary.
	Priority int        // The relative seq number of the packet
	// The Index is needed by update and is maintained by the heap.Interface methods.
	Index         int // The Index of the item in the heap.
	TimeSent      time.Time
	Retransmitted bool
}

func (conn TcpConn) String() string {
	res := "\n"
	res += fmt.Sprintf("localIP: %s\n", conn.localIP)
	res += fmt.Sprintf("localPort: %d\n", conn.localPort)
	res += fmt.Sprintf("foreignIP: %s\n", conn.foreignIP)
	res += fmt.Sprintf("foreignIP: %d\n", conn.foreignPort)
	return res
}
