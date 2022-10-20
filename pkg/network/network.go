package network

import (
	"fmt"
	"ip/pkg/transport"
	"log"
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

	conn *transport.Transport
	lock sync.RWMutex
}

func (ft *FwdTable) Init(links []Link) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	ft.table = make(map[string]Link)
	ft.myInterfaces = make(map[string]bool)
	for _, link := range links {
		ft.table[link.DestIP] = link
		ft.myInterfaces[link.InterfaceIP] = true
	}
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

func (ft *FwdTable) HandlePacket(hdr *ipv4.Header, message []byte) {
	hdrBytes, err := hdr.Marshal()
	if err != nil {
		log.Println("Unable to marshal header in HandlePacket 🙀")
		return
	}

	// check checksum
	if hdr.Checksum != int(header.IPv4.CalculateChecksum(hdrBytes)) {
		log.Println("Received packet with invalid checksum 😵")
		return
	}

	// check if destination is one of the interfaces on this node
	destIP := hdr.Dst.String()
	_, ok := ft.myInterfaces[destIP]
	if ok {
		// we are the destination, call the handler for the appropriate application
		handler, ok := ft.applications[uint8(hdr.Protocol)]
		if !ok {
			fmt.Println("Received packet with invalid protocol number")
			return
		}

		handler(message, []interface{}{hdr})
	} else {
		// not the destination, forward to next hop
		// what do we do if we don't know a next hop for this destination???
		nextHop, ok := ft.table[destIP]
		if !ok {
			log.Println("Don't know how to get to this destination 🤷🏾")
		}
		fmt.Println(nextHop)

		hdr.TTL -= 1
		if hdr.TTL == 0 {
			return
		}

		hdrBytes, err = hdr.Marshal()
		if err != nil {
			log.Println("Unable to marshal header in HandlePacket 🙀")
			return
		}

		// recompute checksum with ttl decremented
		hdr.Checksum = int(header.IPv4.CalculateChecksum(hdrBytes))

		hdrBytes, err = hdr.Marshal()
		if err != nil {
			log.Println("Unable to marshal header in HandlePacket 🙀")
			return
		}

		// we should probably make the node stuff a separate package
		// do the forwarding part
		fullPacket := append(hdrBytes, message...)

		ft.conn.Send("", fullPacket)
	}
}
