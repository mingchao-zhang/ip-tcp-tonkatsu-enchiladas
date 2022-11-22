package tcp

import (
	"errors"
	"log"

	"github.com/smallnest/ringbuffer"
)

func VWrite(socketId int, buff []byte) (int, error) {
	if len(buff) == 0 {
		return 0, nil
	}

	sock := getSocketById(socketId)
	if sock == nil {
		return 0, errors.New("v_write() error: Bad file descriptor")
	}
	writeBuffer := sock.writeBuffer
	totalBytesWritten := 0
	isNotFull := sock.writeBufferIsNotFull
	isNotEmpty := sock.writeBufferIsNotEmpty
	writeBufferLock := sock.writeBufferLock

	writeBufferLock.Lock()

	for totalBytesWritten < len(buff) {
		// we wait until there is available space to write
		if writeBuffer.IsFull() {
			isNotFull.Wait()
		} else {
			bytesWritten, err := writeBuffer.Write(buff[totalBytesWritten:])
			if err != nil && err != ringbuffer.ErrTooManyDataToWrite {
				log.Println("error in VWrite: ", err)
				writeBufferLock.Unlock()
				return totalBytesWritten, err
			}
			totalBytesWritten += bytesWritten
			// we need to broadcast that something is in the buffer everytime we write something to the buffer
			// if we only signal once, it is possible that we go to sleep (waiting on the buffer to not be full)
			// and then handlewrite is never woken up and we 💀🔒
			isNotEmpty.Broadcast()
		}
	}
	// signal that we put something in the buffer
	writeBufferLock.Unlock()

	if totalBytesWritten != len(buff) {
		log.Fatalln("VWrite wrote too many bytes 💀")
	}

	return totalBytesWritten, nil
}
