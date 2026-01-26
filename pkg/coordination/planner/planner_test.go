package planner

import (
	"context"
	"testing"

	"github.com/quidditch/quidditch/pkg/coordination/parser"
	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Mock master client for testing
type mockMasterClient struct {
	metadata *pb.IndexMetadataResponse
	routing  map[int32]*pb.ShardRouting
	err      error
}

func (m *mockMasterClient) GetIndexMetadata(ctx context.Context, indexName string) (*pb.IndexMetadataResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.metadata, nil
}

func (m *mockMasterClient) GetShardRouting(ctx context.Context, indexName string) (map[int32]*pb.ShardRouting, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.routing, nil
}

func createTestPlanner() (*QueryPlanner, *mockMasterClient) {
	mockClient := &mockMasterClient{
		metadata: &pb.IndexMetadataResponse{
			Metadata: &pb.IndexMetadata{
				Settings: &pb.IndexSettings{
					NumberOfShards:   3,
					NumberOfReplicas: 1,
				},
			},
		},
		routing: map[int32]*pb.ShardRouting{
			0: {
				ShardId:   0,
				IsPrimary: true,
				Allocation: &pb.ShardAllocation{
					NodeId: "node-1",
					State:  pb.ShardAllocation_SHARD_STATE_STARTED,
				},
			},
			1: {
				ShardId:   1,
				IsPrimary: true,
				Allocation: &pb.ShardAllocation{
					NodeId: "node-2",
					State:  pb.ShardAllocation_SHARD_STATE_STARTED,
				},
			},
			2: {
				ShardId:   2,
				IsPrimary: true,
				Allocation: &pb.ShardAllocation{
					NodeId: "node-3",
					State:  pb.ShardAllocation_SHARD_STATE_STARTED,
				},
			},
		},
	}

	planner := NewQueryPlanner(mockClient, zap.NewNop())
	return planner, mockClient
}

func TestNewQueryPlanner(t *testing.T) {
	mockClient := &mockMasterClient{}
	planner := NewQueryPlanner(mockClient, zap.NewNop())

	assert.NotNil(t, planner)
	assert.NotNil(t, planner.cache)
	assert.NotNil(t, planner.logger)
	assert.NotNil(t, planner.masterClient)
}

func TestPlanQuery_MatchAll(t *testing.T) {
	planner, _ := createTestPlanner()

	searchReq := &parser.SearchRequest{
		Query: &parser.Query{
			Type: parser.QueryTypeMatchAll,
		},
		From: 0,
		Size: 10,
	}

	plan, err := planner.PlanQuery(context.Background(), "test-index", searchReq)
	require.NoError(t, err)
	require.NotNil(t, plan)

	assert.Equal(t, searchReq, plan.SearchRequest)
	assert.Equal(t, 1, plan.Complexity)
	assert.Equal(t, 3, len(plan.TargetShards))
	assert.False(t, plan.Cacheable) // match_all is not cacheable
	assert.NotNil(t, plan.Stats)
}

func TestPlanQuery_TermQuery(t *testing.T) {
	planner, _ := createTestPlanner()

	searchReq := &parser.SearchRequest{
		Query: &parser.Query{
			Type: parser.QueryTypeTerm,
			Term: &parser.TermQuery{
				Field: "status",
				Value: "active",
			},
		},
		From: 0,
		Size: 20,
	}

	plan, err := planner.PlanQuery(context.Background(), "test-index", searchReq)
	require.NoError(t, err)
	require.NotNil(t, plan)

	assert.Equal(t, 10, plan.Complexity)
	assert.True(t, plan.Cacheable)
	assert.Greater(t, plan.EstimatedCost, 0.0)
}

func TestPlanQuery_BoolQuery(t *testing.T) {
	planner, _ := createTestPlanner()

	searchReq := &parser.SearchRequest{
		Query: &parser.Query{
			Type: parser.QueryTypeBool,
			Bool: &parser.BoolQuery{
				Must: []*parser.Query{
					{Type: parser.QueryTypeTerm},
					{Type: parser.QueryTypeMatch},
				},
				Filter: []*parser.Query{
					{Type: parser.QueryTypeRange},
				},
			},
		},
		From: 0,
		Size: 10,
	}

	plan, err := planner.PlanQuery(context.Background(), "test-index", searchReq)
	require.NoError(t, err)
	require.NotNil(t, plan)

	assert.Greater(t, plan.Complexity, 20) // Should be sum of sub-queries
	assert.True(t, plan.Cacheable)
}

func TestAnalyzeComplexity_MatchAll(t *testing.T) {
	planner, _ := createTestPlanner()

	query := &parser.Query{Type: parser.QueryTypeMatchAll}
	complexity := planner.analyzeComplexity(query)

	assert.Equal(t, 1, complexity)
}

func TestAnalyzeComplexity_Term(t *testing.T) {
	planner, _ := createTestPlanner()

	query := &parser.Query{Type: parser.QueryTypeTerm}
	complexity := planner.analyzeComplexity(query)

	assert.Equal(t, 10, complexity)
}

func TestAnalyzeComplexity_Wildcard(t *testing.T) {
	planner, _ := createTestPlanner()

	query := &parser.Query{Type: parser.QueryTypeWildcard}
	complexity := planner.analyzeComplexity(query)

	assert.Equal(t, 30, complexity)
}

func TestAnalyzeComplexity_Fuzzy(t *testing.T) {
	planner, _ := createTestPlanner()

	query := &parser.Query{Type: parser.QueryTypeFuzzy}
	complexity := planner.analyzeComplexity(query)

	assert.Equal(t, 40, complexity)
}

func TestAnalyzeComplexity_Terms(t *testing.T) {
	planner, _ := createTestPlanner()

	query := &parser.Query{
		Type: parser.QueryTypeTerms,
		Terms: &parser.TermsQuery{
			Field:  "category",
			Values: []interface{}{"cat1", "cat2", "cat3"},
		},
	}
	complexity := planner.analyzeComplexity(query)

	assert.Equal(t, 13, complexity) // 10 + 3 values
}

func TestAnalyzeComplexity_NestedBool(t *testing.T) {
	planner, _ := createTestPlanner()

	query := &parser.Query{
		Type: parser.QueryTypeBool,
		Bool: &parser.BoolQuery{
			Must: []*parser.Query{
				{Type: parser.QueryTypeTerm},    // 10
				{Type: parser.QueryTypeMatch},   // 10
				{Type: parser.QueryTypeWildcard}, // 30
			},
			Should: []*parser.Query{
				{Type: parser.QueryTypeFuzzy}, // 40 / 2 = 20
			},
		},
	}

	complexity := planner.analyzeComplexity(query)
	assert.Greater(t, complexity, 60) // Should sum up nested queries
}

func TestOptimizeQuery_Nil(t *testing.T) {
	planner, _ := createTestPlanner()

	optimized := planner.optimizeQuery(nil)
	assert.Nil(t, optimized)
}

func TestOptimizeQuery_Simple(t *testing.T) {
	planner, _ := createTestPlanner()

	query := &parser.Query{Type: parser.QueryTypeTerm}
	optimized := planner.optimizeQuery(query)

	assert.NotNil(t, optimized)
	assert.Equal(t, parser.QueryTypeTerm, optimized.Type)
}

func TestOptimizeBoolQuery(t *testing.T) {
	planner, _ := createTestPlanner()

	boolQuery := &parser.BoolQuery{
		Must: []*parser.Query{
			{Type: parser.QueryTypeTerm},
		},
		Filter: []*parser.Query{
			{Type: parser.QueryTypeRange},
		},
		Should: []*parser.Query{
			{Type: parser.QueryTypeMatch},
		},
		MinimumShouldMatch: 1,
	}

	optimized := planner.optimizeBoolQuery(boolQuery)

	assert.NotNil(t, optimized)
	assert.Equal(t, 1, len(optimized.Must))
	assert.Equal(t, 1, len(optimized.Filter))
	assert.Equal(t, 1, len(optimized.Should))
	assert.Equal(t, 1, optimized.MinimumShouldMatch)
}

func TestSelectShards_AllStarted(t *testing.T) {
	planner, mockClient := createTestPlanner()

	searchReq := &parser.SearchRequest{
		From: 0,
		Size: 10,
	}

	shards := planner.selectShards(mockClient.routing, searchReq)

	assert.Equal(t, 3, len(shards))
	assert.Contains(t, shards, int32(0))
	assert.Contains(t, shards, int32(1))
	assert.Contains(t, shards, int32(2))
}

func TestSelectShards_SomeUnstarted(t *testing.T) {
	planner, mockClient := createTestPlanner()

	// Mark one shard as unstarted
	mockClient.routing[1].Allocation.State = pb.ShardAllocation_SHARD_STATE_INITIALIZING

	searchReq := &parser.SearchRequest{
		From: 0,
		Size: 10,
	}

	shards := planner.selectShards(mockClient.routing, searchReq)

	assert.Equal(t, 2, len(shards))
	assert.Contains(t, shards, int32(0))
	assert.Contains(t, shards, int32(2))
	assert.NotContains(t, shards, int32(1))
}

func TestEstimateCost(t *testing.T) {
	planner, mockClient := createTestPlanner()

	plan := &QueryPlan{
		SearchRequest: &parser.SearchRequest{
			Size: 50,
		},
		TargetShards: []int32{0, 1, 2},
		Complexity:   25,
	}

	cost := planner.estimateCost(plan, mockClient.metadata)

	assert.Greater(t, cost, 0.0)
	// Base: 3 shards * 10 = 30
	// Complexity: 25 * 5 = 125
	// Results: 50 * 0.1 = 5
	// Total: 160
	assert.Equal(t, 160.0, cost)
}

func TestIsCacheable(t *testing.T) {
	planner, _ := createTestPlanner()

	tests := []struct {
		name      string
		searchReq *parser.SearchRequest
		expected  bool
	}{
		{
			name: "simple term query - cacheable",
			searchReq: &parser.SearchRequest{
				Query: &parser.Query{Type: parser.QueryTypeTerm},
				From:  0,
				Size:  10,
			},
			expected: true,
		},
		{
			name: "match_all - not cacheable",
			searchReq: &parser.SearchRequest{
				Query: &parser.Query{Type: parser.QueryTypeMatchAll},
				From:  0,
				Size:  10,
			},
			expected: false,
		},
		{
			name: "size 0 - not cacheable",
			searchReq: &parser.SearchRequest{
				Query: &parser.Query{Type: parser.QueryTypeTerm},
				From:  0,
				Size:  0,
			},
			expected: false,
		},
		{
			name: "deep pagination - not cacheable",
			searchReq: &parser.SearchRequest{
				Query: &parser.Query{Type: parser.QueryTypeTerm},
				From:  2000,
				Size:  10,
			},
			expected: false,
		},
		{
			name: "nil query - not cacheable",
			searchReq: &parser.SearchRequest{
				Query: nil,
				From:  0,
				Size:  10,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := planner.isCacheable(tt.searchReq)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAnalyzeQuery_Wildcard(t *testing.T) {
	planner, _ := createTestPlanner()

	query := &parser.Query{Type: parser.QueryTypeWildcard}
	hints := planner.AnalyzeQuery(query)

	require.Len(t, hints, 1)
	assert.Equal(t, "expensive_query", hints[0].Type)
	assert.Equal(t, "warning", hints[0].Severity)
}

func TestAnalyzeQuery_Fuzzy(t *testing.T) {
	planner, _ := createTestPlanner()

	query := &parser.Query{Type: parser.QueryTypeFuzzy}
	hints := planner.AnalyzeQuery(query)

	require.Len(t, hints, 1)
	assert.Equal(t, "expensive_query", hints[0].Type)
	assert.Equal(t, "warning", hints[0].Severity)
}

func TestAnalyzeQuery_LargeTerms(t *testing.T) {
	planner, _ := createTestPlanner()

	values := make([]interface{}, 200)
	for i := 0; i < 200; i++ {
		values[i] = i
	}

	query := &parser.Query{
		Type: parser.QueryTypeTerms,
		Terms: &parser.TermsQuery{
			Field:  "id",
			Values: values,
		},
	}

	hints := planner.AnalyzeQuery(query)

	require.Len(t, hints, 1)
	assert.Equal(t, "large_terms_query", hints[0].Type)
	assert.Equal(t, "warning", hints[0].Severity)
}

func TestAnalyzeQuery_ComplexBool(t *testing.T) {
	planner, _ := createTestPlanner()

	should := make([]*parser.Query, 25)
	for i := 0; i < 25; i++ {
		should[i] = &parser.Query{Type: parser.QueryTypeTerm}
	}

	query := &parser.Query{
		Type: parser.QueryTypeBool,
		Bool: &parser.BoolQuery{
			Should: should,
		},
	}

	hints := planner.AnalyzeQuery(query)

	assert.Greater(t, len(hints), 0)
	found := false
	for _, hint := range hints {
		if hint.Type == "complex_bool_query" {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestAnalyzeQuery_FilterSuggestion(t *testing.T) {
	planner, _ := createTestPlanner()

	query := &parser.Query{
		Type: parser.QueryTypeBool,
		Bool: &parser.BoolQuery{
			Must: []*parser.Query{
				{Type: parser.QueryTypeTerm},
			},
		},
	}

	hints := planner.AnalyzeQuery(query)

	assert.Greater(t, len(hints), 0)
	found := false
	for _, hint := range hints {
		if hint.Type == "filter_suggestion" {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestQueryCache(t *testing.T) {
	planner, _ := createTestPlanner()

	// Test cache miss
	_, found := planner.GetCachedResults("key1")
	assert.False(t, found)

	// Test cache set and hit
	planner.CacheResults("key1", "value1")
	value, found := planner.GetCachedResults("key1")
	assert.True(t, found)
	assert.Equal(t, "value1", value)

	// Test cache clear
	planner.ClearCache()
	_, found = planner.GetCachedResults("key1")
	assert.False(t, found)
}
