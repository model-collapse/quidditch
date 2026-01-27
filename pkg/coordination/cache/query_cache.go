package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/quidditch/quidditch/pkg/coordination/parser"
	"github.com/quidditch/quidditch/pkg/coordination/planner"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for query cache
var (
	cacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "quidditch_query_cache_hits_total",
			Help: "Total number of query cache hits",
		},
		[]string{"cache_type", "index"},
	)

	cacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "quidditch_query_cache_misses_total",
			Help: "Total number of query cache misses",
		},
		[]string{"cache_type", "index"},
	)

	cacheEvictions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "quidditch_query_cache_evictions_total",
			Help: "Total number of query cache evictions",
		},
		[]string{"cache_type"},
	)

	cacheSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "quidditch_query_cache_size",
			Help: "Current number of entries in query cache",
		},
		[]string{"cache_type"},
	)

	cacheSizeBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "quidditch_query_cache_size_bytes",
			Help: "Current size of query cache in bytes",
		},
		[]string{"cache_type"},
	)

	cacheHitRate = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "quidditch_query_cache_hit_rate",
			Help: "Query cache hit rate (hits / total requests)",
		},
		[]string{"cache_type"},
	)
)

// CacheType represents the type of cache
type CacheType string

const (
	CacheTypeLogical  CacheType = "logical"
	CacheTypePhysical CacheType = "physical"
)

// QueryCache provides multi-level caching for query plans
type QueryCache struct {
	logicalCache  *LRUCache
	physicalCache *LRUCache

	// Configuration
	enableLogical  bool
	enablePhysical bool
}

// QueryCacheConfig configures the query cache
type QueryCacheConfig struct {
	// Logical plan cache settings
	LogicalCacheSize     int           // Max number of logical plans
	LogicalCacheMaxBytes int64         // Max size in bytes (0 = unlimited)
	LogicalCacheTTL      time.Duration // TTL for logical plans

	// Physical plan cache settings
	PhysicalCacheSize     int           // Max number of physical plans
	PhysicalCacheMaxBytes int64         // Max size in bytes (0 = unlimited)
	PhysicalCacheTTL      time.Duration // TTL for physical plans

	// Feature flags
	EnableLogical  bool
	EnablePhysical bool
}

// DefaultQueryCacheConfig returns default cache configuration
func DefaultQueryCacheConfig() *QueryCacheConfig {
	return &QueryCacheConfig{
		LogicalCacheSize:      1000,
		LogicalCacheMaxBytes:  100 * 1024 * 1024, // 100 MB
		LogicalCacheTTL:       5 * time.Minute,
		PhysicalCacheSize:     1000,
		PhysicalCacheMaxBytes: 100 * 1024 * 1024, // 100 MB
		PhysicalCacheTTL:      5 * time.Minute,
		EnableLogical:         true,
		EnablePhysical:        true,
	}
}

// NewQueryCache creates a new query cache
func NewQueryCache(config *QueryCacheConfig) *QueryCache {
	if config == nil {
		config = DefaultQueryCacheConfig()
	}

	qc := &QueryCache{
		enableLogical:  config.EnableLogical,
		enablePhysical: config.EnablePhysical,
	}

	if config.EnableLogical {
		qc.logicalCache = NewLRUCache(
			config.LogicalCacheSize,
			config.LogicalCacheMaxBytes,
			config.LogicalCacheTTL,
		)
	}

	if config.EnablePhysical {
		qc.physicalCache = NewLRUCache(
			config.PhysicalCacheSize,
			config.PhysicalCacheMaxBytes,
			config.PhysicalCacheTTL,
		)
	}

	return qc
}

// LogicalPlanCacheEntry represents a cached logical plan
type LogicalPlanCacheEntry struct {
	Plan      planner.LogicalPlan
	IndexName string
	ShardIDs  []int32
}

// PhysicalPlanCacheEntry represents a cached physical plan
type PhysicalPlanCacheEntry struct {
	Plan      planner.PhysicalPlan
	IndexName string
}

