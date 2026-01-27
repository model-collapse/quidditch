package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryPool_Basic(t *testing.T) {
	mp := NewMemoryPool([]int{1024, 4096, 16384})

	t.Run("GetSmallBuffer", func(t *testing.T) {
		buf := mp.Get(512)
		require.NotNil(t, buf)
		assert.Equal(t, 512, len(buf))
		assert.GreaterOrEqual(t, cap(buf), 512)

		// Can write to buffer
		for i := range buf {
			buf[i] = byte(i % 256)
		}

		// Return to pool
		mp.Put(buf)
	})

	t.Run("GetExactSizeBuffer", func(t *testing.T) {
		buf := mp.Get(1024)
		require.NotNil(t, buf)
		assert.Equal(t, 1024, len(buf))

		mp.Put(buf)
	})

	t.Run("GetLargeBuffer", func(t *testing.T) {
		buf := mp.Get(100000)
		require.NotNil(t, buf)
		assert.Equal(t, 100000, len(buf))

		// Should allocate directly, not from pool
		mp.Put(buf)
	})
}

func TestMemoryPool_Reuse(t *testing.T) {
	mp := NewMemoryPool([]int{1024})

	// Get and return a buffer
	buf1 := mp.Get(512)
	cap1 := cap(buf1)
	mp.Put(buf1)

	// Get another buffer of same size
	buf2 := mp.Get(512)
	cap2 := cap(buf2)

	// Should reuse the same underlying array (same capacity)
	assert.Equal(t, cap1, cap2)
	mp.Put(buf2)
}

func TestMemoryPool_Concurrent(t *testing.T) {
	mp := DefaultMemoryPool()

	// Concurrent access
	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func() {
			buf := mp.Get(4096)
			// Simulate work
			for j := range buf {
				buf[j] = byte(j % 256)
			}
			mp.Put(buf)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Pool should still be functional
	buf := mp.Get(1024)
	assert.NotNil(t, buf)
	mp.Put(buf)
}

func TestDefaultMemoryPool(t *testing.T) {
	mp := DefaultMemoryPool()

	stats := mp.GetStats()
	assert.Len(t, stats.PoolSizes, 6) // 6 default sizes

	// Test each size category
	sizes := []int{500, 2048, 10000, 50000, 200000, 500000}

	for _, size := range sizes {
		buf := mp.Get(size)
		assert.Equal(t, size, len(buf))
		assert.GreaterOrEqual(t, cap(buf), size)
		mp.Put(buf)
	}
}

func TestMemoryPool_Clear(t *testing.T) {
	mp := NewMemoryPool([]int{1024})

	// Get some buffers
	bufs := make([][]byte, 10)
	for i := range bufs {
		bufs[i] = mp.Get(512)
	}

	// Return them
	for _, buf := range bufs {
		mp.Put(buf)
	}

	// Clear the pool
	mp.Clear()

	// Pool should still work
	buf := mp.Get(512)
	assert.NotNil(t, buf)
	mp.Put(buf)
}

func TestMemoryPool_PutNil(t *testing.T) {
	mp := NewMemoryPool([]int{1024})

	// Should not panic
	mp.Put(nil)
}

func TestMemoryPool_Stats(t *testing.T) {
	mp := NewMemoryPool([]int{1024, 4096})

	stats := mp.GetStats()
	assert.Len(t, stats.PoolSizes, 2)
	assert.Contains(t, stats.PoolSizes, 1024)
	assert.Contains(t, stats.PoolSizes, 4096)
}

// Benchmark memory pool vs direct allocation
func BenchmarkMemoryPool_Get(b *testing.B) {
	mp := DefaultMemoryPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := mp.Get(4096)
		mp.Put(buf)
	}
}

func BenchmarkDirectAllocation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 4096)
		_ = buf
	}
}

func BenchmarkMemoryPool_Concurrent(b *testing.B) {
	mp := DefaultMemoryPool()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := mp.Get(4096)
			mp.Put(buf)
		}
	})
}
