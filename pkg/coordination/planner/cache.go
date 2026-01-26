package planner

import (
	"sync"
	"time"
)

// cacheEntry represents a single cache entry
type cacheEntry struct {
	value      interface{}
	expiration time.Time
}

// queryCache is a simple in-memory cache for query results
type queryCache struct {
	entries map[string]*cacheEntry
	mu      sync.RWMutex
	maxSize int
	ttl     time.Duration
}

// newQueryCache creates a new query cache
func newQueryCache(maxSize int, ttl time.Duration) *queryCache {
	cache := &queryCache{
		entries: make(map[string]*cacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// Get retrieves a value from the cache
func (qc *queryCache) Get(key string) (interface{}, bool) {
	qc.mu.RLock()
	defer qc.mu.RUnlock()

	entry, exists := qc.entries[key]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.expiration) {
		return nil, false
	}

	return entry.value, true
}

// Set stores a value in the cache
func (qc *queryCache) Set(key string, value interface{}) {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	// Evict oldest entry if at capacity
	if len(qc.entries) >= qc.maxSize {
		qc.evictOldest()
	}

	qc.entries[key] = &cacheEntry{
		value:      value,
		expiration: time.Now().Add(qc.ttl),
	}
}

// Delete removes a value from the cache
func (qc *queryCache) Delete(key string) {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	delete(qc.entries, key)
}

// Clear removes all entries from the cache
func (qc *queryCache) Clear() {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	qc.entries = make(map[string]*cacheEntry)
}

// Size returns the current number of entries in the cache
func (qc *queryCache) Size() int {
	qc.mu.RLock()
	defer qc.mu.RUnlock()

	return len(qc.entries)
}

// evictOldest removes the oldest entry (simple LRU approximation)
func (qc *queryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range qc.entries {
		if oldestKey == "" || entry.expiration.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.expiration
		}
	}

	if oldestKey != "" {
		delete(qc.entries, oldestKey)
	}
}

// cleanup periodically removes expired entries
func (qc *queryCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		qc.removeExpired()
	}
}

// removeExpired removes all expired entries
func (qc *queryCache) removeExpired() {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	now := time.Now()
	for key, entry := range qc.entries {
		if now.After(entry.expiration) {
			delete(qc.entries, key)
		}
	}
}
