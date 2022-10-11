package fwdtable

import (
	"fmt"
	"sync"
)

type HandlerFunc = func([]byte, []interface{})

type fwdTable struct {
	table map[string]string
	lock  *sync.RWMutex
	// uint8 is the protocol number for the application
	applications map[uint8]HandlerFunc
}

var fwdtable fwdTable

func InitFwdTable() {
	fwdtable.table = make(map[string]string)
	fwdtable.lock = new(sync.RWMutex)
}

func AddRecord(ip string, nextHop string) {
	fwdtable.lock.Lock()
	defer fwdtable.lock.Unlock()

	fwdtable.table[ip] = nextHop
}

func RegisterApplication(protocolNum uint8, hf HandlerFunc) {

}

func RemoveRecord(ip string) {
	fwdtable.lock.Lock()
	defer fwdtable.lock.Unlock()

	delete(fwdtable.table, ip)
}

func Print() {
	fwdtable.lock.RLock()
	defer fwdtable.lock.RUnlock()

	for k, v := range fwdtable.table {
		fmt.Printf("key[%s] value[%s]\n", k, v)
	}
}
