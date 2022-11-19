package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
)

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
