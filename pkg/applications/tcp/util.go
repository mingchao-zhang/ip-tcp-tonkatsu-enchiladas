package tcp

import (
	"fmt"
	link "ip/pkg/ipinterface"

	"github.com/google/netstack/tcpip/header"
)

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
