package tcp

import (
	"container/heap"
	"math/rand"

	"github.com/smallnest/ringbuffer"
	"go.uber.org/atomic"
)

type TcpSocket struct {
	sockId    int
	connState byte
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

var nextSockId = atomic.NewInt32(-1)

func MakeTcpSocket(connState byte, tcpConn *TcpConn, foreignInitSeqNum uint32) (*TcpSocket, error) {
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

// func (TcpSocket) GetAbsoluteSeqNum(relativeSeqNum uint32) {

// }

// func (TcpSocket) GetRelativeSeqNum(absoluteSeqNum uint32) {

// }
