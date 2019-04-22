package lfreequeue

import (
	"unsafe"
	"sync/atomic"
)

// private structure
type node struct {
	value interface{}
	next *node
}

type queue struct {
	dummy *node
	tail *node
}

func newQueue() *queue {
	q := new(queue)
	q.dummy = new(node)
	q.tail = q.dummy

	return q
}

func (q *queue) enqueue(v interface{}) {
	var oldTail, oldTailNext *node

	newNode := new(node)
	newNode.value = v

	newNodeAdded := false

	for !newNodeAdded {
		oldTail = q.tail
		oldTailNext = oldTail.next

		if q.tail != oldTail {
			continue
		}

		if oldTailNext != nil {
			atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(oldTail), unsafe.Pointer(oldTailNext))
			continue
		}

		newNodeAdded = atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&oldTail.next)), unsafe.Pointer(oldTailNext), unsafe.Pointer(newNode))
	}

	atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(oldTail), unsafe.Pointer(newNode))
}

func (q *queue) dequeue() (interface{}, bool) {
	var temp interface{}
	var oldDummy, oldHead *node

	removed := false

	for !removed {
		oldDummy = q.dummy
		oldHead = oldDummy.next
		oldTail := q.tail

		if q.dummy != oldDummy {
			continue
		}

		if oldHead == nil {
			return nil, false
		}

		if oldTail == oldDummy {
			atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(oldTail), unsafe.Pointer(oldHead))
			continue
		}

		temp = oldHead.value
		removed = atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.dummy)), unsafe.Pointer(oldDummy), unsafe.Pointer(oldHead))
	}

	return temp, true
}

func (q *queue) iterate(c chan<- interface{}) {
	for {
		datum, ok := q.dequeue()
		if !ok {
			break
		}

		c <- datum
	}
	close(c)
}

func (q *queue) iter() <-chan interface{} {
	c := make(chan interface{})
	go q.iterate(c)
	return c
}

// Public structure
type Queue struct {
	q *queue
	wakeUpNotifiesQueue *queue
}

func NewQueue() *Queue {
	return &Queue{
		q: newQueue(),
		wakeUpNotifiesQueue: newQueue(),
	}
}

func (q *Queue) Enqueue(v interface{}) {
	q.q.enqueue(v)

	// notify
	for notify := range q.wakeUpNotifiesQueue.iter() {
		notify2 := notify.(chan int)

		go func(){
			notify2 <- 1
		}()
	}
}

func (q *Queue) Dequeue() (interface{}, bool) {
	return q.q.dequeue()
}

func (q *Queue) Iter() <-chan interface{} {
	return q.q.iter()
}

func (q *Queue) Watch() <-chan int {
	c := make(chan int)
	q.wakeUpNotifiesQueue.enqueue(c)

	return c
}

func (q *Queue) NewWatchIterator() *watchIterator {
	return &watchIterator{
		queue: q,
		quit: make(chan int),
	}
}