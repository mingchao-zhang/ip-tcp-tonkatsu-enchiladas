package tcp

import (
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"
	"math/rand"
	"time"

	"github.com/google/netstack/tcpip/header"
	"golang.org/x/net/ipv4"
)

var state *TcpState

func TcpHandler(rawMsg []byte, params []interface{}) {
	ipHdr := params[0].(*ipv4.Header)
	srcIP := link.IntIPFromNetIP(ipHdr.Src)
	tcpPacket := UnmarshalTcpPacket(rawMsg, srcIP)

	tcpConn := TcpConn{
		localIP:     link.IntIPFromNetIP(ipHdr.Dst),
		localPort:   tcpPacket.header.DstPort,
		foreignIP:   srcIP,
		foreignPort: tcpPacket.header.SrcPort,
	}

	if tcpConn.localIP != state.myIP {
		// we will never make connections from any IP other than myIP
		fmt.Printf("Packet dropped in TcpHandler: %s is not the IP for this node\n", ipHdr.Dst)
		return
	}

	socket, ok := state.sockets[tcpConn]
	if !ok {
		// check if there is a server listening on that port
		listener, ok := state.listeners[tcpConn.localPort]
		if !ok {
			fmt.Printf("Packet dropped in TcpHandler: port %d is not a listen port.\n", tcpConn.localPort)
			return
		}
		listener.ch <- tcpPacket
	} else {
		socket.ch <- tcpPacket
	}
}

func TCPInit(fwdTable *network.FwdTable) {
	fwdTable.RegisterHandlerSafe(TcpProtocolNum, TcpHandler)

	rand.Seed(time.Now().UnixMicro())

	state = &TcpState{
		sockets:        make(map[TcpConn]*TcpSocket),
		listeners:      make(map[uint16]*TcpListener),
		ports:          make(map[uint16]bool),
		fwdTable:       fwdTable,
		nextUnusedPort: 2000,
		myIP:           link.GetSmallestLocalIP(fwdTable.IpInterfaces),
	}
}

// HELPER FUNCTIONS --------------------------------------------------
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

func UnmarshalTcpPacket(rawMsg []byte, srcIP link.IntIP) *TcpPacket {
	tcpHeader := ParseTCPHeader(rawMsg)
	tcpPacket := TcpPacket{
		header: tcpHeader,
		data:   rawMsg[tcpHeader.DataOffset:],
		srcIP:  srcIP,
	}

	return &tcpPacket
}

func (p *TcpPacket) Marshal() []byte {
	tcphdr := make(header.TCP, TcpHeaderLen)
	tcphdr.Encode(&p.header)

	// return header appended to data
	return append([]byte(tcphdr), p.data...)
}

func GetSocketInfo() *string {
	state.lock.Lock()
	defer state.lock.Unlock()
	res := "socket  local-addr      port            dst-addr        port  status\n"
	res += "--------------------------------------------------------------------\n"
	// get lister sockets string
	// Ex: 0       0.0.0.0         9000            0.0.0.0         0     LISTEN
	for port := range state.listeners {
		res += fmt.Sprintf("0       0.0.0.0         %d            0.0.0.0         0     LISTEN\n", port)
	}

	// get normal sockets string
	// Ex: 1       192.168.0.1     9000            192.168.0.2     1024  ESTAB
	for conn, sock := range state.sockets {
		res += fmt.Sprintf("%d       ", sock.sockId)
		res += fmt.Sprintf("%s     ", conn.localIP)
		res += fmt.Sprintf("%d            ", conn.localPort)
		res += fmt.Sprintf("%s     ", conn.foreignIP)
		res += fmt.Sprintf("%d  ", conn.foreignPort)
		res += fmt.Sprintf("%s  ", sock.connState)
		res += "\n"
	}
	return &res
}
