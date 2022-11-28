package tcp

import (
	"encoding/binary"
	"ip/pkg/ipinterface"
	"net"
	"time"

	"github.com/google/netstack/tcpip/header"
)

const (
	TcpPseudoHeaderLen = 12
	IpProtoTcp         = header.TCPProtocolNumber
)

// Safe
func GetSocketById(id int) *TcpSocket {
	state.lock.Lock()
	defer state.lock.Unlock()

	for _, sock := range state.sockets {
		if sock.sockId == id {
			return sock
		}
	}

	return nil
}

// allocatePort unsafely (without locking)
// returns an unused port and modify state
func allocatePortUnsafe() uint16 {

	// find the port we could use
	_, inMap := state.ports[state.nextUnusedPort]
	for inMap {
		state.nextUnusedPort += 1
		_, inMap = state.ports[state.nextUnusedPort]
	}

	res := state.nextUnusedPort
	state.nextUnusedPort += 1
	return res
}

func deleteConnSafe(conn *TcpConn) {
	state.lock.Lock()
	delete(state.sockets, *conn)
	state.lock.Unlock()
}

func minTime(a time.Duration, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func maxTime(a time.Duration, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func min(a int, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func sendTcp(foreignIP ipinterface.IntIP, packetBytes []byte) error {
	state.fwdTable.Lock.RLock()
	defer state.fwdTable.Lock.RUnlock()

	err := state.fwdTable.SendMsgToDestIP(foreignIP, TcpProtocolNum, packetBytes)
	return err
}

func computeTCPChecksum(tcpHdr *header.TCPFields,
	sourceIP net.IP, destIP net.IP, payload []byte) uint16 {

	// Fill in the pseudo header
	pseudoHeaderBytes := make([]byte, TcpPseudoHeaderLen)

	// First are the source and dest IPs.  This function only supports
	// IPv4, so make sure the IPs are IPv4 addresses
	if ip := sourceIP.To4(); ip != nil {
		copy(pseudoHeaderBytes[0:4], ip)
	} else {
		// This error shouldn't ever occur in our project
		// If it did, would it be appropriate to call panic()?
		// No.  If we encounter a packet that has a processing
		// error, we should really just drop the packet, not
		// crash the node!
		panic("Invalid source IP length, only IPv4 supported")
	}

	if ip := destIP.To4(); ip != nil {
		copy(pseudoHeaderBytes[4:8], ip)
	} else {
		panic("Invalid dest IP length, only IPv4 supported")
	}

	// Next, add the protocol number and header length
	pseudoHeaderBytes[8] = uint8(0)
	pseudoHeaderBytes[9] = uint8(IpProtoTcp)

	totalLength := TcpHeaderLen + len(payload)
	binary.BigEndian.PutUint16(pseudoHeaderBytes[10:12], uint16(totalLength))

	// Turn the TcpFields struct into a byte array
	headerBytes := header.TCP(make([]byte, TcpHeaderLen))
	headerBytes.Encode(tcpHdr)

	// Compute the checksum for each individual part and combine To combine the
	// checksums, we leverage the "initial value" argument of the netstack's
	// checksum package to carry over the value from the previous part
	pseudoHeaderChecksum := header.Checksum(pseudoHeaderBytes, 0)
	headerChecksum := header.Checksum(headerBytes, pseudoHeaderChecksum)
	fullChecksum := header.Checksum(payload, headerChecksum)

	// Return the inverse of the computed value,
	// which seems to be the convention of the checksum algorithm
	// in the netstack package's implementation
	return fullChecksum ^ 0xffff
}

func isValidTcpCheckSum(tcpHdr *header.TCPFields, sourceIP net.IP, destIP net.IP, payload []byte) bool {
	tcpChecksumFromHeader := tcpHdr.Checksum // Save original
	tcpHdr.Checksum = 0
	tcpComputedChecksum := computeTCPChecksum(tcpHdr, sourceIP, destIP, payload)

	if tcpComputedChecksum == tcpChecksumFromHeader {
		return true
	} else {
		return false
	}
}
