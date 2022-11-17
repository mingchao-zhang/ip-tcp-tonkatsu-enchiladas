package tcp

import (
	"errors"
	"log"
	"time"
)

func VWrite(socketId int, buff []byte) (int, error) {
	// TODO: validate TCP checksum

	sock := getSocketById(socketId)
	if sock == nil {
		return 0, errors.New("v_write() error: Bad file descriptor")
	}
	writeBuffer := sock.writeBuffer
	totalBytesWritten := 0

	for totalBytesWritten < len(buff) {
		// we wait until there is available space to write
		if writeBuffer.IsFull() {
			time.Sleep(READ_WRITE_SLEEP_TIME)
		} else {
			bytesWritten, err := writeBuffer.Write(buff[totalBytesWritten:])
			if err != nil {
				log.Println("error in VWrite: ", err)
				return totalBytesWritten, err
			}
			totalBytesWritten += bytesWritten
		}
	}

	if totalBytesWritten != len(buff) {
		log.Fatalln("VWrite wrote too many bytes 💀")
	}

	return totalBytesWritten, nil
}
