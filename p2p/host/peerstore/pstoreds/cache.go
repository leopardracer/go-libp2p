package pstoreds

// cache abstracts all methods we access from ARCCache, to enable alternate
// implementations such as a no-op one.
type cache[K comparable, V any] interface {
	Get(key K) (value V, ok bool)
	Add(key K, value V)
	Remove(key K)
	Contains(key K) bool
	Peek(key K) (value V, ok bool)
	Keys() []K
}

// noopCache is a dummy implementation that's used when the cache is disabled.
type noopCache[K comparable, V any] struct {
}

var _ cache[int, int] = (*noopCache[int, int])(nil)

func (*noopCache[K, V]) Get(_ K) (value V, ok bool) {
	return *new(V), false
}

func (*noopCache[K, V]) Add(_ K, _ V) {
}

func (*noopCache[K, V]) Remove(_ K) {
}

func (*noopCache[K, V]) Contains(_ K) bool {
	return false
}

func (*noopCache[K, V]) Peek(_ K) (value V, ok bool) {
	return *new(V), false
}

func (*noopCache[K, V]) Keys() (keys []K) {
	return keys
}
