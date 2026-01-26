package data

import (
	"context"
	"fmt"
	"testing"

	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/quidditch/quidditch/pkg/data/diagon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewShardManager(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	assert.NotNil(t, sm)
	assert.Equal(t, cfg, sm.cfg)
	assert.NotNil(t, sm.shards)
	assert.Equal(t, 0, len(sm.shards))
}

func TestShardManager_CreateShard(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()

	// Start shard manager
	err = sm.Start(ctx)
	require.NoError(t, err)

	// Create a shard
	err = sm.CreateShard(ctx, "test-index", 0, true)
	assert.NoError(t, err)

	// Verify shard was created
	assert.Equal(t, 1, sm.Count())

	// Try to create the same shard again
	err = sm.CreateShard(ctx, "test-index", 0, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Clean up
	sm.Stop(ctx)
}

func TestShardManager_GetShard(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Create a shard
	err = sm.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	// Get the shard
	shard, err := sm.GetShard("test-index", 0)
	assert.NoError(t, err)
	assert.NotNil(t, shard)
	assert.Equal(t, "test-index", shard.IndexName)
	assert.Equal(t, int32(0), shard.ShardID)
	assert.True(t, shard.IsPrimary)
	assert.Equal(t, ShardStateStarted, shard.State)

	// Try to get non-existent shard
	_, err = sm.GetShard("test-index", 99)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestShardManager_DeleteShard(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Create a shard
	err = sm.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)
	assert.Equal(t, 1, sm.Count())

	// Delete the shard
	err = sm.DeleteShard(ctx, "test-index", 0)
	assert.NoError(t, err)
	assert.Equal(t, 0, sm.Count())

	// Try to delete non-existent shard
	err = sm.DeleteShard(ctx, "test-index", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestShardManager_MaxShards(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 2, // Set low limit for testing
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Create shards up to the limit
	err = sm.CreateShard(ctx, "test-index", 0, true)
	require.NoError(t, err)

	err = sm.CreateShard(ctx, "test-index", 1, false)
	require.NoError(t, err)

	// Try to create one more (should fail)
	err = sm.CreateShard(ctx, "test-index", 2, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max shards limit")
}

func TestShardManager_List(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Initially empty
	shards := sm.List()
	assert.Equal(t, 0, len(shards))

	// Create some shards
	sm.CreateShard(ctx, "test-index-1", 0, true)
	sm.CreateShard(ctx, "test-index-1", 1, false)
	sm.CreateShard(ctx, "test-index-2", 0, true)

	// List should return all shards
	shards = sm.List()
	assert.Equal(t, 3, len(shards))
}

func TestShard_IndexDocument(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Create a shard
	sm.CreateShard(ctx, "test-index", 0, true)
	shard, err := sm.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index a document
	doc := map[string]interface{}{
		"title": "Test Document",
		"body":  "This is a test",
	}
	err = shard.IndexDocument(ctx, "doc-1", doc)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), shard.DocsCount)

	// Index another document
	err = shard.IndexDocument(ctx, "doc-2", doc)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), shard.DocsCount)
}

func TestShard_GetDocument(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Create a shard
	sm.CreateShard(ctx, "test-index", 0, true)
	shard, err := sm.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index a document
	doc := map[string]interface{}{
		"title": "Test Document",
		"body":  "This is a test",
	}
	err = shard.IndexDocument(ctx, "doc-1", doc)
	require.NoError(t, err)

	// Get the document
	retrievedDoc, err := shard.GetDocument(ctx, "doc-1")
	assert.NoError(t, err)
	assert.NotNil(t, retrievedDoc)
	assert.Equal(t, "Test Document", retrievedDoc["title"])

	// Try to get non-existent document
	_, err = shard.GetDocument(ctx, "non-existent")
	assert.Error(t, err)
}

func TestShard_DeleteDocument(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Create a shard
	sm.CreateShard(ctx, "test-index", 0, true)
	shard, err := sm.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index a document
	doc := map[string]interface{}{
		"title": "Test Document",
	}
	err = shard.IndexDocument(ctx, "doc-1", doc)
	require.NoError(t, err)
	assert.Equal(t, int64(1), shard.DocsCount)

	// Delete the document
	err = shard.DeleteDocument(ctx, "doc-1")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), shard.DocsCount)
}

func TestShard_Search(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Create a shard
	sm.CreateShard(ctx, "test-index", 0, true)
	shard, err := sm.GetShard("test-index", 0)
	require.NoError(t, err)

	// Index some documents
	docs := []map[string]interface{}{
		{"title": "First Document", "body": "Test content"},
		{"title": "Second Document", "body": "More test content"},
	}
	for i, doc := range docs {
		err = shard.IndexDocument(ctx, fmt.Sprintf("doc-%d", i), doc)
		require.NoError(t, err)
	}

	// Execute search (empty query for now)
	query := []byte("{}")
	result, err := shard.Search(ctx, query)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestShard_RefreshAndFlush(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Create a shard
	sm.CreateShard(ctx, "test-index", 0, true)
	shard, err := sm.GetShard("test-index", 0)
	require.NoError(t, err)

	// Test refresh
	err = shard.Refresh()
	assert.NoError(t, err)

	// Test flush
	err = shard.Flush()
	assert.NoError(t, err)
}

func TestShard_Stats(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Create a shard
	sm.CreateShard(ctx, "test-index", 0, true)
	shard, err := sm.GetShard("test-index", 0)
	require.NoError(t, err)

	// Get stats
	stats := shard.Stats()
	assert.NotNil(t, stats)
	assert.Equal(t, "test-index", stats.IndexName)
	assert.Equal(t, int32(0), stats.ShardID)
	assert.True(t, stats.IsPrimary)
	assert.Equal(t, ShardStateStarted, stats.State)
	assert.Equal(t, int64(0), stats.DocsCount)
}

func TestShard_Close(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Create a shard
	sm.CreateShard(ctx, "test-index", 0, true)
	shard, err := sm.GetShard("test-index", 0)
	require.NoError(t, err)

	// Close the shard
	err = shard.Close()
	assert.NoError(t, err)
	assert.Equal(t, ShardStateClosed, shard.State)

	// Closing again should be no-op
	err = shard.Close()
	assert.NoError(t, err)
}

func TestShard_OperationsOnClosedShard(t *testing.T) {
	cfg := &config.DataNodeConfig{
		NodeID:    "node-1",
		DataDir:   "/tmp/test-data",
		MaxShards: 10,
	}
	logger := zap.NewNop()
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir: cfg.DataDir,
		Logger:  logger,
	})
	require.NoError(t, err)

	sm := NewShardManager(cfg, logger, diagonBridge, nil)

	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop(ctx)

	// Create and close a shard
	sm.CreateShard(ctx, "test-index", 0, true)
	shard, err := sm.GetShard("test-index", 0)
	require.NoError(t, err)
	shard.Close()

	// Try operations on closed shard
	doc := map[string]interface{}{"title": "Test"}

	err = shard.IndexDocument(ctx, "doc-1", doc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not ready")

	_, err = shard.GetDocument(ctx, "doc-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not ready")

	err = shard.DeleteDocument(ctx, "doc-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not ready")

	_, err = shard.Search(ctx, []byte("{}"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not ready")

	err = shard.Refresh()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not ready")

	err = shard.Flush()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not ready")
}
