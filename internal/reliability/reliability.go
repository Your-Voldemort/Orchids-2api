// Package reliability provides circuit breaker and retry utilities for upstream calls.
package reliability

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"sync"
	"time"

	"orchids-api/internal/util"

	"github.com/sony/gobreaker"
)

// Common errors
var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrMaxRetries      = errors.New("max retries exceeded")
	ErrContextCanceled = errors.New("context canceled")
)

// CircuitBreaker wraps gobreaker with sensible defaults for API calls.
type CircuitBreaker struct {
	cb *gobreaker.CircuitBreaker
}

// CircuitBreakerConfig configures the circuit breaker.
type CircuitBreakerConfig struct {
	Name         string
	MaxRequests  uint32        // Requests allowed in half-open state
	Interval     time.Duration // Cyclic period for clearing counters
	Timeout      time.Duration // Time to wait before half-open
	FailureRatio float64       // Ratio of failures to trip
	MinRequests  uint32        // Min requests before evaluating ratio
}

// DefaultCircuitConfig returns sensible defaults.
func DefaultCircuitConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:         name,
		MaxRequests:  3,
		Interval:     60 * time.Second,
		Timeout:      30 * time.Second,
		FailureRatio: 0.5,
		MinRequests:  5,
	}
}

// NewCircuitBreaker creates a circuit breaker with the given config.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < cfg.MinRequests {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= cfg.FailureRatio
		},
	}
	return &CircuitBreaker{
		cb: gobreaker.NewCircuitBreaker(settings),
	}
}

// Execute runs the given function through the circuit breaker.
func (c *CircuitBreaker) Execute(fn func() (interface{}, error)) (interface{}, error) {
	return c.cb.Execute(fn)
}

// State returns the current state of the circuit breaker.
func (c *CircuitBreaker) State() gobreaker.State {
	return c.cb.State()
}

// RetryConfig configures retry behavior.
type RetryConfig struct {
	MaxRetries     int
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	Multiplier     float64
	Jitter         float64 // 0.0 to 1.0
	RetryableCheck func(error) bool
}

// DefaultRetryConfig returns sensible retry defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		RetryableCheck: func(err error) bool {
			return err != nil && !errors.Is(err, context.Canceled)
		},
	}
}

// Retry executes fn with exponential backoff.
func Retry(ctx context.Context, cfg RetryConfig, fn func() error) error {
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return ErrContextCanceled
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if !cfg.RetryableCheck(lastErr) {
			return lastErr
		}

		if attempt == cfg.MaxRetries {
			break
		}

		// Calculate delay with jitter
		jitter := 1.0 + (rand.Float64()*2-1)*cfg.Jitter
		actualDelay := time.Duration(float64(delay) * jitter)
		if actualDelay > cfg.MaxDelay {
			actualDelay = cfg.MaxDelay
		}

		if !util.SleepWithContext(ctx, actualDelay) {
			return ErrContextCanceled
		}

		delay = time.Duration(float64(delay) * cfg.Multiplier)
	}

	return ErrMaxRetries
}

// RetryWithResult executes fn with exponential backoff and returns a result.
func RetryWithResult[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return result, ErrContextCanceled
		}

		result, lastErr = fn()
		if lastErr == nil {
			return result, nil
		}

		if !cfg.RetryableCheck(lastErr) {
			return result, lastErr
		}

		if attempt == cfg.MaxRetries {
			break
		}

		jitter := 1.0 + (rand.Float64()*2-1)*cfg.Jitter
		actualDelay := time.Duration(float64(delay) * jitter)
		if actualDelay > cfg.MaxDelay {
			actualDelay = cfg.MaxDelay
		}

		if !util.SleepWithContext(ctx, actualDelay) {
			return result, ErrContextCanceled
		}

		delay = time.Duration(float64(delay) * cfg.Multiplier)
	}

	return result, ErrMaxRetries
}


// CircuitBreakerManager 管理多个熔断器实例
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
	config   CircuitBreakerConfig
}

var (
	globalManager     *CircuitBreakerManager
	globalManagerOnce sync.Once
)

