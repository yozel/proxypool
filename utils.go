package proxypool

import (
	"math/rand"
	"sort"
	"time"

	"golang.org/x/exp/constraints"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func sortSlice[T any](items []T, val func(i, j T) bool) []T {
	r := make([]T, len(items))
	copy(r, items)
	sort.Slice(r, func(i, j int) bool {
		return val(r[i], r[j])
	})
	return r
}

func shuffleSlice[T any](items []T) []T {
	r := make([]T, len(items))
	copy(r, items)
	rand.Shuffle(len(r), func(i, j int) {
		r[i], r[j] = r[j], r[i]
	})
	return r
}

func splitSlice[T any](items []T, index int) ([]T, []T) {
	index = min(index, len(items))
	return items[:index], items[index:]
}

func concatSlice[T any](items ...[]T) []T {
	r := make([]T, 0)
	for _, i := range items {
		r = append(r, i...)
	}
	return r
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func filter[T any](f func(T) bool, xs []T) []T {
	var ys []T
	for _, x := range xs {
		if f(x) {
			ys = append(ys, x)
		}
	}
	return ys
}

func values[K comparable, V any](m map[K]V) []V {
	var xs []V
	for _, v := range m {
		xs = append(xs, v)
	}
	return xs
}
