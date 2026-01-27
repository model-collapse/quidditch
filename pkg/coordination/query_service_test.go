package coordination

import (
	"context"
	"testing"
	"time"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"github.com/quidditch/quidditch/pkg/coordination/executor"
	"github.com/quidditch/quidditch/pkg/coordination/planner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Mock master client for testing
type mockMasterClient struct {
	shardRouting map[int32]*pb.ShardRouting
	metadata     *pb.IndexMetadataResponse
}

func (m *mockMasterClient) GetShardRouting(ctx context.Context, indexName string) (map[int32]*pb.ShardRouting, error) {
	if m.shardRouting == nil {
		return map[int32]*pb.ShardRouting{
			0: {
				ShardId:   0,
				IsPrimary: true,
				Allocation: &pb.ShardAllocation{
					NodeId: "node1",
					State:  pb.ShardAllocation_SHARD_STATE_STARTED,
				},
			},
		}, nil
	}
	return m.shardRouting, nil
}

func (m *mockMasterClient) GetIndexMetadata(ctx context.Context, indexName string) (*pb.IndexMetadataResponse, error) {
	if m.metadata == nil {
		return &pb.IndexMetadataResponse{
			Metadata: &pb.IndexMetadata{
				IndexName: indexName,
				Settings: &pb.IndexSettings{
					NumberOfShards: 1,
				},
			},
		}, nil
	}
	return m.metadata, nil
}

// Mock query executor for testing
type mockQueryExecutor struct {
	searchFunc func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error)
}

func (m *mockQueryExecutor) ExecuteSearch(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, indexName, query, filterExpr, from, size)
	}
	return &executor.SearchResult{
		TotalHits:  0,
		MaxScore:   0,
		Hits:       []*executor.SearchHit{},
		TookMillis: 1,
	}, nil
}

func TestNewQueryService(t *testing.T) {
	logger := zap.NewNop()
	mockExec := &mockQueryExecutor{}
	mockMaster := &mockMasterClient{}

	service := NewQueryService(mockExec, mockMaster, logger)

	assert.NotNil(t, service)
	assert.NotNil(t, service.converter)
	assert.NotNil(t, service.optimizer)
	assert.NotNil(t, service.physicalPlanner)
	assert.NotNil(t, service.queryExecutor)
}

func TestExecuteSearchMatchAll(t *testing.T) {
	logger := zap.NewNop()

	mockExec := &mockQueryExecutor{
		searchFunc: func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
			return &executor.SearchResult{
				TotalHits:  100,
				MaxScore:   1.0,
				TookMillis: 5,
				Hits: []*executor.SearchHit{
					{ID: "1", Score: 1.0, Source: map[string]interface{}{"title": "Doc 1"}},
					{ID: "2", Score: 0.9, Source: map[string]interface{}{"title": "Doc 2"}},
				},
			}, nil
		},
	}

	mockMaster := &mockMasterClient{}

	service := NewQueryService(mockExec, mockMaster, logger)

	// Test with empty body (match_all query)
	result, err := service.ExecuteSearch(context.Background(), "products", []byte{})

	require.NoError(t, err)
	assert.Equal(t, int64(100), result.TotalHits)
	assert.Equal(t, 1.0, result.MaxScore)
	assert.Len(t, result.Hits, 2)
	assert.Equal(t, "1", result.Hits[0].ID)
	assert.Equal(t, "Doc 1", result.Hits[0].Source["title"])
}

func TestExecuteSearchTermQuery(t *testing.T) {
	logger := zap.NewNop()

	mockExec := &mockQueryExecutor{
		searchFunc: func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
			return &executor.SearchResult{
				TotalHits:  10,
				MaxScore:   2.5,
				TookMillis: 3,
				Hits: []*executor.SearchHit{
					{ID: "1", Score: 2.5, Source: map[string]interface{}{"status": "active", "name": "Product 1"}},
				},
			}, nil
		},
	}

	mockMaster := &mockMasterClient{}

	service := NewQueryService(mockExec, mockMaster, logger)

	queryJSON := `{"query": {"term": {"status": "active"}}, "size": 10}`
	result, err := service.ExecuteSearch(context.Background(), "products", []byte(queryJSON))

	require.NoError(t, err)
	assert.Equal(t, int64(10), result.TotalHits)
	assert.Equal(t, 2.5, result.MaxScore)
	assert.Len(t, result.Hits, 1)
	assert.Equal(t, "1", result.Hits[0].ID)
	assert.Equal(t, "active", result.Hits[0].Source["status"])
}

