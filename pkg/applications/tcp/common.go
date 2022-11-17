package tcp

// Safe
func getSocketById(id int) *TcpSocket {
	state.lock.Lock()
	defer state.lock.Unlock()

	for _, sock := range state.sockets {
		if sock.sockId == id {
			return sock
		}
	}

	return nil
}

// allocatePort unsafely (without locking)
// returns an unused port and modify state
func allocatePortUnsafe() uint16 {

	// find the port we could use
	_, inMap := state.ports[state.nextUnusedPort]
	for inMap {
		state.nextUnusedPort += 1
		_, inMap = state.ports[state.nextUnusedPort]
	}

	res := state.nextUnusedPort
	state.nextUnusedPort += 1
	return res
}

func deleteConnSafe(conn *TcpConn) {
	state.lock.Lock()
	delete(state.sockets, *conn)
	state.lock.Unlock()
}

func min(a int, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}
