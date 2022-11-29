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

	"github.com/sasha-s/go-deadlock"

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

	closed   bool
	isClosed chan bool

	readBuffer            *ringbuffer.RingBuffer
	readBufferLock        *deadlock.Mutex
	readBufferIsNotEmpty  *sync.Cond
	earlyArrivalQueue     PriorityQueue
	earlyArrivalSeqNumSet map[uint32]bool
	earlyArrivalQueueLock *deadlock.Mutex
	// TODO: need to decrease the size
	earlyArrivalPacketSize *atomic.Uint32

	writeBuffer           *ringbuffer.RingBuffer
	writeBufferLock       *deadlock.Mutex
	writeBufferIsNotFull  *sync.Cond
	writeBufferIsNotEmpty *sync.Cond
	inFlightList          *list.List
	inFlightListLock      *deadlock.Mutex
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
	windowNotEmpty     *sync.Cond

	//roundtrip time
	srtt time.Duration
	rto  time.Duration

	// lock nextExpectedByte, srtt, rto
	varLock *deadlock.Mutex
}

func MakeTcpSocket(connState string, tcpConn *TcpConn, foreignInitSeqNum uint32) (*TcpSocket, error) {
	sock := TcpSocket{
		sockId:    int(nextSockId.Add(1)),
		connState: connState,
		conn:      tcpConn,

		canRead:  true,
		canWrite: true,

		isClosed: make(chan bool),

		readBuffer:             ringbuffer.New(BufferSize),
		readBufferLock:         &deadlock.Mutex{},
		earlyArrivalQueue:      PriorityQueue{},
		earlyArrivalSeqNumSet:  make(map[uint32]bool),
		earlyArrivalQueueLock:  &deadlock.Mutex{},
		earlyArrivalPacketSize: atomic.NewUint32(0),

		writeBuffer:        ringbuffer.New(BufferSize),
		writeBufferLock:    &deadlock.Mutex{},
		inFlightList:       &list.List{},
		inFlightListLock:   &deadlock.Mutex{},
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

		varLock: &deadlock.Mutex{},
	}

	sock.readBufferIsNotEmpty = sync.NewCond(sock.readBufferLock)
	sock.writeBufferIsNotFull = sync.NewCond(sock.writeBufferLock)
	sock.writeBufferIsNotEmpty = sync.NewCond(sock.writeBufferLock)
	sock.windowNotEmpty = sync.NewCond(sock.inFlightListLock)
	heap.Init(&sock.earlyArrivalQueue)

	return &sock, nil
}

func (sock *TcpSocket) GetConn() *TcpConn {
	return sock.conn
}

