// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package coordination

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"github.com/quidditch/quidditch/pkg/coordination/pipeline"
	"github.com/quidditch/quidditch/pkg/coordination/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Mock document router for pipeline testing
type mockDocumentRouter struct {
	indexDocumentFunc func(ctx context.Context, indexName string, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error)
}

func (m *mockDocumentRouter) RouteIndexDocument(ctx context.Context, indexName string, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error) {
	if m.indexDocumentFunc != nil {
		return m.indexDocumentFunc(ctx, indexName, docID, document)
	}
	return &pb.IndexDocumentResponse{
		Acknowledged: true,
		Version:      1,
	}, nil
}

func (m *mockDocumentRouter) RouteGetDocument(ctx context.Context, indexName string, docID string) (*pb.GetDocumentResponse, error) {
	return nil, nil
}

func (m *mockDocumentRouter) RouteDeleteDocument(ctx context.Context, indexName string, docID string) (*pb.DeleteDocumentResponse, error) {
	return nil, nil
}

func (m *mockDocumentRouter) RouteSearchDocuments(ctx context.Context, indexName string, query []byte, from, size int) ([]*pb.SearchHit, error) {
	return nil, nil
}

func (m *mockDocumentRouter) AddDataNode(nodeID string, client router.DataNodeClient) error {
	return nil
}

// Mock master client for document pipeline testing
type mockDocPipelineMasterClient struct {
	getIndexMetadataFunc func(ctx context.Context, indexName string) (*pb.IndexMetadataResponse, error)
}

func (m *mockDocPipelineMasterClient) GetShardRouting(ctx context.Context, indexName string) (map[int32]*pb.ShardRouting, error) {
	return map[int32]*pb.ShardRouting{
		0: {
			Allocation: &pb.ShardAllocation{State: pb.ShardAllocation_SHARD_STATE_STARTED},
		},
	}, nil
}

func (m *mockDocPipelineMasterClient) GetIndexMetadata(ctx context.Context, indexName string) (*pb.IndexMetadataResponse, error) {
	if m.getIndexMetadataFunc != nil {
		return m.getIndexMetadataFunc(ctx, indexName)
	}
	return &pb.IndexMetadataResponse{}, nil
}

type testCoordinationNode struct {
	*CoordinationNode
	mockDocRouter *mockDocumentRouter
}

// Override handleIndexDocument to use mockDocRouter
func (tc *testCoordinationNode) handleIndexDocument(ctx *gin.Context) {
	tc.logger.Info("==> handleIndexDocument ENTRY POINT")
	indexName := ctx.Param("index")
	docID := ctx.Param("id")

	tc.logger.Info("handleIndexDocument called",
		zap.String("index", indexName),
		zap.String("doc_id", docID))

	// Parse document from request body
	var document map[string]interface{}
	if err := ctx.ShouldBindJSON(&document); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":   "parse_exception",
				"reason": fmt.Sprintf("Failed to parse document: %v", err),
			},
		})
		return
	}

	// Execute document pipeline if configured
	if tc.pipelineRegistry != nil && tc.pipelineExecutor != nil {
		modifiedDoc, err := tc.executeDocumentPipeline(ctx.Request.Context(), indexName, docID, document)
		if err != nil {
			tc.logger.Warn("Document pipeline failed, continuing with original document",
				zap.String("index", indexName),
				zap.String("doc_id", docID),
				zap.Error(err))
		} else if modifiedDoc != nil {
			document = modifiedDoc
		}
	}

	tc.logger.Debug("About to call RouteIndexDocument",
		zap.String("index", indexName),
		zap.String("doc_id", docID))

	// Route to mock document router
	resp, err := tc.mockDocRouter.RouteIndexDocument(ctx.Request.Context(), indexName, docID, document)
	if err != nil {
		tc.logger.Error("Failed to index document",
			zap.String("index", indexName),
			zap.String("doc_id", docID),
			zap.Error(err))

		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":   "index_failed_exception",
				"reason": fmt.Sprintf("Failed to index document: %v", err),
			},
		})
		return
	}

	// Return success response
	result := "created"
	statusCode := http.StatusCreated
	if resp.Version > 1 {
		result = "updated"
		statusCode = http.StatusOK
	}

	ctx.JSON(statusCode, gin.H{
		"_index":   indexName,
		"_id":      docID,
		"_version": resp.Version,
		"result":   result,
	})
}

