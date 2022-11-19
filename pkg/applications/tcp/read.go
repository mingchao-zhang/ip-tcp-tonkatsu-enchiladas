package tcp

import (
	"errors"
	"log"
	"time"
)

// we need to write len(buff) bytes from the read buffer to buff
func VRead(socketId int, buff []byte) (int, error) {
	// TODO: validate TCP checksum

	sock := getSocketById(socketId)
	if sock == nil {
		return 0, errors.New("v_read() error: Bad file descriptor")
	}
	readBuffer := sock.readBuffer
	var totalBytesRead = 0

	for totalBytesRead < len(buff) {
		// we wait until there are more bytes to read
		if readBuffer.IsEmpty() {
			time.Sleep(READ_WRITE_SLEEP_TIME)
		} else {
			bytesRead, err := readBuffer.Read(buff[totalBytesRead:])
			if err != nil {
				log.Println("error in VRead: ", err)
				return totalBytesRead, err
			}
			totalBytesRead += bytesRead
		}
	}

	if totalBytesRead != len(buff) {
		log.Fatalln("VRead read too many bytes 💀")
	}

	return totalBytesRead, nil

}
