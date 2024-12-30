package utils

type Supplier[T any] func() T
type Consumer[T any] func(T)
type Function[T, K any] func(T) K

func Chain[T, K, V any](first Function[V, T], second Function[T, K]) Function[V, K] {
	return func(val V) K {
		return second(first(val))
	}
}
