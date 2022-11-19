package main

import (
	"fmt"
	"ip/pkg/applications/tcp"
	link "ip/pkg/ipinterface"
	"log"
	"os"
	"strconv"
	"strings"
)

func handleAccept(words []string) {
	// a <port>
	port, err := strconv.Atoi(words[1])
	if err != nil {
		log.Printf("Invalid TCP port: %s", words[1])
		return
	}

	listener, err := tcp.VListen(uint16(port))
	if err != nil {
		log.Println(err)
		return
	}

	// we don't want the server to block cli
	go func() {
		for {
			listener.VAccept()
		}
	}()
}

func handleConnect(words []string) {
	// c <ip> <port>
	foreignIP := words[1]
	port, err := strconv.Atoi(words[2])
	if err != nil {
		log.Printf("Invalid TCP port: %s", words[2])
		return
	}

	go func() {
		_, err = tcp.VConnect(link.IntIPFromString(foreignIP), uint16(port))
		if err != nil {
			log.Println(err)
		}
	}()
}

func handleSendString(words []string) {
	// s <socket ID> <data>
	socketId, err := strconv.Atoi(words[1])
	if err != nil {
		log.Printf("Invalid socket Id: %s", words[1])
		return
	}
	payload := []byte(words[2])

	bytesWritten, err := tcp.VWrite(socketId, payload)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Printf("v_write() on %d bytes returned %d\n", len(payload), bytesWritten)
	}
}

func handleReadString(words []string) {
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

	// v_read() on 2 bytes returned 1; contents of buffer: 'o'
	if block {
		readFn()
	} else {
		go readFn()
	}
	fmt.Println(string(payload))
}

func handleShutdownTcpSocket(words []string) {
	// sd <socket ID> <read|write|both>
	socketId, err := strconv.Atoi(words[1])
	option := words[2]

	if err != nil {
		log.Printf("Invalid socket Id: %s", words[1])
	} else if !(option == "read" || option == "write" || option == "both") {
		log.Println("syntax error (type option must be 'read', 'write', or 'both')")
	} else {
		fmt.Println(socketId)
		if option == "read" {
			fmt.Println("read")
		} else if option == "write" {
			fmt.Println("write")
		} else { // option == "both"
			fmt.Println("both")
		}
	}
}

func handleCloseTcpSocket(words []string) {
	// cl <socket ID>
	socketId, err := strconv.Atoi(words[1])
	if err != nil {
		log.Printf("Invalid socket Id: %s", words[1])
	}
	fmt.Println(socketId)
}

func handleSendFile(words []string) {
	// sf <filename> <ip> <port>
	filename := words[1]
	foreignIP := words[2]
	port, err := strconv.Atoi(words[3])
	if err != nil {
		log.Printf("Invalid TCP port: %s", words[3])
		return
	}

	fmt.Println(filename, foreignIP, port)
}

func handleReadFile(words []string) {
	// rf <filename> <port>
	filename := words[1]
	port, err := strconv.Atoi(words[2])
	if err != nil {
		log.Printf("Invalid TCP port: %s", words[2])
		return
	}
	fmt.Println(filename, port)
}

func handleInput(text string, node *Node) {
	words := strings.Fields(text)
	if len(words) == 0 {
		return
	} else if words[0] == "q" && len(words) == 1 {
		node.close()
		os.Exit(0)
	} else if len(words) == 2 && words[0] == "a" {
		handleAccept(words)
	} else if len(words) == 3 && words[0] == "c" {
		handleConnect(words)
	} else if len(words) == 3 && words[0] == "s" {
		handleSendString(words)
	} else if (len(words) == 3 || len(words) == 4) && words[0] == "r" {
		handleReadString(words)
	} else if len(words) == 1 && words[0] == "ls" {
		fmt.Print(*tcp.GetSocketInfo())
	} else if len(words) == 1 && words[0] == "h" {
		fmt.Println(HELP_MSG)
	} else if len(words) == 3 && words[0] == "sd" {
		handleShutdownTcpSocket(words)
	} else if len(words) == 2 && words[0] == "cl" {
		handleCloseTcpSocket(words)
	} else if len(words) == 4 && words[0] == "sf" {
		handleSendFile(words)

	} else if len(words) == 3 && words[0] == "rf" {
		handleReadFile(words)
	} else {
		handleIPInput(text, node)
	}
}

// IP --------------------------------------------------------------------
func handleIPInput(text string, node *Node) {
	words := strings.Fields(text)
	if words[0] == "interfaces" || words[0] == "li" {
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
	} else {
		fmt.Println("Unsupported command")
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
