package tcp

// TODO
func (l *TcpListener) VClose() error {
	// remove the listener from list of listeners
	// remove all open sockets and send value on close channel
	// send value on listener close to stop it from waiting on new connections
	return nil
}
