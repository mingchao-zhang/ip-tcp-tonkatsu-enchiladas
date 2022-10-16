package fwdtable

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
func InitFwdTable() {
	fwdtable.table = make(map[string]string)
	fwdtable.lock = new(sync.RWMutex)
}

func AddRecord(ip string, nextHop string) {
	fwdtable.lock.Lock()
	defer fwdtable.lock.Unlock()

	fwdtable.table[ip] = nextHop
}

func RegisterApplication(protocolNum uint8, hf HandlerFunc) {
	fwdtable.lock.Lock()
	defer fwdtable.lock.Unlock()

	fwdtable.applications[protocolNum] = hf
}

func RemoveRecord(ip string) {
	fwdtable.lock.Lock()
	defer fwdtable.lock.Unlock()

	delete(fwdtable.table, ip)
}

func GetRecord(ip string) (nextHop string) {
	return ""
}

func Print() {
	fwdtable.lock.RLock()
	defer fwdtable.lock.RUnlock()

	for k, v := range fwdtable.table {
		fmt.Printf("key[%s] value[%s]\n", k, v)
	}
}

func HandlePacket(hdr *ipv4.Header, message []byte) {
	hdrBytes, err := hdr.Marshal()
	if err != nil {
		log.Println("Unable to marshal header in HandlePacket 🙀")
		return
	}

	// check checksum
	if hdr.Checksum != int(header.Checksum(hdrBytes, 0)) {
		log.Println("Received packet with invalid checksum 😵")
		return
	}

	// check if destination is one of the interfaces on this node
	destIP := hdr.Dst.String()
	_, ok := fwdtable.myInterfaces[destIP]
	if ok {
		// we are the destination, call the handler for the appropriate application
	} else {
		// not the destination, forward to next hop
		// what do we do if we don't know a next hop for this destination???
		nextHop, ok := fwdtable.table[destIP]
		if !ok {
			log.Println("Don't know how to get to this destination 🤷🏾")
		}

		// we should probably make the node stuff a separate package
		node.Send()
	}
}
