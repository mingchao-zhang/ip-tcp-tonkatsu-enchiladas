package ipinterface

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
)

const (
	INTERFACEUP   InterfaceState = true
	INTERFACEDOWN InterfaceState = false
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

type InterfaceState bool

func (i InterfaceState) String() string {
	if i {
		return "up"
	} else {
		return "down"
	}
}

func InterfaceStateFromString(state string) InterfaceState {
	if state == INTERFACEDOWN.String() {
		return INTERFACEDOWN
	} else {
		return INTERFACEUP
	}
}

type IpInterface struct {
	Id          int
	State       InterfaceState
	DestAddr    IntIP
	DestUdpPort uint16

	Ip     IntIP
	DestIp IntIP
}

func (inter *IpInterface) SetState(newState bool) {
	inter.State = InterfaceState(newState)
}

func interfaceMapToSortedArray(interfaceMap map[IntIP]*IpInterface) []*IpInterface {
	var arr []*IpInterface
	for _, v := range interfaceMap {
		arr = append(arr, v)
	}

	sort.Slice(arr, func(i, j int) bool {
		return arr[i].Id < arr[j].Id
	})
	return arr
}

func getInterfacesString(interfaceMap map[IntIP]*IpInterface) string {
	interfaceArr := interfaceMapToSortedArray(interfaceMap)

	res := "id\tstate\tlocal\tremote\tport\n"
	for _, link := range interfaceArr {
		res += fmt.Sprintf("%v\t%v\t%v\t%v\t%v\n", link.Id, link.State, link.Ip, link.DestIp, link.DestUdpPort)
	}
	return res
}

func PrintInterfaces(interfaceMap map[IntIP]*IpInterface) {
	str := getInterfacesString(interfaceMap)
	fmt.Print(str)
}

func PrintInterfacesToFile(interfaceMap map[IntIP]*IpInterface, filename string) {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		log.Fatalln("Error opening the file: ", err)
	}
	str := getInterfacesString(interfaceMap)
	file.Write([]byte(str))
	file.Close()
}
