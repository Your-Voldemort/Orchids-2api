package summarycache

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"orchids-api/internal/prompt"
)

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
	prefix string
}

func NewRedisCache(addr, password string, db int, ttl time.Duration, prefix string) *RedisCache {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil
	}
	if prefix == "" {
		prefix = "orchids:summary:"
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &RedisCache{
		client: client,
		ttl:    ttl,
		prefix: prefix,
	}
}

func (c *RedisCache) Get(ctx context.Context, key string) (prompt.SummaryCacheEntry, bool) {
	if c == nil || c.client == nil {
		return prompt.SummaryCacheEntry{}, false
	}
	value, err := c.client.Get(ctx, c.prefix+key).Result()
	if err == redis.Nil || err != nil {
		return prompt.SummaryCacheEntry{}, false
	}

	var entry prompt.SummaryCacheEntry
	if err := json.Unmarshal([]byte(value), &entry); err != nil {
		return prompt.SummaryCacheEntry{}, false
	}
	return entry, true
}

func (c *RedisCache) Put(ctx context.Context, key string, entry prompt.SummaryCacheEntry) {
	if c == nil || c.client == nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	if c.ttl > 0 {
		_ = c.client.Set(ctx, c.prefix+key, data, c.ttl).Err()
		return
	}
	_ = c.client.Set(ctx, c.prefix+key, data, 0).Err()
}

func (c *RedisCache) GetStats(ctx context.Context) (int64, int64, error) {
	if c == nil || c.client == nil {
		return 0, 0, nil
	}

	var count int64
	var cursor uint64
	var err error
	var keys []string

	for {
		keys, cursor, err = c.client.Scan(ctx, cursor, c.prefix+"*", 100).Result()
		if err != nil {
			return 0, 0, err
		}
		count += int64(len(keys))
		if cursor == 0 {
			break
		}
	}

	// Size estimation is expensive in Redis (need to fetch all items or debug object).
	// We can skip effective size for now or sample.
	// Returning 0 for size to indicate "unknown" or expensive-to-calculate
	return count, 0, nil
}

func (c *RedisCache) Clear(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}

	var cursor uint64
	var err error
	var keys []string

	for {
		keys, cursor, err = c.client.Scan(ctx, cursor, c.prefix+"*", 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		if cursor == 0 {
			break
		}
	}
	return nil
}
