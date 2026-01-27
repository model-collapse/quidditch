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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/quidditch/quidditch/pkg/coordination/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupPipelineTestRouter() (*gin.Engine, *pipeline.Registry, *pipeline.Executor) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := zap.NewNop()

	// Create registry and executor
	registry := pipeline.NewRegistry(logger)
	executor := pipeline.NewExecutor(registry, logger)

	// Create handlers and register routes
	handlers := NewPipelineHandlers(registry, executor, logger)
	api := router.Group("/api/v1")
	handlers.RegisterRoutes(api)

	return router, registry, executor
}

func TestPipelineHandlers_CreatePipeline(t *testing.T) {
	router, registry, _ := setupPipelineTestRouter()

	t.Run("ValidPipeline", func(t *testing.T) {
		reqBody := PipelineCreateRequest{
			Name:        "test-pipeline",
			Version:     "1.0.0",
			Type:        pipeline.PipelineTypeQuery,
			Description: "Test pipeline",
			Stages: []pipeline.StageDefinition{
				{
					Name:    "stage1",
					Type:    pipeline.StageTypeNative,
					Enabled: true,
					Config: map[string]interface{}{
						"function": "test_func",
					},
				},
			},
			Enabled: true,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/test-pipeline", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, true, response["acknowledged"])
		assert.Equal(t, "test-pipeline", response["name"])
		assert.Equal(t, "1.0.0", response["version"])
		assert.Equal(t, "query", response["type"])

		// Verify pipeline was registered
		pipe, err := registry.Get("test-pipeline")
		require.NoError(t, err)
		assert.Equal(t, "test-pipeline", pipe.Name())
	})

	t.Run("NameMismatch", func(t *testing.T) {
		reqBody := PipelineCreateRequest{
			Name:    "different-name",
			Version: "1.0.0",
			Type:    pipeline.PipelineTypeQuery,
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

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/test-pipeline", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		assert.Contains(t, response["error"], "Name mismatch")
	})

	t.Run("InvalidType", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"name":    "invalid-type",
			"version": "1.0.0",
			"type":    "invalid",
			"stages": []map[string]interface{}{
				{
					"name":    "stage1",
					"type":    "native",
					"enabled": true,
					"config":  map[string]interface{}{"function": "test"},
				},
			},
			"enabled": true,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/invalid-type", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("MissingStages", func(t *testing.T) {
		reqBody := PipelineCreateRequest{
			Name:    "no-stages",
			Version: "1.0.0",
			Type:    pipeline.PipelineTypeQuery,
			Stages:  []pipeline.StageDefinition{},
			Enabled: true,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/no-stages", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("DuplicatePipeline", func(t *testing.T) {
		// Create first pipeline
		reqBody := PipelineCreateRequest{
			Name:    "duplicate-test",
			Version: "1.0.0",
			Type:    pipeline.PipelineTypeQuery,
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

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/duplicate-test", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)

		// Try to create again
		req = httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/duplicate-test", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestPipelineHandlers_GetPipeline(t *testing.T) {
	router, registry, _ := setupPipelineTestRouter()

	// Register a test pipeline
	def := &pipeline.PipelineDefinition{
		Name:        "get-test",
		Version:     "1.0.0",
		Type:        pipeline.PipelineTypeDocument,
		Description: "Test pipeline for GET",
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
	require.NoError(t, registry.Register(def))

	t.Run("ExistingPipeline", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pipelines/get-test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response PipelineResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "get-test", response.Name)
		assert.Equal(t, "1.0.0", response.Version)
		assert.Equal(t, pipeline.PipelineTypeDocument, response.Type)
		assert.Equal(t, "Test pipeline for GET", response.Description)
		assert.Len(t, response.Stages, 1)
		assert.Equal(t, true, response.Enabled)
	})

	t.Run("NonExistentPipeline", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pipelines/nonexistent", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestPipelineHandlers_ListPipelines(t *testing.T) {
	router, registry, _ := setupPipelineTestRouter()

	// Register multiple pipelines
	pipelines := []struct {
		name string
		typ  pipeline.PipelineType
	}{
		{"query-pipe-1", pipeline.PipelineTypeQuery},
		{"query-pipe-2", pipeline.PipelineTypeQuery},
		{"doc-pipe-1", pipeline.PipelineTypeDocument},
		{"result-pipe-1", pipeline.PipelineTypeResult},
	}

	for _, p := range pipelines {
		def := &pipeline.PipelineDefinition{
			Name:    p.name,
			Version: "1.0.0",
			Type:    p.typ,
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
		require.NoError(t, registry.Register(def))
	}

	t.Run("ListAll", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pipelines", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, float64(4), response["total"])
		pipelines := response["pipelines"].([]interface{})
		assert.Len(t, pipelines, 4)
	})

	t.Run("FilterByType", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pipelines?type=query", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, float64(2), response["total"])
		pipelines := response["pipelines"].([]interface{})
		assert.Len(t, pipelines, 2)

		// Verify all are query type
		for _, p := range pipelines {
			pipe := p.(map[string]interface{})
			assert.Equal(t, "query", pipe["type"])
		}
	})
}

func TestPipelineHandlers_DeletePipeline(t *testing.T) {
	router, registry, _ := setupPipelineTestRouter()

	t.Run("DeleteExisting", func(t *testing.T) {
		// Register a pipeline
		def := &pipeline.PipelineDefinition{
			Name:    "delete-test",
			Version: "1.0.0",
			Type:    pipeline.PipelineTypeQuery,
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
		require.NoError(t, registry.Register(def))

		// Delete it
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/pipelines/delete-test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, true, response["acknowledged"])

		// Verify it's gone
		_, err = registry.Get("delete-test")
		assert.Error(t, err)
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/pipelines/nonexistent", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("DeleteAssociated", func(t *testing.T) {
		// Register a pipeline
		def := &pipeline.PipelineDefinition{
			Name:    "associated-pipe",
			Version: "1.0.0",
			Type:    pipeline.PipelineTypeQuery,
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
		require.NoError(t, registry.Register(def))

		// Associate with an index
		require.NoError(t, registry.AssociatePipeline("test-index", pipeline.PipelineTypeQuery, "associated-pipe"))

		// Try to delete
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/pipelines/associated-pipe", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})
}

func TestPipelineHandlers_ExecutePipeline(t *testing.T) {
	router, registry, _ := setupPipelineTestRouter()

	// Register a pipeline
	def := &pipeline.PipelineDefinition{
		Name:    "execute-test",
		Version: "1.0.0",
		Type:    pipeline.PipelineTypeDocument,
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
	require.NoError(t, registry.Register(def))

	// Get pipeline and set mock stages
	pipe, err := registry.Get("execute-test")
	require.NoError(t, err)
	impl := pipe.(*pipeline.PipelineImpl)

	// Create a mock stage that modifies the input
	mockStage := &mockExecuteStage{
		name:      "stage1",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			doc := input.(map[string]interface{})
			doc["processed"] = true
			return doc, nil
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	t.Run("SuccessfulExecution", func(t *testing.T) {
		reqBody := PipelineExecuteRequest{
			Input: map[string]interface{}{
				"title": "test document",
			},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/execute-test/_execute", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response PipelineExecuteResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.True(t, response.Success)
		assert.Empty(t, response.Error)
		assert.Greater(t, response.Duration, time.Duration(0))

		// Check output
		output := response.Output.(map[string]interface{})
		assert.Equal(t, "test document", output["title"])
		assert.Equal(t, true, output["processed"])
	})

	t.Run("FailedExecution", func(t *testing.T) {
		// Create a failing stage
		failStage := &mockExecuteStage{
			name:      "fail-stage",
			stageType: pipeline.StageTypeNative,
			executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
				return nil, assert.AnError
			},
		}
		impl.SetStages([]pipeline.Stage{failStage})

		reqBody := PipelineExecuteRequest{
			Input: map[string]interface{}{
				"title": "test",
			},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/execute-test/_execute", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code) // Still 200 for test execution

		var response PipelineExecuteResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.False(t, response.Success)
		assert.NotEmpty(t, response.Error)
	})

	t.Run("InvalidRequest", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/execute-test/_execute", bytes.NewBuffer([]byte("{invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("NonExistentPipeline", func(t *testing.T) {
		reqBody := PipelineExecuteRequest{
			Input: map[string]interface{}{"test": "data"},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/nonexistent/_execute", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code) // Still 200 but with error in response

		var response PipelineExecuteResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.False(t, response.Success)
		assert.Contains(t, response.Error, "not found")
	})
}

func TestPipelineHandlers_GetStats(t *testing.T) {
	router, registry, executor := setupPipelineTestRouter()

	// Register a pipeline
	def := &pipeline.PipelineDefinition{
		Name:    "stats-test",
		Version: "1.0.0",
		Type:    pipeline.PipelineTypeDocument,
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
	require.NoError(t, registry.Register(def))

	// Get pipeline and set mock stages
	pipe, _ := registry.Get("stats-test")
	impl := pipe.(*pipeline.PipelineImpl)
	mockStage := &mockExecuteStage{
		name:      "stage1",
		stageType: pipeline.StageTypeNative,
		executeFunc: func(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
			return input, nil
		},
	}
	impl.SetStages([]pipeline.Stage{mockStage})

	// Execute pipeline a few times to generate stats
	input := map[string]interface{}{"test": "data"}
	for i := 0; i < 3; i++ {
		_, _ = executor.ExecutePipeline(nil, "stats-test", input)
	}

	t.Run("GetExistingStats", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pipelines/stats-test/_stats", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "stats-test", response["name"])
		assert.Equal(t, float64(3), response["total_executions"])
		assert.Equal(t, float64(3), response["successful_executions"])
		assert.Equal(t, float64(0), response["failed_executions"])
		assert.NotNil(t, response["average_duration_ms"])
	})

	t.Run("GetNonExistentStats", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pipelines/nonexistent/_stats", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// mockExecuteStage is a mock stage for testing
type mockExecuteStage struct {
	name        string
	stageType   pipeline.StageType
	executeFunc func(ctx *pipeline.StageContext, input interface{}) (interface{}, error)
	config      map[string]interface{}
}

func (m *mockExecuteStage) Name() string {
	return m.name
}

func (m *mockExecuteStage) Type() pipeline.StageType {
	return m.stageType
}

func (m *mockExecuteStage) Execute(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	return input, nil
}

func (m *mockExecuteStage) Config() map[string]interface{} {
	return m.config
}
