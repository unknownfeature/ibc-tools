package utils

import (
	"sync"
	"time"
)

type ConcurrentMap[K comparable, V any] struct {
	internalMap map[K]V
	onChange    []BiConsumer[Entry[K, V], Operation]
	lock        *sync.Mutex
}

type Entry[K comparable, V any] struct {
	key K
	val V
}

type Operation int

const (
	Get Operation = iota
	Put
	Delete
)

func NewConcurrentMap[K comparable, V any](onChange ...BiConsumer[Entry[K, V], Operation]) *ConcurrentMap[K, V] {
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

func (c ConcurrentMap[K, V]) Get(k K) Optional[V] {
	return c.getAndCall(k)
}

func (c ConcurrentMap[K, V]) Delete(k K) Optional[V] {
	return c.deleteAndCall(k)
}

func (c ConcurrentMap[K, V]) ComputeIfAbsent(k K, newValSuppl Function[K, V]) Optional[V] {
	return c.computeIfAbsentAndCall(k, newValSuppl)
}

func (c ConcurrentMap[K, V]) PutIfAbsent(k K, newValSuppl Function[K, V]) {
	c.putIfAbsentAndCall(k, newValSuppl)
}

func (c ConcurrentMap[K, V]) DeleteIf(predicate BiPredicate[K, V]) {
	c.deleteIfAndCall(predicate)
}

func (c ConcurrentMap[K, V]) putAndCall(k K, v V) {
	DoInLock(c.lock, func() {
		c.internalMap[k] = v
		c.callCallback(k, v, Put)
	})
}

func (c ConcurrentMap[K, V]) getAndCall(k K) Optional[V] {
	return DoInLockAndReturn(c.lock, func() Optional[V] {
		if i, ok := c.internalMap[k]; ok {
			c.callCallback(k, i, Get)
			return NewWithValue[V](&i)

		}
		return NewEmpty[V]()
	})
}

func (c ConcurrentMap[K, V]) deleteAndCall(k K) Optional[V] {
	return DoInLockAndReturn(c.lock, func() Optional[V] {
		if i, ok := c.internalMap[k]; ok {
			delete(c.internalMap, k)
			c.callCallback(k, i, Delete)
			return NewWithValue[V](&i)

		}
		return NewEmpty[V]()
	})
}

func (c ConcurrentMap[K, V]) computeIfAbsentAndCall(k K, newValSuppl Function[K, V]) Optional[V] {
	return DoInLockAndReturn[Optional[V]](c.lock, func() Optional[V] {
		if i, ok := c.internalMap[k]; ok {
			return NewWithValue[V](&i)
		}
		val := newValSuppl(k)
		c.internalMap[k] = val
		c.callCallback(k, val, Put)
		return NewWithValue[V](&val)

	})
}

func (c ConcurrentMap[K, V]) putIfAbsentAndCall(k K, newValSuppl Function[K, V]) {
	DoInLock[Optional[V]](c.lock, func() {
		if _, ok := c.internalMap[k]; !ok {
			val := newValSuppl(k)
			c.internalMap[k] = val
			c.callCallback(k, val, Put)
		}

	})
}

func (c ConcurrentMap[K, V]) deleteIfAndCall(predicate BiPredicate[K, V]) {
	DoInLock[Optional[V]](c.lock, func() {
		newMap := make(map[K]V, 0)
		for k, i := range c.internalMap {
			if !predicate(k, i) {
				newMap[k] = i
			} else {
				c.callCallback(k, i, Delete)
			}
		}
		c.internalMap = newMap
	})
}

func (c ConcurrentMap[K, V]) callCallback(k K, v V, operation Operation) {
	if c.onChange != nil {
		for _, fun := range c.onChange {
			fun(Entry[K, V]{k, v}, operation)
		}
	}
}

type ExpiringConcurrentMap[K comparable, V any] struct {
	internal *ConcurrentMap[K, V]
}

func NewExpiringConcurrentMap[K comparable, V any](predicate BiPredicate[Entry[K, V], int]) *ExpiringConcurrentMap[K, V] {
	timeAdded := make(map[K]int, 0)

	onChange := func(en Entry[K, V], op Operation) {
		if op == Put {
			timeAdded[en.key] = time.Now().Second()
		} else if _, ok := timeAdded[en.key]; ok && op == Delete {
			delete(timeAdded, en.key)
		}

	}
	theMap := NewConcurrentMap[K, V](onChange)

	go func() {
		for {
			theMap.DeleteIf(func(k K, v V) bool {

				if theTime, ok := timeAdded[k]; ok {
					return predicate(Entry[K, V]{k, v}, theTime)
				}
				return false
			})
		}
	}()

	return &ExpiringConcurrentMap[K, V]{theMap}
}

func NewExpiresAfterDurationConcurrentMap[K comparable, V any](expireAfter time.Duration) *ExpiringConcurrentMap[K, V] {
	return NewExpiringConcurrentMap[K, V](func(e Entry[K, V], tm int) bool {
		now := time.Now().Second()
		return now-int(expireAfter.Seconds()) > tm

	})
}
