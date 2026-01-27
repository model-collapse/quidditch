package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewLRUCache(t *testing.T) {
	cache := NewLRUCache(10, 1024, 5*time.Minute)
	assert.NotNil(t, cache)
	assert.Equal(t, 0, cache.Size())
	assert.Equal(t, int64(0), cache.CurrentSize())
}

func TestLRUCache_PutAndGet(t *testing.T) {
	cache := NewLRUCache(10, 1024, 5*time.Minute)

	// Put a value
	cache.Put("key1", "value1", 10)
	assert.Equal(t, 1, cache.Size())
	assert.Equal(t, int64(10), cache.CurrentSize())

	// Get the value
	value, found := cache.Get("key1")
	assert.True(t, found)
	assert.Equal(t, "value1", value)

	// Get non-existent key
	value, found = cache.Get("key2")
	assert.False(t, found)
	assert.Nil(t, value)
}

func TestLRUCache_Update(t *testing.T) {
	cache := NewLRUCache(10, 1024, 5*time.Minute)

	// Put initial value
	cache.Put("key1", "value1", 10)
	assert.Equal(t, int64(10), cache.CurrentSize())

	// Update with new value and size
	cache.Put("key1", "value2", 20)
	assert.Equal(t, 1, cache.Size())
	assert.Equal(t, int64(20), cache.CurrentSize())

	// Verify updated value
	value, found := cache.Get("key1")
	assert.True(t, found)
	assert.Equal(t, "value2", value)
}

func TestLRUCache_CapacityEviction(t *testing.T) {
	cache := NewLRUCache(3, 0, 5*time.Minute)

	// Fill cache to capacity
	cache.Put("key1", "value1", 10)
	cache.Put("key2", "value2", 10)
	cache.Put("key3", "value3", 10)
	assert.Equal(t, 3, cache.Size())

	// Add one more - should evict LRU (key1)
	cache.Put("key4", "value4", 10)
	assert.Equal(t, 3, cache.Size())

	// key1 should be evicted
	_, found := cache.Get("key1")
	assert.False(t, found)

	// Other keys should still be present
	_, found = cache.Get("key2")
	assert.True(t, found)
	_, found = cache.Get("key3")
	assert.True(t, found)
	_, found = cache.Get("key4")
	assert.True(t, found)
}

func TestLRUCache_LRUOrdering(t *testing.T) {
	cache := NewLRUCache(3, 0, 5*time.Minute)

	// Add three items
	cache.Put("key1", "value1", 10)
	cache.Put("key2", "value2", 10)
	cache.Put("key3", "value3", 10)

	// Access key1 to make it most recently used
	cache.Get("key1")

	// Add new item - should evict key2 (oldest)
	cache.Put("key4", "value4", 10)

	// key2 should be evicted
	_, found := cache.Get("key2")
	assert.False(t, found)

	// key1, key3, key4 should be present
	_, found = cache.Get("key1")
	assert.True(t, found)
	_, found = cache.Get("key3")
	assert.True(t, found)
	_, found = cache.Get("key4")
	assert.True(t, found)
}

func TestLRUCache_SizeEviction(t *testing.T) {
	cache := NewLRUCache(10, 100, 5*time.Minute) // Max 100 bytes

	// Add items totaling 90 bytes
	cache.Put("key1", "value1", 30)
	cache.Put("key2", "value2", 30)
	cache.Put("key3", "value3", 30)
	assert.Equal(t, 3, cache.Size())
	assert.Equal(t, int64(90), cache.CurrentSize())

	// Add 50 bytes - should evict until under limit
	cache.Put("key4", "value4", 50)

	// Should have evicted enough to stay under limit
	assert.True(t, cache.CurrentSize() <= 100)

	// key4 should be present
	_, found := cache.Get("key4")
	assert.True(t, found)
}

