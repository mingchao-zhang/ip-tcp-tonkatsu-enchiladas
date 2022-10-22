package network

import (
	"fmt"
	"ip/pkg/transport"
	"log"
	"net"
	"sync"

	"github.com/google/netstack/tcpip/header"
	"golang.org/x/net/ipv4"
)

type HandlerFunc = func([]byte, []interface{})

type Link struct {
	Id          int
	State       string
	DestAddr    string
	DestUdpPort string
	InterfaceIP string
	DestIP      string
}

type FwdTable struct {
	table        map[string]Link
	myInterfaces map[string]bool

	// uint8 is the protocol number for the application
	// try sync map
	applications map[uint8]HandlerFunc

	conn transport.Transport
	lock sync.RWMutex
}

func (ft *FwdTable) Init(links []Link, conn transport.Transport) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	ft.table = make(map[string]Link)
	ft.myInterfaces = make(map[string]bool)
	for _, link := range links {
		ft.table[link.DestIP] = link
		ft.myInterfaces[link.InterfaceIP] = true
	}
	ft.conn = conn
}

// Add a (DestIP, link) to the ft.table field
func (ft *FwdTable) AddRecord(destIP string, link *Link) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	ft.table[destIP] = *link
}

func (ft *FwdTable) RemoveRecord(destIP string) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	delete(ft.table, destIP)
}

func (ft *FwdTable) GetRecord(destIP string) (link *Link, ok bool) {
	ft.lock.RLock()
	defer ft.lock.RUnlock()

	nextHop, ok := ft.table[destIP]
	return &nextHop, ok
}

func (ft *FwdTable) RegisterHandler(protocolNum uint8, hf HandlerFunc) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	ft.applications[protocolNum] = hf
}

func (ft *FwdTable) Print() {
	ft.lock.RLock()
	defer ft.lock.RUnlock()

	for destIP, link := range ft.table {
		fmt.Printf("Dest IP: [%s] Interface IP: [%s]\n", destIP, link.InterfaceIP)
	}
}

func ComputeChecksum(b []byte) uint16 {
	checksum := header.Checksum(b, 0)
	checksumInv := checksum ^ 0xffff
	return checksumInv
}

func (ft *FwdTable) SendMsgToDestIP(destIP string, procotol string, msg string) {
	ft.lock.RLock()
	link, ok := ft.table[destIP]
	ft.lock.RUnlock()

	if !ok {
		fmt.Printf("Can't reach the IP address: %s\n", destIP)
		return
	}

	hdr := ipv4.Header{
		Version:  4,
		Len:      20, // Header length is always 20 when no IP options
		TOS:      0,
		TotalLen: ipv4.HeaderLen + len(msg),
		ID:       0,
		Flags:    0,
		FragOff:  0,
		TTL:      32,
		Protocol: 0,
		Checksum: 0, // Should be 0 until checksum is computed
		Src:      net.ParseIP(link.InterfaceIP),
		Dst:      net.ParseIP(destIP),
		Options:  []byte{},
	}

	headerBytes, err := hdr.Marshal()
	if err != nil {
		log.Fatalln("Error marshalling header:  ", err)
	}

	hdr.Checksum = int(ComputeChecksum(headerBytes))
	headerBytes, err = hdr.Marshal()
	if err != nil {
		log.Fatalln("Error marshalling header:  ", err)
	}

	fullPacket := append(headerBytes, []byte(msg)...)
	ft.HandlePacket(fullPacket)
}

func (ft *FwdTable) HandlePacket(buffer []byte) {
	// Verify CheckSum
	hdr, err := ipv4.ParseHeader(buffer)
	if err != nil {
		fmt.Println("Error parsing the ip header: ", err)
		return
	}
	// if hdr.Checksum != int(ComputeChecksum(buffer[:hdr.Len])) {
	// 	fmt.Printf("Correct library checksum: %d\n", ComputeChecksum(buffer[:hdr.Len]))
	// 	fmt.Printf("Incorrect header checksum: %d!\n", hdr.Checksum)
	// 	return
	// }

	destIP := hdr.Dst.String()
	msgBytes := buffer[hdr.Len:]
	interfaceUp, ok := ft.myInterfaces[destIP]
	if ok {
		if !interfaceUp {
			fmt.Println("Interface has been shut down")
			return
		}
		// we are the destination, call the handler for the appropriate application
		handler, ok := ft.applications[uint8(hdr.Protocol)]
		if !ok {
			fmt.Println("Received packet with invalid protocol number")
			return
		}

		handler(msgBytes, []interface{}{hdr})
	} else {
		// not the destination, forward to next hop
		// what do we do if we don't know a next hop for this destination???
		nextHopLink, ok := ft.table[destIP]
		if !ok {
			log.Println("Don't know how to get to this destination 🤷🏾")
			return
		}

		hdr.TTL -= 1
		if hdr.TTL <= 0 {
			return
		}
		newHdrBytes, err := hdr.Marshal()
		if err != nil {
			log.Println("Unable to marshal header in HandlePacket 🙀")
			return
		}

		// Recompute checksum with ttl decremented
		hdr.Checksum = int(ComputeChecksum(newHdrBytes))
		newHdrBytes, err = hdr.Marshal()
		if err != nil {
			log.Println("Unable to marshal header in HandlePacket 🙀")
			return
		}

		fullPacket := append(newHdrBytes, msgBytes...)
		remoteString := fmt.Sprintf("%s:%s", nextHopLink.DestAddr, nextHopLink.DestUdpPort)
		ft.conn.Send(remoteString, fullPacket)
	}

}
