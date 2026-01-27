package coordination

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"github.com/quidditch/quidditch/pkg/coordination/cache"
	"github.com/quidditch/quidditch/pkg/coordination/executor"
	"github.com/quidditch/quidditch/pkg/coordination/parser"
	"github.com/quidditch/quidditch/pkg/coordination/pipeline"
	"github.com/quidditch/quidditch/pkg/coordination/planner"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// Prometheus metrics for query service
var (
	queryPlanningTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "quidditch_query_planning_seconds",
			Help:    "Query planning time in seconds",
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 12), // 0.1ms to ~400ms
		},
		[]string{"index", "stage"},
	)

	queryOptimizationTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "quidditch_query_optimization_seconds",
			Help:    "Query optimization time in seconds",
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 12), // 0.1ms to ~400ms
		},
		[]string{"index"},
	)

	queryExecutionTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "quidditch_query_execution_seconds",
			Help:    "Query execution time in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 14), // 1ms to ~16s
		},
		[]string{"index", "status"},
	)

	logicalPlanComplexity = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "quidditch_logical_plan_complexity",
			Help:    "Logical plan complexity (estimated cardinality)",
			Buckets: prometheus.ExponentialBuckets(1, 2, 20), // 1 to ~1M
		},
		[]string{"index"},
	)

	optimizationPassCount = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "quidditch_optimization_passes",
			Help:    "Number of optimization passes applied",
			Buckets: prometheus.LinearBuckets(0, 1, 10), // 0 to 10 passes
		},
		[]string{"index"},
	)
)

// QueryService provides high-level query execution with the complete planner pipeline
type QueryService struct {
	logger           *zap.Logger
	queryParser      *parser.QueryParser
	converter        *planner.Converter
	optimizer        *planner.Optimizer
	costModel        *planner.CostModel
	physicalPlanner  *planner.Planner
	queryExecutor    queryExecutorInterface
	masterClient     masterClientInterface
	queryCache       *cache.QueryCache
	pipelineRegistry *pipeline.Registry
	pipelineExecutor *pipeline.Executor
}

// queryExecutorInterface defines the methods needed from query executor
type queryExecutorInterface interface {
	ExecuteSearch(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error)
}

// masterClientInterface defines the methods needed from master client
type masterClientInterface interface {
	GetShardRouting(ctx context.Context, indexName string) (map[int32]*pb.ShardRouting, error)
	GetIndexMetadata(ctx context.Context, indexName string) (*pb.IndexMetadataResponse, error)
}

// NewQueryService creates a new query service with the complete planner pipeline
func NewQueryService(
	queryExecutor queryExecutorInterface,
	masterClient masterClientInterface,
	logger *zap.Logger,
) *QueryService {
	return &QueryService{
		logger:           logger,
		queryParser:      parser.NewQueryParser(),
		converter:        planner.NewConverter(),
		optimizer:        planner.NewOptimizer(),
		costModel:        planner.NewDefaultCostModel(),
		physicalPlanner:  planner.NewPlanner(planner.NewDefaultCostModel()),
		queryExecutor:    queryExecutor,
		masterClient:     masterClient,
		queryCache:       cache.NewQueryCache(cache.DefaultQueryCacheConfig()),
		pipelineRegistry: nil, // Pipelines optional
		pipelineExecutor: nil,
	}
}

// NewQueryServiceWithCache creates a new query service with custom cache configuration
func NewQueryServiceWithCache(
	queryExecutor queryExecutorInterface,
	masterClient masterClientInterface,
	logger *zap.Logger,
	cacheConfig *cache.QueryCacheConfig,
) *QueryService {
	return &QueryService{
		logger:           logger,
		queryParser:      parser.NewQueryParser(),
		converter:        planner.NewConverter(),
		optimizer:        planner.NewOptimizer(),
		costModel:        planner.NewDefaultCostModel(),
		physicalPlanner:  planner.NewPlanner(planner.NewDefaultCostModel()),
		queryExecutor:    queryExecutor,
		masterClient:     masterClient,
		queryCache:       cache.NewQueryCache(cacheConfig),
		pipelineRegistry: nil, // Pipelines optional
		pipelineExecutor: nil,
	}
}

// SetPipelineComponents sets the pipeline registry and executor (optional)
func (qs *QueryService) SetPipelineComponents(registry *pipeline.Registry, executor *pipeline.Executor) {
	qs.pipelineRegistry = registry
	qs.pipelineExecutor = executor
}

