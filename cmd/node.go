package main

import (
	"bufio"
	"ip/pkg/applications/rip"
	"ip/pkg/applications/tcp"
	"ip/pkg/applications/testprotocol"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"
	"ip/pkg/transport"
	"log"
	"os"
	"strconv"
	"strings"
)

type Node struct {
	Name     string
	FwdTable network.FwdTable
	Conn     transport.Transport
}

func (node *Node) close() {
	node.Conn.Close()
}

func (node *Node) init(linkFileName string, listenChan *chan []byte) int {
	file, err := os.Open(linkFileName)
	if err != nil {
		log.Fatal(err)
		return 1
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// parse the first line && and start the listen udp connection
	scanner.Scan()
	firstLine := scanner.Text()
	words := strings.Fields(firstLine)
	_, udpPort := words[0], words[1]
	node.Conn.Init(udpPort, listenChan)

	// parse the rest of file to get an array of ip interfaces
	linkId := 0
	var ipInterfaces []*link.IpInterface
	for scanner.Scan() {
		words = strings.Fields(scanner.Text())
		udpPort = words[1]
		udpPortInt, err := strconv.Atoi(udpPort)
		if err != nil {
			log.Fatalf("Unable to convert UDP port to uint16: %v", udpPort)
		}

		link := &link.IpInterface{
			Id:          linkId,
			State:       link.INTERFACEUP,
			DestAddr:    link.IntIPFromString(words[0]),
			DestUdpPort: uint16(udpPortInt),
			Ip:          link.IntIPFromString(words[2]),
			DestIp:      link.IntIPFromString(words[3])}
		ipInterfaces = append(ipInterfaces, link)
		linkId += 1
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
		return 1
	}

	// initialize FwdTable with ip interfaces
	// Safe because it does locking for us
	node.FwdTable.InitSafe(ipInterfaces, node.Conn)

	// register handlers
	testprotocol.TestProtocolInit(&node.FwdTable)
	rip.RIPInit(&node.FwdTable)
	tcp.TCPInit(&node.FwdTable)
	return 0
}
