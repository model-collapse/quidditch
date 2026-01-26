package planner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewQueryCache(t *testing.T) {
	cache := newQueryCache(100, 5*time.Minute)

	assert.NotNil(t, cache)
	assert.Equal(t, 100, cache.maxSize)
	assert.Equal(t, 5*time.Minute, cache.ttl)
	assert.Equal(t, 0, cache.Size())
}

func TestQueryCache_SetAndGet(t *testing.T) {
	cache := newQueryCache(10, 1*time.Hour)

	// Test set and get
	cache.Set("key1", "value1")
	value, found := cache.Get("key1")

	assert.True(t, found)
	assert.Equal(t, "value1", value)
	assert.Equal(t, 1, cache.Size())
}

func TestQueryCache_GetNonExistent(t *testing.T) {
	cache := newQueryCache(10, 1*time.Hour)

	value, found := cache.Get("nonexistent")

	assert.False(t, found)
	assert.Nil(t, value)
}

func TestQueryCache_Expiration(t *testing.T) {
	cache := newQueryCache(10, 100*time.Millisecond)

	cache.Set("key1", "value1")

	// Should exist immediately
	value, found := cache.Get("key1")
	assert.True(t, found)
	assert.Equal(t, "value1", value)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	value, found = cache.Get("key1")
	assert.False(t, found)
	assert.Nil(t, value)
}

func TestQueryCache_Delete(t *testing.T) {
	cache := newQueryCache(10, 1*time.Hour)

	cache.Set("key1", "value1")
	assert.Equal(t, 1, cache.Size())

	cache.Delete("key1")
	assert.Equal(t, 0, cache.Size())

	_, found := cache.Get("key1")
	assert.False(t, found)
}

func TestQueryCache_Clear(t *testing.T) {
	cache := newQueryCache(10, 1*time.Hour)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")
	assert.Equal(t, 3, cache.Size())

	cache.Clear()
	assert.Equal(t, 0, cache.Size())

	_, found := cache.Get("key1")
	assert.False(t, found)
}

func TestQueryCache_MaxSize(t *testing.T) {
	cache := newQueryCache(3, 1*time.Hour)

	// Add 4 entries (should evict oldest)
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")
	cache.Set("key4", "value4")

	// Should have 3 entries
	assert.Equal(t, 3, cache.Size())

	// Oldest (key1) should be evicted
	_, found := cache.Get("key1")
	assert.False(t, found)

	// Others should exist
	_, found = cache.Get("key2")
	assert.True(t, found)
	_, found = cache.Get("key3")
	assert.True(t, found)
	_, found = cache.Get("key4")
	assert.True(t, found)
}

func TestQueryCache_Overwrite(t *testing.T) {
	cache := newQueryCache(10, 1*time.Hour)

	cache.Set("key1", "value1")
	cache.Set("key1", "value2")

	value, found := cache.Get("key1")
	assert.True(t, found)
	assert.Equal(t, "value2", value)
	assert.Equal(t, 1, cache.Size())
}

func TestQueryCache_RemoveExpired(t *testing.T) {
	cache := newQueryCache(10, 100*time.Millisecond)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	assert.Equal(t, 2, cache.Size())

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Manually trigger cleanup
	cache.removeExpired()

	// Both should be removed
	assert.Equal(t, 0, cache.Size())
}

func TestQueryCache_ConcurrentAccess(t *testing.T) {
	cache := newQueryCache(100, 1*time.Hour)
	done := make(chan bool)

	// Multiple goroutines writing
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				cache.Set(string(rune(id*10+j)), id*10+j)
			}
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	// Multiple goroutines reading
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				cache.Get(string(rune(id*10 + j)))
			}
			done <- true
		}(i)
	}

	// Wait for all reads
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have entries
	assert.Greater(t, cache.Size(), 0)
}

func TestQueryCache_Size(t *testing.T) {
	cache := newQueryCache(10, 1*time.Hour)

	assert.Equal(t, 0, cache.Size())

	cache.Set("key1", "value1")
	assert.Equal(t, 1, cache.Size())

	cache.Set("key2", "value2")
	assert.Equal(t, 2, cache.Size())

	cache.Delete("key1")
	assert.Equal(t, 1, cache.Size())

	cache.Clear()
	assert.Equal(t, 0, cache.Size())
}
