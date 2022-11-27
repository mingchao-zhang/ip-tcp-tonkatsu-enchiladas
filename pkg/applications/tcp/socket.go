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

	canRead  bool
	canWrite bool

	readBuffer            *ringbuffer.RingBuffer
	readBufferLock        *sync.Mutex
	readBufferIsNotEmpty  *sync.Cond
	earlyArrivalQueue     PriorityQueue
	earlyArrivalSeqNumSet map[uint32]bool
	earlyArrivalQueueLock *sync.Mutex
	// TODO: need to decrease the size
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
	foreignInitSeqNum uint32 // raw
	// TODO: numBytesSent - largestAckReceived <= foreignWindowSize
	largestAckReceived *atomic.Uint32 // raw
	foreignWindowSize  *atomic.Uint32

	//roundtrip time
	srtt time.Duration
	rto  time.Duration

	// lock nextExpectedByte, srtt, rto
	varLock *sync.Mutex
}

func MakeTcpSocket(connState string, tcpConn *TcpConn, foreignInitSeqNum uint32) (*TcpSocket, error) {
	sock := TcpSocket{
		sockId:    int(nextSockId.Add(1)),
		connState: connState,
		conn:      tcpConn,

		canRead:  true,
		canWrite: true,

		readBuffer:            ringbuffer.New(BufferSize),
		readBufferLock:        &sync.Mutex{},
		earlyArrivalQueue:     PriorityQueue{},
		earlyArrivalSeqNumSet: make(map[uint32]bool),
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

		srtt: time.Microsecond * 50,
		rto:  time.Microsecond * 50,

		varLock: &sync.Mutex{},
	}

	sock.readBufferIsNotEmpty = sync.NewCond(sock.readBufferLock)
	sock.writeBufferIsNotFull = sync.NewCond(sock.writeBufferLock)
	sock.writeBufferIsNotEmpty = sync.NewCond(sock.writeBufferLock)
	heap.Init(&sock.earlyArrivalQueue)

	return &sock, nil
}

