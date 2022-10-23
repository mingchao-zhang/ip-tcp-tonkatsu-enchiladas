package applications

import (
	"fmt"
	"ip/pkg/network"

	"golang.org/x/net/ipv4"
)

const (
	TestProtocolNum = 0
)

func TestProtocolHandler(packet []byte, params []interface{}) {
	// TODO: get hdr from params
	var hdr ipv4.Header
	fmt.Println("---Node received packet!---")
	fmt.Printf("source IP      : %s\n", hdr.Src.String())
	fmt.Printf("destination IP : %s\n", hdr.Dst.String())
	fmt.Printf("protocol       : %d\n", hdr.Protocol)
	fmt.Printf("payload length : %d\n", hdr.TotalLen)
	fmt.Printf("payload        : %s\n", packet)
	fmt.Println("---------------------------")
}

func TestProtocolInit(fwdTable *network.FwdTable) {
	fwdTable.RegisterHandler(0, TestProtocolHandler)
}
