package ipinterface

const (
	INTERFACEUP   InterfaceState = true
	INTERFACEDOWN InterfaceState = false
)

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
