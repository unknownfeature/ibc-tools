package optional

import (
	"main/funcs"
)

type Optional[T any] struct {
	val *T
}

func NewEmpty[T any]() Optional[T] {
	return Optional[T]{}
}

func NewWithValue[T any](val *T) Optional[T] {
	if val == nil {
		return NewEmpty[T]()
	}
	return Optional[T]{val: val}
}

func (o Optional[T]) If(consumer funcs.Consumer[T]) {
	if o.val != nil {
		consumer(*o.val)
	}
}

func (o Optional[T]) IsPresent() bool {
	return o.val != nil
}

func (o Optional[T]) Get() *T {
	return o.val
}
