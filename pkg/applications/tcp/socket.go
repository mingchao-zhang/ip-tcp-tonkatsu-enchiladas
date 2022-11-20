package tcp

import (
	"container/heap"
	"container/list"
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
	inFlightList          *list.List
	inFlightListLock      *sync.Mutex
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

	//roundtrip time
	srtt time.Duration
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
		inFlightList:       &list.List{},
		inFlightListLock:   &sync.Mutex{},
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

		srtt: time.Microsecond * 20,
	}

	sock.readBufferIsNotEmpty = sync.NewCond(sock.readBufferLock)
	sock.writeBufferIsNotFull = sync.NewCond(sock.writeBufferLock)
	sock.writeBufferIsNotEmpty = sync.NewCond(sock.writeBufferLock)
	heap.Init(&sock.earlyArrivalQueue)

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
			return errors.New("we should have enough space in the buffer :(")
		}
		sock.readBufferIsNotEmpty.Broadcast()
		sock.readBufferLock.Unlock()

		sock.nextExpectedByte.Add(uint32(len(p.data)))
	}
	return nil
}

func (sock *TcpSocket) HandlePacket(p *TcpPacket) {
	// what could go wrong if we have multiple packets being handled at the same time?

	// validate checksum in the packet
	tcpHdr := p.header
	if !isValidTcpCheckSum(&tcpHdr, sock.conn.foreignIP.NetIP(), sock.conn.localIP.NetIP(), p.data) {
		fmt.Println("invalid checksum in handle packet")
		return
	}

	// we need to check what packets can be removed from the in flight queue based on the ack number of the packet
	if tcpHdr.Flags == header.TCPFlagAck {
		// 1. modify largestAckReceived and foreignWindowSize
		packetWindowSize := uint32(tcpHdr.WindowSize)
		if tcpHdr.AckNum > sock.largestAckReceived.Load() {

			sock.inFlightListLock.Lock()
			sock.foreignWindowSize.Store(packetWindowSize)
			sock.largestAckReceived.Store(tcpHdr.AckNum)

			relLargestAckNum := sock.largestAckReceived.Load() - sock.myInitSeqNum

			inFlight := sock.inFlightList

			// Maybe change '<=' to '<'
			for inFlight.Len() != 0 {
				item := inFlight.Front().Value.(*TcpPacketItem)
				if item.Priority > int(relLargestAckNum) {
					break
				}
				sock.inFlightPacketSize.Sub(uint32(len(item.Value.data)))
				inFlight.Remove(inFlight.Front())
			}
			sock.inFlightListLock.Unlock()

		} else if tcpHdr.AckNum == sock.largestAckReceived.Load() {
			if sock.foreignWindowSize.Load() < packetWindowSize {
				sock.foreignWindowSize.Store(packetWindowSize)
			}
		} else {
			fmt.Println("In HandlePacket: Old ack received")
			return
		}

		// 2. check if we need to write to buffer
		relSeqNum := tcpHdr.SeqNum - sock.foreignInitSeqNum

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
		}

		// 4. send an ack back
		// increase nextExpectedByte before constructing the header
		ackHdr := *sock.getAckHeader()
		payload := make([]byte, 0)
		ackHdr.Checksum = computeTCPChecksum(&ackHdr, sock.conn.localIP.NetIP(), sock.conn.foreignIP.NetIP(), payload)
		ackPacket := TcpPacket{
			header: ackHdr,
			data:   payload,
		}
		err := sendTcp(sock.conn.foreignIP, ackPacket.Marshal())
		if err != nil {
			fmt.Println("handle packet step 4: ", err)
		}
	} else {
		fmt.Println("should not happen right now")
	}
}

func (sock *TcpSocket) HandleWrites() {
	writeBuffer := sock.writeBuffer
	writeBufferLock := sock.writeBufferLock
	isNotFull := sock.writeBufferIsNotFull
	isNotEmpty := sock.writeBufferIsNotEmpty

	for {
		// TODO: zero window probing!!!
		// either we have sent enough packets, or the receiver can't take any more packets
		for sock.foreignWindowSize.Load() == sock.inFlightPacketSize.Load() || sock.foreignWindowSize.Load() == 0 {
			// we need to keep sending 1 byte until
			time.Sleep(10 * time.Millisecond)
			// write logic to keep sending one byte until we get acks
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
			ackHdr.Checksum = computeTCPChecksum(ackHdr, conn.localIP.NetIP(), conn.foreignIP.NetIP(), payload[:segmentSize])
			packet := TcpPacket{
				header: *ackHdr,
				data:   payload[:segmentSize],
			}
			payload = payload[segmentSize:]

			packetBytes := packet.Marshal()
			err := sendTcp(conn.foreignIP, packetBytes)
			if err != nil {
				fmt.Println("Error in handleWrites from SendMsgToDestIP: ", err)
				break
			}
			sock.inFlightListLock.Lock()
			sock.inFlightList.PushBack(&TcpPacketItem{
				Value:         &packet,
				Priority:      int(packet.header.SeqNum - sock.myInitSeqNum),
				TimeSent:      time.Now(),
				Retransmitted: false,
			})
			sock.inFlightListLock.Unlock()
			// keep track of the size of packets in flight
			// we should stop sending if size of packets in flight == window size
			sock.inFlightPacketSize.Add(uint32(len(packet.data)))
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
