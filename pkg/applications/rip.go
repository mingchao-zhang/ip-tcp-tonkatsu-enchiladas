package applications

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	EntrySize        = 12
	PacketHeaderSize = 4
	CommandRequest   = 1
	CommandResponse  = 2
)

type Entry struct {
	cost    uint32
	address uint32
	mask    uint32
}

type Packet struct {
	command    uint16
	numEntries uint16
	entries    []Entry
}

type Table struct {
}

var table Table
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

func UnmarshalPacket(packet []byte) (p *Packet, e error) {
	// are there enough bytes for the header?
	// are the remaining bytes a multiple of the size of an entry?
	if len(packet) < PacketHeaderSize || (len(packet)-PacketHeaderSize)%EntrySize != 0 {
		return nil, ErrMalformedPacket
	}

	p.command = binary.BigEndian.Uint16(packet[:2])
	p.numEntries = binary.BigEndian.Uint16(packet[2:4])

	if p.command == 1 && p.numEntries != 0 {
		return nil, ErrMalformedPacket
	}

	numEntriesInPacket := (len(packet) - PacketHeaderSize) / 12

	// is the number of entries the same as the entries in the packet?
	if numEntriesInPacket != int(p.numEntries) {
		return nil, ErrMalformedPacket
	}

	// at this point, we have read the command and the number of entries
	for i := 0; i < numEntriesInPacket; i++ {
		entryOffset := PacketHeaderSize + i*EntrySize
		entryBytes := packet[entryOffset : entryOffset+EntrySize]

		entry := Entry{}

		entry.cost = binary.BigEndian.Uint32(entryBytes[:4])
		entry.address = binary.BigEndian.Uint32(entryBytes[4:8])
		entry.mask = binary.BigEndian.Uint32(entryBytes[8:])

		p.entries = append(p.entries, entry)
	}

	return
}

func Handler(data []byte, params []interface{}) {
	//  handler will take a message and parse it

	// we need to parse data into the
	fmt.Printf("data: %v\n", data)
}

func Init() {

}
