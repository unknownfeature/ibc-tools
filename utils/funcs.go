package utils

type Supplier[T any] func() T
type Consumer[T any] func(T)
type Function[T, K any] func(T) K
type BiFunction[T, K, L any] func(T, K) L
type Predicate[T any] Function[T, bool]
type BiPredicate[T, K any] BiFunction[T, K, bool]
type BiConsumer[T, K any] func(T, K)
type Comparator[T any] BiFunction[T, T, int]
type Reducer[T any] BiFunction[*T, *T, *T]

//func Chain[T, K, V any](first Function[V, T], second Function[T, K]) Function[V, K] {
//	return func(Val V) K {
//		return second(first(Val))
//	}
//}

func Identity[T any](t T) T {
	return t
}

func NewInt64Comparator() Comparator[int64] {
	return func(i int64, i2 int64) int {
		return int(i - i2)
	}
}

func Max[T any](one, two T, comp Comparator[T]) T {
	if comp(one, two) > 0 {
		return one
	}
	return two
}

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

func (o Optional[T]) If(consumer Consumer[T]) {
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
