package tcp

import (
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"
	"log"
	"math/rand"
	"time"

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
	// what could go wrong if we have multiple packets being handled at the same time?
	relSeqNum := p.header.SeqNum - sock.foreignInitSeqNum

	if relSeqNum == sock.nextExpectedByte.Load() {
		if sock.readBuffer.Free() >= len(p.data) {
			// write the data to the buffer if there is enough space available
			sock.readBuffer.Write(p.data)
			sock.nextExpectedByte.Add(uint32(len(p.data)))

			// TODO: check if any early arrivals can be added to the read buffer, and
		} else {
			return
			// this ideally should not happen
			// drop the packet
		}
	} else {
		// add the packet to the heap of packets
		log.Println("HandlePacket: Packet arrived out of order: ", p)
		log.Printf("Expect sequence number: %v; Received: %v", sock.nextExpectedByte, relSeqNum)

		// sock.outOfOrderQueue.Push(p)
	}

	conn := sock.conn

	ackHdr := header.TCPFields{
		SrcPort:    conn.localPort,
		DstPort:    conn.foreignPort,
		SeqNum:     sock.myInitSeqNum + sock.numBytesSent.Load(),
		AckNum:     sock.nextExpectedByte.Load() + sock.foreignInitSeqNum,
		DataOffset: TcpHeaderLen,
		Flags:      header.TCPFlagAck, // what flag should we set?
		WindowSize: uint16(sock.readBuffer.Free()),
		// To compute
		Checksum:      0,
		UrgentPointer: 0,
	}

	ackPacket := TcpPacket{
		header: ackHdr,
		data:   []byte{},
	}

	state.fwdTable.Lock.RLock()
	state.fwdTable.SendMsgToDestIP(
		sock.conn.foreignIP,
		TcpProtocolNum,
		ackPacket.Marshal(),
	)
	state.fwdTable.Lock.RUnlock()

}

func (sock *TcpSocket) HandleWrites() {

}

func (sock *TcpSocket) HandleConnection() {
	t := time.NewTicker(READ_WRITE_SLEEP_TIME)
	for {
		select {
		case p := <-sock.ch:
			go sock.HandlePacket(p)
		case <-t.C:
			go sock.HandleWrites()
		}

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

	rand.Seed(time.Now().UnixMicro())

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
