// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package coordination

import (
	"context"
	"testing"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"github.com/quidditch/quidditch/pkg/coordination/executor"
	"github.com/quidditch/quidditch/pkg/coordination/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Mock query executor for pipeline testing
type mockPipelineQueryExecutor struct {
	executeFunc func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error)
}

func (m *mockPipelineQueryExecutor) ExecuteSearch(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, indexName, query, filterExpr, from, size)
	}
	return &executor.SearchResult{
		TotalHits: 3,
		MaxScore:  1.0,
		Hits: []*executor.SearchHit{
			{ID: "doc1", Score: 1.0, Source: map[string]interface{}{"title": "laptop"}},
			{ID: "doc2", Score: 0.9, Source: map[string]interface{}{"title": "notebook"}},
			{ID: "doc3", Score: 0.8, Source: map[string]interface{}{"title": "computer"}},
		},
	}, nil
}

// Mock master client for pipeline testing
type mockPipelineMasterClient struct {
	getShardRoutingFunc  func(ctx context.Context, indexName string) (map[int32]*pb.ShardRouting, error)
	getIndexMetadataFunc func(ctx context.Context, indexName string) (*pb.IndexMetadataResponse, error)
}

func (m *mockPipelineMasterClient) GetShardRouting(ctx context.Context, indexName string) (map[int32]*pb.ShardRouting, error) {
	if m.getShardRoutingFunc != nil {
		return m.getShardRoutingFunc(ctx, indexName)
	}
	return map[int32]*pb.ShardRouting{
		0: {
			Allocation: &pb.ShardAllocation{State: pb.ShardAllocation_SHARD_STATE_STARTED},
		},
	}, nil
}

func (m *mockPipelineMasterClient) GetIndexMetadata(ctx context.Context, indexName string) (*pb.IndexMetadataResponse, error) {
	if m.getIndexMetadataFunc != nil {
		return m.getIndexMetadataFunc(ctx, indexName)
	}
	return &pb.IndexMetadataResponse{}, nil
}

func setupQueryPipelineTest() (*QueryService, *pipeline.Registry, *pipeline.Executor) {
	logger := zap.NewNop()

	// Create mock executor and master client
	mockExec := &mockPipelineQueryExecutor{}
	mockMaster := &mockPipelineMasterClient{}

	// Create query service
	queryService := NewQueryService(mockExec, mockMaster, logger)

	// Create pipeline components
	pipelineRegistry := pipeline.NewRegistry(logger)
	pipelineExecutor := pipeline.NewExecutor(pipelineRegistry, logger)

	// Connect to query service
	queryService.SetPipelineComponents(pipelineRegistry, pipelineExecutor)

	return queryService, pipelineRegistry, pipelineExecutor
}

func TestQueryPipeline_NoConfigured(t *testing.T) {
	qs, _, _ := setupQueryPipelineTest()

	// Execute search without any pipeline configured
	requestBody := []byte(`{"query": {"match": {"title": "laptop"}}, "size": 10}`)

	result, err := qs.ExecuteSearch(context.Background(), "test-index", requestBody)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should return normal results
	assert.Equal(t, int64(3), result.TotalHits)
	assert.Len(t, result.Hits, 3)
	assert.Equal(t, "doc1", result.Hits[0].ID)
}

func TestQueryPipeline_QueryTransformation(t *testing.T) {
	qs, registry, _ := setupQueryPipelineTest()

	// Register a query pipeline that modifies the query
	queryPipelineDef := &pipeline.PipelineDefinition{
		Name:        "query-modifier",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeQuery,
		Description: "Modifies query",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "modifier",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(queryPipelineDef))

	// Get pipeline and set mock stage
	pipe, err := registry.Get("query-modifier")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	// Create mock stage that adds a boost parameter
	mockStage := &mockPipelineStage{
		name:      "modifier",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			inputMap := input.(map[string]interface{})

			// Add boost to query
			if query, ok := inputMap["query"].(map[string]interface{}); ok {
				if match, ok := query["match"].(map[string]interface{}); ok {
					if titleQuery, ok := match["title"].(map[string]interface{}); ok {
						titleQuery["boost"] = 2.0
					} else if titleStr, ok := match["title"].(string); ok {
						match["title"] = map[string]interface{}{
							"query": titleStr,
							"boost": 2.0,
						}
					}
				}
			}

			return inputMap, nil
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	// Associate pipeline with index
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeQuery, "query-modifier"))

	// Execute search
	requestBody := []byte(`{"query": {"match": {"title": "laptop"}}, "size": 10}`)

	result, err := qs.ExecuteSearch(context.Background(), "test-index", requestBody)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Query was modified by pipeline but results still work
	assert.Equal(t, int64(3), result.TotalHits)
	assert.Len(t, result.Hits, 3)
}

