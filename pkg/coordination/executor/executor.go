package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"go.uber.org/zap"
)

// DataNodeClient interface for communication with data nodes
type DataNodeClient interface {
	Search(ctx context.Context, indexName string, shardID int32, query []byte, filterExpression []byte) (*pb.SearchResponse, error)
	Count(ctx context.Context, indexName string, shardID int32, query []byte, filterExpression []byte) (*pb.CountResponse, error)
	IsConnected() bool
	Connect(ctx context.Context) error
	NodeID() string
}

// MasterClient interface for getting cluster state
type MasterClient interface {
	GetShardRouting(ctx context.Context, indexName string) (map[int32]*pb.ShardRouting, error)
}

// QueryExecutor executes search queries across multiple shards
type QueryExecutor struct {
	logger       *zap.Logger
	masterClient MasterClient
	dataClients  map[string]DataNodeClient // nodeID -> client
	mu           sync.RWMutex
}

// NewQueryExecutor creates a new query executor
func NewQueryExecutor(masterClient MasterClient, logger *zap.Logger) *QueryExecutor {
	return &QueryExecutor{
		logger:       logger,
		masterClient: masterClient,
		dataClients:  make(map[string]DataNodeClient),
	}
}

// RegisterDataNode registers a data node client
func (qe *QueryExecutor) RegisterDataNode(client DataNodeClient) {
	qe.mu.Lock()
	defer qe.mu.Unlock()
	qe.dataClients[client.NodeID()] = client
	qe.logger.Info("Registered data node client", zap.String("node_id", client.NodeID()))
}

// UnregisterDataNode unregisters a data node client
func (qe *QueryExecutor) UnregisterDataNode(nodeID string) {
	qe.mu.Lock()
	defer qe.mu.Unlock()
	delete(qe.dataClients, nodeID)
	qe.logger.Info("Unregistered data node client", zap.String("node_id", nodeID))
}

// ExecuteSearch executes a search query across all relevant shards
func (qe *QueryExecutor) ExecuteSearch(ctx context.Context, indexName string, query []byte, filterExpression []byte, from, size int) (*SearchResult, error) {
	startTime := time.Now()

	// Get shard routing from master
	routing, err := qe.masterClient.GetShardRouting(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to get shard routing: %w", err)
	}

	if len(routing) == 0 {
		return &SearchResult{
			TookMillis: time.Since(startTime).Milliseconds(),
			TotalHits:  0,
			MaxScore:   0,
			Hits:       []*SearchHit{},
		}, nil
	}

	// Execute search on all shards in parallel
	type shardResult struct {
		shardID  int32
		response *pb.SearchResponse
		err      error
	}

	resultsChan := make(chan shardResult, len(routing))
	var wg sync.WaitGroup

	for shardID, shard := range routing {
		// Only query primary or started replicas
		if shard.Allocation == nil || shard.Allocation.State != pb.ShardAllocation_SHARD_STATE_STARTED {
			qe.logger.Debug("Skipping shard - not started",
				zap.String("index", indexName),
				zap.Int32("shard_id", shardID))
			continue
		}

		nodeID := shard.Allocation.NodeId
		if nodeID == "" {
			qe.logger.Warn("Shard has no node assignment",
				zap.String("index", indexName),
				zap.Int32("shard_id", shardID))
			continue
		}

		wg.Add(1)
		go func(sid int32, nid string) {
			defer wg.Done()

			// Get data node client
			qe.mu.RLock()
			client, exists := qe.dataClients[nid]
			qe.mu.RUnlock()

			if !exists {
				qe.logger.Error("Data node client not found",
					zap.String("node_id", nid),
					zap.Int32("shard_id", sid))
				resultsChan <- shardResult{
					shardID: sid,
					err:     fmt.Errorf("data node %s not found", nid),
				}
				return
			}

			// Ensure client is connected
			if !client.IsConnected() {
				if err := client.Connect(ctx); err != nil {
					qe.logger.Error("Failed to connect to data node",
						zap.String("node_id", nid),
						zap.Error(err))
					resultsChan <- shardResult{
						shardID: sid,
						err:     fmt.Errorf("failed to connect to node %s: %w", nid, err),
					}
					return
				}
			}

			// Execute search on shard
			resp, err := client.Search(ctx, indexName, sid, query, filterExpression)
			resultsChan <- shardResult{
				shardID:  sid,
				response: resp,
				err:      err,
			}
		}(shardID, nodeID)
	}

	// Wait for all shard searches to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	var shardResponses []*pb.SearchResponse
	var errors []error

	for result := range resultsChan {
		if result.err != nil {
			qe.logger.Error("Shard search failed",
				zap.Int32("shard_id", result.shardID),
				zap.Error(result.err))
			errors = append(errors, result.err)
			continue
		}
		shardResponses = append(shardResponses, result.response)
	}

	// Check if we have any successful results
	if len(shardResponses) == 0 {
		if len(errors) > 0 {
			return nil, fmt.Errorf("all shard searches failed: %v", errors[0])
		}
		return nil, fmt.Errorf("no shards available for index %s", indexName)
	}

	// Aggregate results
	aggregatedResult := qe.aggregateSearchResults(shardResponses, from, size)
	aggregatedResult.TookMillis = time.Since(startTime).Milliseconds()

	qe.logger.Debug("Search completed",
		zap.String("index", indexName),
		zap.Int64("total_hits", aggregatedResult.TotalHits),
		zap.Int("returned_hits", len(aggregatedResult.Hits)),
		zap.Int64("took_ms", aggregatedResult.TookMillis))

	return aggregatedResult, nil
}

