package tcp

import (
	"errors"
	"fmt"
	"time"

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

		synAckPacket := TcpPacket{
			header: tcpHdr,
			data:   make([]byte, 0),
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
				sock.nextExpectedByte.Store(1)
				// at this point we have established a connection
				// check if the appropriate number was acked
				sock.connState = ESTABLISHED
				fmt.Println("we did it :partyemoji:")
				sock.foreignWindowSize.Store(uint32(p.header.WindowSize))
				fmt.Printf("In Accept %s\n", sock)
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
		fmt.Println("here")
		go sock.HandleConnection()

		return &conn, nil
	case <-l.stop:
		return nil, errors.New("connection closed")
	}

}
