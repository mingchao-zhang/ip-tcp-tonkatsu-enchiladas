package tcp

import (
	"log"
	"time"
)

// var bufferSizeLeft = BufferSize

func (conn *TcpConn) VRead(buff []byte) (int, error) {
	// validate TCP checksum
	// TODO

	// we need to write len(buff) bytes from the read buffer to buff

	bytesToRead := len(buff)
	state.lock.RLock()
	sock := state.sockets[*conn]
	state.lock.RUnlock()

	readBuffer := sock.readBuffer

	var totalBytesRead = 0

	for totalBytesRead < bytesToRead {
		// we wait until there are more bytes to read
		if readBuffer.IsEmpty() {
			time.Sleep(time.Millisecond * 10)
		} else {
			bytesRead, err := readBuffer.Read(buff[totalBytesRead:])
			if err != nil {
				log.Println("error in VRead: ", err)
				return totalBytesRead, err
			}
			totalBytesRead += bytesRead
		}
	}

	if totalBytesRead != bytesToRead {
		log.Fatalln("VRead read too many bytes 💀")
	}

	return totalBytesRead, nil

}
