package tcp

import (
	"container/heap"
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

	myInitSeqNum      uint32         // raw
	foreignInitSeqNum uint32         // raw
	numBytesSent      *atomic.Uint32 // rel
	nextExpectedByte  *atomic.Uint32 // rel

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

	// var totalBytesWritten = 0
	// writeBuffer := sock.writeBuffer

	// if !writeBuffer.IsEmpty() {

	// }
}

// for totalBytesWritten < len(buff) {
// 	// we wait until there are more bytes to read
// 	if readBuffer.IsEmpty() {
// 		time.Sleep(READ_WRITE_SLEEP_TIME)
// 	} else {
// 		bytesRead, err := readBuffer.Read(buff[totalBytesWritten:])
// 		if err != nil {
// 			log.Println("error in HandleWrites: ", err)
// 		}
// 		totalBytesWritten += bytesRead
// 	}
// }

// if totalBytesWritten != len(buff) {
// 	log.Fatalln("VRead read too many bytes 💀")
// }

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
