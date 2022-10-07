package main

import "net"

type Node struct {
	Name          string
	Links         []Link
	ListenUDPPort *net.UDPConn
	FwdTable      map[string]Link
}

type Link struct {
	SourceIP      string
	DestinationIP string
	SendUDPPort   *net.UDPConn
}

func (n *Node) RemoveFwdTableEntry(ip string) {

}

func (n *Node) AddFwdTableEntry(ip string, l Link) {

}

func (n *Node) ListenOnPort(portNum uint16) {

}

func (n *Node) HandlePacket() {

}

// are there functions to read IP packets from a UDP connection

func main() {
	// Read from input file to initialize node:
	// -> Read node connection details (ip, port)
	// -> Read all links (source IP, destination IP, destination port)
	// Start listening on our port
	// -> When we get a packet, call function to handle packet
	// Start reading from CLI
}
