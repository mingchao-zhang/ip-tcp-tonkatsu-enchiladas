package tcp

import (
	"errors"
	"fmt"
)

// VListen is a part of the network API available to applications
// Callers do not need to lock
func VListen(port uint16) (*TcpListener, error) {
	fmt.Printf("opening a new listener: %v:%d\n", state.myIP, port)

	state.lock.Lock()
	defer state.lock.Unlock()

	_, ok := state.ports[port]
	if ok {
		return nil, errors.New("vlisten: port already in use")
	}

	// at this point we know that the port is unused
	state.ports[port] = true
	state.listeners[port] = &TcpListener{
		ip:   state.myIP,
		port: port,
		ch:   make(chan *TcpPacket),
		stop: make(chan bool),
	}

	return state.listeners[port], nil
}
