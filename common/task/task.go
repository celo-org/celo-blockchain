package task

import "time"

type StopFn func()

func RunTaskRepeateadly(task func(), period time.Duration) StopFn {
	// Setup the ticket and the channel to signal
	// the ending of the interval
	ticker := time.NewTicker(period)
	stop := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				task()
			case <-stop:
				ticker.Stop()
				return
			}
		}
	}()

	return func() {
		close(stop)
	}
}