func TestQueryPipeline_FailureGracefulDegradation(t *testing.T) {
	qs, registry, _ := setupQueryPipelineTest()

	// Register a query pipeline that fails
	queryPipelineDef := &pipeline.PipelineDefinition{
		Name:        "failing-pipeline",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeQuery,
		Description: "Pipeline that fails",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "failing-stage",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(queryPipelineDef))

	// Get pipeline and set failing mock stage
	pipe, err := registry.Get("failing-pipeline")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	mockStage := &mockPipelineStage{
		name:      "failing-stage",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			return nil, assert.AnError
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	// Associate pipeline with index
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeQuery, "failing-pipeline"))

	// Execute search - should continue with original query despite pipeline failure
	requestBody := []byte(`{"query": {"match": {"title": "laptop"}}, "size": 10}`)

	result, err := qs.ExecuteSearch(context.Background(), "test-index", requestBody)
	require.NoError(t, err) // Pipeline failure doesn't fail the search
	require.NotNil(t, result)

	// Should return normal results (graceful degradation)
	assert.Equal(t, int64(3), result.TotalHits)
	assert.Len(t, result.Hits, 3)
}

func TestResultPipeline_NoConfigured(t *testing.T) {
	qs, _, _ := setupQueryPipelineTest()

	// Execute search without result pipeline
	requestBody := []byte(`{"query": {"match": {"title": "laptop"}}, "size": 10}`)

	result, err := qs.ExecuteSearch(context.Background(), "test-index", requestBody)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should return normal results
	assert.Equal(t, int64(3), result.TotalHits)
	assert.Len(t, result.Hits, 3)
}

func TestResultPipeline_ReRanking(t *testing.T) {
	qs, registry, _ := setupQueryPipelineTest()

	// Register a result pipeline that re-ranks results
	resultPipelineDef := &pipeline.PipelineDefinition{
		Name:        "reranker",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeResult,
		Description: "Re-ranks results",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "rerank",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(resultPipelineDef))

	// Get pipeline and set mock stage
	pipe, err := registry.Get("reranker")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	// Create mock stage that reverses hit order
	mockStage := &mockPipelineStage{
		name:      "rerank",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			inputMap := input.(map[string]interface{})

			// Reverse the hits array
			if results, ok := inputMap["results"].(map[string]interface{}); ok {
				if hits, ok := results["hits"].([]interface{}); ok {
					// Reverse order
					for i, j := 0, len(hits)-1; i < j; i, j = i+1, j-1 {
						hits[i], hits[j] = hits[j], hits[i]
					}
				}
			}

			return inputMap, nil
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	// Associate pipeline with index
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeResult, "reranker"))

	// Execute search
	requestBody := []byte(`{"query": {"match": {"title": "laptop"}}, "size": 10}`)

	result, err := qs.ExecuteSearch(context.Background(), "test-index", requestBody)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Results should be reversed by pipeline
	assert.Equal(t, int64(3), result.TotalHits)
	assert.Len(t, result.Hits, 3)
	assert.Equal(t, "doc3", result.Hits[0].ID) // Was last, now first
	assert.Equal(t, "doc1", result.Hits[2].ID) // Was first, now last
}

func TestResultPipeline_ScoreModification(t *testing.T) {
	qs, registry, _ := setupQueryPipelineTest()

	// Register a result pipeline that modifies scores
	resultPipelineDef := &pipeline.PipelineDefinition{
		Name:        "score-booster",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeResult,
		Description: "Boosts scores",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "boost",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(resultPipelineDef))

	// Get pipeline and set mock stage
	pipe, err := registry.Get("score-booster")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	// Create mock stage that doubles all scores
	mockStage := &mockPipelineStage{
		name:      "boost",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			inputMap := input.(map[string]interface{})

			// Double all scores
			if results, ok := inputMap["results"].(map[string]interface{}); ok {
				if hits, ok := results["hits"].([]interface{}); ok {
					for _, hit := range hits {
						if hitMap, ok := hit.(map[string]interface{}); ok {
							if score, ok := hitMap["_score"].(float64); ok {
								hitMap["_score"] = score * 2.0
							}
						}
					}
				}
			}

			return inputMap, nil
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	// Associate pipeline with index
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeResult, "score-booster"))

	// Execute search
	requestBody := []byte(`{"query": {"match": {"title": "laptop"}}, "size": 10}`)

	result, err := qs.ExecuteSearch(context.Background(), "test-index", requestBody)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Scores should be doubled
	assert.Equal(t, int64(3), result.TotalHits)
	assert.Len(t, result.Hits, 3)
	assert.Equal(t, 2.0, result.Hits[0].Score) // Was 1.0, now 2.0
	assert.Equal(t, 1.8, result.Hits[1].Score) // Was 0.9, now 1.8
	assert.Equal(t, 1.6, result.Hits[2].Score) // Was 0.8, now 1.6
}

