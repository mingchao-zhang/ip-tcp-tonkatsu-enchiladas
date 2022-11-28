package tcp

import "errors"

func (conn *TcpConn) VShutdown(sdType int) error {
	sock := state.sockets[*conn]

	if sdType == SHUTDOWN_READ|SHUTDOWN_WRITE {
		if !sock.canRead && !sock.canWrite {
			return errors.New("socket read and write already shutdown")
		}
		if !sock.canRead {
			return errors.New("socket read already shutdown")
		}
		if !sock.canWrite {
			return errors.New("socket write already shutdown")
		}
		sock.canRead = false
		sock.canWrite = false
	} else if sdType&SHUTDOWN_READ != 0 {
		if sock.canRead {
			sock.canRead = false
		} else {
			return errors.New("socket read already shutdown")
		}
	} else if sdType&SHUTDOWN_WRITE != 0 {
		if sock.canWrite {
			sock.canWrite = false
		} else {
			return errors.New("socket write already shutdown")
		}
	}

	return nil
}
