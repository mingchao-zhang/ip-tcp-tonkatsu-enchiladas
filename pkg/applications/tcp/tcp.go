package tcp

import (
	"errors"
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

// VListen is a part of the network API available to applications
// Callers do not need to lock
func VListen(port uint16) (*TcpListener, error) {
	fmt.Printf("opening a new listener: %v:%d\n", state.myIP, port)

	state.lock.Lock()
	defer state.lock.Unlock()

	_, ok := state.ports[port]
	if ok {
		return nil, errors.New("vlisten: port already in use")
	}

	// at this point we know that the port is unused
	state.ports[port] = true
	state.listeners[port] = &TcpListener{
		ip:   state.myIP,
		port: port,
		ch:   make(chan *TcpPacket),
		stop: make(chan bool),
	}

	return state.listeners[port], nil
}

func (conn *TcpConn) HandleConnection() {
	state.lock.RLock()
	sock, ok := state.sockets[*conn]
	state.lock.RUnlock()

	if !ok {
		// we should already have a socket open
		return
	}

	for {
		// print every packet that we get
		p := <-sock.ch
		fmt.Println(p)
	}
}

func (l *TcpListener) VAccept() (*TcpConn, error) {
	// make sure that the listener is still valid

	select {
	case p := <-l.ch:
		// here we need to make sure p is a SYN
		if p.header.Flags != header.TCPFlagSyn {
			errMsg := fmt.Sprintf("Packet dropped in VAccept: the TCP flag must be SYN. Flags received: %d\n", p.header.Flags)
			return nil, errors.New(errMsg)
		}
		// we only accept connections if the flag is *just* SYN
		// at this point we need to check if there is already a connection from this port and IP address?
		// Question: can we have 2 connections to the same port?
		// for now I won't check

		// spawn a thread to handle this connection?

		state.lock.Lock()

		conn := TcpConn{
			localIP:     l.ip,
			localPort:   l.port,
			foreignIP:   p.srcIP,
			foreignPort: p.header.SrcPort,
		}

		_, ok := state.sockets[conn]
		if ok {
			state.lock.Unlock()
			errMsg := fmt.Sprintf("Packet dropped in VAccept: %s: %d has already been connected\n", conn.foreignIP, conn.foreignPort)
			return nil, errors.New(errMsg)
		}

		sock, err := MakeTcpSocket(SYN_RECEIVED)
		if err != nil {
			state.lock.Unlock()
			return nil, err
		}
		state.sockets[conn] = sock

		state.lock.Unlock()

		// first we need to send a syn-ack
		tcpHdr := header.TCPFields{
			SrcPort: conn.localPort,
			DstPort: conn.foreignPort,
			// ⚠️ ⬇️⬇️⬇️ adjust these values ⬇️⬇️⬇️
			// seq num becomes a random value
			SeqNum:     sock.initSeqNum,
			AckNum:     p.header.SeqNum + 1,
			DataOffset: TcpHeaderLen,
			Flags:      header.TCPFlagSyn | header.TCPFlagAck,
			WindowSize: uint16(BufferSize),
			// we need to set the checksum
			Checksum:      0,
			UrgentPointer: 0,
		}

		synAckPacket := TcpPacket{
			header: tcpHdr,
			data:   make([]byte, 0),
		}

		packetBytes := synAckPacket.Marshal()

		state.fwdTable.Lock.RLock()
		err = state.fwdTable.SendMsgToDestIP(conn.foreignIP, TcpProtocolNum, packetBytes)
		state.fwdTable.Lock.RUnlock()
		if err != nil {
			deleteConnSafe(&conn)
			return nil, err
		}

		select {
		case p := <-sock.ch:
			if (p.header.Flags & header.TCPFlagAck) != 0 {
				// at this point we have established a connection
				// check if the appropriate number was acked
				fmt.Println("we did it :partyemoji:")
			}
			// TODO: When is data pushed into l.stop?
			// when the listener calls close(),
			// removes the listener from listeners
			// and all the active connections from the socket table
		case <-l.stop:
			deleteConnSafe(&conn)
			return nil, errors.New("connection closed")
		}

		go conn.HandleConnection()

		return &conn, nil
	case <-l.stop:
		return nil, errors.New("connection closed")
	}

}

// TODO
func (l *TcpListener) VClose() error {
	// remove the listener from list of listeners
	// remove all open sockets and send value on close channel
	// send value on listener close to stop it from waiting on new connections
	return nil
}

func VConnect(foreignIP link.IntIP, foreignPort uint16) (*TcpConn, error) {
	state.lock.Lock()
	conn := TcpConn{
		localIP:     state.myIP,
		localPort:   allocatePortUnsafe(),
		foreignIP:   foreignIP,
		foreignPort: foreignPort,
	}

	// check if the connection has already been established
	_, ok := state.sockets[conn]
	if ok {
		errMsg := fmt.Sprintf("Error in VConnect: %s: %d has already been connected.\n", conn.foreignIP, conn.foreignPort)
		state.lock.Unlock()
		return nil, errors.New(errMsg)
	}

	// create the TCP socket
	sock, err := MakeTcpSocket(SYN_SENT)
	if err != nil {
		state.lock.Unlock()
		return nil, err
	}
	state.sockets[conn] = sock
	state.lock.Unlock()

	// send SYN
	tcpHdr := header.TCPFields{
		SrcPort:    conn.localPort,
		DstPort:    conn.foreignPort,
		SeqNum:     sock.initSeqNum,
		AckNum:     0,
		DataOffset: TcpHeaderLen,
		Flags:      header.TCPFlagSyn,
		WindowSize: BufferSize,
		// To compute
		Checksum:      0,
		UrgentPointer: 0,
	}
	synPacket := TcpPacket{
		header: tcpHdr,
		data:   make([]byte, 0),
	}
	packetBytes := synPacket.Marshal()

	state.fwdTable.Lock.RLock()
	err = state.fwdTable.SendMsgToDestIP(foreignIP, TcpProtocolNum, packetBytes)
	state.fwdTable.Lock.RUnlock()
	if err != nil {
		deleteConnSafe(&conn)
		return nil, err
	}

	// wait for a SYN-ACK
	// possibly also wait on stop
	select {
	case packet := <-sock.ch:
		receivedHdr := packet.header
		// check if the appropriate number was acked

		// send ACK
		tcpHdr = header.TCPFields{
			SrcPort:    conn.localPort,
			DstPort:    conn.foreignPort,
			SeqNum:     receivedHdr.AckNum,
			AckNum:     receivedHdr.SeqNum + 1,
			DataOffset: TcpHeaderLen,
			Flags:      header.TCPFlagAck,
			WindowSize: uint16(BufferSize),
			// To compute
			Checksum:      0,
			UrgentPointer: 0,
		}
		ackPacket := TcpPacket{
			header: tcpHdr,
			data:   make([]byte, 0),
		}
		packetBytes = ackPacket.Marshal()
		state.fwdTable.Lock.RLock()
		err = state.fwdTable.SendMsgToDestIP(foreignIP, TcpProtocolNum, packetBytes)
		state.fwdTable.Lock.RUnlock()
		if err != nil {
			deleteConnSafe(&conn)
			return nil, err
		}

		// conn established
		go conn.HandleConnection()
		return &conn, nil
	case <-sock.stop:
		deleteConnSafe(&conn)
		return nil, errors.New("connection closed")
	}

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
