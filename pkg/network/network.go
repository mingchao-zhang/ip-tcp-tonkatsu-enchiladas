// DISCLAIMER:
// *Safe() functions acquire the lock themselves - all other functions
// will result in unexpected behaviour if called without locking
package network

import (
	"errors"
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/transport"
	"log"
	"os"
	"sort"
	"time"

	"sync"

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
	Next            link.IntIP // next hop VIP
	Cost            uint32
	LastUpdatedTime time.Time
	Mask            link.IntIP
}

func CreateFwdTableEntry(next link.IntIP, cost uint32, lastUpdatedTime time.Time) FwdTableEntry {
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
	EntryMap     map[link.IntIP]FwdTableEntry
	IpInterfaces map[link.IntIP]*link.IpInterface // physical links
	applications map[uint8]HandlerFunc

	conn transport.Transport
	Lock sync.RWMutex
}

func (ft *FwdTable) InitSafe(links []*link.IpInterface, conn transport.Transport) {
	ft.Lock.Lock()
	defer ft.Lock.Unlock()

	// populate EntryMap and IpInterfaces
	ft.IpInterfaces = make(map[link.IntIP]*link.IpInterface)
	ft.EntryMap = make(map[link.IntIP]FwdTableEntry)
	for _, link := range links {
		ft.IpInterfaces[link.DestIp] = link

		// self entry
		ft.EntryMap[link.Ip] = CreateFwdTableEntry(link.Ip, 0, time.Now().Add(time.Hour*48))
	}

	ft.conn = conn
	ft.applications = make(map[uint8]func([]byte, []interface{}))
}

func (ft *FwdTable) getMyInterface(ip link.IntIP) (*link.IpInterface, bool) {

	for _, inter := range ft.IpInterfaces {
		if inter.Ip == ip {
			return inter, true
		}
	}

	return nil, false
}

func (ft *FwdTable) GetIpInterface(nextHopIP link.IntIP) (*link.IpInterface, bool) {
	inter, ok := ft.IpInterfaces[nextHopIP]
	if ok {
		return inter, ok
	} else {
		return nil, ok
	}
}

func (ft *FwdTable) RegisterHandlerSafe(protocolNum uint8, hf HandlerFunc) {
	ft.Lock.Lock()
	defer ft.Lock.Unlock()

	ft.applications[protocolNum] = hf
}

func (ft *FwdTable) SetInterfaceStateSafe(id int, newState link.InterfaceState) {
	ft.Lock.Lock()
	defer ft.Lock.Unlock()

	for k, ipInterface := range ft.IpInterfaces {
		if ipInterface.Id == id {
			// ip interface changed
			if newState != ipInterface.State {
				// goes from up to down
				if newState == link.INTERFACEDOWN {
					// remove both sides of link from map
					delete(ft.EntryMap, ipInterface.DestIp)
					delete(ft.EntryMap, ipInterface.Ip)
				} else if newState == link.INTERFACEUP {
					ft.EntryMap[ipInterface.Ip] = CreateFwdTableEntry(ipInterface.Ip, 0, time.Now().Add(time.Hour*48))
				}
			}

			// Change the state in the link
			ipInterface.State = newState
			ft.IpInterfaces[k] = ipInterface
		}
	}
}

func computeChecksum(b []byte) uint16 {
	checksum := header.Checksum(b, 0)
	checksumInv := checksum ^ 0xffff
	return checksumInv
}

func validateChecksum(b []byte, fromHeader uint16) uint16 {
	checksum := header.Checksum(b, fromHeader)

	return checksum
}

