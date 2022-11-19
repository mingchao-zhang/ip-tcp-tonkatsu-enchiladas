package tcp

import (
	"errors"
	"fmt"
	link "ip/pkg/ipinterface"
	"time"

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
	sock, err := MakeTcpSocket(SYN_SENT, &conn, 0)
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

	err = sendTcp(foreignIP, packetBytes)
	if err != nil {
		deleteConnSafe(&conn)
		return nil, err
	}

	// wait for a SYN-ACK
	// possibly also wait on stop
	timeout := time.After(time.Second * 2)
	select {
	case packet := <-sock.ch:
		receivedHdr := packet.header
		// check if the appropriate number was acked
		if (receivedHdr.Flags&header.TCPFlagSyn == 0) || (receivedHdr.Flags&header.TCPFlagAck == 0) {
			deleteConnSafe(&conn)
			return nil, errors.New("connect did not receive both SYN and ACK during handshake")
		} else if packet.header.AckNum-sock.myInitSeqNum != 1 {
			deleteConnSafe(&conn)
			return nil, errors.New("in connect: get incorrect ack number in the syn-ack packet")
		}

		sock.foreignInitSeqNum = receivedHdr.SeqNum
		sock.nextExpectedByte.Store(1)

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
		err = sendTcp(foreignIP, packetBytes)
		if err != nil {
			deleteConnSafe(&conn)
			return nil, err
		}

		// conn established
		sock.connState = ESTABLISHED
		sock.foreignWindowSize.Swap(uint32(packet.header.WindowSize))
		fmt.Printf("In Connect: %s\n", sock)
		go sock.HandleConnection()
		return &conn, nil

	case <-timeout:
		fmt.Println("timed out")
		deleteConnSafe(&conn)
		return nil, errors.New("connection timed out")
	case <-sock.stop:
		deleteConnSafe(&conn)
		return nil, errors.New("connection closed")
	}

}