func TestLRUCache_TTLExpiration(t *testing.T) {
	cache := NewLRUCache(10, 0, 100*time.Millisecond)

	// Add a value
	cache.Put("key1", "value1", 10)

	// Should be retrievable immediately
	value, found := cache.Get("key1")
	assert.True(t, found)
	assert.Equal(t, "value1", value)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	value, found = cache.Get("key1")
	assert.False(t, found)
	assert.Nil(t, value)

	// Cache should be empty
	assert.Equal(t, 0, cache.Size())
}

func TestLRUCache_Delete(t *testing.T) {
	cache := NewLRUCache(10, 0, 5*time.Minute)

	// Add values
	cache.Put("key1", "value1", 10)
	cache.Put("key2", "value2", 10)
	assert.Equal(t, 2, cache.Size())

	// Delete key1
	deleted := cache.Delete("key1")
	assert.True(t, deleted)
	assert.Equal(t, 1, cache.Size())

	// key1 should not be retrievable
	_, found := cache.Get("key1")
	assert.False(t, found)

	// key2 should still be present
	_, found = cache.Get("key2")
	assert.True(t, found)

	// Delete non-existent key
	deleted = cache.Delete("key3")
	assert.False(t, deleted)
}

func TestLRUCache_Clear(t *testing.T) {
	cache := NewLRUCache(10, 0, 5*time.Minute)

	// Add values
	cache.Put("key1", "value1", 10)
	cache.Put("key2", "value2", 10)
	cache.Put("key3", "value3", 10)
	assert.Equal(t, 3, cache.Size())

	// Clear cache
	cache.Clear()
	assert.Equal(t, 0, cache.Size())
	assert.Equal(t, int64(0), cache.CurrentSize())

	// All keys should be gone
	_, found := cache.Get("key1")
	assert.False(t, found)
	_, found = cache.Get("key2")
	assert.False(t, found)
	_, found = cache.Get("key3")
	assert.False(t, found)
}

func TestLRUCache_Stats(t *testing.T) {
	cache := NewLRUCache(10, 0, 5*time.Minute)

	// Initial stats
	stats := cache.Stats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, 0.0, stats.HitRate)

	// Add values
	cache.Put("key1", "value1", 10)
	cache.Put("key2", "value2", 10)

	// Hit
	cache.Get("key1")
	// Miss
	cache.Get("key3")

	stats = cache.Stats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, 0.5, stats.HitRate)
	assert.Equal(t, 2, stats.Size)
	assert.Equal(t, int64(20), stats.BytesUsed)
}

func TestLRUCache_CleanupExpired(t *testing.T) {
	cache := NewLRUCache(10, 0, 100*time.Millisecond)

	// Add values
	cache.Put("key1", "value1", 10)
	time.Sleep(50 * time.Millisecond)
	cache.Put("key2", "value2", 10)
	time.Sleep(50 * time.Millisecond)
	cache.Put("key3", "value3", 10)

	// key1 and key2 should be expired, key3 should not
	time.Sleep(50 * time.Millisecond)

	// Cleanup
	removed := cache.CleanupExpired()
	assert.Equal(t, 2, removed)
	assert.Equal(t, 1, cache.Size())

	// key3 should still be present
	_, found := cache.Get("key3")
	assert.True(t, found)
}

func TestLRUCache_ConcurrentAccess(t *testing.T) {
	cache := NewLRUCache(100, 0, 5*time.Minute)

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key%d-%d", id, j)
				cache.Put(key, fmt.Sprintf("value%d-%d", id, j), 10)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Cache should have entries
	assert.True(t, cache.Size() > 0)
	assert.True(t, cache.Size() <= 100)
}

func TestLRUCache_ResetStats(t *testing.T) {
	cache := NewLRUCache(10, 0, 5*time.Minute)

	// Generate some stats
	cache.Put("key1", "value1", 10)
	cache.Get("key1")
	cache.Get("key2")

	stats := cache.Stats()
	assert.True(t, stats.Hits > 0 || stats.Misses > 0)

	// Reset stats
	cache.ResetStats()

	stats = cache.Stats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, 0.0, stats.HitRate)

	// Size should not be reset
	assert.Equal(t, 1, stats.Size)
}