// GetGlobalManager 获取全局熔断器管理器
func GetGlobalManager() *CircuitBreakerManager {
	globalManagerOnce.Do(func() {
		globalManager = NewCircuitBreakerManager(DefaultCircuitConfig("global"))
	})
	return globalManager
}

// NewCircuitBreakerManager 创建新的熔断器管理器
func NewCircuitBreakerManager(defaultConfig CircuitBreakerConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		config:   defaultConfig,
	}
}

// GetBreaker 获取或创建指定名称的熔断器
func (m *CircuitBreakerManager) GetBreaker(name string) *CircuitBreaker {
	m.mu.RLock()
	if cb, ok := m.breakers[name]; ok {
		m.mu.RUnlock()
		return cb
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double check after acquiring write lock
	if cb, ok := m.breakers[name]; ok {
		return cb
	}

	cfg := m.config
	cfg.Name = name
	cb := NewCircuitBreaker(cfg)
	m.breakers[name] = cb
	return cb
}

// GetBreakerWithConfig 使用自定义配置获取或创建熔断器
func (m *CircuitBreakerManager) GetBreakerWithConfig(name string, cfg CircuitBreakerConfig) *CircuitBreaker {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cb, ok := m.breakers[name]; ok {
		return cb
	}

	cfg.Name = name
	cb := NewCircuitBreaker(cfg)
	m.breakers[name] = cb
	return cb
}

// AllStates 返回所有熔断器的状态
func (m *CircuitBreakerManager) AllStates() map[string]gobreaker.State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make(map[string]gobreaker.State, len(m.breakers))
	for name, cb := range m.breakers {
		states[name] = cb.State()
	}
	return states
}

// IsHealthy 检查指定熔断器是否健康（非 Open 状态）
func (m *CircuitBreakerManager) IsHealthy(name string) bool {
	m.mu.RLock()
	cb, ok := m.breakers[name]
	m.mu.RUnlock()

	if !ok {
		return true // 不存在的熔断器视为健康
	}
	return cb.State() != gobreaker.StateOpen
}

// Reset 重置指定熔断器（用于手动恢复）
// 注意：gobreaker 不直接支持重置，这里通过重新创建实现
func (m *CircuitBreakerManager) Reset(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.breakers[name]; ok {
		cfg := m.config
		cfg.Name = name
		m.breakers[name] = NewCircuitBreaker(cfg)
	}
}

// UpstreamBreaker 是上游服务的预配置熔断器
var UpstreamBreaker = NewCircuitBreaker(CircuitBreakerConfig{
	Name:         "upstream",
	MaxRequests:  5,
	Interval:     60 * time.Second,
	Timeout:      30 * time.Second,
	FailureRatio: 0.5,
	MinRequests:  10,
})

// AccountBreaker 是账号服务的预配置熔断器
var AccountBreaker = NewCircuitBreaker(CircuitBreakerConfig{
	Name:         "account",
	MaxRequests:  3,
	Interval:     30 * time.Second,
	Timeout:      15 * time.Second,
	FailureRatio: 0.6,
	MinRequests:  5,
})

// ExecuteWithBreaker 使用指定熔断器执行函数
func ExecuteWithBreaker(name string, fn func() (interface{}, error)) (interface{}, error) {
	cb := GetGlobalManager().GetBreaker(name)
	return cb.Execute(fn)
}

// IsRetryableError 判断错误是否可重试
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 不可重试的错误
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	errStr := err.Error()
	// 认证错误不可重试
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "403") {
		return false
	}
	// 请求错误不可重试
	if strings.Contains(errStr, "400") {
		return false
	}

	return true
}

// HealthCheck 健康检查结果
type HealthCheck struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	State   string `json:"state"`
}

// GetHealthChecks 获取所有熔断器的健康检查结果
func GetHealthChecks() []HealthCheck {
	states := GetGlobalManager().AllStates()
	checks := make([]HealthCheck, 0, len(states))

	for name, state := range states {
		checks = append(checks, HealthCheck{
			Name:    name,
			Healthy: state != gobreaker.StateOpen,
			State:   stateToString(state),
		})
	}

	return checks
}

func stateToString(state gobreaker.State) string {
	switch state {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateHalfOpen:
		return "half-open"
	case gobreaker.StateOpen:
		return "open"
	default:
		return "unknown"
	}
}
