package tcp

import (
	"errors"
	"fmt"
	"log"
)

var ErrReadShutdown = errors.New("read from socket closed")

// we need to write len(buff) bytes from the read buffer to buff
func (conn *TcpConn) VRead(buff []byte) (int, error) {

	sock, ok := state.sockets[*conn]
	if !ok {
		return 0, ErrNoSock
	}

	if !sock.canRead {
		return 0, ErrReadShutdown
	}

	readBuffer := sock.readBuffer
	isNotEmpty := sock.readBufferIsNotEmpty
	sock.readBufferLock.Lock()
	for {
		// we wait until there are more bytes to read
		if readBuffer.IsEmpty() {
			// wait on a condition
			if sock.connState == ESTABLISHED {
				isNotEmpty.Wait()
			} else if sock.connState == CLOSE_WAIT {
				sock.readBufferLock.Unlock()
				return 0, ErrReadShutdown
			} else {
				fmt.Println("other state: ", sock.connState)
			}
		} else {
			bytesRead, err := readBuffer.Read(buff)
			sock.readBufferLock.Unlock()
			if err != nil {
				log.Println("error in VRead: ", err)
				return 0, err
			}
			return bytesRead, nil
		}
	}
}
