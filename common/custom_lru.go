package common

import (
	"github.com/cornelk/hashmap"
	"github.com/scryner/lfreequeue"
	"time"
)

type CustomLRU struct {
	expirationTime int64 // in milliseconds
	cache *hashmap.HashMap
	invalidationQueue *lfreequeue.Queue
}

type InvalidationQueueEntry struct {
	expirationDate int64 // in milliseconds
	keyToRemove interface{}
}

func currentTimeInMilliseconds() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func NewCustomLRU(expirationTime int64 /* in milliseconds */) *CustomLRU {
	lru := &CustomLRU{
		expirationTime: expirationTime,
		cache: &hashmap.HashMap{},
		invalidationQueue: lfreequeue.NewQueue(),
	}

	go lru.invalidationQueueLoop()

	return lru
}

func (lru *CustomLRU) invalidationQueueLoop() {
	for {
		time.Sleep(100 * time.Millisecond)

		value, ok := lru.invalidationQueue.Dequeue()
		if !ok || value == nil {
			continue
		}

		entry := value.(*InvalidationQueueEntry)
		waitTime := currentTimeInMilliseconds() - entry.expirationDate

		if waitTime > 0 {
			timer := time.NewTimer(time.Duration(waitTime) * time.Millisecond)
			<-timer.C
		}

		lru.cache.Del(entry.keyToRemove)
	}
}

func (lru *CustomLRU) Get(key interface{}) (interface{}, bool) {
	return lru.cache.Get(key)
}

func (lru *CustomLRU) Add(key interface{}, value interface{}) bool {
	result := lru.cache.Insert(key, value)
	lru.invalidationQueue.Enqueue(&InvalidationQueueEntry{
		expirationDate: currentTimeInMilliseconds() + lru.expirationTime,
		keyToRemove: key,
	})
	return result
}