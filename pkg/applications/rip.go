package applications

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"ip/pkg/network"
	"log"
	"net"
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

// -----------------------------------------------------------------------------
// DONE
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

// DONE
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

func RIPHandler(rawMsg []byte, params []interface{}) {
	hdr := params[0].(*ipv4.Header)
	ripPacket, err := UnmarshalRipPacket(rawMsg)
	if err != nil {
		// not sure what to do if rip packet was invalid
		fmt.Println("Error in unmarshalling rip packet: ", err)
		return
	}

	if ripPacket.command == CommandResponse {
		fmt.Println(hdr, ripPacket)
		// handle response
		// basically we go through each entry in the response and then update our table
		// send out the updated entries to all the neighbours
	} else {
		// handle request
		// we need to get where the request originated from and send an update to that IP
		requestSrc := hdr.Src.String()

		FwdTable.Lock.RLock()
		defer FwdTable.Lock.RUnlock()

		var ripEntries []RipEntry

		for destIP, fwdEntry := range FwdTable.EntryMap {
			cost := fwdEntry.Cost
			// if the next hop is the IP we're getting the request from
			// use PR and set its cost to INFINITY
			if fwdEntry.Next == requestSrc {
				cost = INFINITY
			}

			destIPUint32 := uint32(binary.BigEndian.Uint32(net.ParseIP(destIP)))

			ripEntry := RipEntry{
				cost:   cost,
				destIP: destIPUint32,
				mask:   fwdEntry.Mask,
			}
			ripEntries = append(ripEntries, ripEntry)
		}

		p := RipPacket{
			command: CommandResponse,
			entries: ripEntries,
		}

		packetBytes, err := p.Marshal()
		if err != nil {
			log.Println("Unable to marshal response packet to IP: ", requestSrc, "\nerror: ", err)
		}

		// construct a rip packet, marshal it, use fwdTable to send to srcIP
		err = FwdTable.SendMsgToDestIP(requestSrc, RipProtocolNum, packetBytes)
		if err != nil {
			log.Println("Error from SendMsgToIP: ", err)
		}
	}
}

// func PeriodicUpdate() {
// 	// keep a last updated variable, if now - then > 12, expire
// 	ticker := time.NewTicker(UpdateInterval)

// 	for {
// 		select {
// 		case _ = <-ticker.C:
// 			// loop through the list of interfaces and send an updated version of the RIP table to each
// 			{
// 				ip := "PLACEHOLDER"
// 				SendUpdateToIP(ip, CommandResponse)
// 			}
// 		}
// 	}
// }

// TODO
func RIPInit(fwdTable *network.FwdTable) {
	FwdTable = fwdTable
	FwdTable.RegisterHandler(RipProtocolNum, RIPHandler)
	// send our entry table to all neighbors
	// TODO

	// request entry table info from all neighbors
	for destIp := range FwdTable.IpInterfaces {
		// create rip packet with command == request, num entries == 0
		ripPacket := RipPacket{
			command: CommandRequest,
		}
		rawBytes, err := ripPacket.Marshal()
		if err != nil {
			log.Fatalln("Unable to marshal request in RIPInit: ", err)
		}
		FwdTable.SendMsgToDestIP(destIp, RipProtocolNum, rawBytes)
	}
	// go PeriodicUpdate()
}
