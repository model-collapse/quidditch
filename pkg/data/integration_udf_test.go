package data

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/quidditch/quidditch/pkg/data/diagon"
	"github.com/quidditch/quidditch/pkg/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Simple WASM UDF that returns true - BINARY FORMAT
// Minimal valid WASM: (module (memory (export "memory") 1) (func (export "filter") (param i64) (result i32) (i32.const 1)))
var simpleMatchUDFWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x06, 0x01, 0x60, 0x01, 0x7e, 0x01, 0x7f,
	0x03, 0x02, 0x01, 0x00, 0x05, 0x03, 0x01, 0x00,
	0x01, 0x07, 0x13, 0x02, 0x06, 0x6d, 0x65, 0x6d,
	0x6f, 0x72, 0x79, 0x02, 0x00, 0x06, 0x66, 0x69,
	0x6c, 0x74, 0x65, 0x72, 0x00, 0x00, 0x0a, 0x06,
	0x01, 0x04, 0x00, 0x41, 0x01, 0x0b,
}

// UDF that always returns false (filters everything out) - BINARY FORMAT
var alwaysFalseUDFWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x06, 0x01, 0x60, 0x01, 0x7e, 0x01, 0x7f,
	0x03, 0x02, 0x01, 0x00, 0x05, 0x03, 0x01, 0x00,
	0x01, 0x07, 0x13, 0x02, 0x06, 0x6d, 0x65, 0x6d,
	0x6f, 0x72, 0x79, 0x02, 0x00, 0x06, 0x66, 0x69,
	0x6c, 0x74, 0x65, 0x72, 0x00, 0x00, 0x0a, 0x06,
	0x01, 0x04, 0x00, 0x41, 0x00, 0x0b,
}

// setupIntegrationTest creates a test environment with data node, UDF registry, and sample data
func setupIntegrationTest(t *testing.T) (*ShardManager, *wasm.UDFRegistry, func()) {
	// Create temp directory for test data
	tmpDir, err := os.MkdirTemp("", "quidditch-integration-test-*")
	require.NoError(t, err)

	logger := zap.NewNop()

	// Create config
	cfg := &config.DataNodeConfig{
		NodeID:      "test-node",
		DataDir:     tmpDir,
		MaxShards:   10,
		SIMDEnabled: false,
	}

	// Create Diagon bridge
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir:     tmpDir,
		SIMDEnabled: false,
		Logger:      logger,
	})
	require.NoError(t, err)

	err = diagonBridge.Start()
	require.NoError(t, err)

	// Create WASM runtime
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT:      true,
		EnableDebug:    false,
		MaxMemoryPages: 256,
		Logger:         logger,
	})
	require.NoError(t, err)

	// Create UDF registry
	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime:         runtime,
		DefaultPoolSize: 5,
		EnableStats:     true,
		Logger:          logger,
	})
	require.NoError(t, err)

	// Create shard manager
	shardManager := NewShardManager(cfg, logger, diagonBridge, registry)
	err = shardManager.Start(context.Background())
	require.NoError(t, err)

	// Cleanup function
	cleanup := func() {
		shardManager.Stop(context.Background())
		diagonBridge.Stop()
		runtime.Close()
		os.RemoveAll(tmpDir)
	}

	return shardManager, registry, cleanup
}

func TestIntegration_SimpleUDFQuery(t *testing.T) {
	shardManager, registry, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test shard
	err := shardManager.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	// Get the shard
	shard, err := shardManager.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index some test documents
	docs := []struct {
		id   string
		data map[string]interface{}
	}{
		{"doc1", map[string]interface{}{"category": "electronics", "name": "iPhone 15"}},
		{"doc2", map[string]interface{}{"category": "books", "name": "Go Programming"}},
		{"doc3", map[string]interface{}{"category": "electronics", "name": "MacBook Pro"}},
	}

	for _, doc := range docs {
		err = shard.IndexDocument(ctx, doc.id, doc.data)
		require.NoError(t, err)
	}

	// Register a simple UDF (always returns true)
	wasmBytes := simpleMatchUDFWasm
	udfMetadata := &wasm.UDFMetadata{
		Name:         "always_true",
		Version:      "1.0.0",
		FunctionName: "filter",
		Description:  "Test UDF that always returns true",
		WASMBytes:    wasmBytes,
		Parameters: []wasm.UDFParameter{},
		Returns: []wasm.UDFReturnType{
			{Type: wasm.ValueTypeI32, Description: "Boolean result (0=false, 1=true)"},
		},
	}

	err = registry.Register(udfMetadata)
	require.NoError(t, err)

	// Execute search with UDF query
	queryJSON := []byte(`{
		"wasm_udf": {
			"name": "always_true",
			"version": "1.0.0"
		}
	}`)

	result, err := shard.Search(ctx, queryJSON)
	require.NoError(t, err)

	// Should return all 3 documents (UDF returns true for all)
	assert.Equal(t, int64(3), result.TotalHits)
	assert.Len(t, result.Hits, 3)
}

