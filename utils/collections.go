package utils

import (
	"sync"
	"time"
)

type MapItem[V any] struct {
	Val       V
	TimeAdded int
}

type ConcurrentTTLMap[K comparable, V any] struct {
	internalMap map[K]MapItem[V]
	isExpired   BiPredicate[MapItem[V], K]
	lock        *sync.Mutex
	maxKey      K
	comparator  Comparator[K]
}

func NewMapThatExpiresItemsAfterSeconds[K comparable, V any](comparator Comparator[K], afterSeconds int) *ConcurrentTTLMap[K, V] {

	return NewMapWithExpirationPredicate[K, V](comparator, func(m MapItem[V], _ K) bool {
		return m.TimeAdded < time.Now().Second()+afterSeconds
	})
}

func NewMapWithExpirationPredicate[K comparable, V any](comparator Comparator[K], predicate BiPredicate[MapItem[V], K]) *ConcurrentTTLMap[K, V] {
	theMap := &ConcurrentTTLMap[K, V]{
		internalMap: make(map[K]MapItem[V], 0),
		isExpired:   predicate,
		lock:        &sync.Mutex{},
		comparator:  comparator,
	}

	go func() {
		for {
			toDelete := make([]K, 0)
			for k, _ := range theMap.internalMap {
				DoInLock(theMap.lock, func() {
					if v, ok := theMap.internalMap[k]; ok && theMap.isExpired(v, theMap.maxKey) {
						toDelete = append(toDelete, k)
					}
				})

			}

			for _, k := range toDelete {
				DoInLock(theMap.lock, func() {
					if len(theMap.internalMap) > 1 {
						delete(theMap.internalMap, k)

					}
				})

			}
		}
	}()
	return theMap
}

func (c ConcurrentTTLMap[K, V]) Put(k K, v V) {
	DoInLock(c.lock, func() {
		c.maxKey = Max(c.maxKey, k, c.comparator)
		c.internalMap[k] = MapItem[V]{TimeAdded: time.Now().Second(), Val: v}
	})
}

func (c ConcurrentTTLMap[K, V]) Get(k K) Optional[V] {
	return DoInLockAndReturn[Optional[V]](c.lock, func() Optional[V] {
		if i, ok := c.internalMap[k]; ok {
			return NewWithValue[V](&i.Val)
		}
		return NewEmpty[V]()

	})
}

func (c ConcurrentTTLMap[K, V]) Delete(k K) Optional[V] {
	return DoInLockAndReturn[Optional[V]](c.lock, func() Optional[V] {
		if i, ok := c.internalMap[k]; ok {
			delete(c.internalMap, k)
			if c.maxKey == k {
				for curr, _ := range c.internalMap {
					c.maxKey = Max[K](c.maxKey, curr, c.comparator)
				}
			}
			return NewWithValue[V](&i.Val)
		}
		return NewEmpty[V]()

	})
}

func (c ConcurrentTTLMap[K, V]) ComputeIfAbsent(k K, newValSuppl Function[K, V]) Optional[V] {
	return DoInLockAndReturn[Optional[V]](c.lock, func() Optional[V] {
		if i, ok := c.internalMap[k]; ok {
			return NewWithValue[V](&i.Val)
		}
		val := newValSuppl(k)
		c.internalMap[k] = MapItem[V]{TimeAdded: time.Now().Second(), Val: val}
		c.maxKey = Max(c.maxKey, k, c.comparator)
		return NewWithValue[V](&val)

	})
}
