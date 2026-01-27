package diagon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

// TestRealDiagonIntegration tests the full integration with real Diagon C++ engine
func TestRealDiagonIntegration(t *testing.T) {
	// Create temporary directory for test index
	tmpDir, err := os.MkdirTemp("", "diagon_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	indexPath := filepath.Join(tmpDir, "test_index")

	// Create index directory
	if err := os.MkdirAll(indexPath, 0755); err != nil {
		t.Fatalf("Failed to create index directory: %v", err)
	}

	// Create logger
	logger := zap.NewNop()

	// Create Diagon bridge
	bridge, err := NewDiagonBridge(&Config{
		DataDir:     tmpDir,
		SIMDEnabled: true,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("Failed to create Diagon bridge: %v", err)
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
	defer shard.Close()

	// Test 1: Index documents
	t.Run("IndexDocuments", func(t *testing.T) {
		docs := []struct {
			id  string
			doc map[string]interface{}
		}{
			{
				"doc1",
				map[string]interface{}{
					"title":   "Golang Programming",
					"content": "Go is a statically typed compiled programming language",
					"tags":    "programming,golang,backend",
					"year":    2023,
				},
			},
			{
				"doc2",
				map[string]interface{}{
					"title":   "Rust Systems Programming",
					"content": "Rust is a systems programming language with memory safety",
					"tags":    "programming,rust,systems",
					"year":    2023,
				},
			},
			{
				"doc3",
				map[string]interface{}{
					"title":   "Python Data Science",
					"content": "Python is widely used for data science and machine learning",
					"tags":    "programming,python,datascience",
					"year":    2024,
				},
			},
		}

		for _, d := range docs {
			err := shard.IndexDocument(d.id, d.doc)
			if err != nil {
				t.Errorf("Failed to index document %s: %v", d.id, err)
			}
		}

		t.Logf("✓ Indexed %d documents", len(docs))
	})

	// Test 2: Commit changes
	t.Run("CommitChanges", func(t *testing.T) {
		if err := shard.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
		t.Log("✓ Committed changes")
	})

	// Test 3: Search with term query
	t.Run("SearchTermQuery", func(t *testing.T) {
		// Search for documents containing "programming" in content field
		query := []byte(`{"term": {"content": "programming"}}`)

		result, err := shard.Search(query, nil)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		t.Logf("✓ Search results: total_hits=%d, max_score=%.4f, num_results=%d",
			result.TotalHits, result.MaxScore, len(result.Hits))

		if result.TotalHits == 0 {
			t.Error("Expected hits > 0, got 0")
		}

		// Verify hits are scored
		for i, hit := range result.Hits {
			t.Logf("  Hit #%d: id=%s, score=%.4f", i+1, hit.ID, hit.Score)
			if hit.Score <= 0 {
				t.Errorf("Hit %d has invalid score: %.4f", i, hit.Score)
			}
		}
	})

	// Test 4: Search with different term
	t.Run("SearchDifferentTerm", func(t *testing.T) {
		// Search for "language" in content field
		query := []byte(`{"term": {"content": "language"}}`)

		result, err := shard.Search(query, nil)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		t.Logf("✓ Search for 'language': total_hits=%d, max_score=%.4f",
			result.TotalHits, result.MaxScore)
	})

	// Test 5: Search with title field
	t.Run("SearchTitleField", func(t *testing.T) {
		// Search for "Golang" in title field
		query := []byte(`{"term": {"title": "Golang"}}`)

		result, err := shard.Search(query, nil)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		t.Logf("✓ Search in title field: total_hits=%d, max_score=%.4f",
			result.TotalHits, result.MaxScore)
	})

	// Test 6: Refresh and search again
	t.Run("RefreshAndSearch", func(t *testing.T) {
		// Refresh to reopen reader
		if err := shard.Refresh(); err != nil {
			t.Fatalf("Failed to refresh: %v", err)
		}
		t.Log("✓ Refreshed shard")

		// Search again after refresh
		query := []byte(`{"term": {"content": "programming"}}`)
		result, err := shard.Search(query, nil)
		if err != nil {
			t.Fatalf("Search after refresh failed: %v", err)
		}

		t.Logf("✓ Search after refresh: total_hits=%d", result.TotalHits)
	})

	// Test 7: Flush to disk
	t.Run("FlushToDisk", func(t *testing.T) {
		if err := shard.Flush(); err != nil {
			t.Fatalf("Failed to flush: %v", err)
		}
		t.Log("✓ Flushed to disk")
	})
}

// TestMultipleShards tests creating and managing multiple shards
func TestMultipleShards(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diagon_multi_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := zap.NewNop()

	bridge, err := NewDiagonBridge(&Config{
		DataDir:     tmpDir,
		SIMDEnabled: true,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	if err := bridge.Start(); err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	// Create multiple shards
	numShards := 3
	shards := make([]*Shard, numShards)

	for i := 0; i < numShards; i++ {
		shardPath := filepath.Join(tmpDir, fmt.Sprintf("shard_%d", i))

		// Create shard directory
		if err := os.MkdirAll(shardPath, 0755); err != nil {
			t.Fatalf("Failed to create shard directory: %v", err)
		}

		shard, err := bridge.CreateShard(shardPath)
		if err != nil {
			t.Fatalf("Failed to create shard %d: %v", i, err)
		}
		shards[i] = shard
		defer shard.Close()

		// Index one document per shard
		doc := map[string]interface{}{
			"shard_id": i,
			"content":  fmt.Sprintf("Document in shard %d with search term", i),
		}
		if err := shard.IndexDocument(fmt.Sprintf("doc_%d", i), doc); err != nil {
			t.Fatalf("Failed to index document in shard %d: %v", i, err)
		}

		if err := shard.Commit(); err != nil {
			t.Fatalf("Failed to commit shard %d: %v", i, err)
		}
	}

	t.Logf("✓ Created and populated %d shards", numShards)

	// Search each shard
	for i, shard := range shards {
		query := []byte(`{"term": {"content": "search"}}`)
		result, err := shard.Search(query, nil)
		if err != nil {
			t.Errorf("Search failed on shard %d: %v", i, err)
			continue
		}

		t.Logf("✓ Shard %d: total_hits=%d", i, result.TotalHits)
	}
}

// TestDiagonPerformance benchmarks indexing and search performance
func TestDiagonPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "diagon_perf_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := zap.NewNop()

	bridge, err := NewDiagonBridge(&Config{
		DataDir:     tmpDir,
		SIMDEnabled: true,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	if err := bridge.Start(); err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	indexPath := filepath.Join(tmpDir, "perf_index")

	// Create index directory
	if err := os.MkdirAll(indexPath, 0755); err != nil {
		t.Fatalf("Failed to create index directory: %v", err)
	}

	shard, err := bridge.CreateShard(indexPath)
	if err != nil {
		t.Fatalf("Failed to create shard: %v", err)
	}
	defer shard.Close()

	// Index 10,000 documents
	numDocs := 10000
	t.Logf("Indexing %d documents...", numDocs)

	for i := 0; i < numDocs; i++ {
		doc := map[string]interface{}{
			"id":      i,
			"title":   fmt.Sprintf("Document %d", i),
			"content": fmt.Sprintf("This is the content of document %d with some searchable terms", i),
			"category": []string{"tech", "science", "programming"}[i%3],
		}

		if err := shard.IndexDocument(fmt.Sprintf("doc_%d", i), doc); err != nil {
			t.Fatalf("Failed to index document %d: %v", i, err)
		}

		// Commit every 1000 docs
		if (i+1)%1000 == 0 {
			if err := shard.Commit(); err != nil {
				t.Fatalf("Failed to commit at doc %d: %v", i, err)
			}
			t.Logf("  Indexed %d/%d documents", i+1, numDocs)
		}
	}

	// Final commit
	if err := shard.Commit(); err != nil {
		t.Fatalf("Failed to final commit: %v", err)
	}

	t.Logf("✓ Indexed %d documents", numDocs)

	// Execute multiple searches
	queries := []string{
		"content",
		"document",
		"searchable",
		"terms",
	}

	for _, term := range queries {
		query := []byte(fmt.Sprintf(`{"term": {"content": "%s"}}`, term))
		result, err := shard.Search(query, nil)
		if err != nil {
			t.Errorf("Search for '%s' failed: %v", term, err)
			continue
		}

		t.Logf("✓ Search '%s': total_hits=%d, max_score=%.4f",
			term, result.TotalHits, result.MaxScore)
	}
}
