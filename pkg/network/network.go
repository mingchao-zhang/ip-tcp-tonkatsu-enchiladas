package network

import (
	"errors"
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/transport"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/google/netstack/tcpip/header"
	"golang.org/x/net/ipv4"
)

const (
	MAX_HOPS = 16
)

type HandlerFunc = func([]byte, []interface{})

// -----------------------------------------------------------------------------
// Route
type FwdTableEntry struct {
	Next            string // next hop VIP
	Cost            uint32
	LastUpdatedTime time.Time
	Mask            uint32
}

func CreateFwdTableEntry(next string, cost uint32, lastUpdatedTime time.Time) FwdTableEntry {
	return FwdTableEntry{
		Next:            next,
		Cost:            cost,
		LastUpdatedTime: lastUpdatedTime,
		Mask:            0xffffffff,
	}
}

// -----------------------------------------------------------------------------
// FwdTable
type FwdTable struct {
	EntryMap     map[string]FwdTableEntry
	IpInterfaces map[string]link.IpInterface // physical links
	applications map[uint8]HandlerFunc

	conn transport.Transport
	Lock sync.RWMutex
}

// DONE
func (ft *FwdTable) Init(links []link.IpInterface, conn transport.Transport) {
	// 	fmt.Println("---Init")
	ft.Lock.Lock()
	defer ft.Lock.Unlock()

	// populate EntryMap and IpInterfaces
	ft.IpInterfaces = make(map[string]link.IpInterface)
	ft.EntryMap = make(map[string]FwdTableEntry)
	for _, link := range links {
		ft.IpInterfaces[link.DestIp] = link

		// self entry
		ft.EntryMap[link.Ip] = CreateFwdTableEntry(link.Ip, 0, time.Now().Add(time.Hour*42))
	}

	ft.conn = conn
	ft.applications = make(map[uint8]func([]byte, []interface{}))
}

// DONE
func (ft *FwdTable) getMyInterface(ip string) (*link.IpInterface, bool) {
	ft.Lock.RLock()
	defer ft.Lock.RUnlock()

	for _, inter := range ft.IpInterfaces {
		if inter.Ip == ip {
			return &inter, true
		}
	}

	return nil, false
}

// DONE
func (ft *FwdTable) GetIpInterface(nextHopIP string) (*link.IpInterface, bool) {
	// 	fmt.Println("---GetIpInterface")
	ft.Lock.RLock()
	defer ft.Lock.RUnlock()

	inter, ok := ft.IpInterfaces[nextHopIP]
	if ok {
		return &inter, ok
	} else {
		return nil, ok
	}
}

// DONE
func (ft *FwdTable) RegisterHandler(protocolNum uint8, hf HandlerFunc) {
	ft.Lock.Lock()
	defer ft.Lock.Unlock()

	ft.applications[protocolNum] = hf
}

// -----------------------------------------------------------------------------
func (ft *FwdTable) SetInterfaceState(id int, newState string) {
	// 	fmt.Println("---SetInterfaceState")
	ft.Lock.Lock()
	defer ft.Lock.Unlock()

	for k, ipInterface := range ft.IpInterfaces {
		if ipInterface.Id == id {
			// other side of link
			destIP := ipInterface.DestIp
			// ip interface changed
			if newState != ipInterface.State {
				// goes from up to down
				if newState == link.INTERFACEDOWN {
					// remove both sides of link from map
					delete(ft.EntryMap, destIP)
					delete(ft.EntryMap, ipInterface.Ip)
				} else if newState == link.INTERFACEUP {
					ft.EntryMap[ipInterface.Ip] = CreateFwdTableEntry(ipInterface.Ip, 0, time.Now())
					ft.EntryMap[destIP] = CreateFwdTableEntry(destIP, 1, time.Now())
				}
			}

			// Change the state in the link
			ipInterface.State = newState
			ft.IpInterfaces[k] = ipInterface
		}
	}
}

func ComputeChecksum(b []byte) uint16 {
	checksum := header.Checksum(b, 0)
	checksumInv := checksum ^ 0xffff
	return checksumInv
}

func ValidateChecksum(b []byte, fromHeader uint16) uint16 {
	checksum := header.Checksum(b, fromHeader)

	return checksum
}

