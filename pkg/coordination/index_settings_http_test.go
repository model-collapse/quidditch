// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package coordination

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/quidditch/quidditch/pkg/coordination/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupIndexSettingsHTTPTestRouter() (*gin.Engine, *pipeline.Registry) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := zap.NewNop()

	// Create pipeline registry and executor
	registry := pipeline.NewRegistry(logger)
	executor := pipeline.NewExecutor(registry, logger)

	// Register test pipelines
	queryPipelineDef := &pipeline.PipelineDefinition{
		Name:        "test-query-pipeline",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeQuery,
		Description: "Test query pipeline",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "stage1",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(nil, registry.Register(queryPipelineDef))

	documentPipelineDef := &pipeline.PipelineDefinition{
		Name:        "test-document-pipeline",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeDocument,
		Description: "Test document pipeline",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "stage1",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(nil, registry.Register(documentPipelineDef))

	resultPipelineDef := &pipeline.PipelineDefinition{
		Name:        "test-result-pipeline",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeResult,
		Description: "Test result pipeline",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "stage1",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(nil, registry.Register(resultPipelineDef))

	// Create mock coordination node components
	node := &CoordinationNode{
		logger:           logger,
		ginRouter:        router,
		pipelineRegistry: registry,
		pipelineExecutor: executor,
	}

	// Register only the settings routes we're testing
	router.GET(":index/_settings", node.handleGetSettings)
	router.PUT(":index/_settings", node.handlePutSettings)

	return router, registry
}

func TestIndexSettingsHTTP_GetSettings_NoPipelines(t *testing.T) {
	router, _ := setupIndexSettingsHTTPTestRouter()

	// Test GET with no pipelines associated
	req := httptest.NewRequest(http.MethodGet, "/test-index/_settings", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify basic settings are returned
	indexData := response["test-index"].(map[string]interface{})
	settings := indexData["settings"].(map[string]interface{})
	indexSettings := settings["index"].(map[string]interface{})

	assert.Equal(t, "1", indexSettings["number_of_shards"])
	assert.Equal(t, "0", indexSettings["number_of_replicas"])

	// Verify no pipeline settings
	_, hasQuery := indexSettings["query"]
	_, hasDocument := indexSettings["document"]
	_, hasResult := indexSettings["result"]

	assert.False(t, hasQuery, "Should not have query pipeline settings")
	assert.False(t, hasDocument, "Should not have document pipeline settings")
	assert.False(t, hasResult, "Should not have result pipeline settings")
}

func TestIndexSettingsHTTP_PutSettings_QueryPipeline(t *testing.T) {
	router, registry := setupIndexSettingsHTTPTestRouter()
	indexName := "test-index"

	// Create request to set query pipeline
	reqBody := map[string]interface{}{
		"index": map[string]interface{}{
			"query": map[string]interface{}{
				"default_pipeline": "test-query-pipeline",
			},
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/"+indexName+"/_settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, true, response["acknowledged"])

	// Verify association in registry
	pipe, err := registry.GetPipelineForIndex(indexName, pipeline.PipelineTypeQuery)
	require.NoError(t, err)
	assert.Equal(t, "test-query-pipeline", pipe.Name())
}

func TestIndexSettingsHTTP_PutSettings_AllPipelineTypes(t *testing.T) {
	router, registry := setupIndexSettingsHTTPTestRouter()
	indexName := "test-index"

	// Create request to set all three pipeline types
	reqBody := map[string]interface{}{
		"index": map[string]interface{}{
			"query": map[string]interface{}{
				"default_pipeline": "test-query-pipeline",
			},
			"document": map[string]interface{}{
				"default_pipeline": "test-document-pipeline",
			},
			"result": map[string]interface{}{
				"default_pipeline": "test-result-pipeline",
			},
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/"+indexName+"/_settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify all associations in registry
	queryPipe, err := registry.GetPipelineForIndex(indexName, pipeline.PipelineTypeQuery)
	require.NoError(t, err)
	assert.Equal(t, "test-query-pipeline", queryPipe.Name())

	docPipe, err := registry.GetPipelineForIndex(indexName, pipeline.PipelineTypeDocument)
	require.NoError(t, err)
	assert.Equal(t, "test-document-pipeline", docPipe.Name())

	resultPipe, err := registry.GetPipelineForIndex(indexName, pipeline.PipelineTypeResult)
	require.NoError(t, err)
	assert.Equal(t, "test-result-pipeline", resultPipe.Name())
}

func TestIndexSettingsHTTP_GetSettings_WithPipelines(t *testing.T) {
	router, registry := setupIndexSettingsHTTPTestRouter()
	indexName := "test-index"

	// Associate pipelines directly via registry
	require.NoError(t, registry.AssociatePipeline(indexName, pipeline.PipelineTypeQuery, "test-query-pipeline"))
	require.NoError(t, registry.AssociatePipeline(indexName, pipeline.PipelineTypeDocument, "test-document-pipeline"))
	require.NoError(t, registry.AssociatePipeline(indexName, pipeline.PipelineTypeResult, "test-result-pipeline"))

	// Get settings via HTTP
	req := httptest.NewRequest(http.MethodGet, "/"+indexName+"/_settings", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Navigate to index settings
	indexData := response[indexName].(map[string]interface{})
	settings := indexData["settings"].(map[string]interface{})
	indexSettings := settings["index"].(map[string]interface{})

	// Verify pipeline settings are returned
	querySettings := indexSettings["query"].(map[string]interface{})
	assert.Equal(t, "test-query-pipeline", querySettings["default_pipeline"])

	documentSettings := indexSettings["document"].(map[string]interface{})
	assert.Equal(t, "test-document-pipeline", documentSettings["default_pipeline"])

	resultSettings := indexSettings["result"].(map[string]interface{})
	assert.Equal(t, "test-result-pipeline", resultSettings["default_pipeline"])
}

func TestIndexSettingsHTTP_PutSettings_NonExistentPipeline(t *testing.T) {
	router, _ := setupIndexSettingsHTTPTestRouter()
	indexName := "test-index"

	// Try to set non-existent pipeline
	reqBody := map[string]interface{}{
		"index": map[string]interface{}{
			"query": map[string]interface{}{
				"default_pipeline": "non-existent-pipeline",
			},
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/"+indexName+"/_settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	errorData := response["error"].(map[string]interface{})
	assert.Equal(t, "pipeline_association_exception", errorData["type"])
	assert.Contains(t, errorData["reason"], "not found")
}

func TestIndexSettingsHTTP_PutSettings_WrongPipelineType(t *testing.T) {
	router, _ := setupIndexSettingsHTTPTestRouter()
	indexName := "test-index"

	// Try to associate query pipeline as document pipeline
	reqBody := map[string]interface{}{
		"index": map[string]interface{}{
			"document": map[string]interface{}{
				"default_pipeline": "test-query-pipeline", // Wrong type!
			},
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/"+indexName+"/_settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	errorData := response["error"].(map[string]interface{})
	assert.Equal(t, "pipeline_association_exception", errorData["type"])
	assert.Contains(t, errorData["reason"], "is type 'query'")
	assert.Contains(t, errorData["reason"], "cannot associate with type 'document'")
}

func TestIndexSettingsHTTP_PutSettings_InvalidJSON(t *testing.T) {
	router, _ := setupIndexSettingsHTTPTestRouter()
	indexName := "test-index"

	// Send invalid JSON
	req := httptest.NewRequest(http.MethodPut, "/"+indexName+"/_settings",
		bytes.NewBuffer([]byte("{invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	errorData := response["error"].(map[string]interface{})
	assert.Equal(t, "parsing_exception", errorData["type"])
}

func TestIndexSettingsHTTP_PutSettings_UpdatePipeline(t *testing.T) {
	router, registry := setupIndexSettingsHTTPTestRouter()
	indexName := "test-index"

	// Register another query pipeline
	newPipelineDef := &pipeline.PipelineDefinition{
		Name:        "test-query-pipeline-v2",
		Version:     "2.0.0",
		Type:        pipeline.PipelineTypeQuery,
		Description: "Test query pipeline v2",
		Stages: []pipeline.StageDefinition{
			{
				Name:    "stage1",
				Type:    pipeline.StageTypeNative,
				Enabled: true,
				Config:  map[string]interface{}{"function": "test"},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(newPipelineDef))

	// Set initial pipeline
	reqBody1 := map[string]interface{}{
		"index": map[string]interface{}{
			"query": map[string]interface{}{
				"default_pipeline": "test-query-pipeline",
			},
		},
	}
	body1, _ := json.Marshal(reqBody1)
	req1 := httptest.NewRequest(http.MethodPut, "/"+indexName+"/_settings", bytes.NewBuffer(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Update to new pipeline
	reqBody2 := map[string]interface{}{
		"index": map[string]interface{}{
			"query": map[string]interface{}{
				"default_pipeline": "test-query-pipeline-v2",
			},
		},
	}
	body2, _ := json.Marshal(reqBody2)
	req2 := httptest.NewRequest(http.MethodPut, "/"+indexName+"/_settings", bytes.NewBuffer(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	// Verify update
	pipe, err := registry.GetPipelineForIndex(indexName, pipeline.PipelineTypeQuery)
	require.NoError(t, err)
	assert.Equal(t, "test-query-pipeline-v2", pipe.Name())
}

func TestIndexSettingsHTTP_GetSettings_MultipleIndexes(t *testing.T) {
	router, registry := setupIndexSettingsHTTPTestRouter()

	// Associate different pipelines with different indexes
	require.NoError(t, registry.AssociatePipeline("index1", pipeline.PipelineTypeQuery, "test-query-pipeline"))
	require.NoError(t, registry.AssociatePipeline("index2", pipeline.PipelineTypeDocument, "test-document-pipeline"))

	// Get settings for index1
	req1 := httptest.NewRequest(http.MethodGet, "/index1/_settings", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	var response1 map[string]interface{}
	json.Unmarshal(w1.Body.Bytes(), &response1)
	indexData1 := response1["index1"].(map[string]interface{})
	settings1 := indexData1["settings"].(map[string]interface{})
	indexSettings1 := settings1["index"].(map[string]interface{})

	// index1 should have query pipeline
	querySettings := indexSettings1["query"].(map[string]interface{})
	assert.Equal(t, "test-query-pipeline", querySettings["default_pipeline"])
	_, hasDocument := indexSettings1["document"]
	assert.False(t, hasDocument)

	// Get settings for index2
	req2 := httptest.NewRequest(http.MethodGet, "/index2/_settings", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	var response2 map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &response2)
	indexData2 := response2["index2"].(map[string]interface{})
	settings2 := indexData2["settings"].(map[string]interface{})
	indexSettings2 := settings2["index"].(map[string]interface{})

	// index2 should have document pipeline
	documentSettings := indexSettings2["document"].(map[string]interface{})
	assert.Equal(t, "test-document-pipeline", documentSettings["default_pipeline"])
	_, hasQuery := indexSettings2["query"]
	assert.False(t, hasQuery)
}
