package applications

import "fmt"

func TestProtocolHandler(packet []byte, params []interface{}) {
	fmt.Println(string(packet))
}

func TestProtocolInit() {
	// nothing to do
}
