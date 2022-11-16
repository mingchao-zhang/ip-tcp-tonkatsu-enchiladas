package tcp

import (
	"errors"
	"fmt"
	link "ip/pkg/ipinterface"

	"github.com/google/netstack/tcpip/header"
)

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
	sock, err := MakeTcpSocket(SYN_SENT, 0)
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
		SeqNum:     sock.myInitSeqNum,
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
		if (receivedHdr.Flags&header.TCPFlagSyn == 0) || (receivedHdr.Flags&header.TCPFlagAck == 0) {
			deleteConnSafe(&conn)
			return nil, errors.New("received packet with wrong flags during handshake")
		}
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
		go sock.HandleConnection()
		return &conn, nil
	case <-sock.stop:
		deleteConnSafe(&conn)
		return nil, errors.New("connection closed")
	}

}
