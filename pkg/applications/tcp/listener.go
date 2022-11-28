package tcp

import (
	"errors"
	"fmt"
	link "ip/pkg/ipinterface"
	"time"

	"github.com/google/netstack/tcpip/header"
)

type TcpListener struct {
	socketId int
	ip       link.IntIP
	port     uint16
	ch       chan *TcpPacket
	stop     chan bool
}

// VListen is a part of the network API available to applications
// Callers do not need to lock
func VListen(port uint16) (*TcpListener, error) {

	state.lock.Lock()
	defer state.lock.Unlock()

	_, ok := state.ports[port]
	if ok {
		return nil, errors.New("vlisten: port already in use")
	}

	// at this point we know that the port is unused
	state.ports[port] = true
	state.listeners[port] = &TcpListener{
		socketId: int(nextSockId.Add(1)),
		ip:       state.myIP,
		port:     port,
		ch:       make(chan *TcpPacket),
		stop:     make(chan bool),
	}

	return state.listeners[port], nil
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
		} else if !isValidTcpCheckSum(&p.header, conn.foreignIP.NetIP(), conn.localIP.NetIP(), p.data) {
			state.lock.Unlock()
			return nil, errors.New("invalid checksum in Vaccept")
		}

		sock, err := MakeTcpSocket(SYN_RECEIVED, &conn, p.header.SeqNum)
		if err != nil {
			state.lock.Unlock()
			return nil, err
		}
		state.sockets[conn] = sock

		state.lock.Unlock()

		// first we need to send a syn-ack
		tcpHdr := header.TCPFields{
			SrcPort:    conn.localPort,
			DstPort:    conn.foreignPort,
			SeqNum:     sock.myInitSeqNum,
			AckNum:     p.header.SeqNum + 1,
			DataOffset: TcpHeaderLen,
			Flags:      header.TCPFlagSyn | header.TCPFlagAck,
			WindowSize: uint16(BufferSize),
			// we need to set the checksum
			Checksum:      0,
			UrgentPointer: 0,
		}
		sock.nextExpectedByte.Store(1)

		payload := make([]byte, 0)
		tcpHdr.Checksum = computeTCPChecksum(&tcpHdr, conn.localIP.NetIP(), conn.foreignIP.NetIP(), payload)
		synAckPacket := TcpPacket{
			header: tcpHdr,
			data:   payload,
		}

		packetBytes := synAckPacket.Marshal()
		err = sendTcp(conn.foreignIP, packetBytes)
		if err != nil {
			deleteConnSafe(&conn)
			return nil, err
		}
		sock.connState = SYN_SENT

		timeout := time.After(time.Second * 2)
		select {
		case p := <-sock.ch:
			if (p.header.Flags & header.TCPFlagAck) != 0 {
				// we got an ack from the client
				if p.header.SeqNum-sock.foreignInitSeqNum != 1 {
					fmt.Println("Received unexpected sequence number")
					deleteConnSafe(&conn)
					return nil, errors.New("unexpected sequence number received")
				}
				if p.header.AckNum-sock.myInitSeqNum != 1 {
					fmt.Println("Received unexpected ack number")
					deleteConnSafe(&conn)
					return nil, errors.New("unexpected ack number received")
				}

				// at this point we have established a connection
				// check if the appropriate number was acked
				sock.connState = ESTABLISHED
				sock.foreignWindowSize.Store(uint32(p.header.WindowSize))
			}

			// TODO: When is data pushed into l.stop?
			// when the listener calls close(),
			// removes the listener from listeners
			// and all the active connections from the socket table
		case <-timeout:
			fmt.Println("timed out")
			deleteConnSafe(&conn)
			return nil, errors.New("connection timed out")
		case <-l.stop:
			deleteConnSafe(&conn)
			return nil, errors.New("connection closed")
		}
		go sock.HandleConnection()

		return &conn, nil
	case <-l.stop:
		return nil, errors.New("connection closed")
	}

}

// TODO
func (l *TcpListener) VCloseListener() error {
	// remove the listener from list of listeners
	// remove all open sockets and send value on close channel
	// send value on listener close to stop it from waiting on new connections

	// call vclose on all of the listeners' open sockets
	return nil
}
