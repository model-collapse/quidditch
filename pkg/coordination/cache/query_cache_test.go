package cache

import (
	"testing"
	"time"

	"github.com/quidditch/quidditch/pkg/coordination/parser"
	"github.com/quidditch/quidditch/pkg/coordination/planner"
	"github.com/stretchr/testify/assert"
)

func TestNewQueryCache(t *testing.T) {
	config := DefaultQueryCacheConfig()
	cache := NewQueryCache(config)

	assert.NotNil(t, cache)
	assert.True(t, cache.enableLogical)
	assert.True(t, cache.enablePhysical)
	assert.NotNil(t, cache.logicalCache)
	assert.NotNil(t, cache.physicalCache)
}

func TestNewQueryCache_CustomConfig(t *testing.T) {
	config := &QueryCacheConfig{
		LogicalCacheSize:      500,
		LogicalCacheMaxBytes:  50 * 1024 * 1024,
		LogicalCacheTTL:       2 * time.Minute,
		PhysicalCacheSize:     300,
		PhysicalCacheMaxBytes: 30 * 1024 * 1024,
		PhysicalCacheTTL:      3 * time.Minute,
		EnableLogical:         true,
		EnablePhysical:        false, // Disable physical cache
	}

	cache := NewQueryCache(config)
	assert.NotNil(t, cache)
	assert.True(t, cache.enableLogical)
	assert.False(t, cache.enablePhysical)
	assert.NotNil(t, cache.logicalCache)
	assert.Nil(t, cache.physicalCache)
}

func TestQueryCache_LogicalPlan_PutAndGet(t *testing.T) {
	cache := NewQueryCache(DefaultQueryCacheConfig())

	// Create a search request
	searchReq := &parser.SearchRequest{
		ParsedQuery: &parser.TermQuery{
			Field: "status",
			Value: "active",
		},
		Size: 10,
		From: 0,
	}

	indexName := "products"
	shardIDs := []int32{0, 1, 2}

	// Initially should not be in cache
	_, found := cache.GetLogicalPlan(indexName, searchReq, shardIDs)
	assert.False(t, found)

	// Create a mock logical plan
	mockPlan := &planner.LogicalScan{
		IndexName:     indexName,
		Shards:        shardIDs,
		EstimatedRows: 1000,
	}

	// Put in cache
	cache.PutLogicalPlan(indexName, searchReq, shardIDs, mockPlan)

	// Should now be in cache
	cachedPlan, found := cache.GetLogicalPlan(indexName, searchReq, shardIDs)
	assert.True(t, found)
	assert.NotNil(t, cachedPlan)

	// Verify it's the same plan
	scan, ok := cachedPlan.(*planner.LogicalScan)
	assert.True(t, ok)
	assert.Equal(t, indexName, scan.IndexName)
	assert.Equal(t, shardIDs, scan.Shards)
}

func TestQueryCache_LogicalPlan_DifferentQueries(t *testing.T) {
	cache := NewQueryCache(DefaultQueryCacheConfig())

	// Create two different search requests
	searchReq1 := &parser.SearchRequest{
		ParsedQuery: &parser.TermQuery{
			Field: "status",
			Value: "active",
		},
		Size: 10,
	}

	searchReq2 := &parser.SearchRequest{
		ParsedQuery: &parser.TermQuery{
			Field: "status",
			Value: "inactive",
		},
		Size: 10,
	}

	indexName := "products"
	shardIDs := []int32{0, 1, 2}

	mockPlan1 := &planner.LogicalScan{IndexName: indexName, Shards: shardIDs}
	mockPlan2 := &planner.LogicalScan{IndexName: indexName, Shards: shardIDs}

	// Cache both plans
	cache.PutLogicalPlan(indexName, searchReq1, shardIDs, mockPlan1)
	cache.PutLogicalPlan(indexName, searchReq2, shardIDs, mockPlan2)

	// Both should be retrievable independently
	plan1, found1 := cache.GetLogicalPlan(indexName, searchReq1, shardIDs)
	plan2, found2 := cache.GetLogicalPlan(indexName, searchReq2, shardIDs)

	assert.True(t, found1)
	assert.True(t, found2)
	assert.NotNil(t, plan1)
	assert.NotNil(t, plan2)
}

