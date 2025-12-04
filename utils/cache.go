package utils

import (
	"sync"
	"time"
)

// CacheItem represents a cached item with expiration
type CacheItem struct {
	Value      interface{}
	Expiration time.Time
}

// Cache is a simple in-memory cache with TTL
type Cache struct {
	items map[string]CacheItem
	mutex sync.RWMutex
}

var (
	globalCache *Cache
	cacheOnce   sync.Once
)

// GetCache returns the singleton cache instance
func GetCache() *Cache {
	cacheOnce.Do(func() {
		globalCache = &Cache{
			items: make(map[string]CacheItem),
		}
		// Start cleanup goroutine
		go globalCache.cleanup()
	})
	return globalCache
}

// Set stores a value in the cache with TTL
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.items[key] = CacheItem{
		Value:      value,
		Expiration: time.Now().Add(ttl),
	}
}

// Get retrieves a value from the cache
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(item.Expiration) {
		return nil, false
	}

	return item.Value, true
}

// Delete removes a value from the cache
func (c *Cache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.items, key)
}

// Clear removes all items from the cache
func (c *Cache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.items = make(map[string]CacheItem)
}

// Stats returns cache statistics
func (c *Cache) Stats() CacheStats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	var expired, valid int
	now := time.Now()
	for _, item := range c.items {
		if now.After(item.Expiration) {
			expired++
		} else {
			valid++
		}
	}

	return CacheStats{
		TotalItems:   len(c.items),
		ValidItems:   valid,
		ExpiredItems: expired,
	}
}

// CacheStats holds cache statistics
type CacheStats struct {
	TotalItems   int `json:"total_items"`
	ValidItems   int `json:"valid_items"`
	ExpiredItems int `json:"expired_items"`
}

// cleanup periodically removes expired items
func (c *Cache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mutex.Lock()
		now := time.Now()
		for key, item := range c.items {
			if now.After(item.Expiration) {
				delete(c.items, key)
			}
		}
		c.mutex.Unlock()
	}
}
