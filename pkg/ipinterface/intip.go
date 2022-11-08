package ipinterface

import (
	"encoding/binary"
	"net"
)

type IntIP uint32

func (ip IntIP) NetIP() net.IP {
	ipStruct := make(net.IP, 4)
	binary.BigEndian.PutUint32(ipStruct, uint32(ip))
	return ipStruct
}

func IntIPFromNetIP(ip net.IP) IntIP {
	return IntIP(binary.BigEndian.Uint32(ip.To4()))
}

func (ip IntIP) String() string {
	// convert to IP struct and use its string method
	return ip.NetIP().String()
}

func IntIPFromString(ip string) IntIP {
	if ip == "localhost" {
		ip = "127.0.0.1"
	}
	ipStruct := net.ParseIP(ip)
	return IntIPFromNetIP(ipStruct)
}
