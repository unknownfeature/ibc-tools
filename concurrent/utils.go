package concurrent

import (
	"sync"
)

func WaitForTheConditionToBecomeTrue[T any](resProvider func() T, predicate func(T) bool) T {
	var res T

	for res = resProvider(); !predicate(res); {
		res = resProvider()
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
