package applications

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"ip/pkg/network"
	"time"
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

// -----------------------------------------------------------------------------
func SendUpdateToIP(ip string, command uint16) (err error) {
	// packet := RipPacket{
	// 	command: command,
	// 	entries: state.table,
	// }
	// packetBytes, err := packet.Marshal()

	// if err != nil {
	// fmt.Println(err)
	// 	return
	// }
	// // how do I build up the header here?
	// // also need to find the appropriate interface to
	// // send the data on - basically the next hop for the packet
	// // should be the interface that the packet is to be sent on
	// // need to ask Nick about this
	// state.transport.Send(ip, packetBytes)

	return
}

func RIPHandler(packet []byte, params []interface{}) {
	// hdr := params[0].(*ipv4.Header)
	// fmt.Println(hdr)

	ripPacket, err := UnmarshalRipPacket(packet)
	if err != nil {
		// not sure what to do if rip packet was invalid
		fmt.Println("Error in unmarshalling packet: ", err)
		return
	}

	if ripPacket.command == CommandResponse {
		// handle response
		// basically we go through each entry in the response and then update our table
		// send out the updated entries to all the neighbours
	} else {
		// handle request
		// we need to get where the request originated from and send an update to that IP
		ip := "PLACEHOLDER"
		SendUpdateToIP(ip, CommandResponse)
	}
}

func PeriodicUpdate() {
	// keep a last updated variable, if now - then > 12, expire
	ticker := time.NewTicker(UpdateInterval)

	for {
		select {
		case _ = <-ticker.C:
			// loop through the list of interfaces and send an updated version of the RIP table to each
			{
				ip := "PLACEHOLDER"
				SendUpdateToIP(ip, CommandResponse)
			}
		}
	}
}

// TODO
func RIPInit(fwdTable *network.FwdTable) {
	fwdTable.RegisterHandler(RipProtocolNum, RIPHandler)
	// send our entry table to all neighbors
	// TODO
	// request entry table info from all neighbors
	// TODO
	go PeriodicUpdate()
}
