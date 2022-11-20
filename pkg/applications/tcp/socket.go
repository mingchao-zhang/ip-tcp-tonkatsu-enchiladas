package tcp

import (
	"container/heap"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/google/netstack/tcpip/header"
	"github.com/smallnest/ringbuffer"
	"go.uber.org/atomic"
)

type TcpSocket struct {
	sockId    int
	connState string
	conn      *TcpConn

	readBuffer            *ringbuffer.RingBuffer
	readBufferLock        *sync.Mutex
	readBufferIsNotEmpty  *sync.Cond
	earlyArrivalQueue     PriorityQueue
	earlyArrivalQueueLock *sync.Mutex
	earlyArrivePacketSize *atomic.Uint32

	writeBuffer           *ringbuffer.RingBuffer
	writeBufferLock       *sync.Mutex
	writeBufferIsNotFull  *sync.Cond
	writeBufferIsNotEmpty *sync.Cond
	inFlightQueue         PriorityQueue
	inFlightQueueLock     *sync.Mutex
	inFlightPacketSize    *atomic.Uint32

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
}

func MakeTcpSocket(connState string, tcpConn *TcpConn, foreignInitSeqNum uint32) (*TcpSocket, error) {
	sock := TcpSocket{
		sockId: int(nextSockId.Add(1)),

		connState: connState,
		conn:      tcpConn,

		readBuffer:            ringbuffer.New(BufferSize),
		readBufferLock:        &sync.Mutex{},
		earlyArrivalQueue:     PriorityQueue{},
		earlyArrivalQueueLock: &sync.Mutex{},
		earlyArrivePacketSize: atomic.NewUint32(0),

		writeBuffer:        ringbuffer.New(BufferSize),
		writeBufferLock:    &sync.Mutex{},
		inFlightQueue:      PriorityQueue{},
		inFlightQueueLock:  &sync.Mutex{},
		inFlightPacketSize: atomic.NewUint32(0),

		ch:   make(chan *TcpPacket),
		stop: make(chan bool),

		myInitSeqNum:     rand.Uint32(),
		numBytesSent:     atomic.NewUint32(0),
		nextExpectedByte: atomic.NewUint32(0),

		// foreign numbers
		foreignInitSeqNum:  foreignInitSeqNum,
		largestAckReceived: atomic.NewUint32(0),
		foreignWindowSize:  atomic.NewUint32(0),
	}

	sock.readBufferIsNotEmpty = sync.NewCond(sock.readBufferLock)
	sock.writeBufferIsNotFull = sync.NewCond(sock.writeBufferLock)
	sock.writeBufferIsNotEmpty = sync.NewCond(sock.writeBufferLock)
	heap.Init(&sock.earlyArrivalQueue)
	heap.Init(&sock.inFlightQueue)

	return &sock, nil
}

func (sock *TcpSocket) writeIntoReadBuffer(p *TcpPacket) error {
	sock.readBufferLock.Lock()
	bytesWritten, err := sock.readBuffer.Write(p.data)
	if err != nil {
		fmt.Println("HandlePacket: Error while writing to the read buffer", err)
		sock.readBufferLock.Unlock()
		return err
	} else {
		if bytesWritten != len(p.data) {
			fmt.Println("HandlePacket: Could not write everything to the read buffer - This should not be happening!!!")
			sock.readBufferLock.Unlock()
			return errors.New("we should have enough space in the buffer!!!")
		}
		sock.readBufferIsNotEmpty.Broadcast()
		sock.readBufferLock.Unlock()

		sock.nextExpectedByte.Add(uint32(len(p.data)))
	}
	return nil
}

