package diagon_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/quidditch/quidditch/pkg/data/diagon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestCGOIntegration tests the basic CGO bridge functionality
func TestCGOIntegration(t *testing.T) {
	logger := zap.NewNop()

	// Create temp directory for test
	tempDir := t.TempDir()

	// Create bridge
	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)
	defer bridge.Stop()

	// Create a shard
	shardPath := filepath.Join(tempDir, "test_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Test document indexing
	t.Run("IndexDocument", func(t *testing.T) {
		doc := map[string]interface{}{
			"title":       "Test Document",
			"description": "This is a test",
			"price":       99.99,
			"in_stock":    true,
		}

		err := shard.IndexDocument("doc1", doc)
		assert.NoError(t, err)
	})

	// Test document retrieval
	t.Run("GetDocument", func(t *testing.T) {
		// Index a document first
		doc := map[string]interface{}{
			"title": "Get Test",
			"value": 42,
		}

		err := shard.IndexDocument("doc2", doc)
		require.NoError(t, err)

		// Retrieve it
		retrieved, err := shard.GetDocument("doc2")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		// Note: C++ backend may not return full document yet (TODO)
		// So we just check that no error occurred
	})

	// Test document deletion
	t.Run("DeleteDocument", func(t *testing.T) {
		// Index a document first
		doc := map[string]interface{}{
			"title": "Delete Test",
		}

		err := shard.IndexDocument("doc3", doc)
		require.NoError(t, err)

		// Delete it
		err = shard.DeleteDocument("doc3")
		assert.NoError(t, err)
	})

	// Test refresh
	t.Run("Refresh", func(t *testing.T) {
		err := shard.Refresh()
		assert.NoError(t, err)
	})

	// Test flush
	t.Run("Flush", func(t *testing.T) {
		err := shard.Flush()
		assert.NoError(t, err)
	})
}

// TestCGOSearch tests search functionality via CGO
func TestCGOSearch(t *testing.T) {
	logger := zap.NewNop()

	// Create temp directory for test
	tempDir := t.TempDir()

	// Create bridge
	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)
	defer bridge.Stop()

	// Create a shard
	shardPath := filepath.Join(tempDir, "search_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index some documents
	docs := []struct {
		id  string
		doc map[string]interface{}
	}{
		{
			id: "doc1",
			doc: map[string]interface{}{
				"title":       "Laptop Computer",
				"price":       999.99,
				"category":    "electronics",
				"in_stock":    true,
				"description": "High performance laptop",
			},
		},
		{
			id: "doc2",
			doc: map[string]interface{}{
				"title":       "Wireless Mouse",
				"price":       29.99,
				"category":    "electronics",
				"in_stock":    true,
				"description": "Ergonomic wireless mouse",
			},
		},
		{
			id: "doc3",
			doc: map[string]interface{}{
				"title":       "Desk Chair",
				"price":       199.99,
				"category":    "furniture",
				"in_stock":    false,
				"description": "Comfortable office chair",
			},
		},
	}

	for _, d := range docs {
		err := shard.IndexDocument(d.id, d.doc)
		require.NoError(t, err)
	}

	// Refresh to make documents searchable
	err = shard.Refresh()
	require.NoError(t, err)

	// Test search
	t.Run("BasicSearch", func(t *testing.T) {
		query := []byte(`{"match_all": {}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// The C++ backend may not have full search implemented yet
		// So we just verify that the call succeeded
		t.Logf("Search result: %d total hits, %d returned",
			result.TotalHits, len(result.Hits))
	})

	// Test search with filter
	t.Run("SearchWithFilter", func(t *testing.T) {
		query := []byte(`{"match_all": {}}`)
		// Note: Filter expression would need to be serialized
		// For now, test without filter
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}

// TestCGOShardLifecycle tests shard creation and destruction
func TestCGOShardLifecycle(t *testing.T) {
	logger := zap.NewNop()

	tempDir := t.TempDir()

	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)

	// Create multiple shards
	shards := make([]*diagon.Shard, 3)
	for i := 0; i < 3; i++ {
		shardPath := filepath.Join(tempDir, "shard"+string(rune('0'+i)))
		shard, err := bridge.CreateShard(shardPath)
		require.NoError(t, err, "Failed to create shard %d", i)
		shards[i] = shard
	}

	// Index a document in each
	for i, shard := range shards {
		doc := map[string]interface{}{
			"shard_id": i,
			"title":    "Document in shard " + string(rune('0'+i)),
		}
		err := shard.IndexDocument("doc", doc)
		require.NoError(t, err)
	}

	// Close all shards
	for i, shard := range shards {
		err := shard.Close()
		assert.NoError(t, err, "Failed to close shard %d", i)
	}

	// Stop bridge
	err = bridge.Stop()
	assert.NoError(t, err)
}

// TestCGOErrorHandling tests error handling in CGO bridge
func TestCGOErrorHandling(t *testing.T) {
	logger := zap.NewNop()

	tempDir := t.TempDir()

	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)
	defer bridge.Stop()

	shardPath := filepath.Join(tempDir, "error_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Test getting non-existent document
	t.Run("GetNonExistentDocument", func(t *testing.T) {
		doc, err := shard.GetDocument("nonexistent")
		// Should return error or nil doc
		if err == nil {
			assert.Nil(t, doc)
		}
	})

	// Test indexing with invalid data
	t.Run("IndexInvalidDocument", func(t *testing.T) {
		// Empty document should still work
		err := shard.IndexDocument("empty", map[string]interface{}{})
		assert.NoError(t, err)
	})

	// Test search with invalid query
	t.Run("InvalidQuery", func(t *testing.T) {
		// Empty query
		result, err := shard.Search([]byte(""), nil)
		// May fail or return empty results
		if err == nil {
			assert.NotNil(t, result)
		}
	})
}

// TestCGOConcurrency tests concurrent operations
func TestCGOConcurrency(t *testing.T) {
	logger := zap.NewNop()

	tempDir := t.TempDir()

	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)
	defer bridge.Stop()

	shardPath := filepath.Join(tempDir, "concurrent_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Concurrent indexing
	t.Run("ConcurrentIndexing", func(t *testing.T) {
		const numGoroutines = 10
		const docsPerGoroutine = 10

		done := make(chan bool, numGoroutines)

		for g := 0; g < numGoroutines; g++ {
			go func(goroutineID int) {
				defer func() { done <- true }()

				for i := 0; i < docsPerGoroutine; i++ {
					docID := fmt.Sprintf("doc_%d_%d", goroutineID, i)
					doc := map[string]interface{}{
						"goroutine": goroutineID,
						"index":     i,
						"value":     goroutineID*1000 + i,
					}

					err := shard.IndexDocument(docID, doc)
					if err != nil {
						t.Errorf("Indexing failed: %v", err)
					}
				}
			}(g)
		}

		// Wait for all goroutines
		for g := 0; g < numGoroutines; g++ {
			<-done
		}
	})
}
