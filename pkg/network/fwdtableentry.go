package network

import (
	link "ip/pkg/ipinterface"
	"time"
)

// -----------------------------------------------------------------------------
// Route
type FwdTableEntry struct {
	Next            link.IntIP // next hop VIP
	Cost            uint32
	LastUpdatedTime time.Time
	Mask            link.IntIP
}

func CreateFwdTableEntry(next link.IntIP, cost uint32, lastUpdatedTime time.Time) FwdTableEntry {
	return FwdTableEntry{
		Next:            next,
		Cost:            cost,
		LastUpdatedTime: lastUpdatedTime,
		Mask:            0xffffffff,
	}
}