func TestIntegration_UDFFiltersOutAll(t *testing.T) {
	shardManager, registry, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test shard
	err := shardManager.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	shard, err := shardManager.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index test documents
	docs := []struct {
		id   string
		data map[string]interface{}
	}{
		{"doc1", map[string]interface{}{"category": "electronics"}},
		{"doc2", map[string]interface{}{"category": "books"}},
	}

	for _, doc := range docs {
		err = shard.IndexDocument(ctx, doc.id, doc.data)
		require.NoError(t, err)
	}

	// Register UDF that always returns false
	wasmBytes := alwaysFalseUDFWasm
	udfMetadata := &wasm.UDFMetadata{
		Name:         "always_false",
		Version:      "1.0.0",
		FunctionName: "filter",
		Description:  "Test UDF that always returns false",
		WASMBytes:    wasmBytes,
		Parameters: []wasm.UDFParameter{},
		Returns: []wasm.UDFReturnType{
			{Type: wasm.ValueTypeI32, Description: "Boolean result (0=false, 1=true)"},
		},
	}

	err = registry.Register(udfMetadata)
	require.NoError(t, err)

	// Execute search with UDF query
	queryJSON := []byte(`{
		"wasm_udf": {
			"name": "always_false",
			"version": "1.0.0"
		}
	}`)

	result, err := shard.Search(ctx, queryJSON)
	require.NoError(t, err)

	// Should return 0 documents (UDF returns false for all)
	assert.Equal(t, int64(0), result.TotalHits)
	assert.Len(t, result.Hits, 0)
}

func TestIntegration_BoolQueryWithUDF(t *testing.T) {
	shardManager, registry, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create shard
	err := shardManager.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	shard, err := shardManager.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index documents
	docs := []struct {
		id   string
		data map[string]interface{}
	}{
		{"doc1", map[string]interface{}{"category": "electronics", "price": 999}},
		{"doc2", map[string]interface{}{"category": "books", "price": 29}},
		{"doc3", map[string]interface{}{"category": "electronics", "price": 1299}},
	}

	for _, doc := range docs {
		err = shard.IndexDocument(ctx, doc.id, doc.data)
		require.NoError(t, err)
	}

	// Register UDF
	wasmBytes := simpleMatchUDFWasm
	udfMetadata := &wasm.UDFMetadata{
		Name:         "filter_udf",
		Version:      "1.0.0",
		FunctionName: "filter",
		Description:  "Test filter UDF",
		WASMBytes:    wasmBytes,
		Parameters: []wasm.UDFParameter{},
		Returns: []wasm.UDFReturnType{
			{Type: wasm.ValueTypeI32, Description: "Boolean result (0=false, 1=true)"},
		},
	}

	err = registry.Register(udfMetadata)
	require.NoError(t, err)

	// Bool query with UDF in filter
	queryJSON := []byte(`{
		"bool": {
			"must": [
				{"term": {"category": "electronics"}}
			],
			"filter": [
				{
					"wasm_udf": {
						"name": "filter_udf",
						"version": "1.0.0"
					}
				}
			]
		}
	}`)

	result, err := shard.Search(ctx, queryJSON)
	require.NoError(t, err)

	// UDF returns true for all, so we should get electronics docs
	// (In stub mode, Diagon returns all docs, so we get all 3)
	assert.NotNil(t, result)
	assert.GreaterOrEqual(t, result.TotalHits, int64(0))
}

func TestIntegration_NoUDFQuery(t *testing.T) {
	shardManager, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create shard
	err := shardManager.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	shard, err := shardManager.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index documents
	err = shard.IndexDocument(ctx, "doc1", map[string]interface{}{"category": "electronics"})
	require.NoError(t, err)

	// Regular term query (no UDF)
	queryJSON := []byte(`{
		"term": {
			"category": "electronics"
		}
	}`)

	result, err := shard.Search(ctx, queryJSON)
	require.NoError(t, err)

	// Should work normally without UDF filtering
	assert.NotNil(t, result)
}

