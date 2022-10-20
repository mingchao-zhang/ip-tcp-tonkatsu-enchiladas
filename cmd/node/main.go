package main

import (
	"bufio"
	"fmt"
	network "ip/pkg/network"
	transport "ip/pkg/transport"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/google/netstack/tcpip/header"
	"golang.org/x/net/ipv4"
)

const (
	UDPADDR    = "localhost"
	MAXMSGSIZE = 1400
	STATEUP    = "up"
	STATEDOWN  = "down"
)

type Node struct {
	Name      string
	Links     []network.Link
	Transport transport.Transport
	FwdTable  network.FwdTable
}

func (node *Node) close() {
	node.Transport.Conn.Close()
}

func (node *Node) getInterfacesString() *string {
	res := "id  state    local       remote      port\n"
	for _, link := range node.Links {
		res += fmt.Sprintf("%d    %s      %s    %s   %s\n", link.Id, link.State, link.InterfaceIP, link.DestIP, link.DestUdpPort)
	}
	return &res
}

func (node *Node) printInterfaces() {
	str := node.getInterfacesString()
	fmt.Print(*str)
}

func (node *Node) printInterfacesToFile(filename string) {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		log.Fatalln("Error opening the file: ", err)
	}
	str := node.getInterfacesString()
	file.Write([]byte(*str))
	file.Close()
}

func (node *Node) setInterfaceState(id int, state string) {
	for i := range node.Links {
		link := &node.Links[i]
		if link.Id == id {
			link.State = state
		}
	}
}

func (n *Node) HandlePacket(buffer []byte) {

	hdr, err := ipv4.ParseHeader(buffer)
	if err != nil {
		fmt.Println("Error parsing the ip header: ", err)
		return
	}

	fmt.Println(hdr.Len)
	oldChecksum := hdr.Checksum
	//hdr.Checksum = 0
	tempBuffer := make([]byte, hdr.Len)
	copy(tempBuffer, buffer[:hdr.Len])
	fmt.Printf("Buffer %d: %v\n", len(tempBuffer), tempBuffer)
	checksum := header.Checksum(tempBuffer, 0)
	fmt.Println(checksum)
	checksum ^= 0xffff
	fmt.Println(checksum)
	if checksum != uint16(oldChecksum) {
		fmt.Println("Correct checksum: ", checksum)
		fmt.Println("Incorrect checksum: ", oldChecksum)
		return
	}

	headerSize := hdr.Len
	msg := buffer[headerSize:]

	fmt.Printf("Received IP packet. Header:  %v\nMessage:  %s\n", hdr, string(msg))
}

func initializeNode(filename string, node *Node) int {
	// parse the input link file and populate the link array
	file, err := os.Open(filename)
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
	// resolve udp4 address
	listenString := fmt.Sprintf(":%s", udpPort)
	listenAddr, err := net.ResolveUDPAddr("udp4", listenString)
	if err != nil {
		log.Fatal("Error resolving udp address: ", err)
	}
	// create connections
	node.Transport = transport.Transport{}
	node.Transport.Conn, err = net.ListenUDP("udp4", listenAddr)
	if err != nil {
		log.Fatal("Cannot create the udp connection: ", err)
	}

	// parse the rest of file and populate node.Links
	linkId := 0
	for scanner.Scan() {
		words = strings.Fields(scanner.Text())
		link := network.Link{
			Id:          linkId,
			State:       STATEUP,
			DestAddr:    words[0],
			DestUdpPort: words[1],
			InterfaceIP: words[2],
			DestIP:      words[3]}
		node.Links = append(node.Links, link)
		linkId += 1
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
		return 1
	}

	// initialize FwdTable
	node.FwdTable.Init(node.Links)
	return 0
}

func handleCli(text string, node *Node) {
	words := strings.Fields(text)
	if len(words) == 0 {
		return
	} else if words[0] == "q" && len(words) == 1 {
		node.close()
		os.Exit(0)
	} else if words[0] == "interfaces" || words[0] == "li" {
		if len(words) == 1 {
			node.printInterfaces()
		} else if len(words) == 2 {
			node.printInterfacesToFile(words[1])
		}
	} else if words[0] == "routes" || words[0] == "lr" {
		if len(words) == 1 {

		} else if len(words) == 2 {

		}
	} else if words[0] == STATEUP || words[0] == STATEDOWN {
		if len(words) == 2 {
			id, err := strconv.Atoi(words[1])
			if err != nil {
				fmt.Printf("Invalid interface id: %s", words[1])
			} else {
				node.setInterfaceState(id, words[0])
			}
		}
	} else {
		fmt.Println("Unsupported command")
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Incorrect number of arguments. Correct usage: node <linksfile>")
		os.Exit(1)
	}

	var node Node
	if initializeNode(os.Args[1], &node) != 0 {
		os.Exit(1)
	}

	// set up channels
	keyboardChan := make(chan string)
	listenChan := make(chan []byte)

	// read input from stdin
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			keyboardChan <- line
		}
	}()

	go node.Transport.Recv(&listenChan)

	// Watch all channels, act on one when something happens
	for {
		select {
		case text := <-keyboardChan:
			handleCli(text, &node)
		case buffer := <-listenChan:
			go node.HandlePacket(buffer)
		}
	}

	// Start listening on our port
	// -> When we get a packet, call fwdTable to hanlde the packet

}
