package network

import (
	"fmt"
	"log"
	"sync"

	"github.com/google/netstack/tcpip/header"
	"golang.org/x/net/ipv4"
)

type HandlerFunc = func([]byte, []interface{})

type FwdTable struct {
	// net.IP
	myInterfaces map[string]bool
	table        map[string]string
	lock         *sync.RWMutex
	// uint8 is the protocol number for the application
	// try sync map
	applications map[uint8]HandlerFunc
}

// Takes a list of links
func (ft *FwdTable) InitFwdTable() {
	ft.table = make(map[string]string)
	ft.lock = new(sync.RWMutex)
}

func (ft *FwdTable) AddRecord(ip string, nextHop string) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	ft.table[ip] = nextHop
}

func (ft *FwdTable) RegisterApplication(protocolNum uint8, hf HandlerFunc) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	ft.applications[protocolNum] = hf
}

func (ft *FwdTable) RemoveRecord(ip string) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	delete(ft.table, ip)
}

func (ft *FwdTable) GetRecord(ip string) (nextHop string) {
	return ""
}

func (ft *FwdTable) Print() {
	ft.lock.RLock()
	defer ft.lock.RUnlock()

	for k, v := range ft.table {
		fmt.Printf("key[%s] value[%s]\n", k, v)
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

		// call the handler and return
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

		// we should probably make the node stuff a separate package
		// do the forwarding part
		tpt.Send()
	}
}
