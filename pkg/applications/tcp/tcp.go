package tcp

import (
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"

	"github.com/google/netstack/tcpip/header"
	"golang.org/x/net/ipv4"
)

const (
	TcpProtocolNum = uint8(header.TCPProtocolNumber)
	TcpHeaderLen   = header.TCPMinimumSize
	BufferSize     = 1<<16 - 1

	SYN_RECEIVED = 0
	SYN_SENT     = 1
	ESTABLISHED  = 2
	FIN_WAIT_1   = 3
	FIN_WAIT_2   = 4
	CLOSE_WAIT   = 5
	LAST_ACK     = 6
)

var state *TcpState

func TcpHandler(rawMsg []byte, params []interface{}) {
	ipHdr := params[0].(*ipv4.Header)
	srcIP := link.IntIPFromNetIP(ipHdr.Src)
	tcpPacket := UnmarshalTcpPacket(rawMsg, srcIP)

	tcpConn := TcpConn{
		localIP:     link.IntIPFromNetIP(ipHdr.Dst),
		localPort:   tcpPacket.header.DstPort,
		foreignIP:   srcIP,
		foreignPort: tcpPacket.header.SrcPort,
	}

	if tcpConn.localIP != state.myIP {
		// we will never make connections from any IP other than myIP
		fmt.Printf("Packet dropped in TcpHandler: %s is not the IP for this node\n", ipHdr.Dst)
		return
	}

	socket, ok := state.sockets[tcpConn]
	if !ok {
		// check if there is a server listening on that port
		listener, ok := state.listeners[tcpConn.localPort]
		if !ok {
			fmt.Printf("Packet dropped in TcpHandler: port %d is not a listen port.\n", tcpConn.localPort)
			return
		}
		listener.ch <- tcpPacket
	} else {
		socket.ch <- tcpPacket
	}
}

func (sock *TcpSocket) HandlePacket(p *TcpPacket) {
	// makes sense of the packet
	// if the packet is an ack, then we have to update our write buffer

	// how do we know if it is an ack or a data packet?

	// if p.header.Flags & header.TCPFlagAck != 0 {
	// 	// we got an ACK

	// } else {
	// 	// this might be a packet with some data in it

	// }

	// if we get a packet we check if it is the next packet we were expecting to get
	// if it is not then we add it to the out of order queue
	// else if it was the next then we update the value of the next packet we expect to see
	// 		and send an ack with that next value

	// if

	// if sequence number of the packet is the same as what we expect
	// we will add it to the read buffer and update the next field
	absSeqNum := p.header.SeqNum - sock.foreignInitSeqNum
	if absSeqNum == sock.nextExpectedByte.Load() {

	}

}

func (conn *TcpConn) HandleConnection() {
	state.lock.RLock()
	sock, ok := state.sockets[*conn]
	state.lock.RUnlock()

	// packetsSeen := make(map[uint32]string)

	if !ok {
		// we should already have a socket open
		return
	}

	for {
		p := <-sock.ch

		go sock.HandlePacket(p)
	}

	// have a thread waiting for data in the write buffer,
	// when it sees data, we have to send it to the person,
	// we're connected to

}

// TODO
func (l *TcpListener) VClose() error {
	// remove the listener from list of listeners
	// remove all open sockets and send value on close channel
	// send value on listener close to stop it from waiting on new connections
	return nil
}

func TCPInit(fwdTable *network.FwdTable) {
	fwdTable.RegisterHandlerSafe(TcpProtocolNum, TcpHandler)

	state = &TcpState{
		sockets:        make(map[TcpConn]*TcpSocket),
		listeners:      make(map[uint16]*TcpListener),
		ports:          make(map[uint16]bool),
		fwdTable:       fwdTable,
		nextUnusedPort: 2000,
		myIP:           link.GetSmallestLocalIP(fwdTable.IpInterfaces),
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

func UnmarshalTcpPacket(rawMsg []byte, srcIP link.IntIP) *TcpPacket {
	tcpHeader := ParseTCPHeader(rawMsg)
	tcpPacket := TcpPacket{
		header: tcpHeader,
		data:   rawMsg[tcpHeader.DataOffset:],
		srcIP:  srcIP,
	}

	return &tcpPacket
}

func (p *TcpPacket) Marshal() []byte {
	tcphdr := make(header.TCP, TcpHeaderLen)
	tcphdr.Encode(&p.header)

	// return header appended to data
	return append([]byte(tcphdr), p.data...)
}

// allocatePort unsafely (without locking) returns an unused port and modify state
func allocatePortUnsafe() uint16 {

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

func deleteConnSafe(conn *TcpConn) {
	state.lock.Lock()
	delete(state.sockets, *conn)
	state.lock.Unlock()
}
