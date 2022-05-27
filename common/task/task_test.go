package task

import (
	"testing"
	"time"
)

type testTicker struct {
	tc chan time.Time
}

func (t *testTicker) stop() {}

func (t *testTicker) tickChan() <-chan time.Time {
	return t.tc
}
func (t *testTicker) sendTick() {
	t.tc <- time.Now()
}

func TestRunTaskRepeateadly(t *testing.T) {
	t.Parallel()

	counter := 0
	ping := func() { println("tick"); counter++ }

	tt := &testTicker{
		tc: make(chan time.Time),
	}

	stopTask := RunTaskRepeateadly(ping, tt)
	tt.sendTick()
	tt.sendTick()
	tt.sendTick()
	stopTask()

	if counter != 3 {
		t.Errorf("Expect task to run 3 times but got %d", counter)
	}
}
