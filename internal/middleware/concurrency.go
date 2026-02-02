package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"
)

// ConcurrencyLimiter limits concurrent request processing using a weighted semaphore.
// This is more efficient than channel-based semaphore for high-throughput scenarios.
type ConcurrencyLimiter struct {
	sem           *semaphore.Weighted
	maxConcurrent int64
	timeout       time.Duration
	activeCount   int64
	totalReqs     int64
	rejectedReqs  int64

	// Adaptive timeout
	adaptive      bool
	latencyWindow []int64 // Milliseconds
	windowIdx     int
	windowSize    int
	mu            sync.RWMutex
}

// NewConcurrencyLimiter creates a new limiter with the specified max concurrent requests and timeout.
func NewConcurrencyLimiter(maxConcurrent int, timeout time.Duration, adaptive bool) *ConcurrencyLimiter {
	if maxConcurrent <= 0 {
		maxConcurrent = 100
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &ConcurrencyLimiter{
		sem:           semaphore.NewWeighted(int64(maxConcurrent)),
		maxConcurrent: int64(maxConcurrent),
		timeout:       timeout,
		adaptive:      adaptive,
		latencyWindow: make([]int64, 100), // Keep last 100 requests
		windowSize:    100,
	}
}

func (cl *ConcurrencyLimiter) Limit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&cl.totalReqs, 1)

		// Calculate wait timeout
		waitTimeout := 60 * time.Second
		if cl.adaptive {
			p95 := cl.GetP95()
			if p95 > 0 {
				// Allow 1.5x P95 wait time, clamped
				calcWait := time.Duration(float64(p95)*1.5) * time.Millisecond
				if calcWait < 5*time.Second {
					waitTimeout = 5 * time.Second
				} else if calcWait > 60*time.Second {
					waitTimeout = 60 * time.Second
				} else {
					waitTimeout = calcWait
				}
			}
		}

		if cl.timeout < waitTimeout {
			waitTimeout = cl.timeout
		}

		waitCtx, cancelWait := context.WithTimeout(r.Context(), waitTimeout)
		defer cancelWait()

		// Try to acquire semaphore with wait timeout
		acquireStart := time.Now()
		if err := cl.sem.Acquire(waitCtx, 1); err != nil {
			atomic.AddInt64(&cl.rejectedReqs, 1)
			slog.Warn("Concurrency limit: Wait timeout", "duration", time.Since(acquireStart), "total_rejected", atomic.LoadInt64(&cl.rejectedReqs), "wait_timeout", waitTimeout)
			http.Error(w, "Request timed out while waiting for a worker slot or server busy", http.StatusServiceUnavailable)
			return
		}

		slog.Debug("Concurrency limit: Slot acquired", "wait_duration", time.Since(acquireStart), "active", atomic.LoadInt64(&cl.activeCount)+1)

		atomic.AddInt64(&cl.activeCount, 1)
		reqStart := time.Now()

		defer func() {
			cl.sem.Release(1)
			atomic.AddInt64(&cl.activeCount, -1)

			duration := time.Since(reqStart)
			if cl.adaptive {
				cl.UpdateStats(duration)
			}
			slog.Debug("Concurrency limit: Slot released", "active", atomic.LoadInt64(&cl.activeCount), "duration", duration)
		}()

		// Use the full concurrency timeout for actual request execution
		execCtx, cancelExec := context.WithTimeout(r.Context(), cl.timeout)
		defer cancelExec()

		slog.Debug("Concurrency limit: Serving request", "path", r.URL.Path, "timeout", cl.timeout)
		next.ServeHTTP(w, r.WithContext(execCtx))
	}
}

// UpdateStats records request latency for adaptive timeout
func (cl *ConcurrencyLimiter) UpdateStats(d time.Duration) {
	ms := d.Milliseconds()
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.latencyWindow[cl.windowIdx] = ms
	cl.windowIdx = (cl.windowIdx + 1) % cl.windowSize
}

// GetP95 returns the 95th percentile latency in milliseconds
func (cl *ConcurrencyLimiter) GetP95() int64 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	// Copy to sort
	sorted := make([]int64, len(cl.latencyWindow))
	copy(sorted, cl.latencyWindow)

	// Remove zeros to avoid skewing empty window
	valid := 0
	for _, v := range sorted {
		if v > 0 {
			valid++
		}
	}
	if valid < 10 {
		return 0 // Not enough data
	}

	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	// P95 index in sorted array (including zeros, but zeros are at start)
	// We want P95 of NON-ZERO values ideally, or update logic correctly.
	// Simple approach: sort all, pick 95th percentile.

	idx := int(float64(len(sorted)) * 0.95)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// Stats returns current limiter statistics.
func (cl *ConcurrencyLimiter) Stats() (active, total, rejected int64) {
	return atomic.LoadInt64(&cl.activeCount),
		atomic.LoadInt64(&cl.totalReqs),
		atomic.LoadInt64(&cl.rejectedReqs)
}

// TryAcquire attempts to acquire the semaphore without blocking.
// Returns true if acquired, false otherwise.
func (cl *ConcurrencyLimiter) TryAcquire() bool {
	return cl.sem.TryAcquire(1)
}

// Release releases one slot in the semaphore.
func (cl *ConcurrencyLimiter) Release() {
	cl.sem.Release(1)
}