// GetLogicalPlan retrieves a logical plan from cache
func (qc *QueryCache) GetLogicalPlan(indexName string, searchReq *parser.SearchRequest, shardIDs []int32) (planner.LogicalPlan, bool) {
	if !qc.enableLogical || qc.logicalCache == nil {
		return nil, false
	}

	key := qc.generateLogicalPlanKey(indexName, searchReq, shardIDs)
	value, found := qc.logicalCache.Get(key)

	if found {
		cacheHits.WithLabelValues(string(CacheTypeLogical), indexName).Inc()
		entry := value.(*LogicalPlanCacheEntry)
		return entry.Plan, true
	}

	cacheMisses.WithLabelValues(string(CacheTypeLogical), indexName).Inc()
	return nil, false
}

// PutLogicalPlan stores a logical plan in cache
func (qc *QueryCache) PutLogicalPlan(indexName string, searchReq *parser.SearchRequest, shardIDs []int32, plan planner.LogicalPlan) {
	if !qc.enableLogical || qc.logicalCache == nil {
		return
	}

	key := qc.generateLogicalPlanKey(indexName, searchReq, shardIDs)
	entry := &LogicalPlanCacheEntry{
		Plan:      plan,
		IndexName: indexName,
		ShardIDs:  shardIDs,
	}

	// Estimate size (simplified)
	size := int64(1000) // Base size
	size += int64(len(indexName))
	size += int64(len(shardIDs) * 4)

	qc.logicalCache.Put(key, entry, size)

	// Update metrics
	stats := qc.logicalCache.Stats()
	cacheSize.WithLabelValues(string(CacheTypeLogical)).Set(float64(stats.Size))
	cacheSizeBytes.WithLabelValues(string(CacheTypeLogical)).Set(float64(stats.BytesUsed))
	cacheHitRate.WithLabelValues(string(CacheTypeLogical)).Set(stats.HitRate)
}

// GetPhysicalPlan retrieves a physical plan from cache
func (qc *QueryCache) GetPhysicalPlan(indexName string, logicalPlan planner.LogicalPlan) (planner.PhysicalPlan, bool) {
	if !qc.enablePhysical || qc.physicalCache == nil {
		return nil, false
	}

	key := qc.generatePhysicalPlanKey(indexName, logicalPlan)
	value, found := qc.physicalCache.Get(key)

	if found {
		cacheHits.WithLabelValues(string(CacheTypePhysical), indexName).Inc()
		entry := value.(*PhysicalPlanCacheEntry)
		return entry.Plan, true
	}

	cacheMisses.WithLabelValues(string(CacheTypePhysical), indexName).Inc()
	return nil, false
}

// PutPhysicalPlan stores a physical plan in cache
func (qc *QueryCache) PutPhysicalPlan(indexName string, logicalPlan planner.LogicalPlan, physicalPlan planner.PhysicalPlan) {
	if !qc.enablePhysical || qc.physicalCache == nil {
		return
	}

	key := qc.generatePhysicalPlanKey(indexName, logicalPlan)
	entry := &PhysicalPlanCacheEntry{
		Plan:      physicalPlan,
		IndexName: indexName,
	}

	// Estimate size (simplified)
	size := int64(2000) // Base size for physical plan
	size += int64(len(indexName))

	qc.physicalCache.Put(key, entry, size)

	// Update metrics
	stats := qc.physicalCache.Stats()
	cacheSize.WithLabelValues(string(CacheTypePhysical)).Set(float64(stats.Size))
	cacheSizeBytes.WithLabelValues(string(CacheTypePhysical)).Set(float64(stats.BytesUsed))
	cacheHitRate.WithLabelValues(string(CacheTypePhysical)).Set(stats.HitRate)
}

// InvalidateIndex removes all cached plans for a specific index
func (qc *QueryCache) InvalidateIndex(indexName string) {
	// For simplicity, we clear all caches when an index is invalidated
	// In production, we'd want to track index-specific entries
	if qc.logicalCache != nil {
		qc.logicalCache.Clear()
	}
	if qc.physicalCache != nil {
		qc.physicalCache.Clear()
	}
}

// Clear removes all cached entries
func (qc *QueryCache) Clear() {
	if qc.logicalCache != nil {
		qc.logicalCache.Clear()
	}
	if qc.physicalCache != nil {
		qc.physicalCache.Clear()
	}
}

// CleanupExpired removes expired entries from all caches
func (qc *QueryCache) CleanupExpired() {
	if qc.logicalCache != nil {
		qc.logicalCache.CleanupExpired()
	}
	if qc.physicalCache != nil {
		qc.physicalCache.CleanupExpired()
	}
}

