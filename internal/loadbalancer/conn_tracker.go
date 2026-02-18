package loadbalancer

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

// ConnTracker tracks active connections per account for weighted least-connections selection.
type ConnTracker interface {
	Acquire(accountID int64)
	Release(accountID int64)
	GetCount(accountID int64) int64
	GetCounts(accountIDs []int64) map[int64]int64
}

// --- Memory Implementation ---

// MemoryConnTracker uses sync.Map with atomic counters (the original implementation).
type MemoryConnTracker struct {
	conns sync.Map // map[int64]*atomic.Int64
}

func NewMemoryConnTracker() *MemoryConnTracker {
	return &MemoryConnTracker{}
}

func (t *MemoryConnTracker) Acquire(accountID int64) {
	val, _ := t.conns.LoadOrStore(accountID, &atomic.Int64{})
	val.(*atomic.Int64).Add(1)
}

func (t *MemoryConnTracker) Release(accountID int64) {
	if val, ok := t.conns.Load(accountID); ok {
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

func (t *MemoryConnTracker) GetCount(accountID int64) int64 {
	if val, ok := t.conns.Load(accountID); ok {
		return val.(*atomic.Int64).Load()
	}
	return 0
}

func (t *MemoryConnTracker) GetCounts(accountIDs []int64) map[int64]int64 {
	counts := make(map[int64]int64, len(accountIDs))
	for _, id := range accountIDs {
		counts[id] = t.GetCount(id)
	}
	return counts
}

// --- Redis Implementation ---

// RedisConnTracker uses Redis INCR/DECR for distributed connection counting.
type RedisConnTracker struct {
	client        *redis.Client
	prefix        string
	releaseScript *redis.Script
}

func NewRedisConnTracker(client *redis.Client, prefix string) *RedisConnTracker {
	t := &RedisConnTracker{
		client: client,
		prefix: prefix + "conns:",
	}
	// Lua script to decrement but never go below 0
	t.releaseScript = redis.NewScript(`
		local key = KEYS[1]
		local val = tonumber(redis.call("GET", key) or "0")
		if val > 0 then
			return redis.call("DECR", key)
		end
		return 0
	`)

	// Clear stale counters on startup
	t.clearAll()
	return t
}

func (t *RedisConnTracker) key(accountID int64) string {
	return fmt.Sprintf("%s%d", t.prefix, accountID)
}

func (t *RedisConnTracker) Acquire(accountID int64) {
	ctx := context.Background()
	t.client.Incr(ctx, t.key(accountID))
}

func (t *RedisConnTracker) Release(accountID int64) {
	ctx := context.Background()
	t.releaseScript.Run(ctx, t.client, []string{t.key(accountID)})
}

func (t *RedisConnTracker) GetCount(accountID int64) int64 {
	ctx := context.Background()
	val, err := t.client.Get(ctx, t.key(accountID)).Int64()
	if err != nil {
		return 0
	}
	return val
}

func (t *RedisConnTracker) GetCounts(accountIDs []int64) map[int64]int64 {
	ctx := context.Background()
	counts := make(map[int64]int64, len(accountIDs))

	if len(accountIDs) == 0 {
		return counts
	}

	keys := make([]string, len(accountIDs))
	for i, id := range accountIDs {
		keys[i] = t.key(id)
	}

	vals, err := t.client.MGet(ctx, keys...).Result()
	if err != nil {
		// Fallback to individual gets
		for _, id := range accountIDs {
			counts[id] = t.GetCount(id)
		}
		return counts
	}

	for i, val := range vals {
		if val == nil {
			counts[accountIDs[i]] = 0
			continue
		}
		if s, ok := val.(string); ok {
			n, _ := strconv.ParseInt(s, 10, 64)
			counts[accountIDs[i]] = n
		}
	}
	return counts
}

// clearAll removes all connection counter keys on startup.
func (t *RedisConnTracker) clearAll() {
	ctx := context.Background()
	var cursor uint64
	pattern := t.prefix + "*"
	for {
		keys, nextCursor, err := t.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			slog.Warn("Failed to clear connection counters", "error", err)
			return
		}
		if len(keys) > 0 {
			t.client.Del(ctx, keys...)
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}
