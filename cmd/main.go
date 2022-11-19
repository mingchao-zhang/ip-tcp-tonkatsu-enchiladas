package main

import (
	"bufio"
	"container/heap"
	"fmt"
	"ip/pkg/applications/tcp"
	"log"
	"os"
)

func main() {
	// Some items and their priorities.
	items := map[string]int{
		"banana": 3, "apple": 2, "pear": 4,
	}

	// Create a priority queue, put the items in it, and
	// establish the priority queue (heap) invariants.
	pq := make(tcp.PriorityQueue, len(items))
	i := 0
	for value, priority := range items {
		pq[i] = &tcp.Item{
			Value:    value,
			Priority: priority,
			Index:    i,
		}
		i++
	}
	heap.Init(&pq)

	// Insert a new item and then modify its priority.
	item := &tcp.Item{
		Value:    "orange",
		Priority: 1,
	}
	heap.Push(&pq, item)
	pq.Update(item, item.Value, 5)

	// Take the items out; they arrive in decreasing priority order.
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*tcp.Item)
		fmt.Printf("%.2d:%s ", item.Priority, item.Value)
	}

	return

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