func setupDocumentPipelineTestNode() (*testCoordinationNode, *pipeline.Registry, *pipeline.Executor) {
	logger := zap.NewNop()

	// Create pipeline components
	pipelineRegistry := pipeline.NewRegistry(logger)
	pipelineExecutor := pipeline.NewExecutor(pipelineRegistry, logger)

	// Create mock document router
	mockDocRouter := &mockDocumentRouter{}

	// Create coordination node with minimal setup
	node := &CoordinationNode{
		logger:           logger,
		ginRouter:        gin.New(),
		pipelineRegistry: pipelineRegistry,
		pipelineExecutor: pipelineExecutor,
	}

	testNode := &testCoordinationNode{
		CoordinationNode: node,
		mockDocRouter:    mockDocRouter,
	}

	return testNode, pipelineRegistry, pipelineExecutor
}

func TestDocumentPipeline_NoConfigured(t *testing.T) {
	testNode, _, _ := setupDocumentPipelineTestNode()

	// Set up HTTP test
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/:index/_doc/:id", testNode.handleIndexDocument)

	// Index document without pipeline
	doc := map[string]interface{}{
		"title":       "Test Document",
		"description": "This is a test",
		"count":       42,
	}
	body, _ := json.Marshal(doc)

	req := httptest.NewRequest(http.MethodPut, "/test-index/_doc/doc1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "test-index", response["_index"])
	assert.Equal(t, "doc1", response["_id"])
	assert.Equal(t, "created", response["result"])
}

func TestDocumentPipeline_FieldTransformation(t *testing.T) {
	testNode, registry, _ := setupDocumentPipelineTestNode()

	// Register a document pipeline that transforms fields
	docPipelineDef := &pipeline.PipelineDefinition{
		Name:        "field-transformer",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeDocument,
		Description: "Transforms document fields",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "uppercase",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(docPipelineDef))

	// Get pipeline and set mock stage
	pipe, err := registry.Get("field-transformer")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	// Track transformed document
	var transformedDoc map[string]interface{}

	// Create mock stage that uppercases title field
	mockStage := &mockDocPipelineStage{
		name:      "uppercase",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			inputMap := input.(map[string]interface{})

			// Transform title to uppercase
			if doc, ok := inputMap["document"].(map[string]interface{}); ok {
				if title, ok := doc["title"].(string); ok {
					doc["title"] = "UPPERCASE: " + title
				}
			}

			return inputMap, nil
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	// Associate pipeline with index
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeDocument, "field-transformer"))

	// Mock document router to capture transformed document
	testNode.mockDocRouter.indexDocumentFunc = func(ctx context.Context, indexName string, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error) {
		transformedDoc = document
		return &pb.IndexDocumentResponse{
			Version: 1,
			Acknowledged: true,
		}, nil
	}

	// Set up HTTP test
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/:index/_doc/:id", testNode.handleIndexDocument)

	// Index document
	doc := map[string]interface{}{
		"title":       "hello world",
		"description": "test document",
	}
	body, _ := json.Marshal(doc)

	req := httptest.NewRequest(http.MethodPut, "/test-index/_doc/doc1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify document was transformed
	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, transformedDoc)
	assert.Equal(t, "UPPERCASE: hello world", transformedDoc["title"])
	assert.Equal(t, "test document", transformedDoc["description"])
}