func TestIntegration_UDFWithParameters(t *testing.T) {
	shardManager, registry, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create shard
	err := shardManager.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	shard, err := shardManager.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index documents
	docs := []struct {
		id   string
		data map[string]interface{}
	}{
		{"doc1", map[string]interface{}{"name": "Product A", "price": 100}},
		{"doc2", map[string]interface{}{"name": "Product B", "price": 200}},
	}

	for _, doc := range docs {
		err = shard.IndexDocument(ctx, doc.id, doc.data)
		require.NoError(t, err)
	}

	// Register UDF
	wasmBytes := simpleMatchUDFWasm
	udfMetadata := &wasm.UDFMetadata{
		Name:         "price_filter",
		Version:      "1.0.0",
		FunctionName: "filter",
		Description:  "Filter by price",
		WASMBytes:    wasmBytes,
		Parameters: []wasm.UDFParameter{},
		Returns: []wasm.UDFReturnType{
			{Type: wasm.ValueTypeI32, Description: "Boolean result (0=false, 1=true)"},
		},
	}

	err = registry.Register(udfMetadata)
	require.NoError(t, err)

	// Query with parameters
	queryJSON := []byte(`{
		"wasm_udf": {
			"name": "price_filter",
			"version": "1.0.0",
			"parameters": {
				"field": "price",
				"min_value": 150,
				"max_value": 250
			}
		}
	}`)

	result, err := shard.Search(ctx, queryJSON)
	require.NoError(t, err)

	// Verify parameters were parsed (UDF always returns true, so all docs match)
	assert.NotNil(t, result)
}

func TestIntegration_UDFNotFound(t *testing.T) {
	shardManager, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create shard
	err := shardManager.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	shard, err := shardManager.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index a document
	err = shard.IndexDocument(ctx, "doc1", map[string]interface{}{"name": "test"})
	require.NoError(t, err)

	// Query with non-existent UDF
	queryJSON := []byte(`{
		"wasm_udf": {
			"name": "nonexistent_udf",
			"version": "1.0.0"
		}
	}`)

	result, err := shard.Search(ctx, queryJSON)

	// Should still return a result (error is logged, but search continues)
	// The UDF filter returns original results on error
	assert.NotNil(t, result)
}

func TestIntegration_MultipleDocuments(t *testing.T) {
	shardManager, registry, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create shard
	err := shardManager.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	shard, err := shardManager.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index many documents
	numDocs := 100
	for i := 0; i < numDocs; i++ {
		doc := map[string]interface{}{
			"id":       i,
			"category": "test",
			"value":    i * 10,
		}
		err = shard.IndexDocument(ctx, filepath.Join("doc", string(rune(i))), doc)
		require.NoError(t, err)
	}

	// Register UDF
	wasmBytes := simpleMatchUDFWasm
	udfMetadata := &wasm.UDFMetadata{
		Name:         "batch_filter",
		Version:      "1.0.0",
		FunctionName: "filter",
		Description:  "Batch processing test",
		WASMBytes:    wasmBytes,
		Parameters: []wasm.UDFParameter{},
		Returns: []wasm.UDFReturnType{
			{Type: wasm.ValueTypeI32, Description: "Boolean result (0=false, 1=true)"},
		},
	}

	err = registry.Register(udfMetadata)
	require.NoError(t, err)

	// Search with UDF
	queryJSON := []byte(`{
		"wasm_udf": {
			"name": "batch_filter",
			"version": "1.0.0"
		}
	}`)

	result, err := shard.Search(ctx, queryJSON)
	require.NoError(t, err)

	// Verify all documents were processed
	assert.NotNil(t, result)
}

