package loadbalancer

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"orchids-api/internal/auth"
	"orchids-api/internal/store"

	"golang.org/x/sync/singleflight"
)

const defaultCacheTTL = 5 * time.Second

type LoadBalancer struct {
	Store          *store.Store
	mu             sync.RWMutex
	cachedAccounts []*store.Account
	cacheExpires   time.Time
	cacheTTL       time.Duration
	activeConns    sync.Map // map[int64]*atomic.Int64
	retry429       time.Duration
	sfGroup        singleflight.Group
}

func New(s *store.Store) *LoadBalancer {
	return NewWithCacheTTL(s, defaultCacheTTL)
}

func NewWithCacheTTL(s *store.Store, cacheTTL time.Duration) *LoadBalancer {
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	return &LoadBalancer{
		Store:    s,
		cacheTTL: cacheTTL,
		retry429: 60 * time.Minute,
	}
}

func (lb *LoadBalancer) SetRetry429Interval(interval time.Duration) {
	if interval <= 0 {
		interval = 60 * time.Minute
	}
	lb.retry429 = interval
}

func (lb *LoadBalancer) GetModelChannel(ctx context.Context, modelID string) string {
	if lb.Store == nil {
		return ""
	}
	m, err := lb.Store.GetModelByModelID(ctx, modelID)
	if err != nil || m == nil {
		return ""
	}
	return m.Channel
}

func (lb *LoadBalancer) GetNextAccount(ctx context.Context) (*store.Account, error) {
	return lb.GetNextAccountExcludingByChannel(ctx, nil, "")
}

func (lb *LoadBalancer) GetNextAccountByChannel(ctx context.Context, channel string) (*store.Account, error) {
	return lb.GetNextAccountExcludingByChannel(ctx, nil, channel)
}

func (lb *LoadBalancer) GetNextAccountExcluding(ctx context.Context, excludeIDs []int64) (*store.Account, error) {
	return lb.GetNextAccountExcludingByChannel(ctx, excludeIDs, "")
}

