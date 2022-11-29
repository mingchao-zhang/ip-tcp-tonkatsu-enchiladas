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

const (
	ONE_MB = 1 << 20
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
	// modify foreignIP for testing purpose
	if foreignIP == "A" {
		foreignIP = "192.168.0.1"
	} else if foreignIP == "B" {
		foreignIP = "192.168.0.2"
	} else if foreignIP == "C" {
		foreignIP = "192.168.0.4"
	}

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
	payload := words[2]

	msg := []byte(payload)
	if payload == "_big_" {
		msg = make([]byte, tcp.BufferSize*2)
		aByte := []byte("a")[0]
		bByte := []byte("b")[0]
		for i := range msg {
			if i == len(msg)-1 {
				msg[i] = aByte
				break
			}
			msg[i] = bByte
		}
	}

	fmt.Println("The length of the input string is:", len(msg))
	sock := tcp.GetSocketById(socketId)
	go func() {
		bytesWritten, err := sock.GetConn().VWrite(msg)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Printf("v_write() on %d bytes (socket %d) returned %d\n", len(msg), socketId, bytesWritten)
		}
	}()
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

	// v_read() on 2 bytes returned 1; contents of buffer: 'o'
	totalBytesRead := 0
	sock := tcp.GetSocketById(socketId)
	if sock == nil {
		fmt.Printf("socketId %d doesn't exist\n", socketId)
		return
	}
	conn := sock.GetConn()
	if block {
		for totalBytesRead < bytesToRead {
			bytesRead, err := conn.VRead(payload[totalBytesRead:])
			if err != nil {
				fmt.Println("Error while reading:", err)
			}
			totalBytesRead += bytesRead
		}

	} else {
		totalBytesRead, err = conn.VRead(payload)
		if err != nil {
			fmt.Println("Error while reading:", err)
		}
	}
	fmt.Println(string(payload[:totalBytesRead]))
	fmt.Println("VRead() returned ", totalBytesRead)
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

	// connect to the given ip and port
	conn, err := tcp.VConnect(link.IntIPFromString(foreignIP), uint16(port))
	if err != nil {
		log.Println(err)
	}

	// read the file into payload array
	payload, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("cannot open %s\n", filename)
		return
	}

	// send the payload
	go func() {
		bytesWritten, err := conn.VWrite(payload)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Printf("v_write() on %d bytes returned %d\n", len(payload), bytesWritten)
		}
		fmt.Println("Calling VClose")
		conn.VClose()
	}()
}

func handleReadFile(words []string) {
	// rf <filename> <port>
	filename := words[1]
	port, err := strconv.Atoi(words[2])
	if err != nil {
		log.Printf("Invalid TCP port: %s", words[2])
		return
	}

	go func() {
		// accept on the port
		listener, err := tcp.VListen(uint16(port))
		if err != nil {
			log.Println(err)
			return
		}
		conn, err := listener.VAccept()
		if err != nil {
			log.Println(err)
			return
		}

		file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
		if err != nil {
			fmt.Printf("Unable to open file %s: %v", filename, err)
			return
		}
		defer file.Close()

		totalBytesRead := 0
		buffer := make([]byte, tcp.BufferSize)
		for conn.GetState() == tcp.ESTABLISHED {
			bytesRead, err := conn.VRead(buffer)
			if err != nil {
				// TODO: handle the shutdown by shutting down the socket from our end
				if err != tcp.ErrReadShutdown {
					fmt.Println("finished reading in VRead:", err)
				} else if err == tcp.ErrReadShutdown {
					fmt.Println("Finished reading from connection")
					conn.VClose()
				}
			} else {
				// write to file and then increment total bytes read
				bytesWritten, err := file.Write(buffer[:bytesRead])
				if err != nil {
					fmt.Println("Error while writing to file:", err)
					return
				}

				if bytesWritten != bytesRead {
					fmt.Println("Unable to write to file")
					return
				}
				totalBytesRead += bytesRead
			}
		}
	}()

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
		fmt.Print(tcp.GetSocketInfo())
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
	} else if len(words) == 2 && words[0] == "pbs" {
		socketId, err := strconv.Atoi(words[1])
		if err != nil {
			fmt.Printf("Bad socketID \"%s\"\n", words[1])
			return
		}
		tcp.PrintBufferSizes(socketId)
	} else if len(words) == 2 && words[0] == "eaq" {
		socketId, err := strconv.Atoi(words[1])
		if err != nil {
			fmt.Printf("Bad socketID \"%s\"\n", words[1])
			return
		}
		tcp.PrintEarlyArrivalSize(socketId)
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