func (sock *TcpSocket) HandlePacket(p *TcpPacket) {
	// what could go wrong if we have multiple packets being handled at the same time?
	// 1. modify largestAckReceived and foreignWindowSize
	packetWindowSize := uint32(p.header.WindowSize)
	if p.header.AckNum > sock.largestAckReceived.Load() {
		sock.largestAckReceived.Store(p.header.AckNum)
		sock.foreignWindowSize.Store(packetWindowSize)
	} else if p.header.AckNum == sock.largestAckReceived.Load() {
		if sock.foreignWindowSize.Load() < packetWindowSize {
			sock.foreignWindowSize.Store(packetWindowSize)
		}
	} else {
		fmt.Println("In HandlePacket: Old ack received")
		return
	}

	// 2. check if we need to write to buffer
	relSeqNum := p.header.SeqNum - sock.foreignInitSeqNum

	if relSeqNum < sock.nextExpectedByte.Load() {
		fmt.Printf("In HandlePacket: relSeqNum: %d, sock.nextExpectedByte: %d\n", relSeqNum, sock.nextExpectedByte.Load())
		return
	} else if len(p.data) == 0 {
		return
	}

	// 3. try to write data either in the read buffer or in the heap
	if relSeqNum == sock.nextExpectedByte.Load() {
		if sock.readBuffer.Free() >= len(p.data) {
			// write the data to the buffer if there is enough space available
			sock.writeIntoReadBuffer(p)
			// tell waiting readers that it is party time

			// TODO: check if any early arrivals can be added to the read buffer, and
			// lock before manipulating early arrivals
			sock.earlyArrivalQueueLock.Lock()
			for sock.earlyArrivalQueue.Len() != 0 && sock.earlyArrivalQueue[0].Priority == int(sock.nextExpectedByte.Load()) {
				smallest := sock.earlyArrivalQueue[0].Value
				sock.earlyArrivalQueue.Pop()
				sock.writeIntoReadBuffer(smallest)
			}
			sock.earlyArrivalQueueLock.Unlock()
		} else {
			// this ideally should not happen
			// drop the packet
			fmt.Println("HandlePacket window size not respected")
			return
		}
	} else { // early arrivals
		// add the packet to the heap of packets
		log.Println("HandlePacket: Packet arrived out of order: ", p)
		log.Printf("Expect sequence number: %v; Received: %v", sock.nextExpectedByte, relSeqNum)

		// add it to the out of order queue
		sock.earlyArrivalQueue.Push(&TcpPacketItem{
			Value:    p,
			Priority: int(relSeqNum),
		})
		sock.earlyArrivePacketSize.Add(uint32(len(p.data)))
		// sock.outOfOrderQueue.Push(p)
	}

	// 4. send an ack back
	// increase nextExpectedByte before constructing the header
	ackPacket := TcpPacket{
		header: *sock.getAckHeader(),
		data:   []byte{},
	}
	err := sendTcp(sock.conn.foreignIP, ackPacket.Marshal())
	if err != nil {
		fmt.Println("handle packet step 4: ", err)
	}
}

func (sock *TcpSocket) HandleWrites() {
	writeBuffer := sock.writeBuffer
	writeBufferLock := sock.writeBufferLock
	isNotFull := sock.writeBufferIsNotFull
	isNotEmpty := sock.writeBufferIsNotEmpty

	for {
		// TODO: zero window polling!!!
		// if sock.foreignWindowSize.Load() == 0 {

		// }
		for sock.foreignWindowSize.Load() == 0 {
			fmt.Println("Zero window probing should happen here")
			time.Sleep(2 * time.Second)
		}
		writeBufferLock.Lock()
		for writeBuffer.IsEmpty() {
			// wait till some vwrite call signals that the buffer has data in it
			isNotEmpty.Wait()
		}

		// we know here that there is data to send

		// calculate how many bytes to send in total
		// either less than the
		sizeToWrite := uint32(min(int(sock.foreignWindowSize.Load()), writeBuffer.Length()))

		// get all the bytes to send
		if sizeToWrite == 0 {
			writeBufferLock.Unlock()
			continue
		}
		payload := make([]byte, sizeToWrite)
		writeBuffer.Read(payload)

		// signal waiting vwrite calls that the buffer is no longer full
		isNotFull.Broadcast()
		writeBufferLock.Unlock()

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
			err := sendTcp(conn.foreignIP, packetBytes)
			if err != nil {
				fmt.Println("Error in handleWrites from SendMsgToDestIP: ", err)
				return
			}

			sizeToWrite -= segmentSize
		}
	}
}

func (sock *TcpSocket) HandleConnection() {
	go sock.HandleWrites()
	for {
		p := <-sock.ch
		go sock.HandlePacket(p)
	}

	// have a thread waiting for data in the write buffer,
	// when it sees data, we have to send it to the person,
	// we're connected to

}

func (sock *TcpSocket) getAckHeader() *header.TCPFields {
	return &header.TCPFields{
		SrcPort:    sock.conn.localPort,
		DstPort:    sock.conn.foreignPort,
		SeqNum:     sock.myInitSeqNum + sock.numBytesSent.Load() + 1,
		AckNum:     sock.nextExpectedByte.Load() + sock.foreignInitSeqNum,
		DataOffset: TcpHeaderLen,
		Flags:      header.TCPFlagAck, // what flag should we set?
		WindowSize: uint16(sock.readBuffer.Free()),
		// To compute
		Checksum:      0,
		UrgentPointer: 0,
	}
}

func (sock *TcpSocket) String() string {
	res := "\n"
	res += fmt.Sprintf("sockId: %d\n", sock.sockId)
	res += fmt.Sprintf("connState: %s\n", sock.connState)
	res += fmt.Sprintf("myInitSeqNum: %d\n", sock.myInitSeqNum)
	res += fmt.Sprintf("numBytesSent: %d\n", sock.numBytesSent.Load())
	res += fmt.Sprintf("nextExpectedByte: %d\n", sock.nextExpectedByte.Load())
	res += fmt.Sprintf("foreignInitSeqNum: %d\n", sock.foreignInitSeqNum)
	res += fmt.Sprintf("largestAckReceived: %d\n", sock.largestAckReceived.Load())
	res += fmt.Sprintf("foreignWindowSize: %d\n", sock.foreignWindowSize.Load())
	res += ": %\n"

	return res
}
