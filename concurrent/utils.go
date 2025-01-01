package concurrent

import (
	"context"
	"main/funcs"
	"sync"
)

func DoInLock(lock *sync.Mutex, cb func()) {
	lock.Lock()
	defer lock.Unlock()
	cb()
}

func DoInLockAndReturn[T any](lock *sync.Mutex, cb funcs.Supplier[T]) T {
	lock.Lock()
	defer lock.Unlock()
	return cb()
}

func WaitForTheConditionToBecomeTrue[T any](resProvider func() T, predicate func(T) bool) T {
	var res T

	for res = resProvider(); !predicate(res); {
		res = resProvider()
	}
	return res
}

func WaitForTheConditionToBecomeTrueAndReturnTransformed[T, K any](resProvider func() T, predicate func(T) bool, transformer func(T) K) K {
	var res T
	for res = resProvider(); !predicate(res); {
		res = resProvider()
	}
	return transformer(res)
}
func WaitForTNoErrorAndReturn[T any](ctx context.Context, resProvider func(ctx context.Context) (T, error)) T {
	var res T
	var err error
	for res, err = resProvider(ctx); err != nil; {
		res, err = resProvider(ctx)
	}
	return res
}
func SubmitWithWaitGroup(task func(), wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		task()
	}()
}

func FuncWaitForNoErrorAndReturn[T any](ctx context.Context, resProvider func(ctx context.Context) (T, error)) func() T {
	return func() T {
		return WaitForTNoErrorAndReturn(ctx, resProvider)
	}
}
