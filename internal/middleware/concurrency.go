package middleware

import (
	"context"
	"log/slog"
	"net/http"
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
}

// NewConcurrencyLimiter creates a new limiter with the specified max concurrent requests and timeout.
func NewConcurrencyLimiter(maxConcurrent int, timeout time.Duration) *ConcurrencyLimiter {
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
	}
}

func (cl *ConcurrencyLimiter) Limit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&cl.totalReqs, 1)

		// Narrow timeout for waiting in the queue
		waitTimeout := 60 * time.Second
		if cl.timeout < waitTimeout {
			waitTimeout = cl.timeout
		}
		waitCtx, cancelWait := context.WithTimeout(r.Context(), waitTimeout)
		defer cancelWait()

		// Try to acquire semaphore with wait timeout
		acquireStart := time.Now()
		if err := cl.sem.Acquire(waitCtx, 1); err != nil {
			atomic.AddInt64(&cl.rejectedReqs, 1)
			slog.Warn("Concurrency limit: Wait timeout", "duration", time.Since(acquireStart), "total_rejected", atomic.LoadInt64(&cl.rejectedReqs))
			http.Error(w, "Request timed out while waiting for a worker slot or server busy", http.StatusServiceUnavailable)
			return
		}

		slog.Debug("Concurrency limit: Slot acquired", "wait_duration", time.Since(acquireStart), "active", atomic.LoadInt64(&cl.activeCount)+1)

		atomic.AddInt64(&cl.activeCount, 1)
		defer func() {
			cl.sem.Release(1)
			atomic.AddInt64(&cl.activeCount, -1)
			slog.Debug("Concurrency limit: Slot released", "active", atomic.LoadInt64(&cl.activeCount))
		}()

		// Use the full concurrency timeout for actual request execution
		execCtx, cancelExec := context.WithTimeout(r.Context(), cl.timeout)
		defer cancelExec()

		slog.Debug("Concurrency limit: Serving request", "path", r.URL.Path, "timeout", cl.timeout)
		next.ServeHTTP(w, r.WithContext(execCtx))
	}
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
