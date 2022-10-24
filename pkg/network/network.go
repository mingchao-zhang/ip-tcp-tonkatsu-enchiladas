package network

import (
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/transport"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/netstack/tcpip/header"
	"golang.org/x/net/ipv4"
)

type HandlerFunc = func([]byte, []interface{})

// -----------------------------------------------------------------------------
// Route
type FwdTableEntry struct {
	Dest            string // dest VIP
	Next            string // next hop VIP
	Cost            int
	LastUpdatedTime time.Time
	Mask            uint32
}

// -----------------------------------------------------------------------------
// FwdTable
type FwdTable struct {
	EntryMap     map[string]FwdTableEntry
	IpInterfaces map[string]link.IpInterface // physical links
	applications map[uint8]HandlerFunc

	conn transport.Transport
	lock sync.RWMutex
}

// DONE
func (ft *FwdTable) Init(links []link.IpInterface, conn transport.Transport) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	// populate EntryMap and IpInterfaces
	ft.IpInterfaces = make(map[string]link.IpInterface)
	ft.EntryMap = make(map[string]FwdTableEntry)
	for _, link := range links {
		ft.IpInterfaces[link.DestIp] = link

		neighborEntry := FwdTableEntry{
			Dest:            link.DestIp,
			Next:            link.DestIp,
			Cost:            1,
			LastUpdatedTime: time.Time{},
			Mask:            0xffffffff,
		}
		ft.EntryMap[link.DestIp] = neighborEntry

		selfEntry := FwdTableEntry{
			Dest:            link.Ip,
			Next:            link.Ip,
			Cost:            0,
			LastUpdatedTime: time.Now().Add(time.Hour * 42),
			Mask:            0xffffffff,
		}
		ft.EntryMap[link.Ip] = selfEntry
	}

	ft.conn = conn
	ft.applications = make(map[uint8]func([]byte, []interface{}))
}

// DONE
func (ft *FwdTable) hasInterface(nextHopIP string) bool {
	ft.lock.RLock()
	defer ft.lock.RUnlock()

	inter, ok := ft.IpInterfaces[nextHopIP]
	if ok && inter.State == link.INTERFACEUP {
		return true
	}
	return false
}

// DONE
func (ft *FwdTable) GetIpInterface(nextHopIP string) (*link.IpInterface, bool) {
	ft.lock.RLock()
	defer ft.lock.RUnlock()

	inter, ok := ft.IpInterfaces[nextHopIP]
	if ok {
		return &inter, ok
	} else {
		return nil, ok
	}
}

// DONE
func (ft *FwdTable) RegisterHandler(protocolNum uint8, hf HandlerFunc) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	ft.applications[protocolNum] = hf
}

// -----------------------------------------------------------------------------
// TODO
// Need to modify Route as well later
func (ft *FwdTable) SetInterfaceState(id int, state string) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	for k, link := range ft.IpInterfaces {
		if link.Id == id {
			link.State = state
			ft.IpInterfaces[k] = link
		}
	}
}

// TODO
func ComputeChecksum(b []byte) uint16 {
	checksum := header.Checksum(b, 0)
	checksumInv := checksum ^ 0xffff
	return checksumInv
}

// TODO: return err
func (ft *FwdTable) SendMsgToDestIP(destIP string, procotol int, msg []byte) {
	ft.lock.RLock()
	ipInterface, ok := ft.IpInterfaces[destIP]
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
		Protocol: procotol,
		Checksum: 0, // Should be 0 until checksum is computed
		Src:      net.ParseIP(ipInterface.Ip),
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

	fullPacket := append(headerBytes, msg...)
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
	if ft.hasInterface(destIP) {
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
		nextHopLink, ok := ft.IpInterfaces[destIP]
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

// -----------------------------------------------------------------------------
func (ft *FwdTable) AddIpInterface(destIP string, inter *link.IpInterface) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	ft.IpInterfaces[destIP] = *inter
}

func (ft *FwdTable) RemoveIpInterface(destIP string) {
	ft.lock.Lock()
	defer ft.lock.Unlock()

	delete(ft.IpInterfaces, destIP)
}
