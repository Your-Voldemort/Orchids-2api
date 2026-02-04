package tokencache

import "time"

func (c *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		c.pruneExpiredLocked(time.Now())
		c.mu.Unlock()
	}
}
