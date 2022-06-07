package task

import "time"

type StopFn func()

// ticker is an interface which allows us to test RunTaskRepeateadly without
// needing to rely on actual timing which is not perfectly accurate and thus
// makes tests flakey.
type ticker interface {
	stop()
	tickChan() <-chan time.Time
}

func NewDefaultTicker(period time.Duration) *DefaultTicker {
	return &DefaultTicker{*time.NewTicker(period)}
}

// DefaultTicker is an implementation of ticker which simply delegates to
// time.Ticker.
type DefaultTicker struct {
	t time.Ticker
}

func (d *DefaultTicker) tickChan() <-chan time.Time {
	return d.t.C
}
func (d *DefaultTicker) stop() {
	d.t.Stop()
}

func RunTaskRepeateadly(task func(), t ticker) StopFn {
	// Setup the ticker and the channel to signal
	// the ending of the interval
	stop := make(chan struct{})

	go func() {
		for {
			select {
			case <-t.tickChan():
				task()
			case <-stop:
				t.stop()
				return
			}
		}
	}()

	return func() {
		stop <- struct{}{}
	}
}
