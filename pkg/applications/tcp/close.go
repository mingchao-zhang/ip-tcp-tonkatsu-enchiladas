package tcp

// TODO
func (conn *TcpConn) VClose() error {
	// remove all open sockets and send value on close channel
	// send value on listener close to stop it from waiting on new connections
	conn.VShutdown(SHUTDOWN_BOTH)
	return nil
}
