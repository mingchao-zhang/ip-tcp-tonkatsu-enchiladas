package tcp

import (
	"errors"
	"fmt"

	"github.com/google/netstack/tcpip/header"
)

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

		sock, err := MakeTcpSocket(SYN_RECEIVED, p.header.SeqNum)
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
