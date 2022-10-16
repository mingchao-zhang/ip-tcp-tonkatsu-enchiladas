package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

const (
	UDPADDR    = "localhost"
	MAXMSGSIZE = 1400
	STATEUP    = "up"
	STATEDOWN  = "down"
)

type Node struct {
	Name    string
	Links   []Link
	UdpConn *net.UDPConn
	// AddrPortMap   map[string]string
}

type Link struct {
	DestUdpPort   string
	SourceIP      string
	DestinationIP string
	State         string
	Id            int
}

func (n *Node) close() {
	n.UdpConn.Close()
}

func (node *Node) printInterfaces() {
	fmt.Println("id  state    local       remote      port")
	for _, link := range node.Links {
		fmt.Printf("%d    %s      %s    %s   %s", link.Id, link.State, link.SourceIP, link.DestinationIP, link.DestUdpPort)
	}
}

func (n *Node) printInterfacesToFile(filename *string) {

}

func (n *Node) HandlePacket(packet []byte) {
	//TODO
}

func send(packet []byte, udpPort string) {
	addrString := fmt.Sprintf("%s:%s", UDPADDR, udpPort)
	remoteAddr, err := net.ResolveUDPAddr("udp4", addrString)
	if err != nil {
		log.Panic("Error resolving udp address: ", err)
	}
	conn, err := net.DialUDP("udp4", nil, remoteAddr)
	if err != nil {
		log.Panic("Error establishing udp conn: ", err)
	}
	bytesWritten, err := conn.Write(packet)
	if err != nil {
		log.Panicln("Error writing to socket: ", err)
	}
	fmt.Printf("Sent %d bytes to the udp port %s\n", bytesWritten, udpPort)
	conn.Close()
}

func recv(udpPort string) {
	// resolve udp4 address
	listenString := fmt.Sprintf(":%s", udpPort)
	listenAddr, err := net.ResolveUDPAddr("udp4", listenString)
	if err != nil {
		log.Fatal("Error resolving udp address: ", err)
	}

	// create connections
	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		log.Fatal("Cannot create the udp connection: ", err)
	}

	// read from the udp port
	for {
		buffer := make([]byte, MAXMSGSIZE)
		bytesRead, sourceAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Panicln("Error reading from the udp port: ", err)
		}
		fmt.Printf("Read %d bytes from %s\n", bytesRead, sourceAddr.String())
		// TODO: return buffer
	}
}

// Read from input file to initialize node:
// -> Read node connection details (ip, port)
// -> Read all links (source IP, destination IP, destination port)
// return 0 if no errors
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
	node.UdpConn, err = net.ListenUDP("udp4", listenAddr)
	if err != nil {
		log.Fatal("Cannot create the udp connection: ", err)
	}

	// parse the rest of file and populate node.Links
	linkId := 0
	for scanner.Scan() {
		words = strings.Fields(scanner.Text())
		link := Link{
			DestUdpPort:   words[1],
			SourceIP:      words[2],
			DestinationIP: words[3],
			State:         STATEUP,
			Id:            linkId}
		node.Links = append(node.Links, link)
		linkId += 1
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
		return 1
	}
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
			fmt.Println("To print to the file")
		}
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
	// listenChan := make(chan byte)

	// read input from stdin
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			keyboardChan <- line
		}
	}()

	// Watch all channels, act on one when something happens
	for {
		select {
		case text := <-keyboardChan:
			handleCli(text, &node)
		}
	}

	// Start listening on our port
	// -> When we get a packet, call fwdTable to hanlde the packet

}