// SearchResult represents a search result with all metadata
type SearchResult struct {
	TookMillis   int64
	TotalHits    int64
	MaxScore     float64
	Hits         []*SearchHit
	Aggregations map[string]*AggregationResult
	Shards       *ShardInfo
}

// SearchHit represents a single hit
type SearchHit struct {
	ID     string
	Score  float64
	Source map[string]interface{}
}

// AggregationResult represents an aggregation result
type AggregationResult struct {
	Type    string
	Buckets []*AggregationBucket

	// For single-value aggregations
	Value float64

	// For stats aggregations
	Count int64
	Min   float64
	Max   float64
	Avg   float64
	Sum   float64
}

// AggregationBucket represents a bucket in a bucket aggregation
type AggregationBucket struct {
	Key      interface{}
	DocCount int64
	SubAggs  map[string]*AggregationResult
}

// ShardInfo represents shard execution information
type ShardInfo struct {
	Total      int
	Successful int
	Skipped    int
	Failed     int
}

// ExecuteSearch executes a search query using the complete planner pipeline
func (qs *QueryService) ExecuteSearch(ctx context.Context, indexName string, requestBody []byte) (*SearchResult, error) {
	startTime := time.Now()

	qs.logger.Info("==> QueryService.ExecuteSearch ENTRY",
		zap.String("index", indexName),
		zap.Int("body_len", len(requestBody)),
		zap.String("body", string(requestBody)))

	// Step 1: Parse query
	parseStart := time.Now()
	var searchReq *parser.SearchRequest
	var err error

	if len(requestBody) > 0 {
		searchReq, err = qs.queryParser.ParseSearchRequest(requestBody)
		if err != nil {
			qs.logger.Error("Failed to parse query", zap.Error(err))
			return nil, fmt.Errorf("failed to parse query: %w", err)
		}

		// Validate parsed query
		if searchReq.ParsedQuery != nil {
			if err := qs.queryParser.Validate(searchReq.ParsedQuery); err != nil {
				qs.logger.Error("Query validation failed", zap.Error(err))
				return nil, fmt.Errorf("query validation failed: %w", err)
			}
		}
	} else {
		// Empty body - match all query
		searchReq = &parser.SearchRequest{
			ParsedQuery: &parser.MatchAllQuery{},
			Size:        10,
		}
	}

	qs.logger.Info("Query parsed successfully",
		zap.String("index", indexName),
		zap.Int("size", searchReq.Size))

	queryPlanningTime.WithLabelValues(indexName, "parse").Observe(time.Since(parseStart).Seconds())

	// Step 1.5: Execute query pipeline if configured
	if qs.pipelineRegistry != nil && qs.pipelineExecutor != nil {
		queryPipelineStart := time.Now()
		modifiedReq, err := qs.executeQueryPipeline(ctx, indexName, searchReq)
		if err != nil {
			// Log warning but continue with original request (graceful degradation)
			qs.logger.Warn("Query pipeline failed, continuing with original request",
				zap.String("index", indexName),
				zap.Error(err))
		} else if modifiedReq != nil {
			searchReq = modifiedReq
			qs.logger.Info("Query pipeline executed successfully",
				zap.String("index", indexName),
				zap.Duration("duration", time.Since(queryPipelineStart)))
		}
		queryPlanningTime.WithLabelValues(indexName, "query_pipeline").Observe(time.Since(queryPipelineStart).Seconds())
	}

	// Step 2: Get shard routing for this index
	routing, err := qs.masterClient.GetShardRouting(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to get shard routing: %w", err)
	}

	// Extract shard IDs
	shardIDs := make([]int32, 0, len(routing))
	for shardID, shard := range routing {
		if shard.Allocation != nil && shard.Allocation.State == pb.ShardAllocation_SHARD_STATE_STARTED {
			shardIDs = append(shardIDs, shardID)
		}
	}

	if len(shardIDs) == 0 {
		return nil, fmt.Errorf("no active shards found for index %s", indexName)
	}

	// Step 3: Check logical plan cache or convert AST to Logical Plan
	convertStart := time.Now()
	var logicalPlan planner.LogicalPlan

	// Try to get from cache
	cachedLogicalPlan, found := qs.queryCache.GetLogicalPlan(indexName, searchReq, shardIDs)
	if found {
		logicalPlan = cachedLogicalPlan
		qs.logger.Debug("Logical plan retrieved from cache",
			zap.String("index", indexName),
			zap.String("plan", logicalPlan.String()))
	} else {
		// Convert AST to Logical Plan
		logicalPlan, err = qs.converter.ConvertSearchRequest(searchReq, indexName, shardIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to convert query to logical plan: %w", err)
		}
		qs.logger.Debug("Logical plan created",
			zap.String("index", indexName),
			zap.String("plan", logicalPlan.String()))
	}
	queryPlanningTime.WithLabelValues(indexName, "convert").Observe(time.Since(convertStart).Seconds())

	// Record logical plan complexity
	logicalPlanComplexity.WithLabelValues(indexName).Observe(float64(logicalPlan.Cardinality()))

	// Step 4: Optimize Logical Plan (if not from cache)
	var optimizedPlan planner.LogicalPlan
	if !found {
		// Plan was just created, so optimize it
		optimizeStart := time.Now()
		optimizedPlan, err = qs.optimizer.Optimize(logicalPlan)
		if err != nil {
			qs.logger.Warn("Optimization failed, using unoptimized plan",
				zap.String("index", indexName),
				zap.Error(err))
			optimizedPlan = logicalPlan
		}
		optimizeTime := time.Since(optimizeStart)
		queryOptimizationTime.WithLabelValues(indexName).Observe(optimizeTime.Seconds())

		// Record optimization passes (simplified - using 1 or 0)
		if optimizedPlan != logicalPlan {
			optimizationPassCount.WithLabelValues(indexName).Observe(1)
		} else {
			optimizationPassCount.WithLabelValues(indexName).Observe(0)
		}

		qs.logger.Debug("Logical plan optimized",
			zap.String("index", indexName),
			zap.String("optimized_plan", optimizedPlan.String()),
			zap.Duration("optimization_time", optimizeTime))

		// Cache the optimized logical plan
		qs.queryCache.PutLogicalPlan(indexName, searchReq, shardIDs, optimizedPlan)
	} else {
		// Plan from cache is already optimized
		optimizedPlan = logicalPlan
	}

	// Step 5: Check physical plan cache or create Physical Plan
	physicalStart := time.Now()
	var physicalPlan planner.PhysicalPlan

	// Try to get from cache
	cachedPhysicalPlan, foundPhysical := qs.queryCache.GetPhysicalPlan(indexName, optimizedPlan)
	if foundPhysical {
		physicalPlan = cachedPhysicalPlan
		qs.logger.Debug("Physical plan retrieved from cache",
			zap.String("index", indexName),
			zap.String("plan", physicalPlan.String()))
	} else {
		// Convert to Physical Plan
		physicalPlan, err = qs.physicalPlanner.Plan(optimizedPlan)
		if err != nil {
			return nil, fmt.Errorf("failed to create physical plan: %w", err)
		}

		qs.logger.Debug("Physical plan created",
			zap.String("index", indexName),
			zap.String("plan", physicalPlan.String()),
			zap.Float64("estimated_cost", physicalPlan.Cost().TotalCost))

		// Cache the physical plan
		qs.queryCache.PutPhysicalPlan(indexName, optimizedPlan, physicalPlan)
	}
	queryPlanningTime.WithLabelValues(indexName, "physical").Observe(time.Since(physicalStart).Seconds())

	// Step 6: Execute Physical Plan
	executeStart := time.Now()

	// Create execution context
	execCtx := &planner.ExecutionContext{
		QueryExecutor: qs.queryExecutor,
		Logger:        qs.logger,
	}
	ctxWithExec := planner.WithExecutionContext(ctx, execCtx)

	// Execute plan
	executionResult, err := physicalPlan.Execute(ctxWithExec)
	executeTime := time.Since(executeStart)

	if err != nil {
		queryExecutionTime.WithLabelValues(indexName, "error").Observe(executeTime.Seconds())
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	queryExecutionTime.WithLabelValues(indexName, "success").Observe(executeTime.Seconds())

	// Convert ExecutionResult to SearchResult
	totalTime := time.Since(startTime)
	result := qs.convertToSearchResult(executionResult, totalTime, len(shardIDs))

	// Step 7: Execute result pipeline if configured
	if qs.pipelineRegistry != nil && qs.pipelineExecutor != nil {
		resultPipelineStart := time.Now()
		modifiedResult, err := qs.executeResultPipeline(ctx, indexName, result, searchReq)
		if err != nil {
			// Log warning but continue with original results (graceful degradation)
			qs.logger.Warn("Result pipeline failed, continuing with original results",
				zap.String("index", indexName),
				zap.Error(err))
		} else if modifiedResult != nil {
			result = modifiedResult
			qs.logger.Info("Result pipeline executed successfully",
				zap.String("index", indexName),
				zap.Duration("duration", time.Since(resultPipelineStart)))
		}
		queryPlanningTime.WithLabelValues(indexName, "result_pipeline").Observe(time.Since(resultPipelineStart).Seconds())
	}

	qs.logger.Info("Query executed successfully",
		zap.String("index", indexName),
		zap.Int64("total_hits", result.TotalHits),
		zap.Int("hits_returned", len(result.Hits)),
		zap.Duration("total_time", totalTime),
		zap.Duration("execute_time", executeTime))

	return result, nil
}

// convertToSearchResult converts ExecutionResult to SearchResult
func (qs *QueryService) convertToSearchResult(execResult *planner.ExecutionResult, totalTime time.Duration, totalShards int) *SearchResult {
	result := &SearchResult{
		TookMillis:   totalTime.Milliseconds(),
		TotalHits:    execResult.TotalHits,
		MaxScore:     execResult.MaxScore,
		Hits:         make([]*SearchHit, len(execResult.Rows)),
		Aggregations: make(map[string]*AggregationResult),
		Shards: &ShardInfo{
			Total:      totalShards,
			Successful: totalShards,
			Skipped:    0,
			Failed:     0,
		},
	}

	// Convert hits
	for i, row := range execResult.Rows {
		hit := &SearchHit{
			Source: make(map[string]interface{}),
		}

		// Extract _id and _score
		if id, ok := row["_id"].(string); ok {
			hit.ID = id
			delete(row, "_id")
		}
		if score, ok := row["_score"].(float64); ok {
			hit.Score = score
			delete(row, "_score")
		}

		// Copy remaining fields to source
		for k, v := range row {
			hit.Source[k] = v
		}

		result.Hits[i] = hit
	}

	// Convert aggregations
	for name, agg := range execResult.Aggregations {
		result.Aggregations[name] = qs.convertAggregation(agg)
	}

	return result
}

// convertAggregation converts planner.AggregationResult to SearchResult.AggregationResult
func (qs *QueryService) convertAggregation(agg *planner.AggregationResult) *AggregationResult {
	result := &AggregationResult{
		Type:    string(agg.Type),
		Buckets: make([]*AggregationBucket, len(agg.Buckets)),
		Value:   agg.Value,
	}

	// Convert buckets
	for i, bucket := range agg.Buckets {
		result.Buckets[i] = &AggregationBucket{
			Key:      bucket.Key,
			DocCount: bucket.DocCount,
			SubAggs:  make(map[string]*AggregationResult),
		}

		// Convert sub-aggregations recursively
		for subName, subAgg := range bucket.SubAggs {
			result.Buckets[i].SubAggs[subName] = qs.convertAggregation(subAgg)
		}
	}

	// For stats aggregations
	if agg.Stats != nil {
		result.Count = agg.Stats.Count
		result.Min = agg.Stats.Min
		result.Max = agg.Stats.Max
		result.Avg = agg.Stats.Avg
		result.Sum = agg.Stats.Sum
	}

	return result
}

// executeQueryPipeline executes the query pipeline for an index if configured
func (qs *QueryService) executeQueryPipeline(ctx context.Context, indexName string, req *parser.SearchRequest) (*parser.SearchRequest, error) {
	// Get query pipeline for this index
	pipe, err := qs.pipelineRegistry.GetPipelineForIndex(indexName, pipeline.PipelineTypeQuery)
	if err != nil {
		// No pipeline configured - not an error
		return req, nil
	}

	qs.logger.Debug("Executing query pipeline",
		zap.String("index", indexName),
		zap.String("pipeline", pipe.Name()))

	// Convert SearchRequest to map for pipeline execution
	requestMap, err := qs.searchRequestToMap(req)
	if err != nil {
		return nil, fmt.Errorf("failed to convert search request: %w", err)
	}

	// Execute pipeline
	output, err := pipe.Execute(ctx, requestMap)
	if err != nil {
		return nil, fmt.Errorf("pipeline execution failed: %w", err)
	}

	// Convert back to SearchRequest
	outputMap, ok := output.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("pipeline output is not a map, got %T", output)
	}

	modifiedReq, err := qs.mapToSearchRequest(outputMap)
	if err != nil {
		return nil, fmt.Errorf("failed to convert pipeline output: %w", err)
	}

	return modifiedReq, nil
}

