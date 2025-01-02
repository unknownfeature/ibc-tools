package concurrent

import (
	"main/funcs"
	"main/funcs/optional"
	"sync"
)

type ConcurrentMap[K comparable, V any] struct {
	internalMap map[K]V
	onChange    []funcs.BiConsumer[Entry[K, V], Operation]
	lock        *sync.Mutex
}

type Entry[K comparable, V any] struct {
	Key K
	Val V
}

type Operation int

const (
	Get Operation = iota
	Put
	Delete
)

func NewConcurrentMap[K comparable, V any](onChange ...funcs.BiConsumer[Entry[K, V], Operation]) *ConcurrentMap[K, V] {
	theMap := &ConcurrentMap[K, V]{
		internalMap: make(map[K]V, 0),
		lock:        &sync.Mutex{},
		onChange:    onChange,
	}

	return theMap
}

func (c ConcurrentMap[K, V]) Put(k K, v V) {
	c.putAndCall(k, v)
}

func (c ConcurrentMap[K, V]) Get(k K) optional.Optional[V] {
	return c.getAndCall(k)
}

func (c ConcurrentMap[K, V]) Delete(k K) optional.Optional[V] {
	return c.deleteAndCall(k)
}

func (c ConcurrentMap[K, V]) ComputeIfAbsent(k K, newValSuppl funcs.Function[K, V]) V {
	return c.computeIfAbsentAndCall(k, newValSuppl)
}

func (c ConcurrentMap[K, V]) Compute(k K, newValSuppl funcs.Function[K, V]) V {
	return c.computeIfAbsentAndCall(k, newValSuppl)
}
func (c ConcurrentMap[K, V]) PutIfAbsent(k K, newValSuppl funcs.Function[K, V]) {
	c.putIfAbsentAndCall(k, newValSuppl)
}

func (c ConcurrentMap[K, V]) DeleteIf(predicate funcs.BiPredicate[K, V]) {
	c.deleteIfAndCall(predicate)
}

func (c ConcurrentMap[K, V]) ReduceKeys(reducer funcs.Reducer[K]) optional.Optional[K] {
	c.lock.Lock()
	defer c.lock.Unlock()

	var res *K
	for k, _ := range c.internalMap {
		res = reducer(res, &k)

	}
	return optional.NewWithValue(res)

}

func (c ConcurrentMap[K, V]) putAndCall(k K, v V) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.internalMap[k] = v
	c.callCallback(k, v, Put)
}

func (c ConcurrentMap[K, V]) getAndCall(k K) optional.Optional[V] {
	c.lock.Lock()
	defer c.lock.Unlock()

	if i, ok := c.internalMap[k]; ok {
		c.callCallback(k, i, Get)
		return optional.NewWithValue[V](&i)

	}
	return optional.NewEmpty[V]()

}

func (c ConcurrentMap[K, V]) deleteAndCall(k K) optional.Optional[V] {
	c.lock.Lock()
	defer c.lock.Unlock()

	if i, ok := c.internalMap[k]; ok {
		delete(c.internalMap, k)
		c.callCallback(k, i, Delete)
		return optional.NewWithValue[V](&i)

	}
	return optional.NewEmpty[V]()

}

func (c ConcurrentMap[K, V]) computeAndCall(k K, newValSuppl funcs.Function[K, V]) V {
	c.lock.Lock()
	defer c.lock.Unlock()
	val := newValSuppl(k)
	c.internalMap[k] = val
	c.callCallback(k, val, Put)
	return val

}

func (c ConcurrentMap[K, V]) computeIfAbsentAndCall(k K, newValSuppl funcs.Function[K, V]) V {
	c.lock.Lock()
	defer c.lock.Unlock()

	if i, ok := c.internalMap[k]; ok {
		return i
	}
	val := newValSuppl(k)
	c.internalMap[k] = val
	c.callCallback(k, val, Put)
	return val
}

func (c ConcurrentMap[K, V]) putIfAbsentAndCall(k K, newValSuppl funcs.Function[K, V]) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.internalMap[k]; !ok {
		val := newValSuppl(k)
		c.internalMap[k] = val
		c.callCallback(k, val, Put)
	}

}

func (c ConcurrentMap[K, V]) deleteIfAndCall(predicate funcs.BiPredicate[K, V]) {
	c.lock.Lock()
	defer c.lock.Unlock()
	newMap := make(map[K]V, 0)
	for k, i := range c.internalMap {
		if !predicate(k, i) {
			newMap[k] = i
		} else {
			c.callCallback(k, i, Delete)
		}
	}
	c.internalMap = newMap
}

func (c ConcurrentMap[K, V]) callCallback(k K, v V, operation Operation) {
	if c.onChange != nil {
		for _, fun := range c.onChange {
			fun(Entry[K, V]{k, v}, operation)
		}
	}
}
