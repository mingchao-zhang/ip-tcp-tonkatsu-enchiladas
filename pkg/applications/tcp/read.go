package tcp

import (
	"errors"
	"log"
)

// we need to write len(buff) bytes from the read buffer to buff
func VRead(socketId int, buff []byte) (int, error) {
	// TODO: validate TCP checksum

	sock := getSocketById(socketId)
	if sock == nil {
		return 0, errors.New("v_read() error: Bad file descriptor")
	}
	readBuffer := sock.readBuffer
	isNotEmpty := sock.readBufferIsNotEmpty
	sock.readBufferLock.Lock()
	for {
		// we wait until there are more bytes to read
		if readBuffer.IsEmpty() {
			// wait on a condition
			isNotEmpty.Wait()
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
