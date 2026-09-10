package utils

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/yafyx/baak-api/config"
)

// CacheItem represents a cached item with expiration
type CacheItem struct {
	Value      interface{}
	Expiration time.Time
}

// Cache is a simple in-memory cache with TTL
type Cache struct {
	items      map[string]CacheItem
	loading    map[string]*cacheLoad
	mutex      sync.RWMutex
	maxEntries int
}

type cacheLoad struct {
	done     chan struct{}
	value    interface{}
	err      error
	canceled bool
}

var (
	globalCache *Cache
	cacheOnce   sync.Once
)

// GetCache returns the singleton cache instance
func GetCache() *Cache {
	cacheOnce.Do(func() {
		globalCache = &Cache{
			items:      make(map[string]CacheItem),
			maxEntries: config.AppConfig.CacheMaxEntries,
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
	if ttl <= 0 {
		delete(c.items, key)
		return
	}
	if c.items == nil {
		c.items = make(map[string]CacheItem)
	}
	limit := c.maxEntries
	if limit <= 0 {
		limit = 1024
	}
	if _, exists := c.items[key]; !exists && len(c.items) >= limit {
		// Evict the entry nearest expiration, including already expired entries.
		var oldestKey string
		var oldest time.Time
		for candidate, item := range c.items {
			if oldest.IsZero() || item.Expiration.Before(oldest) {
				oldestKey, oldest = candidate, item.Expiration
			}
		}
		delete(c.items, oldestKey)
	}
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

// GetOrLoad lets concurrent cache misses share one load. A waiting request can
// cancel independently; a failed load is shared with waiters but never cached.
func (c *Cache) GetOrLoad(
	ctx context.Context,
	key string,
	ttl time.Duration,
	load func(context.Context) (interface{}, error),
) (interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if ttl <= 0 {
			return load(ctx)
		}

		c.mutex.Lock()
		if item, ok := c.items[key]; ok && time.Now().Before(item.Expiration) {
			c.mutex.Unlock()
			return item.Value, nil
		}
		if pending, ok := c.loading[key]; ok {
			c.mutex.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-pending.done:
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				// A canceled leader must not cancel requests that are still waiting.
				if pending.canceled {
					continue
				}
				return pending.value, pending.err
			}
		}
		if c.loading == nil {
			c.loading = make(map[string]*cacheLoad)
		}
		pending := &cacheLoad{
			done: make(chan struct{}),
			err:  errors.New("cache load did not complete"),
		}
		c.loading[key] = pending
		c.mutex.Unlock()

		// Always release waiters, including when the loader panics.
		defer func() {
			panicValue := recover()
			c.mutex.Lock()
			if panicValue != nil {
				pending.canceled = true
			}
			delete(c.loading, key)
			close(pending.done)
			c.mutex.Unlock()
			if panicValue != nil {
				panic(panicValue)
			}
		}()
		value, err := load(ctx)
		if ctx.Err() != nil {
			err = ctx.Err()
			pending.canceled = true
		}
		if err != nil {
			pending.err = err
			return nil, err
		}
		c.Set(key, value, ttl)
		pending.value, pending.err = value, nil
		return value, nil
	}
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
