package network

import (
	"fmt"
	link "ip/pkg/ipinterface"
	"log"
	"os"
	"sort"
)

func (ft *FwdTable) getFwdTableEntriesString() *string {
	var destIPs []link.IntIP
	for k := range ft.EntryMap {
		destIPs = append(destIPs, k)
	}
	sort.Slice(destIPs, func(i, j int) bool { return destIPs[i] < destIPs[j] })

	res := "dest               next       cost\n"
	for _, destIP := range destIPs {
		fwdEntry := ft.EntryMap[destIP]
		res += fmt.Sprintf("%s     %s    %d\n", destIP, fwdEntry.Next, fwdEntry.Cost)
	}
	return &res
}

func (ft *FwdTable) PrintFwdTableEntriesSafe() {
	ft.Lock.RLock()
	defer ft.Lock.RUnlock()

	entriesStr := ft.getFwdTableEntriesString()
	fmt.Print(*entriesStr)
}

func (ft *FwdTable) PrintFwdTableEntriesToFileSafe(filename string) {
	ft.Lock.RLock()
	defer ft.Lock.RUnlock()
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		log.Fatalln("Error opening the file: ", err)
	}
	str := ft.getFwdTableEntriesString()
	file.Write([]byte(*str))
	file.Close()
}
