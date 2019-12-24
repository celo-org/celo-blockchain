package task

import (
	"testing"
	"time"
)

func TestRunTaskRepeateadly(t *testing.T) {
	counter := 0
	ping := func() { counter++ }

	stopTask := RunTaskRepeateadly(ping, 7*time.Millisecond)
	time.Sleep(25 * time.Millisecond)
	stopTask()
	time.Sleep(25 * time.Millisecond)

	if counter != 3 {
		t.Errorf("Expect task to run 3 times but got %d", counter)
	}
}