// Stats returns cache statistics
func (qc *QueryCache) Stats() map[CacheType]CacheStats {
	stats := make(map[CacheType]CacheStats)

	if qc.logicalCache != nil {
		stats[CacheTypeLogical] = qc.logicalCache.Stats()
	}
	if qc.physicalCache != nil {
		stats[CacheTypePhysical] = qc.physicalCache.Stats()
	}

	return stats
}

// generateLogicalPlanKey creates a cache key for a logical plan
func (qc *QueryCache) generateLogicalPlanKey(indexName string, searchReq *parser.SearchRequest, shardIDs []int32) string {
	// Create a normalized representation of the search request
	keyData := struct {
		Index        string
		Query        interface{}
		Aggregations interface{}
		Size         int
		From         int
		Sort         interface{}
		ShardIDs     []int32
	}{
		Index:        indexName,
		Query:        normalizeQuery(searchReq.ParsedQuery),
		Aggregations: searchReq.Aggregations, // Use raw aggregations map
		Size:         searchReq.Size,
		From:         searchReq.From,
		Sort:         searchReq.Sort, // Use raw sort slice
		ShardIDs:     shardIDs,
	}

	// Serialize to JSON for consistent hashing
	jsonData, err := json.Marshal(keyData)
	if err != nil {
		// Fallback to simple key
		return fmt.Sprintf("logical:%s:%d", indexName, time.Now().UnixNano())
	}

	// Hash the JSON data
	hash := sha256.Sum256(jsonData)
	return "logical:" + hex.EncodeToString(hash[:])
}

// generatePhysicalPlanKey creates a cache key for a physical plan
func (qc *QueryCache) generatePhysicalPlanKey(indexName string, logicalPlan planner.LogicalPlan) string {
	// Use the logical plan's string representation as part of the key
	planStr := logicalPlan.String()
	keyStr := fmt.Sprintf("%s:%s", indexName, planStr)

	// Hash the key
	hash := sha256.Sum256([]byte(keyStr))
	return "physical:" + hex.EncodeToString(hash[:])
}

// normalizeQuery normalizes a query for consistent caching
func normalizeQuery(query parser.Query) interface{} {
	if query == nil {
		return nil
	}

	// Return a map representation that can be consistently serialized
	// This is a simplified normalization - in production, you'd want more sophisticated handling
	switch q := query.(type) {
	case *parser.TermQuery:
		return map[string]interface{}{
			"type":  "term",
			"field": q.Field,
			"value": q.Value,
		}
	case *parser.MatchQuery:
		return map[string]interface{}{
			"type":  "match",
			"field": q.Field,
			"query": q.Query,
		}
	case *parser.RangeQuery:
		return map[string]interface{}{
			"type":  "range",
			"field": q.Field,
			"gte":   q.Gte,
			"gt":    q.Gt,
			"lte":   q.Lte,
			"lt":    q.Lt,
		}
	case *parser.BoolQuery:
		return map[string]interface{}{
			"type":               "bool",
			"must":               normalizeQueryList(q.Must),
			"should":             normalizeQueryList(q.Should),
			"must_not":           normalizeQueryList(q.MustNot),
			"filter":             normalizeQueryList(q.Filter),
			"minimum_should_match": q.MinimumShouldMatch,
		}
	case *parser.MatchAllQuery:
		return map[string]interface{}{
			"type": "match_all",
		}
	case *parser.PrefixQuery:
		return map[string]interface{}{
			"type":  "prefix",
			"field": q.Field,
			"value": q.Value,
		}
	case *parser.WildcardQuery:
		return map[string]interface{}{
			"type":  "wildcard",
			"field": q.Field,
			"value": q.Value,
		}
	case *parser.FuzzyQuery:
		return map[string]interface{}{
			"type":      "fuzzy",
			"field":     q.Field,
			"value":     q.Value,
			"fuzziness": q.Fuzziness,
		}
	case *parser.ExistsQuery:
		return map[string]interface{}{
			"type":  "exists",
			"field": q.Field,
		}
	default:
		// Fallback: use string representation
		return map[string]interface{}{
			"type":   "unknown",
			"string": fmt.Sprintf("%v", query),
		}
	}
}

// normalizeQueryList normalizes a list of queries
func normalizeQueryList(queries []parser.Query) []interface{} {
	if len(queries) == 0 {
		return nil
	}
	result := make([]interface{}, len(queries))
	for i, q := range queries {
		result[i] = normalizeQuery(q)
	}
	return result
}
