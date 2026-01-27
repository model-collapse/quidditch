package cache

import (
	"container/list"
	"sync"
	"time"
)

// Entry represents a cached entry with TTL
type Entry struct {
	Key        string
	Value      interface{}
	ExpiresAt  time.Time
	AccessedAt time.Time
	CreatedAt  time.Time
	Size       int64 // Estimated size in bytes
}

// LRUCache is a thread-safe LRU cache with TTL support
type LRUCache struct {
	mu          sync.RWMutex
	capacity    int           // Maximum number of entries
	maxSize     int64         // Maximum total size in bytes (0 = unlimited)
	ttl         time.Duration // Time-to-live for entries
	entries     map[string]*list.Element
	evictionList *list.List
	currentSize int64

	// Statistics
	hits      int64
	misses    int64
	evictions int64
	expirations int64
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(capacity int, maxSize int64, ttl time.Duration) *LRUCache {
	return &LRUCache{
		capacity:     capacity,
		maxSize:      maxSize,
		ttl:          ttl,
		entries:      make(map[string]*list.Element),
		evictionList: list.New(),
		currentSize:  0,
		hits:         0,
		misses:       0,
		evictions:    0,
		expirations:  0,
	}
}

// Get retrieves a value from the cache
func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, exists := c.entries[key]
	if !exists {
		c.misses++
		return nil, false
	}

	entry := elem.Value.(*Entry)

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		c.removeElement(elem)
		c.expirations++
		c.misses++
		return nil, false
	}

	// Move to front (most recently used)
	c.evictionList.MoveToFront(elem)
	entry.AccessedAt = time.Now()
	c.hits++

	return entry.Value, true
}

// Put adds or updates a value in the cache
func (c *LRUCache) Put(key string, value interface{}, size int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Check if entry already exists
	if elem, exists := c.entries[key]; exists {
		// Update existing entry
		entry := elem.Value.(*Entry)
		c.currentSize -= entry.Size
		entry.Value = value
		entry.Size = size
		entry.ExpiresAt = now.Add(c.ttl)
		entry.AccessedAt = now
		c.currentSize += size
		c.evictionList.MoveToFront(elem)
		return
	}

	// Create new entry
	entry := &Entry{
		Key:        key,
		Value:      value,
		ExpiresAt:  now.Add(c.ttl),
		AccessedAt: now,
		CreatedAt:  now,
		Size:       size,
	}

	// Add to front of eviction list
	elem := c.evictionList.PushFront(entry)
	c.entries[key] = elem
	c.currentSize += size

	// Evict if necessary
	c.evictIfNeeded()
}

// evictIfNeeded removes entries if cache is over capacity or size limit
func (c *LRUCache) evictIfNeeded() {
	// Evict based on count capacity
	for len(c.entries) > c.capacity {
		c.evictOldest()
	}

	// Evict based on size limit
	if c.maxSize > 0 {
		for c.currentSize > c.maxSize && c.evictionList.Len() > 0 {
			c.evictOldest()
		}
	}
}

// evictOldest removes the least recently used entry
func (c *LRUCache) evictOldest() {
	elem := c.evictionList.Back()
	if elem != nil {
		c.removeElement(elem)
		c.evictions++
	}
}

// removeElement removes an element from the cache
func (c *LRUCache) removeElement(elem *list.Element) {
	c.evictionList.Remove(elem)
	entry := elem.Value.(*Entry)
	delete(c.entries, entry.Key)
	c.currentSize -= entry.Size
}

// Delete removes a specific key from the cache
func (c *LRUCache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, exists := c.entries[key]
	if !exists {
		return false
	}

	c.removeElement(elem)
	return true
}

// Clear removes all entries from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*list.Element)
	c.evictionList = list.New()
	c.currentSize = 0
	// Keep statistics
}

// Size returns the current number of entries
func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// CurrentSize returns the current total size in bytes
func (c *LRUCache) CurrentSize() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentSize
}

// Stats returns cache statistics
func (c *LRUCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalRequests := c.hits + c.misses
	hitRate := 0.0
	if totalRequests > 0 {
		hitRate = float64(c.hits) / float64(totalRequests)
	}

	return CacheStats{
		Hits:        c.hits,
		Misses:      c.misses,
		HitRate:     hitRate,
		Evictions:   c.evictions,
		Expirations: c.expirations,
		Size:        len(c.entries),
		BytesUsed:   c.currentSize,
	}
}

// ResetStats resets cache statistics
func (c *LRUCache) ResetStats() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hits = 0
	c.misses = 0
	c.evictions = 0
	c.expirations = 0
}

// CleanupExpired removes all expired entries
func (c *LRUCache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	// Iterate through all entries and remove expired ones
	for elem := c.evictionList.Back(); elem != nil; {
		entry := elem.Value.(*Entry)
		prev := elem.Prev()

		if now.After(entry.ExpiresAt) {
			c.removeElement(elem)
			c.expirations++
			removed++
		}

		elem = prev
	}

	return removed
}

// CacheStats represents cache statistics
type CacheStats struct {
	Hits        int64
	Misses      int64
	HitRate     float64
	Evictions   int64
	Expirations int64
	Size        int
	BytesUsed   int64
}