// modify nextExpectedByte
func (sock *TcpSocket) writeIntoReadBuffer(p *TcpPacket) error {
	sock.readBufferLock.Lock()
	bytesWritten, err := sock.readBuffer.Write(p.data)
	if err != nil {
		// TODO: check here if there is some issue in the future
		// fmt.Println("HandlePacket: Error while writing to the read buffer", err)
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

func (sock *TcpSocket) timeWait() {
	fmt.Println("Reached timewait")
	time.Sleep(time.Second * 60)
	deleteConnSafe(sock.conn)
}

func (sock *TcpSocket) HandlePacket(p *TcpPacket) {
	sock.varLock.Lock()
	defer sock.varLock.Unlock()

	if sock.closed {
		return
	}

	// validate checksum in the packet
	tcpHdr := p.header
	if !isValidTcpCheckSum(&tcpHdr, sock.conn.foreignIP.NetIP(), sock.conn.localIP.NetIP(), p.data) {
		fmt.Println("invalid checksum in handle packet")
		return
	}

	relSeqNum := tcpHdr.SeqNum - sock.foreignInitSeqNum

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
			sock.inFlightListLock.Lock()
			if sock.inFlightPacketSize.Load() < sock.foreignWindowSize.Load() {
				// wake up the zwp
				sock.windowNotEmpty.Broadcast()
			}
			sock.inFlightListLock.Unlock()
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
					if firstItem.Value.header.Flags == header.TCPFlagFin {
						if sock.connState == FIN_WAIT_1 {
							sock.connState = FIN_WAIT_2
						} else if sock.connState == LAST_ACK {
							sock.connState = CLOSED
							sock.isClosed <- true
							sock.closed = true
							deleteConnSafe(sock.conn)
							sock.inFlightListLock.Unlock()
							// close everything
							return
						}
					}
					inFlight.Remove(inFlight.Front())
					sock.inFlightPacketSize.Sub(uint32(payloadSize))
					if !firstItem.Retransmitted {
						sock.updateRTO(time.Since(firstItem.TimeSent))
					}
				} else {
					break
				}
			}

			if sock.inFlightPacketSize.Load() < sock.foreignWindowSize.Load() {
				// wake up the zwp
				sock.windowNotEmpty.Broadcast()
			}
			sock.inFlightListLock.Unlock()
		}

		// if there's no payload, we don't need to do anything else
		if len(p.data) == 0 {
			return
		}

		// 2. try to write data either in the read buffer or in the heap

		if relSeqNum == sock.nextExpectedByte.Load() {
			if sock.readBuffer.Free() >= len(p.data) {
				sock.writeIntoReadBuffer(p)
				for sock.earlyArrivalQueue.Len() != 0 && sock.earlyArrivalQueue[0].Priority == int(sock.nextExpectedByte.Load()) {

					smallest := sock.earlyArrivalQueue[0].Value
					heap.Pop(&sock.earlyArrivalQueue)
					sock.earlyArrivalPacketSize.Sub(uint32(len(smallest.data)))
					if smallest.header.Flags == header.TCPFlagFin {
						fmt.Println("Popped fin off EAQ")
						if sock.connState == ESTABLISHED {
							sock.connState = CLOSE_WAIT
							sock.readBufferLock.Lock()
							sock.readBufferIsNotEmpty.Broadcast()
							sock.readBufferLock.Unlock()
						} else if sock.connState == FIN_WAIT_2 {
							sock.connState = TIME_WAIT
							sock.timeWait()
						} else {
							fmt.Println("I genuinely hope this does not happen")
						}
					}
					sock.writeIntoReadBuffer(smallest)
					// delete(sock.earlyArrivalSeqNumSet, seqNum)
				}
				if sock.earlyArrivalQueue.Len() != 0 && sock.earlyArrivalQueue[0].Priority < int(sock.nextExpectedByte.Load()) {
					fmt.Println("💀")
				}
			} else {
				// we got a zwp
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
				sock.earlyArrivalPacketSize.Add(uint32(len(p.data)))
			}
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
	} else if p.header.Flags&header.TCPFlagFin != 0 {
		// send ack
		// we need to check if it is the next expected byte, if not then add it to the early arrival
		if relSeqNum == sock.nextExpectedByte.Load() {
			ackHdr := sock.getAckHeader()
			payload := make([]byte, 0)
			ackHdr.Checksum = computeTCPChecksum(ackHdr, sock.conn.localIP.NetIP(), sock.conn.foreignIP.NetIP(), payload)
			ackPacket := TcpPacket{
				header: *ackHdr,
				data:   payload,
			}
			err := sendTcp(sock.conn.foreignIP, ackPacket.Marshal())
			if err != nil {
				log.Println("sendTcp: ", err)
			}
			if sock.connState == ESTABLISHED {
				sock.connState = CLOSE_WAIT
				sock.readBufferLock.Lock()
				sock.readBufferIsNotEmpty.Broadcast()
				sock.readBufferLock.Unlock()
			} else if sock.connState == FIN_WAIT_2 {
				sock.connState = TIME_WAIT
				sock.timeWait()
			} else if sock.connState == CLOSE_WAIT {
				fmt.Println("Received fin when in close wait, socket can write: ", sock.canWrite)
			} else {
				fmt.Println("I sincerely hope this does not happen")
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
				sock.earlyArrivalPacketSize.Add(uint32(len(p.data)))
			}
		}
	}
}

