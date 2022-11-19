package main

import (
	"bufio"
	"fmt"
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

func handleInput(text string, node *Node) {
	words := strings.Fields(text)
	if len(words) == 0 {
		return
	} else if words[0] == "q" && len(words) == 1 {
		node.close()
		os.Exit(0)
	} else if words[0] == "interfaces" || words[0] == "li" {
		// we have to lock because we are reading from shared data
		node.FwdTable.Lock.RLock()
		defer node.FwdTable.Lock.RUnlock()
		if len(words) == 1 {
			link.PrintIpInterfaces(node.FwdTable.IpInterfaces)
		} else if len(words) == 2 {
			link.PrintIpInterfacesToFile(node.FwdTable.IpInterfaces, words[1])
		}
	} else if words[0] == "routes" || words[0] == "lr" {
		if len(words) == 1 {
			node.FwdTable.PrintFwdTableEntriesSafe()
		} else if len(words) == 2 {
			node.FwdTable.PrintFwdTableEntriesToFileSafe(words[1])
		}
	} else if words[0] == link.INTERFACEUP.String() || words[0] == link.INTERFACEDOWN.String() {
		if len(words) == 2 {
			interfaceId, err := strconv.Atoi(words[1])
			if err != nil {
				log.Printf("Invalid interface id: %s", words[1])
			} else {
				node.FwdTable.SetInterfaceStateSafe(interfaceId, link.InterfaceStateFromString(words[0]))
			}
		}
	} else if len(words) >= 4 && words[0] == "send" {
		msgStartIdx := len("send") + 1 + len(words[1]) + 1 + len(words[2]) + 1
		msg := text[msgStartIdx:]
		protocolNum, err := strconv.Atoi(words[2])
		if err != nil {
			log.Println("Error converting protocol number: ", err)
			return
		}

		node.FwdTable.Lock.RLock()
		defer node.FwdTable.Lock.RUnlock()
		err = node.FwdTable.SendMsgToDestIP(link.IntIPFromString(words[1]), uint8(protocolNum), []byte(msg))
		if err != nil {
			log.Printf("Error: %v\nUnable to send message \"%v\" to %v\n", err, msg, words[1])
		}
	} else if len(words) == 2 && words[0] == "a" {
		port, err := strconv.Atoi(words[1])
		if err != nil {
			log.Printf("Invalid TCP port: %s", words[1])
		}

		listener, err := tcp.VListen(uint16(port))
		if err != nil {
			log.Println(err)
		}
		// we don't want the server to block cli
		go func() {
			for {
				listener.VAccept()
			}
		}()
	} else if len(words) == 3 && words[0] == "c" {
		foreignIP := words[1]
		port, err := strconv.Atoi(words[2])
		if err != nil {
			log.Printf("Invalid TCP port: %s", words[1])
		}
		fmt.Printf("TCP Connecting to %s: %d\n", foreignIP, port)

		go func() {
			_, err = tcp.VConnect(link.IntIPFromString(foreignIP), uint16(port))
			if err != nil {
				log.Println(err)
			}
		}()
	} else if len(words) == 3 && words[0] == "s" { // SEND
		// s <socket ID> <data>
		socketId, err := strconv.Atoi(words[1])
		if err != nil {
			log.Printf("Invalid socket Id: %s", words[1])
		}
		payload := []byte(words[2])

		tcp.VWrite(socketId, payload)
	} else if (len(words) == 3 || len(words) == 4) && words[0] == "r" { // WRITE
		// r <socket ID> <numbytes> <y|N>
		socketId, err := strconv.Atoi(words[1])
		if err != nil {
			log.Printf("Invalid socket Id: %s", words[1])
		}
		bytesToRead, err := strconv.Atoi(words[2])
		if err != nil {
			log.Printf("Invalid bytesToRead: %s", words[2])
		}
		block := false
		if len(words) == 4 && (words[3] == "y" || words[3] == "Y") {
			block = true
		}

		payload := make([]byte, bytesToRead)

		readFn := func() {
			tcp.VRead(socketId, payload)
			fmt.Println(string(payload))
		}

		if block {
			bytesRead := 0
			for bytesRead != bytesToRead {
				nRead, err := tcp.VRead(socketId, payload)
				if err != nil {
					fmt.Printf("Error while reading from socket %v: %v\n", socketId, err)
					break
				}
				bytesRead += nRead
			}
		} else {
			go readFn()
		}
		fmt.Println(string(payload))
	} else if len(words) == 1 && words[0] == "ls" {
		fmt.Print(*tcp.GetSocketInfo())
	} else if len(words) == 1 && words[0] == "h" {
		fmt.Println(HELP_MSG)
	} else {
		fmt.Println("Unsupported command")
	}
}

func main() {
	if len(os.Args) != 2 {
		log.Println("Incorrect number of arguments. Correct usage: node <linksfile>")
		os.Exit(1)
	}

	listenChan := make(chan []byte, 1)
	var node Node
	if node.init(os.Args[1], &listenChan) != 0 {
		os.Exit(1)
	}

	// read from stdin
	keyboardChan := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			keyboardChan <- line
		}
	}()

	// start receiving udp packets
	go node.Conn.Recv()

	go func() {
		for {
			buffer := <-listenChan
			go node.FwdTable.HandlePacketSafe(buffer)
		}
	}()

	// Watch all channels, act on one when something happens
	for {
		fmt.Printf(">>> ")
		text := <-keyboardChan
		handleInput(text, &node)
	}
}

const (
	HELP_MSG = `
    Commands:
    a <port>                       - Spawn a socket, bind it to the given port,
                                     and start accepting connections on that port.
    c <ip> <port>                  - Attempt to connect to the given ip address,
                                     in dot notation, on the given port.
    s <socket> <data>              - Send a string on a socket.
    r <socket> <numbytes> [y|n]    - Try to read data from a given socket. If
                                     the last argument is y, then you should
                                     block until numbytes is received, or the
                                     connection closes. If n, then don.t block;
                                     return whatever recv returns. Default is n.
    sf <filename> <ip> <port>      - Connect to the given ip and port, send the
                                     entirety of the specified file, and close
                                     the connection.
    rf <filename> <port>           - Listen for a connection on the given port.
                                     Once established, write everything you can
                                     read from the socket to the given file.
                                     Once the other side closes the connection,
                                     close the connection as well.
    sd <socket> [read|write|both]  - v_shutdown on the given socket.
    cl <socket>                    - v_close on the given socket.
    up <id>                        - enable interface with id
    down <id>                      - disable interface with id
    li, interfaces                 - list interfaces
    lr, routes                     - list routing table rows
    ls, sockets                    - list sockets (fd, ip, port, state)
    window <socket>                - lists window sizes for socket
    q, quit                        - no cleanup, exit(0)
    h, help                        - show this help
    `
)
