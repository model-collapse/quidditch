package diagon

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

// TestDoubleRangeQuery tests unified numeric range query with double fields
func TestDoubleRangeQuery(t *testing.T) {
	// Create temporary directory for test index
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "test_double_index")

	logger := zap.NewNop()

	// Create index directory
	if err := os.MkdirAll(indexPath, 0755); err != nil {
		t.Fatalf("Failed to create index directory: %v", err)
	}

	// Create bridge
	bridge, err := NewDiagonBridge(&Config{
		DataDir:     tmpDir,
		SIMDEnabled: true,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	// Start bridge
	if err := bridge.Start(); err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	// Create shard
	shard, err := bridge.CreateShard(indexPath)
	if err != nil {
		t.Fatalf("Failed to create shard: %v", err)
	}
	defer func() {
		if err := shard.Close(); err != nil {
			t.Errorf("Failed to close shard: %v", err)
		}
		os.RemoveAll(tmpDir)
	}()

	t.Run("IndexDoubleFields", func(t *testing.T) {
		// Index documents with double fields
		docs := []map[string]interface{}{
			{
				"id":    "product_1",
				"name":  "Laptop",
				"price": 999.99,
			},
			{
				"id":    "product_2",
				"name":  "Mouse",
				"price": 29.99,
			},
			{
				"id":    "product_3",
				"name":  "Keyboard",
				"price": 89.99,
			},
			{
				"id":    "product_4",
				"name":  "Monitor",
				"price": 299.99,
			},
			{
				"id":    "product_5",
				"name":  "Headphones",
				"price": 149.99,
			},
		}

		for _, doc := range docs {
			if err := shard.IndexDocument("test_id", doc); err != nil {
				t.Fatalf("Failed to index document: %v", err)
			}
		}

		// Commit changes
		if err := shard.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Refresh to make changes visible to searches
		if err := shard.Refresh(); err != nil {
			t.Fatalf("Failed to refresh: %v", err)
		}

		t.Logf("✓ Indexed %d documents with double price field", len(docs))
	})

	t.Run("VerifyDocumentsSearchable", func(t *testing.T) {
		// First verify that documents are searchable at all
		// Search for "Laptop" in name field
		queryJSON := `{
			"term": {"name": "Laptop"}
		}`

		results, err := shard.Search([]byte(queryJSON), nil)
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}

		t.Logf("✓ Term search for 'Laptop': found %d hits", results.TotalHits)
		if results.TotalHits == 0 {
			t.Error("No documents found! Documents may not be indexed properly")
		}
	})

	t.Run("RangeQuery_BothBounds", func(t *testing.T) {
		// Query: price >= 50.0 AND price <= 200.0
		// Should match: Keyboard (89.99), Headphones (149.99)
		queryJSON := `{
			"range": {
				"price": {
					"gte": 50.0,
					"lte": 200.0
				}
			}
		}`

		results, err := shard.Search([]byte(queryJSON), nil)
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}

		if results.TotalHits != 2 {
			t.Errorf("Expected 2 hits, got %d", results.TotalHits)
		}

		t.Logf("✓ Range query [50.0, 200.0]: found %d hits", results.TotalHits)
	})

	t.Run("RangeQuery_LowerOnly", func(t *testing.T) {
		// Query: price >= 200.0
		// Should match: Monitor (299.99), Laptop (999.99)
		queryJSON := `{
			"range": {
				"price": {
					"gte": 200.0
				}
			}
		}`

		results, err := shard.Search([]byte(queryJSON), nil)
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}

		if results.TotalHits != 2 {
			t.Errorf("Expected 2 hits, got %d", results.TotalHits)
		}

		t.Logf("✓ Range query [200.0, *]: found %d hits", results.TotalHits)
	})

	t.Run("RangeQuery_UpperOnly", func(t *testing.T) {
		// Query: price <= 100.0
		// Should match: Mouse (29.99), Keyboard (89.99)
		queryJSON := `{
			"range": {
				"price": {
					"lte": 100.0
				}
			}
		}`

		results, err := shard.Search([]byte(queryJSON), nil)
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}

		if results.TotalHits != 2 {
			t.Errorf("Expected 2 hits, got %d", results.TotalHits)
		}

		t.Logf("✓ Range query [*, 100.0]: found %d hits", results.TotalHits)
	})

	t.Run("RangeQuery_Exclusive", func(t *testing.T) {
		// Query: price > 50.0 AND price < 200.0
		// Should match: Keyboard (89.99), Headphones (149.99)
		queryJSON := `{
			"range": {
				"price": {
					"gt": 50.0,
					"lt": 200.0
				}
			}
		}`

		results, err := shard.Search([]byte(queryJSON), nil)
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}

		if results.TotalHits != 2 {
			t.Errorf("Expected 2 hits, got %d", results.TotalHits)
		}

		t.Logf("✓ Range query (50.0, 200.0): found %d hits", results.TotalHits)
	})

	t.Run("RangeQuery_PreciseDouble", func(t *testing.T) {
		// Query: price >= 29.99 AND price <= 29.99 (exact match)
		// Should match: Mouse (29.99)
		queryJSON := `{
			"range": {
				"price": {
					"gte": 29.99,
					"lte": 29.99
				}
			}
		}`

		results, err := shard.Search([]byte(queryJSON), nil)
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}

		if results.TotalHits != 1 {
			t.Errorf("Expected 1 hit for exact match, got %d", results.TotalHits)
		}

		t.Logf("✓ Exact range query [29.99, 29.99]: found %d hit", results.TotalHits)
	})
}
