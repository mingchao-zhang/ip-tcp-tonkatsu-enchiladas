package tcp

import (
	"errors"
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"
	"net"

	"github.com/alecthomas/units"
	"github.com/armon/circbuf"
	"github.com/google/netstack/tcpip/header"
	"golang.org/x/net/ipv4"
)

const (
	TcpProtocolNum = uint8(header.TCPProtocolNumber)
	TcpHeaderLen   = header.TCPMinimumSize
	BufferSize     = int64(64 * units.KiB)
	SYN_RECEIVED   = 0
	SYN_SENT       = 1
	ESTABLISHED    = 2
	FIN_WAIT_1     = 3
	FIN_WAIT_2     = 4
	CLOSE_WAIT     = 5
	LAST_ACK       = 6
)

var state *TcpState

func TcpHandler(rawMsg []byte, params []interface{}) {
	ipHdr := params[0].(*ipv4.Header)
	tcpPacket := UnmarshalTcpPacket(rawMsg)
	fmt.Println(tcpPacket)

	tcpConn := TcpConn{
		localIP:     link.IntIPFromNetIP(ipHdr.Dst),
		foreignIP:   link.IntIPFromNetIP(ipHdr.Src),
		localPort:   tcpPacket.header.DstPort,
		foreignPort: tcpPacket.header.SrcPort,
	}

	socket, ok := state.sockets[tcpConn]
	fmt.Println(ok, socket)

	// srcIP := link.IntIPFromNetIP(hdr.Src)
}

// VListen is a part of the network API available to applications
// Callers do not need to lock
func VListen(port uint16) (*TcpListener, error) {
	state.lock.Lock()
	defer state.lock.Unlock()

	_, ok := state.ports[port]
	if ok {
		return nil, errors.New("port in use")
	}

	// at this point we know that the port is unused
	state.ports[port] = true
	state.listeners[port] = true

	return &TcpListener{
		ip:   state.myIP,
		port: port,
		ch:   make(chan *TcpPacket),
		stop: make(chan bool),
	}, nil
}

func (conn *TcpConn) HandleConnection() {

}

func (l *TcpListener) VAccept() (*TcpConn, error) {
	select {
	case p := <-l.ch:
		// here we need to make sure p is a SYN
		if p.header.Flags == header.TCPFlagSyn {
			// we only accept connections if the flag is *just* SYN
			fmt.Println(p.header.Flags)
			// at this point we need to check if there is already a connection from this port and IP address?
			// Question: can we have 2 connections to the same port?
			// for now I won't check

			// create an entry in the socket table using a random port number
			// spawn a thread to handle this connection?

			state.lock.Lock()

			conn := TcpConn{
				localIP:     l.ip,
				localPort:   l.port,
				foreignIP:   p.srcIP,
				foreignPort: p.header.SrcPort,
			}

			_, ok := state.sockets[conn]
			if ok {
				return nil, errors.New("already connected to a client at this port")
			}

			readBuf, err := circbuf.NewBuffer(BufferSize)
			if err != nil {
				return nil, err
			}

			writeBuf, err := circbuf.NewBuffer(BufferSize)
			if err != nil {
				return nil, err
			}

			sock := &TcpSocket{
				readBuffer:  readBuf,
				writeBuffer: writeBuf,
				recvChan:    make(chan *TcpPacket),
				state:       SYN_RECEIVED,
			}
			state.sockets[conn] = sock

			state.lock.Unlock()

			// first we need to send a syn-ack
			tcpHdr := header.TCPFields{
				SrcPort: conn.localPort,
				DstPort: conn.foreignPort,
				// ⚠️ ⬇️⬇️⬇️ adjust these values ⬇️⬇️⬇️
				SeqNum:        1,
				AckNum:        1,
				DataOffset:    20,
				Flags:         header.TCPFlagSyn | header.TCPFlagAck,
				WindowSize:    65535,
				Checksum:      0,
				UrgentPointer: 0,
			}

			synAckPacket := TcpPacket{
				header: tcpHdr,
				data:   make([]byte, 0),
			}

			packetBytes := synAckPacket.Marshal()

			state.fwdTable.Lock.RLock()
			err = state.fwdTable.SendMsgToDestIP(p.srcIP, TcpProtocolNum, packetBytes)
			state.fwdTable.Lock.RUnlock()
			if err != nil {
				state.lock.Lock()
				delete(state.sockets, conn)
				state.lock.Unlock()
				return nil, err
			}

			// at this point we have established a connection
			select {
			case p := <-l.ch:
				if p.header.Flags&header.TCPFlagAck != 0 {
					fmt.Println("we did it :partyemoji:")
				}
			case <-l.stop:
				return nil, errors.New("connection closed")
			}

			return &conn, nil
		} else {
			return nil, errors.New("invalid flags")
		}
	case <-l.stop:
		return nil, errors.New("connection closed")
	}

}

// TODO
func (l *TcpListener) VClose() error {
	return nil
}

/*
 * Creates a new socket and connects to an
 * address:port (active OPEN in the RFC).
 * Returns a VTCPConn on success or non-nil error on
 * failure.
 * VConnect MUST block until the connection is
 * established, or an error occurs.
 */
func VConnect(addr net.IP, port int16) (*TcpConn, error) {
	// conn := TcpConn {
	// 	localIP     link.IntIP
	// 	localPort   uint16
	// 	foreignIP   link.IntIP
	// 	foreignPort uint16
	// }
	return nil, nil
}

func TCPInit(fwdTable *network.FwdTable) {
	fwdTable.RegisterHandlerSafe(TcpProtocolNum, TcpHandler)
	// TODO: initialize myIP
	state = &TcpState{
		sockets:        make(map[TcpConn]*TcpSocket),
		listeners:      make(map[uint16]bool),
		ports:          make(map[uint16]bool),
		fwdTable:       fwdTable,
		nextUnusedPort: 1000,
		// ⚠️ ADD IP HERE THIS IS INCOMPLETE
		myIP: 0000_0000_0000_0000,
	}
}

// FINISHED FUNCTIONS ----------------------------------------------------------
func ParseTCPHeader(b []byte) header.TCPFields {
	td := header.TCP(b)
	return header.TCPFields{
		SrcPort:    td.SourcePort(),
		DstPort:    td.DestinationPort(),
		SeqNum:     td.SequenceNumber(),
		AckNum:     td.AckNumber(),
		DataOffset: td.DataOffset(),
		Flags:      td.Flags(),
		WindowSize: td.WindowSize(),
		Checksum:   td.Checksum(),
	}
}

func UnmarshalTcpPacket(rawMsg []byte) *TcpPacket {
	tcpHeader := ParseTCPHeader(rawMsg)
	tcpPacket := TcpPacket{
		header: tcpHeader,
		data:   rawMsg[tcpHeader.DataOffset:],
	}

	return &tcpPacket
}

func (p *TcpPacket) Marshal() []byte {
	tcphdr := make(header.TCP, TcpHeaderLen)
	tcphdr.Encode(&p.header)

	// return header appended to data
	return append([]byte(tcphdr), p.data...)
}

// allocatePort unsafely (without locking) returns an unused port and modify state
func allocatePortUnsafe() uint16 {

	// find the port we could use
	_, inMap := state.ports[state.nextUnusedPort]
	for inMap {
		state.nextUnusedPort += 1
		_, inMap = state.ports[state.nextUnusedPort]
	}

	res := state.nextUnusedPort
	state.nextUnusedPort += 1
	return res
}
