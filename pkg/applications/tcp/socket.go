package tcp

import (
	"math/rand"

	"github.com/smallnest/ringbuffer"
	"go.uber.org/atomic"
)

type TcpSocket struct {
	readBuffer        *ringbuffer.RingBuffer
	writeBuffer       *ringbuffer.RingBuffer
	ch                chan *TcpPacket
	stop              chan bool
	connState         byte
	myInitSeqNum      uint32
	foreignInitSeqNum uint32
	nextExpectedByte  *atomic.Uint32
}

func MakeTcpSocket(connState byte, foreignInitSeqNum uint32) (*TcpSocket, error) {
	return &TcpSocket{
		readBuffer:        ringbuffer.New(BufferSize),
		writeBuffer:       ringbuffer.New(BufferSize),
		ch:                make(chan *TcpPacket),
		stop:              make(chan bool),
		connState:         connState,
		myInitSeqNum:      rand.Uint32(),
		foreignInitSeqNum: foreignInitSeqNum,
		nextExpectedByte:  atomic.NewUint32(0),
	}, nil
}

// func (TcpSocket) GetAbsoluteSeqNum(relativeSeqNum uint32) {

// }

// func (TcpSocket) GetRelativeSeqNum(absoluteSeqNum uint32) {

// }
