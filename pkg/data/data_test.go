package data

import (
	"context"
	"testing"

	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewDataNode(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:      "node-1",
		BindAddr:    "127.0.0.1",
		GRPCPort:    9090,
		DataDir:     "/tmp/test-data",
		MasterAddr:  "localhost:9000",
		StorageTier: "hot",
		MaxShards:   10,
		SIMDEnabled: true,
	}
	logger := zap.NewNop()

	node, err := NewDataNode(cfg, logger)
	assert.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, cfg, node.cfg)
	assert.NotNil(t, node.logger)
	assert.NotNil(t, node.grpcServer)
	assert.NotNil(t, node.diagon)
	assert.NotNil(t, node.shards)
	assert.NotNil(t, node.masterClient)
}

func TestNewDataNode_NilLogger(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:   "node-1",
		DataDir:  "/tmp/test-data",
		MaxShards: 10,
	}

	node, err := NewDataNode(cfg, nil)
	assert.Error(t, err)
	assert.Nil(t, node)
	assert.Contains(t, err.Error(), "logger is required")
}

func TestDataNode_CreateShard(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:      "node-1",
		DataDir:     "/tmp/test-data",
		MasterAddr:  "localhost:9000",
		StorageTier: "hot",
		MaxShards:   10,
	}
	logger := zap.NewNop()

	node, err := NewDataNode(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a shard
	err = node.CreateShard(ctx, "test-index", 0, true)
	assert.NoError(t, err)

	// Verify shard was created
	assert.Equal(t, 1, node.shards.Count())
}

func TestDataNode_DeleteShard(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:      "node-1",
		DataDir:     "/tmp/test-data",
		MasterAddr:  "localhost:9000",
		StorageTier: "hot",
		MaxShards:   10,
	}
	logger := zap.NewNop()

	node, err := NewDataNode(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a shard
	err = node.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	// Delete the shard
	err = node.DeleteShard(ctx, "test-index", 0)
	assert.NoError(t, err)

	// Verify shard was deleted
	assert.Equal(t, 0, node.shards.Count())
}

func TestDataNode_IndexDocument(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:      "node-1",
		DataDir:     "/tmp/test-data",
		MasterAddr:  "localhost:9000",
		StorageTier: "hot",
		MaxShards:   10,
	}
	logger := zap.NewNop()

	node, err := NewDataNode(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a shard
	err = node.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	// Index a document
	doc := map[string]interface{}{
		"title": "Test Document",
		"body":  "This is a test",
	}
	err = node.IndexDocument(ctx, "test-index", 0, "doc-1", doc)
	assert.NoError(t, err)
}

func TestDataNode_IndexDocument_NonExistentShard(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:      "node-1",
		DataDir:     "/tmp/test-data",
		MasterAddr:  "localhost:9000",
		StorageTier: "hot",
		MaxShards:   10,
	}
	logger := zap.NewNop()

	node, err := NewDataNode(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Try to index to non-existent shard
	doc := map[string]interface{}{
		"title": "Test Document",
	}
	err = node.IndexDocument(ctx, "test-index", 0, "doc-1", doc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDataNode_SearchShard(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:      "node-1",
		DataDir:     "/tmp/test-data",
		MasterAddr:  "localhost:9000",
		StorageTier: "hot",
		MaxShards:   10,
	}
	logger := zap.NewNop()

	node, err := NewDataNode(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a shard
	err = node.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	// Execute search
	query := []byte(`{"query": {"match_all": {}}}`)
	result, err := node.SearchShard(ctx, "test-index", 0, query)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestDataNode_SearchShard_NonExistentShard(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:      "node-1",
		DataDir:     "/tmp/test-data",
		MasterAddr:  "localhost:9000",
		StorageTier: "hot",
		MaxShards:   10,
	}
	logger := zap.NewNop()

	node, err := NewDataNode(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Try to search non-existent shard
	query := []byte(`{"query": {"match_all": {}}}`)
	_, err = node.SearchShard(ctx, "test-index", 0, query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDataNode_CollectStats(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:      "node-1",
		DataDir:     "/tmp/test-data",
		MasterAddr:  "localhost:9000",
		StorageTier: "hot",
		MaxShards:   10,
	}
	logger := zap.NewNop()

	node, err := NewDataNode(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create some shards
	node.CreateShard(ctx, "test-index", 0, true)
	node.CreateShard(ctx, "test-index", 1, false)

	// Collect stats
	stats := node.collectStats()
	assert.NotNil(t, stats)
	assert.Equal(t, "node-1", stats.NodeID)
	assert.Equal(t, 2, stats.ActiveShards)
	assert.Equal(t, int64(0), stats.DocsCount)
	assert.Equal(t, int64(0), stats.StoreSizeBytes)
}

func TestShardKey(t *testing.T) {
	key := shardKey("test-index", 5)
	assert.Equal(t, "test-index:5", key)
}

func TestNodeStats(t *testing.T) {
	stats := &NodeStats{
		NodeID:         "node-1",
		ActiveShards:   5,
		DocsCount:      1000,
		StoreSizeBytes: 5000000,
		CPUPercent:     45.5,
		MemoryPercent:  60.0,
		DiskPercent:    70.0,
	}

	assert.Equal(t, "node-1", stats.NodeID)
	assert.Equal(t, 5, stats.ActiveShards)
	assert.Equal(t, int64(1000), stats.DocsCount)
	assert.Equal(t, int64(5000000), stats.StoreSizeBytes)
	assert.Equal(t, 45.5, stats.CPUPercent)
	assert.Equal(t, 60.0, stats.MemoryPercent)
	assert.Equal(t, 70.0, stats.DiskPercent)
}

func TestSearchResult(t *testing.T) {
	result := &SearchResult{
		Took:      10,
		TotalHits: 100,
		MaxScore:  9.5,
		Hits: []*Hit{
			{
				ID:    "doc-1",
				Score: 9.5,
				Source: map[string]interface{}{
					"title": "Test",
				},
			},
		},
	}

	assert.Equal(t, int64(10), result.Took)
	assert.Equal(t, int64(100), result.TotalHits)
	assert.Equal(t, 9.5, result.MaxScore)
	assert.Equal(t, 1, len(result.Hits))
	assert.Equal(t, "doc-1", result.Hits[0].ID)
}

func TestHit(t *testing.T) {
	hit := &Hit{
		ID:    "doc-1",
		Score: 8.5,
		Source: map[string]interface{}{
			"title": "Test Document",
			"body":  "Test body",
		},
	}

	assert.Equal(t, "doc-1", hit.ID)
	assert.Equal(t, 8.5, hit.Score)
	assert.Equal(t, "Test Document", hit.Source["title"])
	assert.Equal(t, "Test body", hit.Source["body"])
}