// only call this function if you have already locked
func (ft *FwdTable) SendMsgToDestIP(destIP link.IntIP, procotol int, msg []byte) (err error) {
	var nextHopInterface *link.IpInterface
	var ok bool
	fwdEntry, inFwdEntryMap := ft.EntryMap[destIP]
	if inFwdEntryMap {
		nextHopInterface, ok = ft.GetIpInterface(fwdEntry.Next)
		if !ok {
			err = errors.New("cannot find interface even given the next Hop in SendMsgToDestIP: " + fwdEntry.Next.String())
			return
		}
	} else {
		nextHopInterface, ok = ft.GetIpInterface(destIP)
		if !ok {
			err = errors.New("cannot find interface even given the next Hop in SendMsgToDestIP: " + fwdEntry.Next.String())
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
		Src:      nextHopInterface.Ip.NetIP(),
		Dst:      destIP.NetIP(),
		Options:  []byte{},
	}

	headerBytes, err := hdr.Marshal()
	if err != nil {
		return
	}

	hdr.Checksum = int(computeChecksum(headerBytes))
	headerBytes, err = hdr.Marshal()
	if err != nil {
		return
	}

	fullPacket := append(headerBytes, msg...)
	remoteString := fmt.Sprintf("%v:%v", nextHopInterface.DestAddr.String(), nextHopInterface.DestUdpPort)
	ft.conn.Send(remoteString, fullPacket)

	return nil
}

func (ft *FwdTable) HandlePacketSafe(buffer []byte) (err error) {
	// Verify Checksum
	hdr, err := ipv4.ParseHeader(buffer)
	if err != nil {
		log.Println("Unable to Parse Header in HandlePacket")
		return
	}

	headerSize := hdr.Len
	headerBytes := buffer[:headerSize]
	checksumFromHeader := uint16(hdr.Checksum)
	computedChecksum := validateChecksum(headerBytes, checksumFromHeader)

	if computedChecksum != checksumFromHeader {
		log.Println("Invalid checksum: ", hdr)
		return errors.New("invalid checksum")
	}

	destIP := link.IntIPFromNetIP(hdr.Dst)
	msgBytes := buffer[hdr.Len:hdr.TotalLen]

	ft.Lock.RLock()
	myInterface, ok := ft.getMyInterface(destIP)
	ft.Lock.RUnlock()

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
		if !ok {
			// log.Println("Don't know how to get to this destination: ", destIP)
			ft.Lock.RUnlock()
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
			ft.Lock.RUnlock()
			return errors.New("unable to marshal header")
		}

		// Recompute checksum with ttl decremented
		hdr.Checksum = int(computeChecksum(newHdrBytes))
		newHdrBytes, err = hdr.Marshal()
		if err != nil {
			log.Println("Unable to marshal header in HandlePacket 🙀")
			ft.Lock.RUnlock()
			return errors.New("unable to marshal header")
		}

		nextHopLink, ok := ft.IpInterfaces[nextHopEntry.Next]
		ft.Lock.RUnlock()
		if !ok {
			fmt.Println("Shouldn't happening: nextHop can't be found in IpInterfaces")
		}

		remoteString := fmt.Sprintf("%v:%v", nextHopLink.DestAddr, nextHopLink.DestUdpPort)

		fullPacket := append(newHdrBytes, msgBytes...)
		ft.conn.Send(remoteString, fullPacket)
	}
	return nil

}

// -----------------------------------------------------------------------------
func (ft *FwdTable) getFwdTableEntriesString() *string {
	var destIPs []link.IntIP
	for k := range ft.EntryMap {
		destIPs = append(destIPs, k)
	}
	sort.Slice(destIPs, func(i, j int) bool { return destIPs[i] < destIPs[j] })

	res := "dest               next       cost\n"
	for _, destIP := range destIPs {
		fwdEntry := ft.EntryMap[destIP]
		res += fmt.Sprintf("%s     %s    %d\n", destIP, fwdEntry.Next, fwdEntry.Cost)
	}
	return &res
}

func (ft *FwdTable) PrintFwdTableEntriesSafe() {
	ft.Lock.RLock()
	defer ft.Lock.RUnlock()

	entriesStr := ft.getFwdTableEntriesString()
	fmt.Print(*entriesStr)
}

func (ft *FwdTable) PrintFwdTableEntriesToFileSafe(filename string) {
	ft.Lock.RLock()
	defer ft.Lock.RUnlock()
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		log.Fatalln("Error opening the file: ", err)
	}
	str := ft.getFwdTableEntriesString()
	file.Write([]byte(*str))
	file.Close()
}