func TestExecuteSearchWithAggregations(t *testing.T) {
	logger := zap.NewNop()

	mockExec := &mockQueryExecutor{
		searchFunc: func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
			return &executor.SearchResult{
				TotalHits:  100,
				MaxScore:   1.0,
				TookMillis: 8,
				Hits:       []*executor.SearchHit{},
				Aggregations: map[string]*executor.AggregationResult{
					"categories": {
						Type: "terms",
						Buckets: []*executor.AggregationBucket{
							{Key: "electronics", DocCount: 50},
							{Key: "books", DocCount: 30},
						},
					},
					"avg_price": {
						Type: "avg",
						Avg:  45.5,
					},
				},
			}, nil
		},
	}

	mockMaster := &mockMasterClient{}

	service := NewQueryService(mockExec, mockMaster, logger)

	queryJSON := `{
		"query": {"match_all": {}},
		"aggs": {
			"categories": {"terms": {"field": "category"}},
			"avg_price": {"avg": {"field": "price"}}
		},
		"size": 0
	}`

	result, err := service.ExecuteSearch(context.Background(), "products", []byte(queryJSON))

	require.NoError(t, err)
	assert.Equal(t, int64(100), result.TotalHits)
	assert.Len(t, result.Aggregations, 2)

	// Check terms aggregation
	termsAgg, ok := result.Aggregations["categories"]
	require.True(t, ok)
	assert.Equal(t, "terms", termsAgg.Type)
	assert.Len(t, termsAgg.Buckets, 2)
	assert.Equal(t, "electronics", termsAgg.Buckets[0].Key)
	assert.Equal(t, int64(50), termsAgg.Buckets[0].DocCount)

	// Check avg aggregation
	avgAgg, ok := result.Aggregations["avg_price"]
	require.True(t, ok)
	assert.Equal(t, "avg", avgAgg.Type)
	assert.Equal(t, 45.5, avgAgg.Value)
}

func TestExecuteSearchInvalidQuery(t *testing.T) {
	logger := zap.NewNop()
	mockExec := &mockQueryExecutor{}
	mockMaster := &mockMasterClient{}

	service := NewQueryService(mockExec, mockMaster, logger)

	// Invalid JSON
	_, err := service.ExecuteSearch(context.Background(), "products", []byte(`{invalid json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestExecuteSearchMultipleShards(t *testing.T) {
	logger := zap.NewNop()

	mockExec := &mockQueryExecutor{
		searchFunc: func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
			return &executor.SearchResult{
				TotalHits:  1000,
				MaxScore:   3.0,
				TookMillis: 15,
				Hits: []*executor.SearchHit{
					{ID: "1", Score: 3.0, Source: map[string]interface{}{"title": "Top Result"}},
				},
			}, nil
		},
	}

	// Mock master with multiple shards
	mockMaster := &mockMasterClient{
		shardRouting: map[int32]*pb.ShardRouting{
			0: {ShardId: 0, IsPrimary: true, Allocation: &pb.ShardAllocation{NodeId: "node1", State: pb.ShardAllocation_SHARD_STATE_STARTED}},
			1: {ShardId: 1, IsPrimary: true, Allocation: &pb.ShardAllocation{NodeId: "node2", State: pb.ShardAllocation_SHARD_STATE_STARTED}},
			2: {ShardId: 2, IsPrimary: true, Allocation: &pb.ShardAllocation{NodeId: "node3", State: pb.ShardAllocation_SHARD_STATE_STARTED}},
		},
	}

	service := NewQueryService(mockExec, mockMaster, logger)

	queryJSON := `{"query": {"match_all": {}}, "size": 10}`
	result, err := service.ExecuteSearch(context.Background(), "products", []byte(queryJSON))

	require.NoError(t, err)
	assert.Equal(t, int64(1000), result.TotalHits)
	assert.Equal(t, 3, result.Shards.Total)
	assert.Equal(t, 3, result.Shards.Successful)
}

func TestConvertSearchResultToConvert(t *testing.T) {
	logger := zap.NewNop()
	mockExec := &mockQueryExecutor{}
	mockMaster := &mockMasterClient{}

	service := NewQueryService(mockExec, mockMaster, logger)

	// Create a test result
	execResult := &planner.ExecutionResult{
		TotalHits:  50,
		MaxScore:   2.5,
		TookMillis: 10,
		Rows: []map[string]interface{}{
			{"_id": "1", "_score": 2.5, "title": "Doc 1"},
			{"_id": "2", "_score": 2.0, "title": "Doc 2"},
		},
		Aggregations: map[string]*planner.AggregationResult{
			"categories": {
				Type: planner.AggregationType("terms"),
				Buckets: []*planner.Bucket{
					{Key: "cat1", DocCount: 30},
					{Key: "cat2", DocCount: 20},
				},
			},
		},
	}

	// Convert to search result
	result := service.convertToSearchResult(execResult, 10*time.Millisecond, 3)

	assert.Equal(t, int64(50), result.TotalHits)
	assert.Equal(t, 2.5, result.MaxScore)
	assert.Len(t, result.Hits, 2)
	assert.Equal(t, "1", result.Hits[0].ID)
	assert.Equal(t, "Doc 1", result.Hits[0].Source["title"])
	assert.Contains(t, result.Aggregations, "categories")
}
