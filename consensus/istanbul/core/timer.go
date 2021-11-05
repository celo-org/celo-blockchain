package core

import (
	"sync"
	"time"
)

type Timers struct {
	RoundChange       Timer
	ResendRoundChange Timer
	FuturePreprepare  Timer
}

func NewDefaultTimers() *Timers {
	return &Timers{
		RoundChange:       NewDefaultTimer(),
		ResendRoundChange: NewDefaultTimer(),
		FuturePreprepare:  NewDefaultTimer(),
	}
}

type Timer interface {
	AfterFunc(time.Duration, func())
	Stop()
}

type DefaultTimer struct {
	mu sync.Mutex
	t  *time.Timer
}

func NewDefaultTimer() *DefaultTimer {
	return &DefaultTimer{}
}

func (dt *DefaultTimer) AfterFunc(d time.Duration, f func()) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	dt.t = time.AfterFunc(d, f)
}

func (dt *DefaultTimer) Stop() {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	if dt.t != nil {
		dt.t.Stop()
	}
}