func (lb *LoadBalancer) GetNextAccountExcludingByChannel(ctx context.Context, excludeIDs []int64, channel string) (*store.Account, error) {
	accounts, err := lb.getEnabledAccounts(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []*store.Account
	excludeSet := make(map[int64]bool)
	for _, id := range excludeIDs {
		excludeSet[id] = true
	}

	for _, acc := range accounts {
		if excludeSet[acc.ID] {
			continue
		}
		if !lb.isAccountAvailable(ctx, acc) {
			continue
		}
		if channel != "" {
			accType := acc.AccountType
			if strings.TrimSpace(accType) == "" {
				accType = "orchids"
			}
			if !strings.EqualFold(accType, channel) && !strings.EqualFold(acc.AgentMode, channel) {
				continue
			}
		}
		filtered = append(filtered, acc)
	}
	accounts = filtered

	if len(accounts) == 0 {
		return nil, fmt.Errorf("no enabled accounts available for channel: %s", channel)
	}

	account := lb.selectAccount(accounts)

	slog.Info("Selected account", "name", account.Name, "email", account.Email, "session", auth.MaskSensitive(account.SessionID))

	if err := lb.Store.IncrementRequestCount(ctx, account.ID); err != nil {
		return nil, err
	}

	return account, nil
}

func (lb *LoadBalancer) getEnabledAccounts(ctx context.Context) ([]*store.Account, error) {
	now := time.Now()

	// Use singleflight to prevent cache stampede
	val, err, _ := lb.sfGroup.Do("getEnabledAccounts", func() (interface{}, error) {
		lb.mu.RLock()
		if len(lb.cachedAccounts) > 0 && now.Before(lb.cacheExpires) {
			accounts := make([]*store.Account, len(lb.cachedAccounts))
			copy(accounts, lb.cachedAccounts)
			lb.mu.RUnlock()
			return accounts, nil
		}
		lb.mu.RUnlock()

		accounts, err := lb.Store.GetEnabledAccounts(ctx)
		if err != nil {
			return nil, err
		}

		lb.mu.Lock()
		lb.cachedAccounts = accounts
		lb.cacheExpires = now.Add(lb.cacheTTL)
		lb.mu.Unlock()

		cached := make([]*store.Account, len(accounts))
		copy(cached, accounts)
		return cached, nil
	})

	if err != nil {
		return nil, err
	}
	return val.([]*store.Account), nil
}

func (lb *LoadBalancer) selectAccount(accounts []*store.Account) *store.Account {
	if len(accounts) == 0 {
		return nil
	}
	if len(accounts) == 1 {
		return accounts[0]
	}

	var bestAccounts []*store.Account
	minScore := float64(-1)

	for _, acc := range accounts {
		weight := acc.Weight
		if weight <= 0 {
			weight = 1
		}

		var conns int64
		if val, ok := lb.activeConns.Load(acc.ID); ok {
			conns = val.(*atomic.Int64).Load()
		}
		score := float64(conns) / float64(weight)

		if bestAccounts == nil || score < minScore {
			bestAccounts = []*store.Account{acc}
			minScore = score
		} else if score == minScore {
			bestAccounts = append(bestAccounts, acc)
		}
	}

	if len(bestAccounts) > 0 {
		// Randomly select one from the best accounts to ensure load balancing
		return bestAccounts[rand.IntN(len(bestAccounts))]
	}
	return accounts[0]
}

func (lb *LoadBalancer) GetStats() map[int64]int {
	stats := make(map[int64]int)
	lb.activeConns.Range(func(key, value interface{}) bool {
		stats[key.(int64)] = int(value.(*atomic.Int64).Load())
		return true
	})
	return stats
}

func (lb *LoadBalancer) AcquireConnection(accountID int64) {
	val, _ := lb.activeConns.LoadOrStore(accountID, &atomic.Int64{})
	val.(*atomic.Int64).Add(1)
}

func (lb *LoadBalancer) ReleaseConnection(accountID int64) {
	if val, ok := lb.activeConns.Load(accountID); ok {
		counter := val.(*atomic.Int64)
		for {
			current := counter.Load()
			if current <= 0 {
				break
			}
			if counter.CompareAndSwap(current, current-1) {
				break
			}
		}
	}
}

func (lb *LoadBalancer) isAccountAvailable(ctx context.Context, acc *store.Account) bool {
	status := strings.TrimSpace(acc.StatusCode)
	if status == "" {
		return true
	}

	now := time.Now()
	switch status {
	case "429":
		if acc.LastAttempt.IsZero() {
			return false
		}
		if now.Sub(acc.LastAttempt) >= lb.retry429 {
			lb.clearAccountStatus(ctx, acc, "429 冷却完成，自动恢复")
			return true
		}
		return false
	case "quota_exceeded":
		resetAt := acc.QuotaResetAt
		if resetAt.IsZero() {
			resetAt = nextMonthStart(now)
			acc.QuotaResetAt = resetAt
			lb.persistAccountStatus(ctx, acc, "记录配额重置时间")
			return false
		}
		if now.After(resetAt) {
			lb.clearAccountStatus(ctx, acc, "配额到期，自动恢复")
			return true
		}
		return false
	default:
		return false
	}
}

func (lb *LoadBalancer) clearAccountStatus(ctx context.Context, acc *store.Account, reason string) {
	acc.StatusCode = ""
	acc.LastAttempt = time.Time{}
	acc.QuotaResetAt = time.Time{}
	lb.persistAccountStatus(ctx, acc, reason)
}

func (lb *LoadBalancer) persistAccountStatus(ctx context.Context, acc *store.Account, reason string) {
	if lb.Store == nil {
		return
	}
	if err := lb.Store.UpdateAccount(ctx, acc); err != nil {
		slog.Warn("账号状态更新失败", "account_id", acc.ID, "reason", reason, "error", err)
		return
	}
	slog.Info("账号状态已更新", "account_id", acc.ID, "status", acc.StatusCode, "reason", reason)
}

func nextMonthStart(now time.Time) time.Time {
	year, month, _ := now.Date()
	loc := now.Location()
	if month == time.December {
		return time.Date(year+1, time.January, 1, 0, 0, 0, 0, loc)
	}
	return time.Date(year, month+1, 1, 0, 0, 0, 0, loc)
}
