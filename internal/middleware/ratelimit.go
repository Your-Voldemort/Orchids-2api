package middleware

import (
	"net"
	"strings"
	"sync"
	"time"
)

// RateLimiter implements a sliding-window rate limiter keyed by IP.
type RateLimiter struct {
	mu          sync.Mutex
	attempts    map[string][]time.Time
	maxAttempts int
	window      time.Duration
}

// NewRateLimiter creates a rate limiter that allows maxAttempts within the
// given window duration per IP address.
func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		attempts:    make(map[string][]time.Time),
		maxAttempts: maxAttempts,
		window:      window,
	}
	go rl.cleanupLoop()
	return rl
}

// Allow reports whether the given IP is allowed to make another attempt.
// It records the current time as an attempt regardless of the result.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Prune expired entries for this IP.
	entries := rl.attempts[ip]
	valid := entries[:0]
	for _, t := range entries {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.maxAttempts {
		rl.attempts[ip] = valid
		return false
	}

	rl.attempts[ip] = append(valid, now)
	return true
}

// cleanupLoop periodically removes expired entries to prevent unbounded
// memory growth.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.cleanup()
	}
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	for ip, entries := range rl.attempts {
		valid := entries[:0]
		for _, t := range entries {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(rl.attempts, ip)
		} else {
			rl.attempts[ip] = valid
		}
	}
}

// ExtractIP returns the client IP from the request, checking
// X-Forwarded-For and X-Real-IP before falling back to RemoteAddr.
func ExtractIP(r_remoteAddr string, xForwardedFor string, xRealIP string) string {
	if xff := strings.TrimSpace(xForwardedFor); xff != "" {
		// Take the first IP from X-Forwarded-For.
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			xff = strings.TrimSpace(xff[:idx])
		}
		if xff != "" {
			return xff
		}
	}
	if xri := strings.TrimSpace(xRealIP); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r_remoteAddr)
	if err != nil {
		return r_remoteAddr
	}
	return host
}
