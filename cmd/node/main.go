package main

import "net"

type Node struct {
	Name          string
	Links         []Link
	ListenUDPPort *net.UDPConn
	// AddrPortMap   map[string]string
}

type Link struct {
	SourceIP      string
	DestinationIP string
	SendUDPPort   *net.UDPConn
}

func (n *Node) ListenOnPort(portNum uint16) {

}

func (n *Node) HandlePacket() {

}

func send() {

}

func recv() {

}

// are there functions to read IP packets from a UDP connection

func main() {
	// Read from input file to initialize node:
	// -> Read node connection details (ip, port)
	// -> Read all links (source IP, destination IP, destination port)

	// Start listening on our port
	// -> When we get a packet, call fwdTable to hanlde the packet

	// Start reading from CLI
}
