// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package coordination

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/quidditch/quidditch/pkg/coordination/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupIndexSettingsTestRouter() (*gin.Engine, *pipeline.Registry) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := zap.NewNop()

	// Create pipeline registry
	registry := pipeline.NewRegistry(logger)

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

	return router, registry
}

func TestIndexSettings_SetPipelineOnCreation(t *testing.T) {
	_, registry := setupIndexSettingsTestRouter()

	// Associate pipelines with an index (simulating index creation)
	indexName := "test-index"
	err := registry.AssociatePipeline(indexName, pipeline.PipelineTypeQuery, "test-query-pipeline")
	require.NoError(t, err)

	// Verify association
	pipe, err := registry.GetPipelineForIndex(indexName, pipeline.PipelineTypeQuery)
	require.NoError(t, err)
	assert.Equal(t, "test-query-pipeline", pipe.Name())
}

func TestIndexSettings_SetMultiplePipelines(t *testing.T) {
	_, registry := setupIndexSettingsTestRouter()

	// Associate multiple pipelines with an index
	indexName := "test-index"

	err := registry.AssociatePipeline(indexName, pipeline.PipelineTypeQuery, "test-query-pipeline")
	require.NoError(t, err)

	err = registry.AssociatePipeline(indexName, pipeline.PipelineTypeDocument, "test-document-pipeline")
	require.NoError(t, err)

	err = registry.AssociatePipeline(indexName, pipeline.PipelineTypeResult, "test-result-pipeline")
	require.NoError(t, err)

	// Verify all associations
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

func TestIndexSettings_UpdatePipeline(t *testing.T) {
	_, registry := setupIndexSettingsTestRouter()

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

	indexName := "test-index"

	// Associate initial pipeline
	err := registry.AssociatePipeline(indexName, pipeline.PipelineTypeQuery, "test-query-pipeline")
	require.NoError(t, err)

	// Update to new pipeline
	err = registry.AssociatePipeline(indexName, pipeline.PipelineTypeQuery, "test-query-pipeline-v2")
	require.NoError(t, err)

	// Verify update
	pipe, err := registry.GetPipelineForIndex(indexName, pipeline.PipelineTypeQuery)
	require.NoError(t, err)
	assert.Equal(t, "test-query-pipeline-v2", pipe.Name())
}

func TestIndexSettings_NonExistentPipeline(t *testing.T) {
	_, registry := setupIndexSettingsTestRouter()

	indexName := "test-index"

	// Try to associate non-existent pipeline
	err := registry.AssociatePipeline(indexName, pipeline.PipelineTypeQuery, "non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestIndexSettings_WrongPipelineType(t *testing.T) {
	_, registry := setupIndexSettingsTestRouter()

	indexName := "test-index"

	// Try to associate query pipeline as document pipeline
	err := registry.AssociatePipeline(indexName, pipeline.PipelineTypeDocument, "test-query-pipeline")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline 'test-query-pipeline' is type 'query', cannot associate with type 'document'")
}

func TestIndexSettings_GetWithNoPipeline(t *testing.T) {
	_, registry := setupIndexSettingsTestRouter()

	indexName := "test-index"

	// Try to get pipeline when none is associated
	_, err := registry.GetPipelineForIndex(indexName, pipeline.PipelineTypeQuery)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no pipelines configured for index")
}

func TestIndexSettings_DisassociatePipeline(t *testing.T) {
	_, registry := setupIndexSettingsTestRouter()

	indexName := "test-index"

	// Associate pipeline
	err := registry.AssociatePipeline(indexName, pipeline.PipelineTypeQuery, "test-query-pipeline")
	require.NoError(t, err)

	// Verify association
	_, err = registry.GetPipelineForIndex(indexName, pipeline.PipelineTypeQuery)
	require.NoError(t, err)

	// Dissociate pipeline
	err = registry.DisassociatePipeline(indexName, pipeline.PipelineTypeQuery)
	require.NoError(t, err)

	// Verify dissociation
	_, err = registry.GetPipelineForIndex(indexName, pipeline.PipelineTypeQuery)
	assert.Error(t, err)
}

func TestIndexSettings_MultipleIndexes(t *testing.T) {
	_, registry := setupIndexSettingsTestRouter()

	// Associate same pipeline with multiple indexes
	err := registry.AssociatePipeline("index1", pipeline.PipelineTypeQuery, "test-query-pipeline")
	require.NoError(t, err)

	err = registry.AssociatePipeline("index2", pipeline.PipelineTypeQuery, "test-query-pipeline")
	require.NoError(t, err)

	// Verify both associations
	pipe1, err := registry.GetPipelineForIndex("index1", pipeline.PipelineTypeQuery)
	require.NoError(t, err)
	assert.Equal(t, "test-query-pipeline", pipe1.Name())

	pipe2, err := registry.GetPipelineForIndex("index2", pipeline.PipelineTypeQuery)
	require.NoError(t, err)
	assert.Equal(t, "test-query-pipeline", pipe2.Name())

	// Dissociate from one index should not affect the other
	err = registry.DisassociatePipeline("index1", pipeline.PipelineTypeQuery)
	require.NoError(t, err)

	_, err = registry.GetPipelineForIndex("index1", pipeline.PipelineTypeQuery)
	assert.Error(t, err)

	pipe2, err = registry.GetPipelineForIndex("index2", pipeline.PipelineTypeQuery)
	require.NoError(t, err)
	assert.Equal(t, "test-query-pipeline", pipe2.Name())
}

func TestIndexSettings_DeletePipelineWithAssociations(t *testing.T) {
	_, registry := setupIndexSettingsTestRouter()

	indexName := "test-index"

	// Associate pipeline
	err := registry.AssociatePipeline(indexName, pipeline.PipelineTypeQuery, "test-query-pipeline")
	require.NoError(t, err)

	// Try to delete pipeline (should fail because it's associated)
	err = registry.Unregister("test-query-pipeline")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "still associated")

	// Dissociate first
	err = registry.DisassociatePipeline(indexName, pipeline.PipelineTypeQuery)
	require.NoError(t, err)

	// Now delete should succeed
	err = registry.Unregister("test-query-pipeline")
	require.NoError(t, err)
}
