package applications

import (
	"fmt"
	"ip/pkg/network"

	"golang.org/x/net/ipv4"
)

const (
	TestProtocolNum = 0
)

func TestProtocolHandler(rawMsg []byte, params []interface{}) {
	hdr := params[0].(*ipv4.Header)
	fmt.Println("---Node received packet!---")
	fmt.Printf("source IP      : %s\n", hdr.Src.String())
	fmt.Printf("destination IP : %s\n", hdr.Dst.String())
	fmt.Printf("protocol       : %d\n", hdr.Protocol)
	fmt.Printf("payload length : %d\n", hdr.TotalLen)
	fmt.Printf("payload        : %s\n", rawMsg)
	fmt.Println("---------------------------")
}

func TestProtocolInit(fwdTable *network.FwdTable) {
	fwdTable.RegisterHandlerSafe(TestProtocolNum, TestProtocolHandler)
}
