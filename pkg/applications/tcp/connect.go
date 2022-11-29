package tcp

import (
	"errors"
	"fmt"
	link "ip/pkg/ipinterface"
	"log"
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

	payload := make([]byte, 0)
	tcpHdr.Checksum = computeTCPChecksum(&tcpHdr, conn.localIP.NetIP(), conn.foreignIP.NetIP(), payload)
	synPacket := TcpPacket{
		header: tcpHdr,
		data:   payload,
	}
	packetBytes := synPacket.Marshal()

	// wait for a SYN-ACK
	// possibly also wait on stop

	err = sendTcp(foreignIP, packetBytes)
	if err != nil {
		deleteConnSafe(&conn)
		log.Fatalln("Unable to send packet in connect() (some problem with IP)")
		return nil, err
	}

	timeoutSynAck := time.After(time.Millisecond * 5)
	select {
	case packet := <-sock.ch:
		receivedHdr := packet.header
		// check if the appropriate number was acked
		if receivedHdr.Flags != header.TCPFlagAck|header.TCPFlagSyn {
			deleteConnSafe(&conn)
			return nil, errors.New("did not receive SYN-ACK during handshake")
		} else if !isValidTcpCheckSum(&packet.header, conn.foreignIP.NetIP(), conn.localIP.NetIP(), packet.data) {
			deleteConnSafe(&conn)
			return nil, errors.New("incorrect checksum in syn-ack packet")
		} else if packet.header.AckNum-sock.myInitSeqNum != 1 {
			deleteConnSafe(&conn)
			return nil, errors.New("incorrect ack number in the SYN-ACK packet")
		}

		sock.foreignInitSeqNum = receivedHdr.SeqNum
		sock.nextExpectedByte.Store(1)
		sock.largestAckReceived.Store(packet.header.AckNum)

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
		payload := make([]byte, 0)
		tcpHdr.Checksum = computeTCPChecksum(&tcpHdr, conn.localIP.NetIP(), conn.foreignIP.NetIP(), payload)
		ackPacket := TcpPacket{
			header: tcpHdr,
			data:   payload,
		}
		// what if this ack is dropped???
		packetBytes = ackPacket.Marshal()
		// fmt.Println("resending ACK")
		err := sendTcp(foreignIP, packetBytes)
		if err != nil {
			deleteConnSafe(&conn)
			log.Fatalln("Unable to send packet in connect() (some problem with IP)")
		}

		// conn established
		sock.connState = ESTABLISHED
		sock.foreignWindowSize.Store(uint32(packet.header.WindowSize))
		go sock.HandleConnection()
		return &conn, nil

	case <-timeoutSynAck:
		fmt.Println("handshake timed out")
		deleteConnSafe(&conn)
		return nil, errors.New("connection timed out")

	case <-sock.stop:
		deleteConnSafe(&conn)
		return nil, errors.New("connection closed")
	}
	return nil, errors.New("impossible case")
}
