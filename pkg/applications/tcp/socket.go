package tcp

import (
	"container/heap"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/google/netstack/tcpip/header"
	"github.com/smallnest/ringbuffer"
	"go.uber.org/atomic"
)

type TcpSocket struct {
	sockId    int
	connState string
	conn      *TcpConn

	readBuffer  *ringbuffer.RingBuffer
	writeBuffer *ringbuffer.RingBuffer

	ch   chan *TcpPacket
	stop chan bool

	// my numbers
	myInitSeqNum     uint32         // raw; const
	numBytesSent     *atomic.Uint32 // rel
	nextExpectedByte *atomic.Uint32 // rel

	// foreign numbers
	foreignInitSeqNum  uint32         // raw
	largestAckReceived *atomic.Uint32 // raw
	foreignWindowSize  *atomic.Uint32

	outOfOrderQueue heap.Interface
}

func MakeTcpSocket(connState string, tcpConn *TcpConn, foreignInitSeqNum uint32) (*TcpSocket, error) {
	return &TcpSocket{
		sockId: int(nextSockId.Add(1)),

		connState: connState,
		conn:      tcpConn,

		readBuffer:  ringbuffer.New(BufferSize),
		writeBuffer: ringbuffer.New(BufferSize),

		ch:   make(chan *TcpPacket),
		stop: make(chan bool),

		myInitSeqNum:      rand.Uint32(),
		foreignInitSeqNum: foreignInitSeqNum,
		numBytesSent:      atomic.NewUint32(0),
		nextExpectedByte:  atomic.NewUint32(0),
	}, nil
}

func (sock *TcpSocket) HandlePacket(p *TcpPacket) {
	// what could go wrong if we have multiple packets being handled at the same time?
	// 1. try to write data either in the read buffer or in the heap
	relSeqNum := p.header.SeqNum - sock.foreignInitSeqNum

	if relSeqNum == sock.nextExpectedByte.Load() {
		if sock.readBuffer.Free() >= len(p.data) {
			// write the data to the buffer if there is enough space available
			sock.readBuffer.Write(p.data)
			sock.nextExpectedByte.Add(uint32(len(p.data)))

			// TODO: check if any early arrivals can be added to the read buffer, and
		} else {
			// this ideally should not happen
			// drop the packet
			fmt.Println("HandlePacket window size not respected")
			return
		}
	} else {
		// add the packet to the heap of packets
		log.Println("HandlePacket: Packet arrived out of order: ", p)
		log.Printf("Expect sequence number: %v; Received: %v", sock.nextExpectedByte, relSeqNum)

		// sock.outOfOrderQueue.Push(p)
	}

	// 2. modify largestAckReceived and foreignWindowSize
	packetWindowSize := uint32(p.header.WindowSize)
	if p.header.AckNum > sock.largestAckReceived.Load() {
		sock.largestAckReceived.Swap(p.header.AckNum)
		sock.foreignWindowSize.Swap(packetWindowSize)
	} else if p.header.AckNum > sock.largestAckReceived.Load() {
		if sock.foreignWindowSize.Load() < packetWindowSize {
			sock.foreignWindowSize.Swap(packetWindowSize)
		}
	} else {
		fmt.Println("Old invalid packets")
	}

	// 3. send an ack back
	// increase nextExpectedByte before constructing the header
	ackPacket := TcpPacket{
		header: *sock.getAckHeader(),
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
	writeBuffer := sock.writeBuffer
	if writeBuffer.IsEmpty() {
		return
	}

	// calculate how many bytes to send in total
	sizeToWrite := uint32(min(int(sock.foreignWindowSize.Load()), writeBuffer.Length()))

	// get all the bytes to send
	payload := make([]byte, sizeToWrite)
	writeBuffer.Read(payload)

	// split bytes into segments, construct tcp packets and send them
	conn := sock.conn
	for sizeToWrite > 0 {
		segmentSize := uint32(min(TcpMaxSegmentSize, int(sizeToWrite)))

		// get hdr
		// increase numBytesSent after constructing the header
		ackHdr := sock.getAckHeader()
		sock.numBytesSent.Add(segmentSize)

		// add payload
		ackPacket := TcpPacket{
			header: *ackHdr,
			data:   payload[:segmentSize],
		}
		payload = payload[segmentSize:]

		packetBytes := ackPacket.Marshal()
		state.fwdTable.Lock.RLock()
		err := state.fwdTable.SendMsgToDestIP(conn.foreignIP, TcpProtocolNum, packetBytes)
		state.fwdTable.Lock.RUnlock()
		if err != nil {
			fmt.Println("Error in handleWrites from SendMsgToDestIP: ", err)
			return
		}

		sizeToWrite -= segmentSize
	}
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

func (sock *TcpSocket) getAckHeader() *header.TCPFields {
	return &header.TCPFields{
		SrcPort:    sock.conn.localPort,
		DstPort:    sock.conn.foreignPort,
		SeqNum:     sock.myInitSeqNum + sock.numBytesSent.Load(),
		AckNum:     sock.nextExpectedByte.Load() + sock.foreignInitSeqNum,
		DataOffset: TcpHeaderLen,
		Flags:      header.TCPFlagAck, // what flag should we set?
		WindowSize: uint16(sock.readBuffer.Free()),
		// To compute
		Checksum:      0,
		UrgentPointer: 0,
	}
}