func TestQueryCache_LogicalPlan_SameQueryDifferentIndices(t *testing.T) {
	cache := NewQueryCache(DefaultQueryCacheConfig())

	searchReq := &parser.SearchRequest{
		ParsedQuery: &parser.TermQuery{
			Field: "status",
			Value: "active",
		},
		Size: 10,
	}

	shardIDs := []int32{0, 1, 2}

	mockPlan1 := &planner.LogicalScan{IndexName: "products", Shards: shardIDs}
	mockPlan2 := &planner.LogicalScan{IndexName: "orders", Shards: shardIDs}

	// Cache plans for different indices
	cache.PutLogicalPlan("products", searchReq, shardIDs, mockPlan1)
	cache.PutLogicalPlan("orders", searchReq, shardIDs, mockPlan2)

	// Both should be cached independently
	plan1, found1 := cache.GetLogicalPlan("products", searchReq, shardIDs)
	plan2, found2 := cache.GetLogicalPlan("orders", searchReq, shardIDs)

	assert.True(t, found1)
	assert.True(t, found2)

	scan1 := plan1.(*planner.LogicalScan)
	scan2 := plan2.(*planner.LogicalScan)
	assert.Equal(t, "products", scan1.IndexName)
	assert.Equal(t, "orders", scan2.IndexName)
}

func TestQueryCache_PhysicalPlan_PutAndGet(t *testing.T) {
	cache := NewQueryCache(DefaultQueryCacheConfig())

	indexName := "products"
	shardIDs := []int32{0, 1, 2}

	// Create a logical plan
	logicalPlan := &planner.LogicalScan{
		IndexName: indexName,
		Shards:    shardIDs,
	}

	// Initially should not be in cache
	_, found := cache.GetPhysicalPlan(indexName, logicalPlan)
	assert.False(t, found)

	// Create a mock physical plan
	mockPhysicalPlan := &planner.PhysicalScan{
		IndexName: indexName,
		Shards:    shardIDs,
		EstimatedCost: &planner.Cost{
			CPUCost:     100,
			MemoryCost:  1024,
			NetworkCost: 500,
			TotalCost:   1624,
		},
	}

	// Put in cache
	cache.PutPhysicalPlan(indexName, logicalPlan, mockPhysicalPlan)

	// Should now be in cache
	cachedPlan, found := cache.GetPhysicalPlan(indexName, logicalPlan)
	assert.True(t, found)
	assert.NotNil(t, cachedPlan)

	// Verify it's the same plan
	scan, ok := cachedPlan.(*planner.PhysicalScan)
	assert.True(t, ok)
	assert.Equal(t, indexName, scan.IndexName)
	assert.Equal(t, shardIDs, scan.Shards)
}

func TestQueryCache_InvalidateIndex(t *testing.T) {
	cache := NewQueryCache(DefaultQueryCacheConfig())

	searchReq := &parser.SearchRequest{
		ParsedQuery: &parser.MatchAllQuery{},
		Size:        10,
	}

	indexName := "products"
	shardIDs := []int32{0, 1, 2}

	mockLogicalPlan := &planner.LogicalScan{IndexName: indexName, Shards: shardIDs}
	mockPhysicalPlan := &planner.PhysicalScan{IndexName: indexName, Shards: shardIDs}

	// Cache some plans
	cache.PutLogicalPlan(indexName, searchReq, shardIDs, mockLogicalPlan)
	cache.PutPhysicalPlan(indexName, mockLogicalPlan, mockPhysicalPlan)

	// Verify cached
	_, found1 := cache.GetLogicalPlan(indexName, searchReq, shardIDs)
	_, found2 := cache.GetPhysicalPlan(indexName, mockLogicalPlan)
	assert.True(t, found1)
	assert.True(t, found2)

	// Invalidate index
	cache.InvalidateIndex(indexName)

	// Should no longer be cached
	_, found1 = cache.GetLogicalPlan(indexName, searchReq, shardIDs)
	_, found2 = cache.GetPhysicalPlan(indexName, mockLogicalPlan)
	assert.False(t, found1)
	assert.False(t, found2)
}

func TestQueryCache_Clear(t *testing.T) {
	cache := NewQueryCache(DefaultQueryCacheConfig())

	searchReq := &parser.SearchRequest{
		ParsedQuery: &parser.MatchAllQuery{},
		Size:        10,
	}

	indexName := "products"
	shardIDs := []int32{0, 1, 2}

	mockLogicalPlan := &planner.LogicalScan{IndexName: indexName, Shards: shardIDs}
	mockPhysicalPlan := &planner.PhysicalScan{IndexName: indexName, Shards: shardIDs}

	// Cache some plans
	cache.PutLogicalPlan(indexName, searchReq, shardIDs, mockLogicalPlan)
	cache.PutPhysicalPlan(indexName, mockLogicalPlan, mockPhysicalPlan)

	// Clear all
	cache.Clear()

	// Nothing should be cached
	_, found1 := cache.GetLogicalPlan(indexName, searchReq, shardIDs)
	_, found2 := cache.GetPhysicalPlan(indexName, mockLogicalPlan)
	assert.False(t, found1)
	assert.False(t, found2)
}