func (sock *TcpSocket) HandleWrites() {
	writeBuffer := sock.writeBuffer
	writeBufferLock := sock.writeBufferLock
	isNotFull := sock.writeBufferIsNotFull
	isNotEmpty := sock.writeBufferIsNotEmpty
	conn := sock.conn

	for {
		writeBufferLock.Lock()
		for writeBuffer.IsEmpty() {
			sock.varLock.Lock()
			if sock.canWrite {
				sock.varLock.Unlock()
				isNotEmpty.Wait()
				fmt.Printf("woken up, current state is %s", sock.connState)
			} else {
				writeBufferLock.Unlock()
				// send fin packet
				finHdr := header.TCPFields{
					SrcPort: sock.conn.localPort,
					DstPort: sock.conn.foreignPort,
					SeqNum:  sock.myInitSeqNum + sock.numBytesSent.Load() + 1,
					// convert to absolute next expected byte
					AckNum:     sock.nextExpectedByte.Load() + sock.foreignInitSeqNum,
					DataOffset: TcpHeaderLen,
					Flags:      header.TCPFlagFin,
					WindowSize: uint16(sock.readBuffer.Free()),
					// To compute
					Checksum:      0,
					UrgentPointer: 0,
				}
				sock.numBytesSent.Add(1)

				finHdr.Checksum = computeTCPChecksum(&finHdr, conn.localIP.NetIP(), conn.foreignIP.NetIP(), []byte{})
				packet := TcpPacket{
					header: finHdr,
					data:   []byte{},
				}

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
				fmt.Println("Sending fin")
				if sock.connState == ESTABLISHED {
					sock.connState = FIN_WAIT_1
				} else if sock.connState == CLOSE_WAIT {
					sock.connState = LAST_ACK
				} else {
					fmt.Println("I hope this does not happen")
				}
				// now that we have added the fin packet to the queue we can stop handling writes
				sock.varLock.Unlock()
				return

			}
			// we
			// wait till some vwrite call signals that the buffer has data in it

		}

		// TODO: zero window probing!!!
		// either we have sent enough packets, or the receiver can't take any more packets
		if sock.foreignWindowSize.Load() <= sock.inFlightPacketSize.Load() {
			// we need to keep sending 1 byte until
			// over here we need to make a packet of size 1

			fmt.Println("Sending zwp")

			oneByte := make([]byte, 1)

			writeBuffer.Read(oneByte)

			isNotFull.Broadcast()
			writeBufferLock.Unlock()

			ackHdr := sock.getAckHeader()
			sock.numBytesSent.Add(1)

			ackHdr.Checksum = computeTCPChecksum(ackHdr, conn.localIP.NetIP(), conn.foreignIP.NetIP(), oneByte)
			packet := TcpPacket{
				header: *ackHdr,
				data:   oneByte,
			}

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
				isZwp:         true,
			})
			// zwp will get retransmitted
			sock.inFlightPacketSize.Add(uint32(len(packet.data)))
			sock.windowNotEmpty.Wait()
			sock.inFlightListLock.Unlock()
		} else {

			// we know here that there is data to send

			// calculate how many bytes to send in total
			// either less than the
			sizeToWrite := uint32(min(int(sock.foreignWindowSize.Load()), writeBuffer.Length()))

			fmt.Println("sending", sizeToWrite, "bytes")

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
}

func (sock *TcpSocket) HandleRetransmission() {
	conn := sock.conn
	inFlight := sock.inFlightList
	listLock := sock.inFlightListLock
	for !sock.closed {
		listLock.Lock()
		if inFlight.Len() != 0 {
			item := inFlight.Front().Value.(*TcpPacketItem)

			if time.Since(item.TimeSent) > sock.srtt {
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

		time.Sleep(sock.srtt)
	}
}

func (sock *TcpSocket) HandleConnection() {
	go sock.HandleWrites()
	go sock.HandleRetransmission()
	for !sock.closed {
		select {
		case p := <-sock.ch:
			go sock.HandlePacket(p)
		case <-sock.isClosed:
			return
		}
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

func PrintBufferSizes(socketId int) {
	sock := GetSocketById(socketId)
	if sock == nil {
		fmt.Println("Invalid socket number")
	} else {
		fmt.Printf("Read buffer free space: %d\nWrite buffer free space: %d\n", sock.readBuffer.Free(), sock.writeBuffer.Free())
	}
}

func PrintEarlyArrivalSize(socketId int) {
	sock := GetSocketById(socketId)
	if sock == nil {
		fmt.Println("Invalid socket number")
	} else {
		fmt.Printf("Early arrival num packets: %d\nEarly arrival queue size: %d\n", sock.earlyArrivalQueue.Len(), sock.earlyArrivalPacketSize.Load())
	}
}
