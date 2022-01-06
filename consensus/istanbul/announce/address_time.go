package announce

import (
	"sync"
	"time"

	"github.com/celo-org/celo-blockchain/common"
)

type AddressTime struct {
	at map[common.Address]time.Time
	mx sync.RWMutex
}

func NewAddressTime() *AddressTime {
	return &AddressTime{
		at: make(map[common.Address]time.Time),
	}
}

func (a *AddressTime) RemoveIf(predicate func(common.Address, time.Time) bool) {
	a.mx.Lock()
	defer a.mx.Unlock()
	for addr, t := range a.at {
		if predicate(addr, t) {
			delete(a.at, addr)
		}
	}
}

func (a *AddressTime) Set(addr common.Address, t time.Time) {
	a.mx.Lock()
	defer a.mx.Unlock()
	a.at[addr] = t
}

func (a *AddressTime) Get(addr common.Address) (time.Time, bool) {
	a.mx.RLock()
	defer a.mx.RUnlock()
	t, ok := a.at[addr]
	return t, ok
}
