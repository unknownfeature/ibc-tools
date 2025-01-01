package funcs

type Supplier[T any] func() T
type Consumer[T any] func(T)
type Function[T, K any] func(T) K
type BiFunction[T, K, L any] func(T, K) L
type Predicate[T any] Function[T, bool]
type BiPredicate[T, K any] BiFunction[T, K, bool]
type BiConsumer[T, K any] func(T, K)
type Comparator[T any] BiFunction[T, T, int]
type Reducer[T any] BiFunction[*T, *T, *T]