func TestQueryCache_Stats(t *testing.T) {
	cache := NewQueryCache(DefaultQueryCacheConfig())

	searchReq := &parser.SearchRequest{
		ParsedQuery: &parser.MatchAllQuery{},
		Size:        10,
	}

	indexName := "products"
	shardIDs := []int32{0, 1, 2}

	mockLogicalPlan := &planner.LogicalScan{IndexName: indexName, Shards: shardIDs}

	// Cache a plan
	cache.PutLogicalPlan(indexName, searchReq, shardIDs, mockLogicalPlan)

	// Hit
	cache.GetLogicalPlan(indexName, searchReq, shardIDs)
	// Miss
	cache.GetLogicalPlan("other", searchReq, shardIDs)

	// Get stats
	stats := cache.Stats()
	assert.Contains(t, stats, CacheTypeLogical)

	logicalStats := stats[CacheTypeLogical]
	assert.Equal(t, int64(1), logicalStats.Hits)
	assert.Equal(t, int64(1), logicalStats.Misses)
	assert.Equal(t, 0.5, logicalStats.HitRate)
	assert.Equal(t, 1, logicalStats.Size)
}

func TestQueryCache_BoolQuery_Normalization(t *testing.T) {
	cache := NewQueryCache(DefaultQueryCacheConfig())

	// Two identical bool queries (same structure, different object instances)
	searchReq1 := &parser.SearchRequest{
		ParsedQuery: &parser.BoolQuery{
			Must: []parser.Query{
				&parser.TermQuery{Field: "status", Value: "active"},
				&parser.RangeQuery{Field: "price", Gte: 10.0, Lte: 100.0},
			},
		},
		Size: 10,
	}

	searchReq2 := &parser.SearchRequest{
		ParsedQuery: &parser.BoolQuery{
			Must: []parser.Query{
				&parser.TermQuery{Field: "status", Value: "active"},
				&parser.RangeQuery{Field: "price", Gte: 10.0, Lte: 100.0},
			},
		},
		Size: 10,
	}

	indexName := "products"
	shardIDs := []int32{0, 1, 2}

	mockPlan := &planner.LogicalScan{IndexName: indexName, Shards: shardIDs}

	// Cache with first request
	cache.PutLogicalPlan(indexName, searchReq1, shardIDs, mockPlan)

	// Should be retrievable with second request (identical structure)
	plan, found := cache.GetLogicalPlan(indexName, searchReq2, shardIDs)
	assert.True(t, found)
	assert.NotNil(t, plan)
}

func TestQueryCache_DisabledCaches(t *testing.T) {
	config := &QueryCacheConfig{
		EnableLogical:  false,
		EnablePhysical: false,
	}
	cache := NewQueryCache(config)

	searchReq := &parser.SearchRequest{
		ParsedQuery: &parser.MatchAllQuery{},
		Size:        10,
	}

	indexName := "products"
	shardIDs := []int32{0, 1, 2}

	mockLogicalPlan := &planner.LogicalScan{IndexName: indexName, Shards: shardIDs}
	mockPhysicalPlan := &planner.PhysicalScan{IndexName: indexName, Shards: shardIDs}

	// Try to cache (should be no-op)
	cache.PutLogicalPlan(indexName, searchReq, shardIDs, mockLogicalPlan)
	cache.PutPhysicalPlan(indexName, mockLogicalPlan, mockPhysicalPlan)

	// Should not be cached
	_, found1 := cache.GetLogicalPlan(indexName, searchReq, shardIDs)
	_, found2 := cache.GetPhysicalPlan(indexName, mockLogicalPlan)
	assert.False(t, found1)
	assert.False(t, found2)
}

func TestQueryCache_ComplexQuery_Normalization(t *testing.T) {
	cache := NewQueryCache(DefaultQueryCacheConfig())

	// Complex nested query
	searchReq := &parser.SearchRequest{
		ParsedQuery: &parser.BoolQuery{
			Must: []parser.Query{
				&parser.MatchQuery{Field: "title", Query: "laptop"},
			},
			Filter: []parser.Query{
				&parser.TermQuery{Field: "status", Value: "active"},
				&parser.RangeQuery{Field: "price", Gte: 100.0},
			},
			Should: []parser.Query{
				&parser.TermQuery{Field: "brand", Value: "Apple"},
				&parser.TermQuery{Field: "brand", Value: "Dell"},
			},
			MinimumShouldMatch: 1,
		},
		Aggregations: map[string]interface{}{
			"brands": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "brand",
				},
			},
		},
		Size: 20,
		From: 10,
	}

	indexName := "products"
	shardIDs := []int32{0, 1, 2}

	mockPlan := &planner.LogicalScan{IndexName: indexName, Shards: shardIDs}

	// Cache the plan
	cache.PutLogicalPlan(indexName, searchReq, shardIDs, mockPlan)

	// Should be retrievable
	plan, found := cache.GetLogicalPlan(indexName, searchReq, shardIDs)
	assert.True(t, found)
	assert.NotNil(t, plan)
}
