package task

import (
	"testing"
	"time"
)

func TestRunTaskRepeateadly(t *testing.T) {
	t.Parallel()

	counter := 0
	ping := func() { counter++ }

	stopTask := RunTaskRepeateadly(ping, 50*time.Millisecond)
	time.Sleep((3*50 + 30) * time.Millisecond)
	stopTask()
	time.Sleep(30 * time.Millisecond)

	if counter != 3 {
		t.Errorf("Expect task to run 3 times but got %d", counter)
	}
}
