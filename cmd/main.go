package main

import (
	"bufio"
	"container/heap"
	"fmt"
	"ip/pkg/applications/tcp"
	"log"
	"os"
)

func testHeap() {
	pq := make(tcp.PriorityQueue, 3)

	banana := "banana"
	pq[0] = &tcp.Item{
		Value:    &banana,
		Priority: 3,
	}

	apple := "apple"
	pq[1] = &tcp.Item{
		Value:    &apple,
		Priority: 2,
	}

	pear := "pear"
	pq[2] = &tcp.Item{
		Value:    &pear,
		Priority: 4,
	}

	heap.Init(&pq)

	// Insert a new item and then modify its priority.
	orange := "orange"
	item := &tcp.Item{
		Value:    &orange,
		Priority: 1,
	}
	heap.Push(&pq, item)
	pq.Update(item, item.Value, 5)

	// Take the items out; they arrive in decreasing priority order.
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*tcp.Item)
		fmt.Printf("%.2d:%s ", item.Priority, *item.Value)
	}
	fmt.Println("")

	return
}

func main() {
	// testHeap()

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