func (ft *FwdTable) SendMsgToDestIP(destIP string, procotol int, msg []byte) (err error) {
	// 	fmt.Println("---SendMsgToDestIP")
	ft.Lock.RLock()
	defer ft.Lock.RUnlock()

	var nextHopInterface *link.IpInterface
	var ok bool
	fwdEntry, inFwdEntryMap := ft.EntryMap[destIP]
	if inFwdEntryMap {
		nextHopInterface, ok = ft.GetIpInterface(fwdEntry.Next)
		if !ok {
			err = errors.New("Cannot find interface even given the next Hop in SendMsgToDestIP: " + fwdEntry.Next)
			return
		}
	} else {
		nextHopInterface, ok = ft.GetIpInterface(destIP)
		if !ok {
			err = errors.New("Cannot find interface even given the next Hop in SendMsgToDestIP: " + fwdEntry.Next)
			return
		}
	}

	hdr := ipv4.Header{
		Version:  4,
		Len:      ipv4.HeaderLen, // Header length is always 20 when no IP options
		TOS:      0,
		TotalLen: ipv4.HeaderLen + len(msg),
		ID:       0,
		Flags:    0,
		FragOff:  0,
		TTL:      MAX_HOPS,
		Protocol: procotol,
		Checksum: 0, // Should be 0 until checksum is computed
		Src:      net.ParseIP(nextHopInterface.Ip),
		Dst:      net.ParseIP(destIP),
		Options:  []byte{},
	}

	headerBytes, err := hdr.Marshal()
	if err != nil {
		return
	}

	hdr.Checksum = int(ComputeChecksum(headerBytes))
	headerBytes, err = hdr.Marshal()
	if err != nil {
		return
	}

	fullPacket := append(headerBytes, msg...)
	remoteString := fmt.Sprintf("%s:%s", nextHopInterface.DestAddr, nextHopInterface.DestUdpPort)
	ft.conn.Send(remoteString, fullPacket)

	return nil
}

func (ft *FwdTable) HandlePacket(buffer []byte) (err error) {
	// 	fmt.Println("---HandlePacket")
	// Verify Checksum
	hdr, err := ipv4.ParseHeader(buffer)
	if err != nil {
		log.Println("Unable to Parse Header in HandlePacket")
		return
	}

	headerSize := hdr.Len
	headerBytes := buffer[:headerSize]
	checksumFromHeader := uint16(hdr.Checksum)
	computedChecksum := ValidateChecksum(headerBytes, checksumFromHeader)

	if computedChecksum != checksumFromHeader {
		log.Println("Invalid checksum: ", hdr)
		return errors.New("invalid checksum")
	}

	destIP := hdr.Dst.String()
	msgBytes := buffer[hdr.Len:hdr.TotalLen]

	myInterface, ok := ft.getMyInterface(destIP)
	if ok {
		if myInterface.State == link.INTERFACEDOWN {
			return
		}
		// we are the destination, call the handler for the appropriate application
		handler, ok := ft.applications[uint8(hdr.Protocol)]
		if !ok {
			log.Println("Invalid protocol number in HandlePacket")
			return errors.New("invalid protocol number in IP header")
		}
		handler(msgBytes, []interface{}{hdr})
	} else {
		// not the destination, forward to next hop
		// what do we do if we don't know a next hop for this destination???
		ft.Lock.RLock()
		nextHopEntry, ok := ft.EntryMap[destIP]
		ft.Lock.RUnlock()
		if !ok {
			log.Println("Don't know how to get to this destination: ", destIP)
			return errors.New("don't have a next hop for this destination")
		}

		hdr.TTL -= 1
		if hdr.TTL <= 0 {
			return
		}

		hdr.Checksum = 0
		newHdrBytes, err := hdr.Marshal()
		if err != nil {
			log.Println("Unable to marshal header in HandlePacket 🙀")
			return errors.New("unable to marshal header")
		}

		// Recompute checksum with ttl decremented
		hdr.Checksum = int(ComputeChecksum(newHdrBytes))
		newHdrBytes, err = hdr.Marshal()
		if err != nil {
			log.Println("Unable to marshal header in HandlePacket 🙀")
			return errors.New("unable to marshal header")
		}

		ft.Lock.RLock()
		nextHopLink, ok := ft.IpInterfaces[nextHopEntry.Next]
		ft.Lock.RUnlock()
		if !ok {
			ft.PrintFwdTableEntries()
			fmt.Println("*******")
			link.PrintInterfaces(ft.IpInterfaces)
		}

		remoteString := fmt.Sprintf("%s:%s", nextHopLink.DestAddr, nextHopLink.DestUdpPort)

		fullPacket := append(newHdrBytes, msgBytes...)
		ft.conn.Send(remoteString, fullPacket)
	}
	return nil

}

// -----------------------------------------------------------------------------
func (ft *FwdTable) getFwdTableEntriesString() *string {
	ft.Lock.RLock()
	defer ft.Lock.RUnlock()

	res := "dest               next       cost\n"
	for destIP, fwdEntry := range ft.EntryMap {
		res += fmt.Sprintf("%s     %s    %d\n", destIP, fwdEntry.Next, fwdEntry.Cost)
	}
	return &res
}

func (ft *FwdTable) PrintFwdTableEntries() {
	// 	fmt.Println("---PrintFwdTableEntries")
	entriesStr := ft.getFwdTableEntriesString()
	fmt.Print(*entriesStr)
}

func (ft *FwdTable) PrintFwdTableEntriesToFile(filename string) {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		log.Fatalln("Error opening the file: ", err)
	}
	str := ft.getFwdTableEntriesString()
	file.Write([]byte(*str))
	file.Close()
}
