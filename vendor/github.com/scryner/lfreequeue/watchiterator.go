package lfreequeue

type watchIterator struct {
	queue *Queue
	quit chan int
}

func (w *watchIterator) Iter() <-chan interface{} {
	c := make(chan interface{})
	go w.watchAndIterate(c)
	return c
}

func (w *watchIterator) watchAndIterate(c chan<- interface{}) {
	for {
		datum, ok := w.queue.Dequeue()

		if !ok {
			notify := w.queue.Watch()

			select {
			case <-notify:
				continue
			case <-w.quit:
				go func() {
					<-notify
				}()
				goto endIteration
			}

		} else {
			c <- datum
		}
	}

endIteration:
	close(c)
}

func (w *watchIterator) Close() {
	go func(){
		w.quit <- 1
	}()
}