// executeResultPipeline executes the result pipeline for an index if configured
func (qs *QueryService) executeResultPipeline(ctx context.Context, indexName string, result *SearchResult, originalReq *parser.SearchRequest) (*SearchResult, error) {
	// Get result pipeline for this index
	pipe, err := qs.pipelineRegistry.GetPipelineForIndex(indexName, pipeline.PipelineTypeResult)
	if err != nil {
		// No pipeline configured - not an error
		return result, nil
	}

	qs.logger.Debug("Executing result pipeline",
		zap.String("index", indexName),
		zap.String("pipeline", pipe.Name()))

	// Convert SearchResult to map for pipeline execution
	// Include both results and original request for context
	pipelineInput := map[string]interface{}{
		"results": qs.searchResultToMap(result),
		"request": qs.searchRequestToMapSimple(originalReq),
	}

	// Execute pipeline
	output, err := pipe.Execute(ctx, pipelineInput)
	if err != nil {
		return nil, fmt.Errorf("pipeline execution failed: %w", err)
	}

	// Convert back to SearchResult
	outputMap, ok := output.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("pipeline output is not a map, got %T", output)
	}

	// Extract results from output
	resultsMap, ok := outputMap["results"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("pipeline output missing 'results' field")
	}

	modifiedResult, err := qs.mapToSearchResult(resultsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to convert pipeline output: %w", err)
	}

	return modifiedResult, nil
}

