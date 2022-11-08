package ipinterface

import (
	"fmt"
	"log"
	"os"
	"sort"
)

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

func PrintIpInterfaces(interfaceMap map[IntIP]*IpInterface) {
	str := getInterfacesString(interfaceMap)
	fmt.Print(str)
}

func PrintIpInterfacesToFile(interfaceMap map[IntIP]*IpInterface, filename string) {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		log.Fatalln("Error opening the file: ", err)
	}
	str := getInterfacesString(interfaceMap)
	file.Write([]byte(str))
	file.Close()
}

// PRIVATE ---------------------------------------------------------------------
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
