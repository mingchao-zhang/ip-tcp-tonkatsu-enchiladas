package ipinterface

import (
	"fmt"
	"log"
	"os"
	"sort"
)

const (
	INTERFACEUP   = "up"
	INTERFACEDOWN = "down"
)

type IpInterface struct {
	Id          int
	State       string
	DestAddr    string
	DestUdpPort string

	Ip     string
	DestIp string
}

func (inter *IpInterface) SetState(newState string) {
	if newState == INTERFACEUP || newState == INTERFACEDOWN {
		inter.State = newState
	}
}

func interfaceMapToSortedArray(interfaceMap map[string]IpInterface) []IpInterface {
	var arr []IpInterface
	for _, v := range interfaceMap {
		arr = append(arr, v)
	}

	sort.Slice(arr, func(i, j int) bool {
		return arr[i].Id < arr[j].Id
	})
	return arr
}

func getInterfacesString(interfaceMap map[string]IpInterface) *string {
	interfaceArr := interfaceMapToSortedArray(interfaceMap)

	res := "id  state    local          remote        port\n"
	for _, link := range interfaceArr {
		res += fmt.Sprintf("%d    %s      %s    %s   %s\n", link.Id, link.State, link.Ip, link.DestIp, link.DestUdpPort)
	}
	return &res
}

func PrintInterfaces(interfaceMap map[string]IpInterface) {
	str := getInterfacesString(interfaceMap)
	fmt.Print(*str)
}

func PrintInterfacesToFile(interfaceMap map[string]IpInterface, filename string) {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		log.Fatalln("Error opening the file: ", err)
	}
	str := getInterfacesString(interfaceMap)
	file.Write([]byte(*str))
	file.Close()
}