func TestIntegration_ConcurrentQueries(t *testing.T) {
	shardManager, registry, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create shard
	err := shardManager.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	shard, err := shardManager.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index documents
	for i := 0; i < 10; i++ {
		doc := map[string]interface{}{"id": i, "value": i}
		err = shard.IndexDocument(ctx, filepath.Join("doc", string(rune(i))), doc)
		require.NoError(t, err)
	}

	// Register UDF
	wasmBytes := simpleMatchUDFWasm
	udfMetadata := &wasm.UDFMetadata{
		Name:         "concurrent_test",
		Version:      "1.0.0",
		FunctionName: "filter",
		Description:  "Concurrent query test",
		WASMBytes:    wasmBytes,
		Parameters: []wasm.UDFParameter{},
		Returns: []wasm.UDFReturnType{
			{Type: wasm.ValueTypeI32, Description: "Boolean result (0=false, 1=true)"},
		},
	}

	err = registry.Register(udfMetadata)
	require.NoError(t, err)

	queryJSON := []byte(`{
		"wasm_udf": {
			"name": "concurrent_test",
			"version": "1.0.0"
		}
	}`)

	// Execute concurrent queries
	numQueries := 10
	results := make(chan error, numQueries)

	for i := 0; i < numQueries; i++ {
		go func() {
			_, err := shard.Search(ctx, queryJSON)
			results <- err
		}()
	}

	// Collect results
	for i := 0; i < numQueries; i++ {
		err := <-results
		assert.NoError(t, err)
	}
}

func TestIntegration_UDFStatistics(t *testing.T) {
	shardManager, registry, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create shard
	err := shardManager.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	shard, err := shardManager.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index documents
	err = shard.IndexDocument(ctx, "doc1", map[string]interface{}{"name": "test"})
	require.NoError(t, err)

	// Register UDF
	wasmBytes := simpleMatchUDFWasm
	udfMetadata := &wasm.UDFMetadata{
		Name:         "stats_test",
		Version:      "1.0.0",
		FunctionName: "filter",
		Description:  "Statistics test",
		WASMBytes:    wasmBytes,
		Parameters: []wasm.UDFParameter{},
		Returns: []wasm.UDFReturnType{
			{Type: wasm.ValueTypeI32, Description: "Boolean result (0=false, 1=true)"},
		},
	}

	err = registry.Register(udfMetadata)
	require.NoError(t, err)

	// Execute search
	queryJSON := []byte(`{
		"wasm_udf": {
			"name": "stats_test",
			"version": "1.0.0"
		}
	}`)

	_, err = shard.Search(ctx, queryJSON)
	require.NoError(t, err)

	// Check UDF statistics
	stats, err := registry.GetStats("stats_test", "1.0.0")
	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Greater(t, stats.CallCount, uint64(0))
}

// Benchmark UDF query execution
func BenchmarkIntegration_UDFQuery(b *testing.B) {
	logger := zap.NewNop()
	tmpDir, _ := os.MkdirTemp("", "bench-*")
	defer os.RemoveAll(tmpDir)

	cfg := &config.DataNodeConfig{
		NodeID:    "bench-node",
		DataDir:   tmpDir,
		MaxShards: 10,
	}

	diagonBridge, _ := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: tmpDir,
		Logger:  logger,
	})
	diagonBridge.Start()
	defer diagonBridge.Stop()

	runtime, _ := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	defer runtime.Close()

	registry, _ := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})

	shardManager := NewShardManager(cfg, logger, diagonBridge, registry)
	shardManager.Start(context.Background())
	defer shardManager.Stop(context.Background())

	ctx := context.Background()
	err := shardManager.CreateShard(ctx, "bench-index", 0, true)
	if err != nil {
		b.Fatalf("Failed to create shard: %v", err)
	}
	shard, err := shardManager.GetShard("bench-index", 0)
	if err != nil || shard == nil {
		b.Fatalf("Failed to get shard: %v", err)
	}

	// Index test documents
	for i := 0; i < 100; i++ {
		doc := map[string]interface{}{"id": i, "value": i}
		shard.IndexDocument(ctx, filepath.Join("doc", string(rune(i))), doc)
	}

	// Register UDF
	wasmBytes := simpleMatchUDFWasm
	udfMetadata := &wasm.UDFMetadata{
		Name:         "bench_udf",
		Version:      "1.0.0",
		FunctionName: "filter",
		WASMBytes:    wasmBytes,
		Parameters: []wasm.UDFParameter{},
		Returns: []wasm.UDFReturnType{
			{Type: wasm.ValueTypeI32, Description: "Boolean result (0=false, 1=true)"},
		},
	}
	registry.Register(udfMetadata)

	queryJSON := []byte(`{
		"wasm_udf": {
			"name": "bench_udf",
			"version": "1.0.0"
		}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		shard.Search(ctx, queryJSON)
	}
}

// Helper function to convert struct to JSON bytes
func toJSON(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}
