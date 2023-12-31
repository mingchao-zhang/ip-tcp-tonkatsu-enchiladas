package rip

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"
	"log"
	"time"

	"golang.org/x/net/ipv4"
)

const (
	RipEntrySize        = 12
	RipPacketHeaderSize = 4
	CommandRequest      = 1
	CommandResponse     = 2
	INFINITY            = 16
	UpdateInterval      = time.Second * 5
	ExpiryInterval      = time.Second * 1
	EntryStaleAfter     = time.Second * 12
	RipProtocolNum      = 200
)

var ErrMalformedPacket = errors.New("malformed packet")
var FwdTable *network.FwdTable

type RipEntry struct {
	cost   uint32
	destIP uint32
	mask   uint32
}

type RipPacket struct {
	command uint16
	entries []RipEntry
}

func (re RipEntry) String() string {
	return fmt.Sprintf("cost: %v\tdestination IP: %v\n", re.cost, link.IntIP(re.destIP))
}

func (rp RipPacket) String() string {
	var commandTypeStr string
	if rp.command == CommandRequest {
		commandTypeStr = "Request"
	} else if rp.command == CommandResponse {
		commandTypeStr = "Response"
	} else {
		commandTypeStr = "Unknown/Illegal"
	}
	return fmt.Sprintf("command: %v\nnum entries: %v\nentries:\n%v", commandTypeStr, len(rp.entries), rp.entries)
}

// -----------------------------------------------------------------------------
func (p *RipPacket) Marshal() ([]byte, error) {
	buff := new(bytes.Buffer)

	err := binary.Write(buff, binary.BigEndian, p.command)
	if err != nil {
		return nil, err
	}

	err = binary.Write(buff, binary.BigEndian, uint16(len(p.entries)))
	if err != nil {
		return nil, err
	}

	for _, entry := range p.entries {
		err := binary.Write(buff, binary.BigEndian, entry.cost)
		if err != nil {
			return nil, err
		}

		err = binary.Write(buff, binary.BigEndian, entry.destIP)
		if err != nil {
			return nil, err
		}

		err = binary.Write(buff, binary.BigEndian, entry.mask)
		if err != nil {
			return nil, err
		}
	}

	return buff.Bytes(), nil
}

func UnmarshalRipPacket(rawMsg []byte) (*RipPacket, error) {
	// are there enough bytes for the header?
	// are the remaining bytes a multiple of the size of an entry?
	if len(rawMsg) < RipPacketHeaderSize || (len(rawMsg)-RipPacketHeaderSize)%RipEntrySize != 0 {
		return nil, ErrMalformedPacket
	}

	p := RipPacket{}

	p.command = binary.BigEndian.Uint16(rawMsg[:2])
	if (p.command != CommandRequest) && (p.command != CommandResponse) {
		return nil, ErrMalformedPacket
	}

	numEntries := binary.BigEndian.Uint16(rawMsg[2:4])

	if p.command == CommandRequest && numEntries != 0 {
		return nil, ErrMalformedPacket
	}

	numEntriesInPacket := (len(rawMsg) - RipPacketHeaderSize) / RipEntrySize

	// is the number of entries the same as the entries in the rawMsg?
	if numEntriesInPacket != int(numEntries) || numEntries > 64 {
		return nil, ErrMalformedPacket
	}

	// at this point, we have read the command and the number of entries
	for i := 0; i < numEntriesInPacket; i++ {
		entryOffset := RipPacketHeaderSize + (i * RipEntrySize)
		entryBytes := rawMsg[entryOffset : entryOffset+RipEntrySize]

		entry := RipEntry{}

		entry.cost = binary.BigEndian.Uint32(entryBytes[:4])
		if entry.cost > INFINITY {
			return nil, ErrMalformedPacket
		}
		entry.destIP = binary.BigEndian.Uint32(entryBytes[4:8])
		entry.mask = binary.BigEndian.Uint32(entryBytes[8:])

		p.entries = append(p.entries, entry)
	}

	return &p, nil
}

// unsafe pls lock the fwdtable
func SendRIPResponse(neighborIP link.IntIP) error {
	var ripEntries []RipEntry

	for destIP, fwdEntry := range FwdTable.EntryMap {
		cost := fwdEntry.Cost
		// if the next hop is the IP we're getting the request from
		// use PR and set its cost to INFINITY
		if fwdEntry.Next == neighborIP {
			cost = INFINITY
		}

		re := RipEntry{
			cost:   cost,
			destIP: uint32(destIP),
			mask:   uint32(fwdEntry.Mask),
		}
		ripEntries = append(ripEntries, re)
	}

	p := RipPacket{
		command: CommandResponse,
		entries: ripEntries,
	}

	packetBytes, err := p.Marshal()
	if err != nil {
		log.Println("Unable to marshal response packet to IP: ", neighborIP, "\nerror: ", err)
		return err
	}

	err = FwdTable.SendMsgToDestIP(neighborIP, RipProtocolNum, packetBytes)
	if err != nil {
		return err
	}

	return nil
}