func TestDocumentPipeline_FieldEnrichment(t *testing.T) {
	testNode, registry, _ := setupDocumentPipelineTestNode()

	// Register a document pipeline that enriches documents
	docPipelineDef := &pipeline.PipelineDefinition{
		Name:        "field-enricher",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeDocument,
		Description: "Enriches documents with computed fields",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "enrich",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(docPipelineDef))

	// Get pipeline and set mock stage
	pipe, err := registry.Get("field-enricher")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	var transformedDoc map[string]interface{}

	// Create mock stage that adds computed fields
	mockStage := &mockDocPipelineStage{
		name:      "enrich",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			inputMap := input.(map[string]interface{})

			// Add computed fields
			if doc, ok := inputMap["document"].(map[string]interface{}); ok {
				doc["_enriched"] = true
				doc["_timestamp"] = "2026-01-27T20:00:00Z"
				if price, ok := doc["price"].(float64); ok {
					doc["price_with_tax"] = price * 1.2
				}
			}

			return inputMap, nil
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	// Associate pipeline with index
	require.NoError(t, registry.AssociatePipeline("products", pipeline.PipelineTypeDocument, "field-enricher"))

	// Mock document router to capture transformed document
	testNode.mockDocRouter.indexDocumentFunc = func(ctx context.Context, indexName string, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error) {
		transformedDoc = document
		return &pb.IndexDocumentResponse{
			Version: 1,
			Acknowledged: true,
		}, nil
	}

	// Set up HTTP test
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/:index/_doc/:id", testNode.handleIndexDocument)

	// Index document
	doc := map[string]interface{}{
		"title": "Laptop",
		"price": 1000.0,
	}
	body, _ := json.Marshal(doc)

	req := httptest.NewRequest(http.MethodPut, "/products/_doc/prod1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify document was enriched
	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, transformedDoc)
	assert.Equal(t, true, transformedDoc["_enriched"])
	assert.Equal(t, "2026-01-27T20:00:00Z", transformedDoc["_timestamp"])
	assert.Equal(t, 1200.0, transformedDoc["price_with_tax"])
}

func TestDocumentPipeline_FieldFiltering(t *testing.T) {
	testNode, registry, _ := setupDocumentPipelineTestNode()

	// Register a document pipeline that filters sensitive fields
	docPipelineDef := &pipeline.PipelineDefinition{
		Name:        "field-filter",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeDocument,
		Description: "Filters sensitive fields",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "filter",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(docPipelineDef))

	// Get pipeline and set mock stage
	pipe, err := registry.Get("field-filter")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	var transformedDoc map[string]interface{}

	// Create mock stage that removes sensitive fields
	mockStage := &mockDocPipelineStage{
		name:      "filter",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			inputMap := input.(map[string]interface{})

			// Remove sensitive fields
			if doc, ok := inputMap["document"].(map[string]interface{}); ok {
				delete(doc, "password")
				delete(doc, "ssn")
				delete(doc, "credit_card")
			}

			return inputMap, nil
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	// Associate pipeline with index
	require.NoError(t, registry.AssociatePipeline("users", pipeline.PipelineTypeDocument, "field-filter"))

	// Mock document router to capture transformed document
	testNode.mockDocRouter.indexDocumentFunc = func(ctx context.Context, indexName string, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error) {
		transformedDoc = document
		return &pb.IndexDocumentResponse{
			Version: 1,
			Acknowledged: true,
		}, nil
	}

	// Set up HTTP test
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/:index/_doc/:id", testNode.handleIndexDocument)

	// Index document with sensitive fields
	doc := map[string]interface{}{
		"username":    "john_doe",
		"email":       "john@example.com",
		"password":    "secret123",
		"ssn":         "123-45-6789",
		"credit_card": "4111-1111-1111-1111",
	}
	body, _ := json.Marshal(doc)

	req := httptest.NewRequest(http.MethodPut, "/users/_doc/user1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify sensitive fields were removed
	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, transformedDoc)
	assert.Equal(t, "john_doe", transformedDoc["username"])
	assert.Equal(t, "john@example.com", transformedDoc["email"])
	assert.NotContains(t, transformedDoc, "password")
	assert.NotContains(t, transformedDoc, "ssn")
	assert.NotContains(t, transformedDoc, "credit_card")
}

func TestDocumentPipeline_MultipleStages(t *testing.T) {
	testNode, registry, _ := setupDocumentPipelineTestNode()

	// Register a document pipeline with multiple stages
	docPipelineDef := &pipeline.PipelineDefinition{
		Name:        "multi-stage",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeDocument,
		Description: "Multiple transformation stages",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "stage1",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
			{
				Name:    "stage2",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(docPipelineDef))

	// Get pipeline and set mock stages
	pipe, err := registry.Get("multi-stage")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	var transformedDoc map[string]interface{}

	// Stage 1: Add prefix
	stage1 := &mockDocPipelineStage{
		name:      "stage1",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			inputMap := input.(map[string]interface{})
			if doc, ok := inputMap["document"].(map[string]interface{}); ok {
				if title, ok := doc["title"].(string); ok {
					doc["title"] = "PREFIX:" + title
				}
			}
			return inputMap, nil
		},
	}

	// Stage 2: Add suffix
	stage2 := &mockDocPipelineStage{
		name:      "stage2",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			inputMap := input.(map[string]interface{})
			if doc, ok := inputMap["document"].(map[string]interface{}); ok {
				if title, ok := doc["title"].(string); ok {
					doc["title"] = title + ":SUFFIX"
				}
			}
			return inputMap, nil
		},
	}

	impl.SetStages([]pipeline.Stage{stage1, stage2})

	// Associate pipeline with index
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeDocument, "multi-stage"))

	// Mock document router to capture transformed document
	testNode.mockDocRouter.indexDocumentFunc = func(ctx context.Context, indexName string, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error) {
		transformedDoc = document
		return &pb.IndexDocumentResponse{
			Version: 1,
			Acknowledged: true,
		}, nil
	}

	// Set up HTTP test
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/:index/_doc/:id", testNode.handleIndexDocument)

	// Index document
	doc := map[string]interface{}{
		"title": "Test",
	}
	body, _ := json.Marshal(doc)

	req := httptest.NewRequest(http.MethodPut, "/test-index/_doc/doc1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify both stages executed
	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, transformedDoc)
	assert.Equal(t, "PREFIX:Test:SUFFIX", transformedDoc["title"])
}

