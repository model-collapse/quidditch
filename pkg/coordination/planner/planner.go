package planner

import (
	"context"
	"fmt"
	"time"

	"github.com/quidditch/quidditch/pkg/coordination/parser"
	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"go.uber.org/zap"
)

// QueryPlan represents an optimized execution plan for a query
type QueryPlan struct {
	// Original query
	SearchRequest *parser.SearchRequest

	// Optimized query (may be rewritten)
	OptimizedQuery parser.Query

	// Target shards for this query
	TargetShards []int32

	// Estimated cost
	EstimatedCost float64

	// Query complexity score (0-100)
	Complexity int

	// Whether query can use cached results
	Cacheable bool

	// Statistics
	Stats *QueryStats
}

// QueryStats contains statistics about the query
type QueryStats struct {
	PlanningTimeMs   int64
	EstimatedResults int64
	EstimatedLatency int64
	ShardCount       int
}

// QueryPlanner optimizes and plans query execution
type QueryPlanner struct {
	logger *zap.Logger

	// Master client for getting index metadata
	masterClient MasterClient

	// Query cache (simple in-memory cache)
	cache *queryCache
}

// MasterClient interface for getting cluster information
type MasterClient interface {
	GetIndexMetadata(ctx context.Context, indexName string) (*pb.IndexMetadataResponse, error)
	GetShardRouting(ctx context.Context, indexName string) (map[int32]*pb.ShardRouting, error)
}

// NewQueryPlanner creates a new query planner
func NewQueryPlanner(masterClient MasterClient, logger *zap.Logger) *QueryPlanner {
	return &QueryPlanner{
		logger:       logger,
		masterClient: masterClient,
		cache:        newQueryCache(1000, 5*time.Minute), // 1000 entries, 5 min TTL
	}
}

// PlanQuery creates an optimized execution plan for a query
func (qp *QueryPlanner) PlanQuery(ctx context.Context, indexName string, searchReq *parser.SearchRequest) (*QueryPlan, error) {
	startTime := time.Now()

	// Get index metadata
	metadata, err := qp.masterClient.GetIndexMetadata(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to get index metadata: %w", err)
	}

	// Get shard routing
	routing, err := qp.masterClient.GetShardRouting(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to get shard routing: %w", err)
	}

	// Create initial plan
	plan := &QueryPlan{
		SearchRequest:  searchReq,
		OptimizedQuery: searchReq.ParsedQuery,
		Stats: &QueryStats{
			ShardCount: len(routing),
		},
	}

	// Analyze query complexity
	plan.Complexity = qp.analyzeComplexity(searchReq.ParsedQuery)

	// Optimize query
	optimizedQuery := qp.optimizeQuery(searchReq.ParsedQuery)
	plan.OptimizedQuery = optimizedQuery

	// Determine target shards
	targetShards := qp.selectShards(routing, searchReq)
	plan.TargetShards = targetShards

	// Estimate cost
	plan.EstimatedCost = qp.estimateCost(plan, metadata)

	// Determine cacheability
	plan.Cacheable = qp.isCacheable(searchReq)

	// Record planning time
	plan.Stats.PlanningTimeMs = time.Since(startTime).Milliseconds()

	qp.logger.Debug("Query plan created",
		zap.String("index", indexName),
		zap.Int("complexity", plan.Complexity),
		zap.Float64("estimated_cost", plan.EstimatedCost),
		zap.Int("target_shards", len(plan.TargetShards)),
		zap.Bool("cacheable", plan.Cacheable),
		zap.Int64("planning_time_ms", plan.Stats.PlanningTimeMs))

	return plan, nil
}

