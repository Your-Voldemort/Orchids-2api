package perf

import (
	"sync"
	"time"
)

type CacheItem struct {
	Value      interface{}
	Error      string // Cached error message (empty if no error)
	Expiration int64
}

type TTLCache struct {
	items map[string]CacheItem
	mu    sync.RWMutex
	ttl   time.Duration
}

func NewTTLCache(ttl time.Duration) *TTLCache {
	return &TTLCache{
		items: make(map[string]CacheItem),
		ttl:   ttl,
	}
}

func (c *TTLCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = CacheItem{
		Value:      value,
		Error:      "",
		Expiration: time.Now().Add(c.ttl).UnixNano(),
	}
}

func (c *TTLCache) SetError(key string, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = CacheItem{
		Value:      nil,
		Error:      errMsg,
		Expiration: time.Now().Add(c.ttl).UnixNano(),
	}
}

func (c *TTLCache) Get(key string) (interface{}, string, bool) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		return nil, "", false
	}

	// Check expiration
	if time.Now().UnixNano() > item.Expiration {
		// Lazily delete expired item
		c.mu.Lock()
		if current, ok := c.items[key]; ok && current.Expiration == item.Expiration {
			delete(c.items, key)
		}
		c.mu.Unlock()
		return nil, "", false
	}

	return item.Value, item.Error, true
}

func (c *TTLCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]CacheItem)
}