func TestDocumentPipeline_FailureGracefulDegradation(t *testing.T) {
	testNode, registry, _ := setupDocumentPipelineTestNode()

	// Register a document pipeline that fails
	docPipelineDef := &pipeline.PipelineDefinition{
		Name:        "failing-pipeline",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeDocument,
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
	require.NoError(t, registry.Register(docPipelineDef))

	// Get pipeline and set failing mock stage
	pipe, err := registry.Get("failing-pipeline")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	var indexedDoc map[string]interface{}

	mockStage := &mockDocPipelineStage{
		name:      "failing-stage",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			return nil, fmt.Errorf("simulated pipeline failure")
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	// Associate pipeline with index
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeDocument, "failing-pipeline"))

	// Mock document router to capture indexed document
	testNode.mockDocRouter.indexDocumentFunc = func(ctx context.Context, indexName string, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error) {
		indexedDoc = document
		return &pb.IndexDocumentResponse{
			Version: 1,
			Acknowledged: true,
		}, nil
	}

	// Set up HTTP test
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/:index/_doc/:id", testNode.handleIndexDocument)

	// Index document
	doc := map[string]interface{}{
		"title": "Original Document",
	}
	body, _ := json.Marshal(doc)

	req := httptest.NewRequest(http.MethodPut, "/test-index/_doc/doc1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify indexing succeeded with original document (graceful degradation)
	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, indexedDoc)
	assert.Equal(t, "Original Document", indexedDoc["title"])
}

