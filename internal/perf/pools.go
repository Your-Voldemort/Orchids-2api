// Package perf provides performance optimization utilities including object pools.
package perf

import (
	"strings"
	"sync"
	"sync/atomic"
)

// StringBuilderPool provides reusable strings.Builder instances.
// Usage:
//
//	sb := perf.AcquireStringBuilder()
//	defer perf.ReleaseStringBuilder(sb)
//	sb.WriteString("hello")
var StringBuilderPool = sync.Pool{
	New: func() interface{} {
		return &strings.Builder{}
	},
}

// AcquireStringBuilder gets a strings.Builder from the pool.
func AcquireStringBuilder() *strings.Builder {
	return StringBuilderPool.Get().(*strings.Builder)
}

// ReleaseStringBuilder returns a strings.Builder to the pool after resetting it.
func ReleaseStringBuilder(sb *strings.Builder) {
	if sb == nil {
		return
	}
	sb.Reset()
	StringBuilderPool.Put(sb)
}

// MapPool provides reusable map[string]interface{} instances.
// Note: Maps must be cleared before returning to pool.
var MapPool = sync.Pool{
	New: func() interface{} {
		return make(map[string]interface{}, 16)
	},
}

// AcquireMap gets a map from the pool.
func AcquireMap() map[string]interface{} {
	return MapPool.Get().(map[string]interface{})
}

// ReleaseMap clears and returns a map to the pool.
func ReleaseMap(m map[string]interface{}) {
	if m == nil {
		return
	}
	// Prevent memory leak: don't pool overly large maps
	if len(m) > 256 {
		return
	}
	// Clear map before returning (Go 1.21+ optimized)
	clear(m)
	MapPool.Put(m)
}

// ByteSlicePool provides reusable byte slices with default capacity of 4KB.
var ByteSlicePool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 4096)
		return &b
	},
}

// AcquireByteSlice gets a byte slice from the pool.
func AcquireByteSlice() *[]byte {
	return ByteSlicePool.Get().(*[]byte)
}

// ReleaseByteSlice returns a byte slice to the pool after resetting length.
func ReleaseByteSlice(b *[]byte) {
	if b == nil {
		return
	}
	// Prevent memory leak: don't pool overly large slices
	if cap(*b) > 65536 { // 64KB limit
		return
	}
	*b = (*b)[:0]
	ByteSlicePool.Put(b)
}

// StringSlicePool provides reusable []string slices.
var StringSlicePool = sync.Pool{
	New: func() interface{} {
		s := make([]string, 0, 16)
		return &s
	},
}

// AcquireStringSlice gets a string slice from the pool.
func AcquireStringSlice() *[]string {
	return StringSlicePool.Get().(*[]string)
}

// ReleaseStringSlice returns a string slice to the pool after resetting.
func ReleaseStringSlice(s *[]string) {
	if s == nil {
		return
	}
	// Prevent memory leak: don't pool overly large slices
	if cap(*s) > 256 {
		return
	}
	// Clear slice to release string references (Go 1.21+ optimized)
	clear(*s)
	*s = (*s)[:0]
	StringSlicePool.Put(s)
}

// PoolStats tracks pool usage statistics.
type PoolStats struct {
	Hits   atomic.Uint64
	Misses atomic.Uint64
}

// Hit increments the hit counter.
func (s *PoolStats) Hit() {
	s.Hits.Add(1)
}

// Miss increments the miss counter.
func (s *PoolStats) Miss() {
	s.Misses.Add(1)
}

// Snapshot returns current hit/miss counts.
func (s *PoolStats) Snapshot() (hits, misses uint64) {
	return s.Hits.Load(), s.Misses.Load()
}

// Reset resets all counters to zero.
func (s *PoolStats) Reset() {
	s.Hits.Store(0)
	s.Misses.Store(0)
}

// HitRate returns the cache hit rate (0.0 to 1.0).
func (s *PoolStats) HitRate() float64 {
	hits := s.Hits.Load()
	misses := s.Misses.Load()
	total := hits + misses
	if total == 0 {
		return 0.0
	}
	return float64(hits) / float64(total)
}

// GenericPool provides a type-safe generic pool with statistics.
type GenericPool[T any] struct {
	pool  sync.Pool
	stats PoolStats
	new   func() T
}

// NewGenericPool creates a new generic pool.
func NewGenericPool[T any](newFunc func() T) *GenericPool[T] {
	p := &GenericPool[T]{
		new: newFunc,
	}
	p.pool.New = func() interface{} {
		p.stats.Miss()
		return newFunc()
	}
	return p
}

// Get retrieves an item from the pool.
func (p *GenericPool[T]) Get() T {
	v := p.pool.Get().(T)
	p.stats.Hit()
	return v
}

// Put returns an item to the pool.
func (p *GenericPool[T]) Put(v T) {
	p.pool.Put(v)
}

// Stats returns the pool statistics.
func (p *GenericPool[T]) Stats() *PoolStats {
	return &p.stats
}

// TieredByteSlicePool provides byte slices of different size tiers.
type TieredByteSlicePool struct {
	small  *GenericPool[*[]byte] // 4KB
	medium *GenericPool[*[]byte] // 16KB
	large  *GenericPool[*[]byte] // 64KB
}

var tieredBytePool = &TieredByteSlicePool{
	small: NewGenericPool(func() *[]byte {
		b := make([]byte, 0, 4096)
		return &b
	}),
	medium: NewGenericPool(func() *[]byte {
		b := make([]byte, 0, 16384)
		return &b
	}),
	large: NewGenericPool(func() *[]byte {
		b := make([]byte, 0, 65536)
		return &b
	}),
}

// AcquireTieredByteSlice gets a byte slice of appropriate size.
func AcquireTieredByteSlice(sizeHint int) *[]byte {
	switch {
	case sizeHint <= 4096:
		return tieredBytePool.small.Get()
	case sizeHint <= 16384:
		return tieredBytePool.medium.Get()
	default:
		return tieredBytePool.large.Get()
	}
}

// ReleaseTieredByteSlice returns a byte slice to the appropriate pool.
func ReleaseTieredByteSlice(b *[]byte) {
	if b == nil {
		return
	}
	*b = (*b)[:0]

	capacity := cap(*b)
	switch {
	case capacity <= 4096:
		tieredBytePool.small.Put(b)
	case capacity <= 16384:
		tieredBytePool.medium.Put(b)
	case capacity <= 65536:
		tieredBytePool.large.Put(b)
		// Don't pool very large slices
	}
}

// GetTieredPoolStats returns statistics for all tiers.
func GetTieredPoolStats() map[string]*PoolStats {
	return map[string]*PoolStats{
		"small":  tieredBytePool.small.Stats(),
		"medium": tieredBytePool.medium.Stats(),
		"large":  tieredBytePool.large.Stats(),
	}
}