func RIPHandler(rawMsg []byte, params []interface{}) {
	hdr := params[0].(*ipv4.Header)
	ripPacket, err := UnmarshalRipPacket(rawMsg)
	if err != nil {
		log.Fatalln("Error in unmarshalling rip packet: ", err)
		return
	}

	srcIP := link.IntIPFromNetIP(hdr.Src)
	if ripPacket.command == CommandRequest {
		FwdTable.Lock.RLock()
		defer FwdTable.Lock.RUnlock()

		err = SendRIPResponse(srcIP)
		if err != nil {
			log.Printf("Error when responding to a request from %v -- %v", srcIP, err)
		}
	} else { // ripPacket.command == CommandResponse
		// first acquire lock
		FwdTable.Lock.Lock()
		defer FwdTable.Lock.Unlock()

		// update Fwd table entries and keep track of what entry is updated
		updatedEntries := make([]RipEntry, 0)
		for _, entry := range ripPacket.entries {

			destIP := link.IntIP(entry.destIP)

			newCost := entry.cost + 1
			if newCost >= INFINITY {
				continue
			}

			newNextHop := srcIP
			newFwdTableEntry := network.CreateFwdTableEntry(srcIP, newCost, time.Now())
			trigUpdateRIPEntry := RipEntry{
				cost:   newCost,
				destIP: entry.destIP,
				mask:   entry.mask,
			}

			currentFwdEntry, ok := FwdTable.EntryMap[destIP]
			if !ok {
				FwdTable.EntryMap[destIP] = newFwdTableEntry
				updatedEntries = append(updatedEntries, trigUpdateRIPEntry)
			} else {
				currentCost := currentFwdEntry.Cost
				currentNextHop := currentFwdEntry.Next

				if newCost < currentCost {
					FwdTable.EntryMap[destIP] = newFwdTableEntry
					updatedEntries = append(updatedEntries, trigUpdateRIPEntry)
				} else if newCost > currentCost && newNextHop == currentNextHop {
					FwdTable.EntryMap[destIP] = newFwdTableEntry
					updatedEntries = append(updatedEntries, trigUpdateRIPEntry)
				} else if newCost == currentCost && currentNextHop == newNextHop {
					FwdTable.EntryMap[destIP] = newFwdTableEntry
				}
			}
		}

		// trigger updates
		// send updated entries to all neighbors that's not srcIP

		p := RipPacket{
			command: CommandResponse,
			entries: updatedEntries,
		}
		packetBytes, err := p.Marshal()
		if err != nil {
			log.Println("Unable to marshal response packet that's going to send to neighbors", "\nerror: ", err)
		} else {
			for destIP, inter := range FwdTable.IpInterfaces {
				if destIP == srcIP || inter.State == link.INTERFACEDOWN {
					continue
				}

				err = FwdTable.SendMsgToDestIP(destIP, RipProtocolNum, packetBytes)
				if err != nil {
					log.Println("Error sending fwdTable to neighbors: ", err)
				}
			}
		}

		// // send updated entries with poison to srcIP
		// if FwdTable.IpInterfaces[srcIP].State == link.INTERFACEUP {
		// 	updatedEntriesWithPoison := make([]RipEntry, 0)
		// 	for _, ripEntry := range updatedEntries {
		// 		ripEntry.cost = INFINITY
		// 		updatedEntriesWithPoison = append(updatedEntriesWithPoison, ripEntry)
		// 	}

		// 	p = RipPacket{
		// 		command: CommandResponse,
		// 		entries: updatedEntriesWithPoison,
		// 	}

		// 	packetBytes, err = p.Marshal()
		// 	if err != nil {
		// 		log.Println("Unable to marshal response packet that's going to send to srcIP with posion reverse", "\nerror: ", err)
		// 	} else {
		// 		err = FwdTable.SendMsgToDestIP(srcIP, RipProtocolNum, packetBytes)
		// 		if err != nil {
		// 			log.Println("Error sending fwdTable to srcIP: ", err)
		// 		}
		// 	}
		// }
	}
}

func PeriodicExpiry() {
	ticker := time.NewTicker(ExpiryInterval)

	for {
		// wait for ticker to go off
		<-ticker.C
		now := time.Now()
		FwdTable.Lock.Lock()
		toDelete := make([]link.IntIP, 0)
		for destIP, entry := range FwdTable.EntryMap {
			if (now.Sub(entry.LastUpdatedTime) > EntryStaleAfter) || (entry.Cost >= INFINITY) {
				toDelete = append(toDelete, destIP)
			}
		}
		for _, destIP := range toDelete {
			delete(FwdTable.EntryMap, destIP)
		}
		FwdTable.Lock.Unlock()
	}
}

func PeriodicUpdate() {
	// keep a last updated variable, if now - then > 12, expire
	ticker := time.NewTicker(UpdateInterval)

	for {
		// wait for ticker to go off
		// loop through the list of interfaces and send an updated version of the RIP table to each
		FwdTable.Lock.RLock()
		for _, inter := range FwdTable.IpInterfaces {
			if inter.State == link.INTERFACEUP {
				neighborIP := inter.DestIp
				err := SendRIPResponse(neighborIP)
				if err != nil {
					log.Printf("Unable to send Periodic Update to: %v\nError: %v\n", neighborIP, err)
				}
			}
		}
		FwdTable.Lock.RUnlock()
		<-ticker.C
	}
}

func RIPInit(fwdTable *network.FwdTable) {
	FwdTable = fwdTable
	FwdTable.RegisterHandlerSafe(RipProtocolNum, RIPHandler)

	// request entry table info from all neighbors
	ripPacket := RipPacket{
		command: CommandRequest,
	}
	rawBytes, err := ripPacket.Marshal()
	if err != nil {
		log.Fatalln("Unable to marshal request in RIPInit: ", err)
	}
	fwdTable.Lock.RLock()
	for destIp := range FwdTable.IpInterfaces {
		// create rip packet with command == request, num entries == 0

		FwdTable.SendMsgToDestIP(destIp, RipProtocolNum, rawBytes)
	}
	fwdTable.Lock.RUnlock()

	// add function to remove stale entries from fwdtable perioridically (every 2 seconds or so)

	go PeriodicUpdate()
	go PeriodicExpiry()
}
