package diagon

import (
	"testing"

	"go.uber.org/zap"
)

func TestCGOIntegration(t *testing.T) {
	// Create logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create bridge
	cfg := &Config{
		DataDir:     "/tmp/test_diagon",
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := NewDiagonBridge(cfg)
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	// Start engine
	if err := bridge.Start(); err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	// Verify CGO is enabled
	if !bridge.cgoEnabled {
		t.Fatal("CGO should be enabled")
	}

	t.Log("✅ CGO bridge initialized successfully")
}

func TestShardCreation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &Config{
		DataDir:     "/tmp/test_diagon",
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := NewDiagonBridge(cfg)
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	if err := bridge.Start(); err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	// Create shard
	shard, err := bridge.CreateShard("/tmp/test_shard_1")
	if err != nil {
		t.Fatalf("Failed to create shard: %v", err)
	}

	// Verify shard was created
	if shard == nil {
		t.Fatal("Shard should not be nil")
	}

	// Verify C++ shard pointer was created
	if bridge.cgoEnabled && shard.shardPtr == nil {
		t.Fatal("C++ shard pointer should not be nil when CGO enabled")
	}

	t.Log("✅ C++ shard created successfully")
}

func TestSearchWithoutFilter(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &Config{
		DataDir:     "/tmp/test_diagon",
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := NewDiagonBridge(cfg)
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	if err := bridge.Start(); err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	shard, err := bridge.CreateShard("/tmp/test_shard_2")
	if err != nil {
		t.Fatalf("Failed to create shard: %v", err)
	}

	// Execute search without filter
	query := []byte(`{"match_all":{}}`)
	result, err := shard.Search(query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Verify result structure
	if result == nil {
		t.Fatal("Result should not be nil")
	}

	if result.Hits == nil {
		t.Fatal("Hits array should not be nil")
	}

	t.Logf("✅ Search completed: took=%dms, total_hits=%d", result.Took, result.TotalHits)
}

func TestSearchWithFilter(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &Config{
		DataDir:     "/tmp/test_diagon",
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := NewDiagonBridge(cfg)
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	if err := bridge.Start(); err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	shard, err := bridge.CreateShard("/tmp/test_shard_3")
	if err != nil {
		t.Fatalf("Failed to create shard: %v", err)
	}

	// Execute search with mock filter expression
	// Note: This is a placeholder - real filter expression would be serialized
	query := []byte(`{"match_all":{}}`)
	filterExpr := []byte{} // Empty for now - would be serialized expression tree

	result, err := shard.Search(query, filterExpr)
	if err != nil {
		t.Fatalf("Search with filter failed: %v", err)
	}

	// Verify result
	if result == nil {
		t.Fatal("Result should not be nil")
	}

	t.Logf("✅ Search with filter completed: took=%dms, total_hits=%d", result.Took, result.TotalHits)
}

func TestIndexAndSearch(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &Config{
		DataDir:     "/tmp/test_diagon",
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := NewDiagonBridge(cfg)
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	if err := bridge.Start(); err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	shard, err := bridge.CreateShard("/tmp/test_shard_4")
	if err != nil {
		t.Fatalf("Failed to create shard: %v", err)
	}

	// Index some documents
	docs := []map[string]interface{}{
		{"id": "1", "price": 100.0, "name": "Product A"},
		{"id": "2", "price": 200.0, "name": "Product B"},
		{"id": "3", "price": 150.0, "name": "Product C"},
	}

	for _, doc := range docs {
		if err := shard.IndexDocument(doc["id"].(string), doc); err != nil {
			t.Fatalf("Failed to index document: %v", err)
		}
	}

	t.Log("✅ Indexed 3 documents")

	// Search
	query := []byte(`{"match_all":{}}`)
	result, err := shard.Search(query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	t.Logf("✅ Search found %d documents", result.TotalHits)
}