// modify nextExpectedByte
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
	sock.varLock.Lock()
	defer sock.varLock.Unlock()

	// validate checksum in the packet
	tcpHdr := p.header
	if !isValidTcpCheckSum(&tcpHdr, sock.conn.foreignIP.NetIP(), sock.conn.localIP.NetIP(), p.data) {
		fmt.Println("invalid checksum in handle packet")
		return
	}

	if tcpHdr.Flags == header.TCPFlagAck {
		packetWindowSize := uint32(tcpHdr.WindowSize)

		// 1. modify largestAckReceived and foreignWindowSize
		if tcpHdr.AckNum < sock.largestAckReceived.Load() {
			// fmt.Println("In HandlePacket: Old ack received")
			return
		} else if tcpHdr.AckNum == sock.largestAckReceived.Load() {
			if sock.foreignWindowSize.Load() < packetWindowSize {
				sock.foreignWindowSize.Store(packetWindowSize)
			}
		} else { // tcpHdr.AckNum > sock.largestAckReceived.Load()
			sock.foreignWindowSize.Store(packetWindowSize)
			sock.largestAckReceived.Store(tcpHdr.AckNum)

			// remove items from the inFlightList if necessary
			relLargestAckNum := tcpHdr.AckNum - sock.myInitSeqNum
			sock.inFlightListLock.Lock()
			inFlight := sock.inFlightList
			for inFlight.Len() != 0 {
				firstItem := inFlight.Front().Value.(*TcpPacketItem)
				relSeq := firstItem.Priority
				payloadSize := len(firstItem.Value.data)

				if relSeq+payloadSize <= int(relLargestAckNum) {
					inFlight.Remove(inFlight.Front())
					sock.inFlightPacketSize.Sub(uint32(payloadSize))
					if !firstItem.Retransmitted {
						sock.updateRTO(time.Since(firstItem.TimeSent))
					}
				} else {
					break
				}
			}
			sock.inFlightListLock.Unlock()
		}

		// if there's no payload, we don't need to do anything else
		if len(p.data) == 0 {
			return
		}

		// 2. try to write data either in the read buffer or in the heap
		relSeqNum := tcpHdr.SeqNum - sock.foreignInitSeqNum

		if relSeqNum == sock.nextExpectedByte.Load() {
			if sock.readBuffer.Free() >= len(p.data) {
				sock.writeIntoReadBuffer(p)

				// pop earlyArrivalQueue elements if possible;
				sock.earlyArrivalQueueLock.Lock()
				for sock.earlyArrivalQueue.Len() != 0 && sock.earlyArrivalQueue[0].Priority == int(sock.nextExpectedByte.Load()) {

					smallest := sock.earlyArrivalQueue[0].Value
					heap.Pop(&sock.earlyArrivalQueue)
					sock.writeIntoReadBuffer(smallest)
					// delete(sock.earlyArrivalSeqNumSet, seqNum)
				}
				if sock.earlyArrivalQueue.Len() != 0 && sock.earlyArrivalQueue[0].Priority < int(sock.nextExpectedByte.Load()) {
					fmt.Println("💀")
				}
				// maybe we don't need the early arrival queue anymore (because we're locking the entire socket)
				sock.earlyArrivalQueueLock.Unlock()
			} else {
				fmt.Println("HandlePacket window size not respected")
				return
			}
		} else if relSeqNum > sock.nextExpectedByte.Load() { // early arrivals
			// add it to the out of order queue only if we haven't seen it before
			_, ok := sock.earlyArrivalSeqNumSet[relSeqNum]
			if !ok {
				heap.Push(&sock.earlyArrivalQueue, &TcpPacketItem{
					Value:    p,
					Priority: int(relSeqNum),
				})
				sock.earlyArrivalSeqNumSet[relSeqNum] = true
				sock.earlyArrivePacketSize.Add(uint32(len(p.data)))
			}
		} else {
			// TODO: possibly return
		}

		// 4. send an ack back
		// remember to increase nextExpectedByte before constructing the header
		ackHdr := *sock.getAckHeader()
		payload := make([]byte, 0)
		ackHdr.Checksum = computeTCPChecksum(&ackHdr, sock.conn.localIP.NetIP(), sock.conn.foreignIP.NetIP(), payload)
		ackPacket := TcpPacket{
			header: ackHdr,
			data:   payload,
		}
		err := sendTcp(sock.conn.foreignIP, ackPacket.Marshal())
		if err != nil {
			log.Println("sendTcp: ", err)
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
			fmt.Println("Should do zwp")
			time.Sleep(time.Second * 3)
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
			fmt.Println("In HandleWrites: sizeToWrite is 0. Shouldn't happen ")
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

func (sock *TcpSocket) HandleRetransmission() {
	conn := sock.conn
	inFlight := sock.inFlightList
	listLock := sock.inFlightListLock
	for {
		listLock.Lock()
		if inFlight.Len() != 0 {
			item := inFlight.Front().Value.(*TcpPacketItem)

			if time.Since(item.TimeSent) > sock.rto {
				item.Retransmitted = true
				packet := item.Value

				packetBytes := packet.Marshal()
				err := sendTcp(conn.foreignIP, packetBytes)
				if err != nil {
					fmt.Println("Error in handleRetransmission from SendMsgToDestIP: ", err)
				}
			}
		}
		listLock.Unlock()

		time.Sleep(sock.rto)
	}
}

func (sock *TcpSocket) HandleConnection() {
	go sock.HandleWrites()
	go sock.HandleRetransmission()
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
		SrcPort: sock.conn.localPort,
		DstPort: sock.conn.foreignPort,
		SeqNum:  sock.myInitSeqNum + sock.numBytesSent.Load() + 1,
		// convert to absolute next expected byte
		AckNum:     sock.nextExpectedByte.Load() + sock.foreignInitSeqNum,
		DataOffset: TcpHeaderLen,
		Flags:      header.TCPFlagAck,
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

func (sock *TcpSocket) updateRTO(obsRTT time.Duration) {
	sock.srtt = time.Duration((float64(sock.srtt) * Alpha) + (float64(obsRTT) * (1 - Alpha)))
	sock.rto = maxTime(RTOMin, minTime(time.Duration(float64(sock.srtt)*Beta), RTOMax))
}