// searchRequestToMap converts SearchRequest to map for pipeline
func (qs *QueryService) searchRequestToMap(req *parser.SearchRequest) (map[string]interface{}, error) {
	// Marshal to JSON then unmarshal to map (simple conversion)
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// searchRequestToMapSimple converts SearchRequest to map (simplified version)
func (qs *QueryService) searchRequestToMapSimple(req *parser.SearchRequest) map[string]interface{} {
	return map[string]interface{}{
		"from": req.From,
		"size": req.Size,
		// Add other relevant fields as needed
	}
}

// mapToSearchRequest converts map back to SearchRequest
func (qs *QueryService) mapToSearchRequest(m map[string]interface{}) (*parser.SearchRequest, error) {
	// Marshal to JSON then unmarshal to SearchRequest
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	// Parse the modified JSON
	return qs.queryParser.ParseSearchRequest(data)
}

// searchResultToMap converts SearchResult to map for pipeline
func (qs *QueryService) searchResultToMap(result *SearchResult) map[string]interface{} {
	// Convert hits
	hits := make([]interface{}, len(result.Hits))
	for i, hit := range result.Hits {
		hits[i] = map[string]interface{}{
			"_id":     hit.ID,
			"_score":  hit.Score,
			"_source": hit.Source,
		}
	}

	return map[string]interface{}{
		"took":       result.TookMillis,
		"total_hits": result.TotalHits,
		"max_score":  result.MaxScore,
		"hits":       hits,
	}
}

// mapToSearchResult converts map back to SearchResult
func (qs *QueryService) mapToSearchResult(m map[string]interface{}) (*SearchResult, error) {
	result := &SearchResult{
		Aggregations: make(map[string]*AggregationResult),
		Hits:         []*SearchHit{},
		Shards: &ShardInfo{
			Total:      0,
			Successful: 0,
			Skipped:    0,
			Failed:     0,
		},
	}

	// Extract basic fields
	if took, ok := m["took"].(float64); ok {
		result.TookMillis = int64(took)
	}
	if took, ok := m["took"].(int64); ok {
		result.TookMillis = took
	}
	if totalHits, ok := m["total_hits"].(float64); ok {
		result.TotalHits = int64(totalHits)
	}
	if totalHits, ok := m["total_hits"].(int64); ok {
		result.TotalHits = totalHits
	}
	if maxScore, ok := m["max_score"].(float64); ok {
		result.MaxScore = maxScore
	}

	// Extract hits
	if hitsData, ok := m["hits"].([]interface{}); ok {
		result.Hits = make([]*SearchHit, 0, len(hitsData))
		for _, hitData := range hitsData {
			hitMap, ok := hitData.(map[string]interface{})
			if !ok {
				continue
			}

			hit := &SearchHit{
				Source: make(map[string]interface{}),
			}
			if id, ok := hitMap["_id"].(string); ok {
				hit.ID = id
			}
			if score, ok := hitMap["_score"].(float64); ok {
				hit.Score = score
			}
			if source, ok := hitMap["_source"].(map[string]interface{}); ok {
				hit.Source = source
			}

			result.Hits = append(result.Hits, hit)
		}
	}

	return result, nil
}
