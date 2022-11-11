package tcp

import (
	"errors"
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"
	"net"

	"github.com/google/netstack/tcpip/header"
	"golang.org/x/net/ipv4"
)

const (
	TcpProtocolNum = uint8(header.TCPProtocolNumber)
	TcpHeaderLen   = header.TCPMinimumSize
)

var state *TcpState

func TcpHandler(rawMsg []byte, params []interface{}) {
	ipHdr := params[0].(*ipv4.Header)
	tcpPacket := UnmarshalTcpPacket(rawMsg)
	fmt.Println(tcpPacket)

	tcpConn := TcpConn{
		localIP:     link.IntIPFromNetIP(ipHdr.Dst),
		foreignIP:   link.IntIPFromNetIP(ipHdr.Src),
		localPort:   tcpPacket.header.DstPort,
		foreignPort: tcpPacket.header.SrcPort,
	}

	socket, ok := state.sockets[tcpConn]
	fmt.Println(ok, socket)

	// srcIP := link.IntIPFromNetIP(hdr.Src)
}

// VListen is a part of the network API available to applications
// Callers do not need to lock
func VListen(port uint16) (*TcpListener, error) {
	state.lock.Lock()
	defer state.lock.Unlock()

	_, ok := state.ports[port]
	if ok {
		return nil, errors.New("port in use")
	}

	// at this point we know that the port is unused
	state.ports[port] = true
	state.listeners[port] = true

	return &TcpListener{
		ip:   state.myIP,
		port: port,
		ch:   make(chan *TcpPacket),
	}, nil
}

func (l *TcpListener) VAccept() {
	for {
		select {
		case p := <-l.ch:
			// here we need to make sure p is a SYN
			if p.header.Flags&header.TCPFlagSyn != 0 {

			}
		}
	}
}

// TODO
func (l *TcpListener) VClose() error {
	return nil
}

/*
 * Creates a new socket and connects to an
 * address:port (active OPEN in the RFC).
 * Returns a VTCPConn on success or non-nil error on
 * failure.
 * VConnect MUST block until the connection is
 * established, or an error occurs.
 */
func VConnect(addr net.IP, port int16) (TcpConn, error) {
	// conn := TcpConn {
	// 	localIP     link.IntIP
	// 	localPort   uint16
	// 	foreignIP   link.IntIP
	// 	foreignPort uint16
	// }
}

func TCPInit(fwdTable *network.FwdTable) {
	fwdTable.RegisterHandlerSafe(TcpProtocolNum, TcpHandler)
	// TODO: initialize myIP
	state = &TcpState{
		sockets:        make(map[TcpConn]TcpSocket),
		listeners:      make(map[uint16]bool),
		nextSocket:     0,
		ports:          make(map[uint16]bool),
		fwdTable:       fwdTable,
		nextUnusedPort: 1000,
		myIP: fwdTable.IpInterfaces[]
	}
}

// FINISHED FUNCTIONS ----------------------------------------------------------
func ParseTCPHeader(b []byte) header.TCPFields {
	td := header.TCP(b)
	return header.TCPFields{
		SrcPort:    td.SourcePort(),
		DstPort:    td.DestinationPort(),
		SeqNum:     td.SequenceNumber(),
		AckNum:     td.AckNumber(),
		DataOffset: td.DataOffset(),
		Flags:      td.Flags(),
		WindowSize: td.WindowSize(),
		Checksum:   td.Checksum(),
	}
}

func UnmarshalTcpPacket(rawMsg []byte) *TcpPacket {
	tcpHeader := ParseTCPHeader(rawMsg)
	tcpPacket := TcpPacket{
		header: tcpHeader,
		data:   rawMsg[tcpHeader.DataOffset:],
	}

	return &tcpPacket
}

func (p *TcpPacket) MarshalTCPPacket() []byte {
	tcphdr := make(header.TCP, TcpHeaderLen)
	tcphdr.Encode(&p.header)

	// return header appended to data
	return append([]byte(tcphdr), p.data...)
}

// allocatePort returns an unused port and modify state
func allocatePort() uint16 {
	state.lock.Lock()
	defer state.lock.Unlock()

	// find the port we could use
	_, inMap := state.ports[state.nextUnusedPort]
	for inMap {
		state.nextUnusedPort += 1
		_, inMap = state.ports[state.nextUnusedPort]
	}

	res := state.nextUnusedPort
	state.nextUnusedPort += 1
	return res
}
