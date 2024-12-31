package utils

import (
	"sync"
	"time"
)

const defaultTimeout = 60

type Future[T any] struct {
	task    func() T
	resChan chan T
	res     T
	done    bool
	lock    *sync.Mutex
	ticker  *time.Ticker
}

func NewFuture[T any](task func() T, to ...int) *Future[T] {
	killAfterSec := defaultTimeout
	if to != nil {
		killAfterSec = to[0]
	}
	f := &Future[T]{task: task, resChan: make(chan T), lock: &sync.Mutex{}, ticker: time.NewTicker(time.Duration(killAfterSec) * time.Second)}
	go func() {
		res := f.task()
		f.resChan <- res
	}()
	return f
}

func (f *Future[T]) Get() T {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.done {
		return f.res
	}
	for {
		select {
		case f.res = <-f.resChan:
			f.done = true
			return f.res
		case <-f.ticker.C:
			f.done = true
			return f.res
		}
	}
}
