package main

import (
	"bufio"
	"fmt"
	app "ip/pkg/applications"
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

func (node *Node) init(linkFileName string) int {
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
	node.Conn.Init(udpPort)

	// parse the rest of file to get an array of ip interfaces
	linkId := 0
	var ipInterfaces []link.IpInterface
	for scanner.Scan() {
		words = strings.Fields(scanner.Text())
		link := link.IpInterface{
			Id:          linkId,
			State:       link.INTERFACEUP,
			DestAddr:    words[0],
			DestUdpPort: words[1],
			Ip:          words[2],
			DestIp:      words[3]}
		ipInterfaces = append(ipInterfaces, link)
		linkId += 1
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
		return 1
	}

	// initialize FwdTable with ip interfaces
	node.FwdTable.Init(ipInterfaces, node.Conn)

	// register handlers
	app.TestProtocolInit(&node.FwdTable)
	app.RIPInit(&node.FwdTable)
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
			link.PrintInterfaces(node.FwdTable.IpInterfaces)
		} else if len(words) == 2 {
			link.PrintInterfacesToFile(node.FwdTable.IpInterfaces, words[1])
		}
	} else if words[0] == "routes" || words[0] == "lr" {
		if len(words) == 1 {

		} else if len(words) == 2 {

		}
	} else if words[0] == link.INTERFACEUP || words[0] == link.INTERFACEDOWN {
		if len(words) == 2 {
			interfaceId, err := strconv.Atoi(words[1])
			if err != nil {
				fmt.Printf("Invalid interface id: %s", words[1])
			} else {
				node.FwdTable.SetInterfaceState(interfaceId, words[0])
			}
		}
	} else if words[0] == "send" && len(words) >= 4 {
		msgStartIdx := len("send") + 1 + len(words[1]) + 1 + len(words[2]) + 1
		msg := text[msgStartIdx:]
		protocolNum, err := strconv.Atoi(text)
		if err != nil {
			fmt.Println("Error converting protocol number: ", err)
			return
		}
		go node.FwdTable.SendMsgToDestIP(words[1], protocolNum, []byte(msg))
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
	if node.init(os.Args[1]) != 0 {
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
	listenChan := make(chan []byte)
	go node.Conn.Recv(&listenChan)

	// Watch all channels, act on one when something happens
	for {
		select {
		case text := <-keyboardChan:
			handleCli(text, &node)
		case buffer := <-listenChan:
			go node.FwdTable.HandlePacket(buffer)
		}
	}
}
