package main

import (
	"bufio"
	"fmt"
	app "ip/pkg/applications"
	"ip/pkg/applications/tcp"
	link "ip/pkg/ipinterface"
	"ip/pkg/network"
	"ip/pkg/transport"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/smallnest/ringbuffer"
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
	app.TestProtocolInit(&node.FwdTable)
	app.RIPInit(&node.FwdTable)
	tcp.TCPInit(&node.FwdTable)
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
	} else if words[0] == "send" && len(words) >= 4 {
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
	} else if words[0] == "a" && len(words) == 2 {
		port, err := strconv.Atoi(words[1])
		if err != nil {
			log.Printf("Invalid TCP port: %s", words[1])
		}

		listener, err := tcp.VListen(uint16(port))
		if err != nil {
			log.Println(err)
		}
		go func() {
			for {
				listener.VAccept()
			}
		}()
	} else if words[0] == "c" && len(words) == 3 {
		foreignIP := words[1]
		port, err := strconv.Atoi(words[2])
		if err != nil {
			log.Printf("Invalid TCP port: %s", words[1])
		}
		fmt.Printf("TCP Connecting to %s: %d\n", foreignIP, port)

		_, err = tcp.VConnect(link.IntIPFromString(foreignIP), uint16(port))
		if err != nil {
			log.Println(err)
		}
	} else {
		fmt.Println("Unsupported command")
	}
}

func main() {
	rb := ringbuffer.New(5)

	// write
	rb.Write([]byte("abcde"))
	fmt.Println(rb.Length())
	fmt.Println("Space after adding 5 bytes: ", rb.Free())
	// we can't write 123 until we read abc
	rb.Write([]byte("123"))

	bite, _ := rb.ReadByte()
	fmt.Printf("%s\n", []byte{bite})
	rb.Write([]byte("123"))

	buf := make([]byte, 5)

	rb.Read(buf)

	fmt.Printf("After read: %s\n", string(buf))

	// // read
	// rb.Read(buf)
	// fmt.Println(string(buf))
	// fmt.Println(rb.Free())
	// rb.Write([]byte("123"))
	// rb.Read(buf)
	// fmt.Println(string(buf))
	// fmt.Println(rb.Free())

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

	// Watch all channels, act on one when something happens
	for {
		select {
		case text := <-keyboardChan:
			go handleCli(text, &node)
		case buffer := <-listenChan:
			go node.FwdTable.HandlePacketSafe(buffer)
		}
	}
}