func TestDocumentPipeline_ValidationPipeline(t *testing.T) {
	testNode, registry, _ := setupDocumentPipelineTestNode()

	// Register a validation pipeline
	docPipelineDef := &pipeline.PipelineDefinition{
		Name:        "validator",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeDocument,
		Description: "Validates required fields",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "validate",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(docPipelineDef))

	// Get pipeline and set mock stage
	pipe, err := registry.Get("validator")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	// Create mock stage that validates required fields
	mockStage := &mockDocPipelineStage{
		name:      "validate",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			inputMap := input.(map[string]interface{})

			// Validate required fields
			if doc, ok := inputMap["document"].(map[string]interface{}); ok {
				if _, ok := doc["title"]; !ok {
					return nil, fmt.Errorf("missing required field: title")
				}
				if _, ok := doc["author"]; !ok {
					return nil, fmt.Errorf("missing required field: author")
				}
				// Add validation marker
				doc["_validated"] = true
			}

			return inputMap, nil
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	// Associate pipeline with index
	require.NoError(t, registry.AssociatePipeline("articles", pipeline.PipelineTypeDocument, "validator"))

	// Mock document router
	var transformedDoc map[string]interface{}
	testNode.mockDocRouter.indexDocumentFunc = func(ctx context.Context, indexName string, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error) {
		transformedDoc = document
		return &pb.IndexDocumentResponse{
			Version: 1,
			Acknowledged: true,
		}, nil
	}

	// Set up HTTP test
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/:index/_doc/:id", testNode.handleIndexDocument)

	// Test 1: Valid document
	doc := map[string]interface{}{
		"title":  "Test Article",
		"author": "John Doe",
	}
	body, _ := json.Marshal(doc)

	req := httptest.NewRequest(http.MethodPut, "/articles/_doc/art1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify document was validated and indexed
	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, transformedDoc)
	assert.Equal(t, true, transformedDoc["_validated"])

	// Test 2: Invalid document (missing author)
	transformedDoc = nil
	doc = map[string]interface{}{
		"title": "Test Article",
	}
	body, _ = json.Marshal(doc)

	req = httptest.NewRequest(http.MethodPut, "/articles/_doc/art2", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify indexing still succeeded (graceful degradation)
	// but document doesn't have validation marker
	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, transformedDoc)
	assert.NotContains(t, transformedDoc, "_validated")
}

func TestDocumentPipeline_BothQueryAndDocumentPipelines(t *testing.T) {
	// This test verifies that both query and document pipelines can coexist
	testNode, registry, _ := setupDocumentPipelineTestNode()

	// Register both document and query pipelines
	docPipelineDef := &pipeline.PipelineDefinition{
		Name:        "doc-pipeline",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeDocument,
		Description: "Document pipeline",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "doc-stage",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(docPipelineDef))

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

	// Get document pipeline and set mock stage
	docPipe, err := registry.Get("doc-pipeline")
	require.NoError(t, err)
	docImpl := docPipe.(*pipeline.PipelineImpl)

	var transformedDoc map[string]interface{}

	mockDocStage := &mockDocPipelineStage{
		name:      "doc-stage",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			inputMap := input.(map[string]interface{})
			if doc, ok := inputMap["document"].(map[string]interface{}); ok {
				doc["_doc_pipeline_executed"] = true
			}
			return inputMap, nil
		},
	}
	docImpl.SetStages([]pipeline.Stage{mockDocStage})

	// Associate both pipelines with index
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeDocument, "doc-pipeline"))
	require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeQuery, "query-pipeline"))

	// Mock document router to capture transformed document
	testNode.mockDocRouter.indexDocumentFunc = func(ctx context.Context, indexName string, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error) {
		transformedDoc = document
		return &pb.IndexDocumentResponse{
			Version: 1,
			Acknowledged: true,
		}, nil
	}

	// Set up HTTP test
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/:index/_doc/:id", testNode.handleIndexDocument)

	// Index document
	doc := map[string]interface{}{
		"title": "Test",
	}
	body, _ := json.Marshal(doc)

	req := httptest.NewRequest(http.MethodPut, "/test-index/_doc/doc1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify document pipeline executed
	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, transformedDoc)
	assert.Equal(t, true, transformedDoc["_doc_pipeline_executed"])
}

// mockDocPipelineStage for testing
type mockDocPipelineStage struct {
	name        string
	stageType   pipeline.StageType
	executeFunc func(ctx *pipeline.StageContext, input interface{}) (interface{}, error)
}

func (m *mockDocPipelineStage) Name() string {
	return m.name
}

func (m *mockDocPipelineStage) Type() pipeline.StageType {
	return m.stageType
}

func (m *mockDocPipelineStage) Execute(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	return input, nil
}

func (m *mockDocPipelineStage) Config() map[string]interface{} {
	return make(map[string]interface{})
}
