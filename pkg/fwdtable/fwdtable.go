package fwdtable

import (
	"fmt"
	"log"
	"sync"

	"github.com/google/netstack/tcpip/header"
	"golang.org/x/net/ipv4"
)

type HandlerFunc = func([]byte, []interface{})

type fwdTable struct {
	// net.IP
	myIntefaces map[string]bool
	table       map[string]string
	lock        *sync.RWMutex
	// uint8 is the protocol number for the application
	// try sync map
	applications map[uint8]HandlerFunc
}

var fwdtable fwdTable

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

}