// analyzeComplexity estimates the computational complexity of a query (0-100)
func (qp *QueryPlanner) analyzeComplexity(query parser.Query) int {
	if query == nil {
		return 1 // match_all
	}

	complexity := 0

	// Use type switch to handle different query types
	switch q := query.(type) {
	case *parser.MatchAllQuery:
		complexity = 1

	case *parser.MatchQuery, *parser.TermQuery:
		complexity = 10

	case *parser.TermsQuery:
		if len(q.Values) > 0 {
			complexity = 10 + len(q.Values)
		}

	case *parser.RangeQuery:
		complexity = 15

	case *parser.BoolQuery:
		// Add complexity for each clause
		complexity = 5
		for _, must := range q.Must {
			complexity += qp.analyzeComplexity(must)
		}
		for _, should := range q.Should {
			complexity += qp.analyzeComplexity(should) / 2 // Should is less critical
		}
		for _, mustNot := range q.MustNot {
			complexity += qp.analyzeComplexity(mustNot)
		}
		for _, filter := range q.Filter {
			complexity += qp.analyzeComplexity(filter)
		}

	case *parser.WildcardQuery, *parser.QueryStringQuery:
		complexity = 30 // Expensive operations

	case *parser.FuzzyQuery:
		complexity = 40 // Very expensive

	case *parser.PrefixQuery:
		complexity = 20

	case *parser.MatchPhraseQuery:
		complexity = 25

	case *parser.MultiMatchQuery:
		complexity = 15 * len(q.Fields)

	case *parser.ExistsQuery:
		complexity = 10

	case *parser.ExpressionQuery:
		complexity = 50 // Expression queries can be complex

	case *parser.WasmUDFQuery:
		complexity = 60 // WASM UDFs can be very complex

	default:
		complexity = 20
	}

	// Cap at 100
	if complexity > 100 {
		complexity = 100
	}

	return complexity
}

// optimizeQuery applies optimization rules to the query
func (qp *QueryPlanner) optimizeQuery(query parser.Query) parser.Query {
	if query == nil {
		return query
	}

	// Use type switch to optimize different query types
	switch q := query.(type) {
	case *parser.BoolQuery:
		// Optimize boolean queries
		return qp.optimizeBoolQuery(q)

	case *parser.TermsQuery:
		// Optimize terms queries with many values
		if len(q.Values) > 1000 {
			qp.logger.Warn("Large terms query detected",
				zap.Int("values_count", len(q.Values)))
		}
		return q

	default:
		// No optimization needed for other query types
		return query
	}
}

// optimizeBoolQuery optimizes boolean queries
func (qp *QueryPlanner) optimizeBoolQuery(boolQuery *parser.BoolQuery) *parser.BoolQuery {
	if boolQuery == nil {
		return boolQuery
	}

	optimized := &parser.BoolQuery{
		Must:                   make([]parser.Query, 0, len(boolQuery.Must)),
		Should:                 make([]parser.Query, 0, len(boolQuery.Should)),
		MustNot:                make([]parser.Query, 0, len(boolQuery.MustNot)),
		Filter:                 make([]parser.Query, 0, len(boolQuery.Filter)),
		MinimumShouldMatch:     boolQuery.MinimumShouldMatch,
		MinimumShouldMatchStr:  boolQuery.MinimumShouldMatchStr,
	}

	// Move filters before must clauses (filters are faster)
	optimized.Filter = append(optimized.Filter, boolQuery.Filter...)
	optimized.Must = append(optimized.Must, boolQuery.Must...)
	optimized.Should = append(optimized.Should, boolQuery.Should...)
	optimized.MustNot = append(optimized.MustNot, boolQuery.MustNot...)

	// Recursively optimize nested queries
	for i, q := range optimized.Must {
		optimized.Must[i] = qp.optimizeQuery(q)
	}
	for i, q := range optimized.Filter {
		optimized.Filter[i] = qp.optimizeQuery(q)
	}

	return optimized
}

// selectShards determines which shards to query
func (qp *QueryPlanner) selectShards(routing map[int32]*pb.ShardRouting, searchReq *parser.SearchRequest) []int32 {
	targetShards := make([]int32, 0, len(routing))

	// For now, query all available shards
	// TODO: Implement intelligent shard selection based on:
	// - Routing values
	// - Shard statistics
	// - Time-based partitioning
	for shardID, shard := range routing {
		if shard.Allocation != nil && shard.Allocation.State == pb.ShardAllocation_SHARD_STATE_STARTED {
			targetShards = append(targetShards, shardID)
		}
	}

	return targetShards
}

