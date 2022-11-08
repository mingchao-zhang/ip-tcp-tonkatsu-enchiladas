package applications

import (
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"

	"github.com/google/netstack/tcpip/header"
	"golang.org/x/net/ipv4"
)

const (
	TcpProtocolNum = uint8(header.TCPProtocolNumber)
)

type TcpState struct {
	table SocketTable
}

var state TcpState

type TcpPacket struct {
	header header.TCPFields
	data   []byte
}

type TcpConn struct {
	localIP     link.IntIP
	localPort   uint16
	foreignIP   link.IntIP
	foreignPort uint16
	// protocolNum uint8

}

type TcpSocket interface{}

type SocketTable map[TcpConn]TcpSocket

func (p *TcpPacket) MarshalTCPPacket() []byte {
	tcphdr := header.TCP{}
	tcphdr.Encode(&p.header)

	// return header appended to data
	return append([]byte(tcphdr), p.data...)
}

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

func TcpHandler(rawMsg []byte, params []interface{}) {
	// hdr := params[0].(*ipv4.Header)
	ipHdr := params[0].(*ipv4.Header)
	tcpPacket := UnmarshalTcpPacket(rawMsg)
	fmt.Println(tcpPacket)

	tcpConn := TcpConn{
		localIP:     link.IntIPFromNetIP(ipHdr.Dst),
		foreignIP:   link.IntIPFromNetIP(ipHdr.Src),
		localPort:   tcpPacket.header.DstPort,
		foreignPort: tcpPacket.header.SrcPort,
	}

	socket := state.table[tcpConn]

	fmt.Println(socket)

	// srcIP := link.IntIPFromNetIP(hdr.Src)
}

func TCPInit(fwdTable *network.FwdTable) {
	FwdTable = fwdTable
	FwdTable.RegisterHandlerSafe(TcpProtocolNum, TcpHandler)
	state.table = make(SocketTable)
}
