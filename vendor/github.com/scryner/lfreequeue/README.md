# lfreequeue

A lock-free queue implementation for golang

# Installation

	# go get github.com/scryner/lfreequeue

# Usage
## Example 1: simple enqueing & dequeing

	package main

	import (
		"fmt"
		"github.com/scryner/lfreequeue"
	)

	func main() {
		q := lfreequeue.NewQueue()

		v1 := "test_string"
		v2 := 100

		// enqueing
		q.Enqueue(v1)
		q.Enqueue(v2)

		// dequeing
		r1 := q.Dequeue()
		r2 := q.Dequeue()

		fmt.Println(r1, r2)
	}

## Example 2: iterator
lfreequeue provides iterator. You can use iterator just as a common 'for' loop statement. 

	q := lfreequeue.NewQueue()

	q.Enqueue("a string")
	q.Enqueue(100)

	for v := range q.Iter() {
		switch v2 := v.(type) {
		case string:
			fmt.Println("print string:", v2)
		case int:
			fmt.Println("print int:", v2)
		default:
			fmt.Println("print other:", v2)
		}
	}

## Example 3: watch iterator
Watch iterator is almost same as iterator, except it would not close channel.
It is useful for producer and consumer pattern.

	q := lfreequeue.NewQueue()

	watchIterator := q.NewWatchIterator()
	iter := watchIterator.Iter()
	defer watchIterator.Close()

	// producer
	go func() {
		for i := 0; i < 10; i++ {
			q.Enqueue(i)
		}
	}()

	// consumer
	L:
	for {
		select {
		case v := <-iter:
			fmt.Println("received:", v)

		case <-time.After(1 * time.Second):
			fmt.Println("timeouted!")
			break L
		}
	}

# Author

Seonghwan Jeong &lt;scryner@gmail.com&gt;