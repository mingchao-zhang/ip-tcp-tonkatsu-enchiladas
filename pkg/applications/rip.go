package applications

import (
	"bytes"
	"encoding/binary"
	"errors"
	"ip/pkg/network"
	"ip/pkg/transport"
	"sync"
	"time"
)

const (
	EntrySize        = 12
	PacketHeaderSize = 4
	CommandRequest   = 1
	CommandResponse  = 2
	INFINITY         = 16
	UpdateInterval   = time.Second * 5
	EntryStaleAfter  = time.Second * 12
)

type Entry struct {
	cost    uint32
	address uint32
	mask    uint32
}

type Packet struct {
	command uint16
	entries []Entry
}

type Table = []Entry

type ripState struct {
	table     Table
	transport transport.Transport
	fwdtable  network.FwdTable
	lock      *sync.RWMutex
}

var state ripState
var ErrMalformedPacket = errors.New("malformed packet")

func (p *Packet) Marshal() ([]byte, error) {
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

		err = binary.Write(buff, binary.BigEndian, entry.address)
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

func SendUpdateToIP(ip string, command uint16) (err error) {
	packet := Packet{
		command: command,
		entries: state.table,
	}
	packetBytes, err := packet.Marshal()
	if err != nil {
		return
	}
	// how do I build up the header here?
	// also need to find the appropriate interface to
	// send the data on - basically the next hop for the packet
	// should be the interface that the packet is to be sent on
	// need to ask Nick about this
	state.transport.Send(ip, packetBytes)

	return
}

func UnmarshalPacket(packet []byte) (*Packet, error) {
	// are there enough bytes for the header?
	// are the remaining bytes a multiple of the size of an entry?
	if len(packet) < PacketHeaderSize || (len(packet)-PacketHeaderSize)%EntrySize != 0 {
		return nil, ErrMalformedPacket
	}

	p := Packet{}

	p.command = binary.BigEndian.Uint16(packet[:2])
	if (p.command != CommandRequest) && (p.command != CommandResponse) {
		return nil, ErrMalformedPacket
	}

	numEntries := binary.BigEndian.Uint16(packet[2:4])

	if p.command == CommandRequest && numEntries != 0 {
		return nil, ErrMalformedPacket
	}

	numEntriesInPacket := (len(packet) - PacketHeaderSize) / 12

	// is the number of entries the same as the entries in the packet?
	if numEntriesInPacket != int(numEntries) || numEntries > 64 {
		return nil, ErrMalformedPacket
	}

	// at this point, we have read the command and the number of entries
	for i := 0; i < numEntriesInPacket; i++ {
		entryOffset := PacketHeaderSize + i*EntrySize
		entryBytes := packet[entryOffset : entryOffset+EntrySize]

		entry := Entry{}

		entry.cost = binary.BigEndian.Uint32(entryBytes[:4])
		if entry.cost > INFINITY {
			return nil, ErrMalformedPacket
		}
		entry.address = binary.BigEndian.Uint32(entryBytes[4:8])
		entry.mask = binary.BigEndian.Uint32(entryBytes[8:])

		p.entries = append(p.entries, entry)
	}

	return &p, nil
}

func RIPHandler(packet []byte, params []interface{}) {
	//  handler will take a message and parse it

	// we need to parse data into the
	// fmt.Printf("data: %v\n", data)

	p, err := UnmarshalPacket(packet)
	if err != nil {
		// not sure what to do if rip packet was invalid
	}

	if p.command == CommandResponse {
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

func RIPInit() {
	// this will need some way to access the fwd table
	// some way to access the node's info (like the interfaces and such)
	// should Init register the handler? I think so

	go PeriodicUpdate()
}
