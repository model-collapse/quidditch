package wasm

import (
	"sync"
)

// MemoryPool manages reusable memory buffers for WASM module execution
// This reduces GC pressure by reusing buffers instead of allocating new ones
type MemoryPool struct {
	pools map[int]*sync.Pool // Pools by size
	sizes []int              // Available sizes (e.g., 1KB, 4KB, 16KB, 64KB)
	mu    sync.RWMutex       // Protects pool configuration
}

// NewMemoryPool creates a new memory pool with specified buffer sizes
func NewMemoryPool(sizes []int) *MemoryPool {
	mp := &MemoryPool{
		pools: make(map[int]*sync.Pool),
		sizes: sizes,
	}

	// Create a pool for each size
	for _, size := range sizes {
		s := size // Capture for closure
		mp.pools[s] = &sync.Pool{
			New: func() interface{} {
				return make([]byte, s)
			},
		}
	}

	return mp
}

// DefaultMemoryPool creates a memory pool with common buffer sizes
func DefaultMemoryPool() *MemoryPool {
	// Common sizes: 1KB, 4KB, 16KB, 64KB, 256KB, 1MB
	return NewMemoryPool([]int{
		1024,          // 1KB
		4096,          // 4KB
		16384,         // 16KB
		65536,         // 64KB
		262144,        // 256KB
		1048576,       // 1MB
	})
}

// Get retrieves a buffer of at least the requested size
// Returns a buffer from the smallest pool that fits the request
func (mp *MemoryPool) Get(size int) []byte {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	// Find smallest pool that fits
	for _, poolSize := range mp.sizes {
		if poolSize >= size {
			buf := mp.pools[poolSize].Get().([]byte)
			return buf[:size] // Return slice of requested size
		}
	}

	// Size too large for any pool, allocate directly
	// This should be rare in practice
	return make([]byte, size)
}

// Put returns a buffer to the appropriate pool
func (mp *MemoryPool) Put(buf []byte) {
	if buf == nil {
		return
	}

	mp.mu.RLock()
	defer mp.mu.RUnlock()

	// Get the actual capacity (not length)
	cap := cap(buf)

	// Return to the matching pool
	if pool, ok := mp.pools[cap]; ok {
		// Reset buffer to full capacity before returning
		pool.Put(buf[:cap])
	}
	// If no matching pool, let GC handle it
}

// Stats returns statistics about pool usage
type MemoryPoolStats struct {
	PoolSizes []int          // Configured pool sizes
	PoolStats map[int]int    // Current pool item counts (best effort)
}

// GetStats returns memory pool statistics
func (mp *MemoryPool) GetStats() MemoryPoolStats {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	stats := MemoryPoolStats{
		PoolSizes: make([]int, len(mp.sizes)),
		PoolStats: make(map[int]int),
	}

	copy(stats.PoolSizes, mp.sizes)

	// Note: sync.Pool doesn't expose size, so this is approximate
	// We can't directly count items in a sync.Pool

	return stats
}

// Clear empties all pools (useful for testing or memory pressure situations)
func (mp *MemoryPool) Clear() {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Recreate all pools
	for _, size := range mp.sizes {
		s := size
		mp.pools[s] = &sync.Pool{
			New: func() interface{} {
				return make([]byte, s)
			},
		}
	}
}
