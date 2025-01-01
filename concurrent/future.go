package concurrent

import (
	"main/funcs"
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

func SupplyAsync[T any](task funcs.Supplier[T]) *Future[T] {
	killAfterSec := defaultTimeout

	f := &Future[T]{task: task, resChan: make(chan T), lock: &sync.Mutex{}, ticker: time.NewTicker(time.Duration(killAfterSec) * time.Second)}
	go func() {
		res := f.task()
		f.resChan <- res
	}()
	return f
}

func (f *Future[T]) Done() bool {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.done
}
func (f *Future[T]) Get() T {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.done {
		return f.res
	}
	f.res = <-f.resChan
	f.done = true
	return f.res
}
