package summarycache

import (
	"container/list"
	"context"
	"sync"
	"time"

	"orchids-api/internal/prompt"
)

type MemoryCache struct {
	mu         sync.RWMutex
	maxEntries int
	ttl        time.Duration
	ll         *list.List
	items      map[string]*list.Element
}

type cacheItem struct {
	key       string
	value     prompt.SummaryCacheEntry
	expiresAt time.Time
}

func NewMemoryCache(maxEntries int, ttl time.Duration) *MemoryCache {
	if maxEntries < 0 {
		maxEntries = 0
	}
	return &MemoryCache{
		maxEntries: maxEntries,
		ttl:        ttl,
		ll:         list.New(),
		items:      make(map[string]*list.Element),
	}
}

func (c *MemoryCache) Get(ctx context.Context, key string) (prompt.SummaryCacheEntry, bool) {
	if c == nil || c.maxEntries <= 0 {
		return prompt.SummaryCacheEntry{}, false
	}

	c.mu.RLock()
	el, ok := c.items[key]
	if !ok {
		c.mu.RUnlock()
		return prompt.SummaryCacheEntry{}, false
	}

	item := el.Value.(*cacheItem)
	if c.ttl > 0 && time.Now().After(item.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		c.removeElement(el)
		c.mu.Unlock()
		return prompt.SummaryCacheEntry{}, false
	}

	c.mu.RUnlock()
	c.mu.Lock()
	c.ll.MoveToFront(el)
	value := item.value
	c.mu.Unlock()

	return value, true
}

func (c *MemoryCache) Put(ctx context.Context, key string, entry prompt.SummaryCacheEntry) {
	if c == nil || c.maxEntries <= 0 {
		return
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = time.Now()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		item := el.Value.(*cacheItem)
		item.value = entry
		item.expiresAt = c.expiryTime()
		c.ll.MoveToFront(el)
		return
	}

	item := &cacheItem{
		key:       key,
		value:     entry,
		expiresAt: c.expiryTime(),
	}
	el := c.ll.PushFront(item)
	c.items[key] = el

	if c.ll.Len() > c.maxEntries {
		c.removeOldest()
	}
}

func (c *MemoryCache) expiryTime() time.Time {
	if c.ttl <= 0 {
		return time.Time{}
	}
	return time.Now().Add(c.ttl)
}

func (c *MemoryCache) removeOldest() {
	el := c.ll.Back()
	if el != nil {
		c.removeElement(el)
	}
}

func (c *MemoryCache) removeElement(el *list.Element) {
	c.ll.Remove(el)
	item := el.Value.(*cacheItem)
	delete(c.items, item.key)
}

func (c *MemoryCache) GetStats(ctx context.Context) (int64, int64, error) {
	if c == nil {
		return 0, 0, nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := int64(len(c.items))
	// Estimate size: simple approximation
	var size int64
	for _, el := range c.items {
		item := el.Value.(*cacheItem)
		size += int64(len(item.key) + len(item.value.Summary))
		for _, line := range item.value.Lines {
			size += int64(len(line))
		}
	}

	return count, size, nil
}

func (c *MemoryCache) Clear(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ll.Init()
	c.items = make(map[string]*list.Element)
	return nil
}