func TestResultPipeline_FailureGracefulDegradation(t *testing.T) {
	qs, registry, _ := setupQueryPipelineTest()

	// Register a result pipeline that fails
	resultPipelineDef := &pipeline.PipelineDefinition{
		Name:        "failing-result-pipeline",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeResult,
		Description: "Pipeline that fails",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "failing-stage",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(resultPipelineDef))

	// Get pipeline and set failing mock stage
	pipe, err := registry.Get("failing-result-pipeline")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	mockStage := &mockPipelineStage{
		name:      "failing-stage",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			return nil, assert.AnError
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	// Associate pipeline with index
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeResult, "failing-result-pipeline"))

	// Execute search - should continue with original results despite pipeline failure
	requestBody := []byte(`{"query": {"match": {"title": "laptop"}}, "size": 10}`)

	result, err := qs.ExecuteSearch(context.Background(), "test-index", requestBody)
	require.NoError(t, err) // Pipeline failure doesn't fail the search
	require.NotNil(t, result)

	// Should return normal results (graceful degradation)
	assert.Equal(t, int64(3), result.TotalHits)
	assert.Len(t, result.Hits, 3)
	assert.Equal(t, "doc1", result.Hits[0].ID)
}

func TestBothPipelines_QueryAndResult(t *testing.T) {
	qs, registry, _ := setupQueryPipelineTest()

	// Register both query and result pipelines
	queryPipelineDef := &pipeline.PipelineDefinition{
		Name:        "query-pipeline",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeQuery,
		Description: "Query pipeline",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "query-stage",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(queryPipelineDef))

	resultPipelineDef := &pipeline.PipelineDefinition{
		Name:        "result-pipeline",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeResult,
		Description: "Result pipeline",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "result-stage",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(resultPipelineDef))

	// Set up mock stages
	queryPipe, _ := registry.Get("query-pipeline")
	queryImpl := queryPipe.(*pipeline.PipelineImpl)
	queryImpl.SetStages([]pipeline.Stage{
		&mockPipelineStage{
			name:      "query-stage",
			stageType: pipeline.StageTypeNative,
			executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
				// Query pipeline: just pass through
				return input, nil
			},
		},
	})

	resultPipe, _ := registry.Get("result-pipeline")
	resultImpl := resultPipe.(*pipeline.PipelineImpl)
	resultImpl.SetStages([]pipeline.Stage{
		&mockPipelineStage{
			name:      "result-stage",
			stageType: pipeline.StageTypeNative,
			executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
				// Result pipeline: just pass through
				return input, nil
			},
		},
	})

	// Associate both pipelines with index
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeQuery, "query-pipeline"))
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeResult, "result-pipeline"))

	// Execute search
	requestBody := []byte(`{"query": {"match": {"title": "laptop"}}, "size": 10}`)

	result, err := qs.ExecuteSearch(context.Background(), "test-index", requestBody)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Both pipelines should have executed
	assert.Equal(t, int64(3), result.TotalHits)
	assert.Len(t, result.Hits, 3)
}

// mockPipelineStage for testing
type mockPipelineStage struct {
	name        string
	stageType   pipeline.StageType
	executeFunc func(ctx *pipeline.StageContext, input interface{}) (interface{}, error)
}

func (m *mockPipelineStage) Name() string {
	return m.name
}

func (m *mockPipelineStage) Type() pipeline.StageType {
	return m.stageType
}

func (m *mockPipelineStage) Execute(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	return input, nil
}

func (m *mockPipelineStage) Config() map[string]interface{} {
	return make(map[string]interface{})
}