// ExecuteCount executes a count query across all relevant shards
func (qe *QueryExecutor) ExecuteCount(ctx context.Context, indexName string, query []byte, filterExpression []byte) (int64, error) {
	// Get shard routing from master
	routing, err := qe.masterClient.GetShardRouting(ctx, indexName)
	if err != nil {
		return 0, fmt.Errorf("failed to get shard routing: %w", err)
	}

	if len(routing) == 0 {
		return 0, nil
	}

	// Execute count on all shards in parallel
	type shardResult struct {
		count int64
		err   error
	}

	resultsChan := make(chan shardResult, len(routing))
	var wg sync.WaitGroup

	for shardID, shard := range routing {
		// Only query primary or started replicas
		if shard.Allocation == nil || shard.Allocation.State != pb.ShardAllocation_SHARD_STATE_STARTED {
			continue
		}

		nodeID := shard.Allocation.NodeId
		if nodeID == "" {
			continue
		}

		wg.Add(1)
		go func(sid int32, nid string) {
			defer wg.Done()

			// Get data node client
			qe.mu.RLock()
			client, exists := qe.dataClients[nid]
			qe.mu.RUnlock()

			if !exists {
				resultsChan <- shardResult{err: fmt.Errorf("data node %s not found", nid)}
				return
			}

			// Ensure client is connected
			if !client.IsConnected() {
				if err := client.Connect(ctx); err != nil {
					resultsChan <- shardResult{err: err}
					return
				}
			}

			// Execute count on shard
			resp, err := client.Count(ctx, indexName, sid, query, filterExpression)
			if err != nil {
				resultsChan <- shardResult{err: err}
				return
			}

			resultsChan <- shardResult{count: resp.Count}
		}(shardID, nodeID)
	}

	// Wait for all shard counts to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Sum up counts
	var totalCount int64
	for result := range resultsChan {
		if result.err != nil {
			qe.logger.Error("Shard count failed", zap.Error(result.err))
			continue
		}
		totalCount += result.count
	}

	return totalCount, nil
}

// SearchResult represents aggregated search results
type SearchResult struct {
	TookMillis int64
	TotalHits  int64
	MaxScore   float64
	Hits       []*SearchHit
}

// SearchHit represents a single search hit
type SearchHit struct {
	ID     string
	Score  float64
	Source map[string]interface{}
}
