package tcp

import (
	"log"
	"time"
)

func (conn *TcpConn) VWrite(buff []byte) (int, error) {
	// TODO: validate TCP checksum

	state.lock.RLock()
	sock := state.sockets[*conn]
	state.lock.RUnlock()
	writeBuffer := sock.readBuffer
	totalBytesWritten := 0

	for totalBytesWritten < len(buff) {
		// we wait until there is available space to write
		if writeBuffer.IsFull() {
			time.Sleep(READ_WRITE_SLEEP_TIME)
		} else {
			bytesRead, err := writeBuffer.Write(buff[totalBytesWritten:])
			if err != nil {
				log.Println("error in VRead: ", err)
				return totalBytesWritten, err
			}
			totalBytesWritten += bytesRead
		}
	}

	if totalBytesWritten != len(buff) {
		log.Fatalln("VWrite wrote too many bytes 💀")
	}

	return totalBytesWritten, nil
}