// estimateCost estimates the cost of executing this query
func (qp *QueryPlanner) estimateCost(plan *QueryPlan, metadata *pb.IndexMetadataResponse) float64 {
	// Simple cost model based on:
	// - Number of shards
	// - Query complexity
	// - Result size

	baseCost := float64(len(plan.TargetShards)) * 10.0
	complexityCost := float64(plan.Complexity) * 5.0
	resultCost := float64(plan.SearchRequest.Size) * 0.1

	totalCost := baseCost + complexityCost + resultCost

	return totalCost
}

// isCacheable determines if query results can be cached
func (qp *QueryPlanner) isCacheable(searchReq *parser.SearchRequest) bool {
	// Don't cache if:
	// - Size is 0 (aggregation only)
	// - From offset is very high (deep pagination)
	// - Query is very simple (match_all)

	if searchReq.Size == 0 {
		return false
	}

	if searchReq.From > 1000 {
		return false // Deep pagination shouldn't be cached
	}

	// Check if it's a match_all query (which changes frequently)
	if _, isMatchAll := searchReq.ParsedQuery.(*parser.MatchAllQuery); isMatchAll {
		return false
	}

	if searchReq.ParsedQuery == nil {
		return false
	}

	return true
}

// GetCachedResults retrieves cached query results if available
func (qp *QueryPlanner) GetCachedResults(cacheKey string) (interface{}, bool) {
	return qp.cache.Get(cacheKey)
}

// CacheResults stores query results in the cache
func (qp *QueryPlanner) CacheResults(cacheKey string, results interface{}) {
	qp.cache.Set(cacheKey, results)
}

// ClearCache clears the query cache
func (qp *QueryPlanner) ClearCache() {
	qp.cache.Clear()
}

// OptimizationHint represents a suggestion for query optimization
type OptimizationHint struct {
	Type        string
	Description string
	Severity    string // "info", "warning", "error"
}

// AnalyzeQuery provides optimization hints for a query
func (qp *QueryPlanner) AnalyzeQuery(query parser.Query) []*OptimizationHint {
	hints := make([]*OptimizationHint, 0)

	if query == nil {
		return hints
	}

	// Use type switch to check for different query types
	switch q := query.(type) {
	case *parser.WildcardQuery, *parser.QueryStringQuery:
		hints = append(hints, &OptimizationHint{
			Type:        "expensive_query",
			Description: "Wildcard and regexp queries are expensive. Consider using prefix or term queries.",
			Severity:    "warning",
		})

	case *parser.FuzzyQuery:
		hints = append(hints, &OptimizationHint{
			Type:        "expensive_query",
			Description: "Fuzzy queries are very expensive. Consider reducing the fuzziness parameter.",
			Severity:    "warning",
		})

	case *parser.TermsQuery:
		if len(q.Values) > 100 {
			hints = append(hints, &OptimizationHint{
				Type:        "large_terms_query",
				Description: fmt.Sprintf("Terms query has %d values. Consider using a different approach.", len(q.Values)),
				Severity:    "warning",
			})
		}
	case *parser.BoolQuery:
		// Check for excessive should clauses
		if len(q.Should) > 20 {
			hints = append(hints, &OptimizationHint{
				Type:        "complex_bool_query",
				Description: fmt.Sprintf("Bool query has %d should clauses. Consider simplifying.", len(q.Should)),
				Severity:    "info",
			})
		}

		// Suggest moving non-scoring clauses to filter
		if len(q.Must) > 0 {
			hints = append(hints, &OptimizationHint{
				Type:        "filter_suggestion",
				Description: "Consider using 'filter' instead of 'must' for clauses that don't need scoring.",
				Severity:    "info",
			})
		}
	}

	return hints
}
