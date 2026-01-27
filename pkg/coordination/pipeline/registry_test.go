// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewRegistry(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)

	assert.NotNil(t, registry)
	assert.NotNil(t, registry.pipelines)
	assert.NotNil(t, registry.indexPipelines)
	assert.NotNil(t, registry.stats)
	assert.Equal(t, logger, registry.logger)
}

func TestRegistry_Register(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)

	t.Run("ValidPipeline", func(t *testing.T) {
		def := &PipelineDefinition{
			Name:        "test-pipeline",
			Version:     "1.0.0",
			Type:        PipelineTypeQuery,
			Description: "Test pipeline",
			Stages: []StageDefinition{
				{
					Name:    "stage1",
					Type:    StageTypePython,
					Enabled: true,
					Config: map[string]interface{}{
						"udf_name":    "test_udf",
						"udf_version": "1.0.0",
					},
				},
			},
			Enabled: true,
		}

		err := registry.Register(def)
		require.NoError(t, err)

		// Verify pipeline was registered
		pipeline, err := registry.Get("test-pipeline")
		require.NoError(t, err)
		assert.NotNil(t, pipeline)
		assert.Equal(t, "test-pipeline", pipeline.Name())
		assert.Equal(t, "1.0.0", pipeline.Version())
		assert.Equal(t, PipelineTypeQuery, pipeline.Type())

		// Verify timestamps were set
		assert.False(t, def.Created.IsZero())
		assert.False(t, def.Updated.IsZero())

		// Verify stats were initialized
		stats, err := registry.GetStats("test-pipeline")
		require.NoError(t, err)
		assert.Equal(t, "test-pipeline", stats.Name)
		assert.Equal(t, int64(0), stats.TotalExecutions)
	})

	t.Run("DuplicatePipeline", func(t *testing.T) {
		def := &PipelineDefinition{
			Name:    "duplicate-pipeline",
			Version: "1.0.0",
			Type:    PipelineTypeQuery,
			Stages: []StageDefinition{
				{
					Name:    "stage1",
					Type:    StageTypePython,
					Enabled: true,
					Config: map[string]interface{}{
						"udf_name": "test_udf",
					},
				},
			},
			Enabled: true,
		}

		// Register first time
		err := registry.Register(def)
		require.NoError(t, err)

		// Try to register again
		err = registry.Register(def)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("MissingName", func(t *testing.T) {
		def := &PipelineDefinition{
			Version: "1.0.0",
			Type:    PipelineTypeQuery,
			Stages: []StageDefinition{
				{
					Name:    "stage1",
					Type:    StageTypePython,
					Enabled: true,
					Config: map[string]interface{}{
						"udf_name": "test_udf",
					},
				},
			},
			Enabled: true,
		}

		err := registry.Register(def)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingVersion", func(t *testing.T) {
		def := &PipelineDefinition{
			Name: "no-version",
			Type: PipelineTypeQuery,
			Stages: []StageDefinition{
				{
					Name:    "stage1",
					Type:    StageTypePython,
					Enabled: true,
					Config: map[string]interface{}{
						"udf_name": "test_udf",
					},
				},
			},
			Enabled: true,
		}

		err := registry.Register(def)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version is required")
	})

	t.Run("InvalidPipelineType", func(t *testing.T) {
		def := &PipelineDefinition{
			Name:    "invalid-type",
			Version: "1.0.0",
			Type:    "invalid",
			Stages: []StageDefinition{
				{
					Name:    "stage1",
					Type:    StageTypePython,
					Enabled: true,
					Config: map[string]interface{}{
						"udf_name": "test_udf",
					},
				},
			},
			Enabled: true,
		}

		err := registry.Register(def)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pipeline type")
	})

	t.Run("NoStages", func(t *testing.T) {
		def := &PipelineDefinition{
			Name:    "no-stages",
			Version: "1.0.0",
			Type:    PipelineTypeQuery,
			Stages:  []StageDefinition{},
			Enabled: true,
		}

		err := registry.Register(def)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one stage")
	})

	t.Run("DuplicateStageNames", func(t *testing.T) {
		def := &PipelineDefinition{
			Name:    "duplicate-stages",
			Version: "1.0.0",
			Type:    PipelineTypeQuery,
			Stages: []StageDefinition{
				{
					Name:    "same-name",
					Type:    StageTypePython,
					Enabled: true,
					Config: map[string]interface{}{
						"udf_name": "test_udf",
					},
				},
				{
					Name:    "same-name",
					Type:    StageTypePython,
					Enabled: true,
					Config: map[string]interface{}{
						"udf_name": "test_udf2",
					},
				},
			},
			Enabled: true,
		}

		err := registry.Register(def)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate stage name")
	})

	t.Run("PythonStageWithoutUDFName", func(t *testing.T) {
		def := &PipelineDefinition{
			Name:    "missing-udf",
			Version: "1.0.0",
			Type:    PipelineTypeQuery,
			Stages: []StageDefinition{
				{
					Name:    "stage1",
					Type:    StageTypePython,
					Enabled: true,
					Config:  map[string]interface{}{
						// Missing udf_name
					},
				},
			},
			Enabled: true,
		}

		err := registry.Register(def)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "udf_name is required")
	})

	t.Run("NativeStageWithoutFunction", func(t *testing.T) {
		def := &PipelineDefinition{
			Name:    "missing-function",
			Version: "1.0.0",
			Type:    PipelineTypeQuery,
			Stages: []StageDefinition{
				{
					Name:    "stage1",
					Type:    StageTypeNative,
					Enabled: true,
					Config:  map[string]interface{}{
						// Missing function
					},
				},
			},
			Enabled: true,
		}

		err := registry.Register(def)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "function is required")
	})
}

func TestRegistry_Get(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)

	// Register a pipeline
	def := &PipelineDefinition{
		Name:    "test-get",
		Version: "1.0.0",
		Type:    PipelineTypeQuery,
		Stages: []StageDefinition{
			{
				Name:    "stage1",
				Type:    StageTypePython,
				Enabled: true,
				Config: map[string]interface{}{
					"udf_name": "test_udf",
				},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(def))

	t.Run("ExistingPipeline", func(t *testing.T) {
		pipeline, err := registry.Get("test-get")
		require.NoError(t, err)
		assert.NotNil(t, pipeline)
		assert.Equal(t, "test-get", pipeline.Name())
	})

	t.Run("NonExistentPipeline", func(t *testing.T) {
		pipeline, err := registry.Get("does-not-exist")
		assert.Error(t, err)
		assert.Nil(t, pipeline)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRegistry_List(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)

	// Register multiple pipelines of different types
	pipelines := []*PipelineDefinition{
		{
			Name:    "query-pipeline-1",
			Version: "1.0.0",
			Type:    PipelineTypeQuery,
			Stages: []StageDefinition{
				{
					Name:    "stage1",
					Type:    StageTypePython,
					Enabled: true,
					Config: map[string]interface{}{
						"udf_name": "test_udf",
					},
				},
			},
			Enabled: true,
		},
		{
			Name:    "query-pipeline-2",
			Version: "1.0.0",
			Type:    PipelineTypeQuery,
			Stages: []StageDefinition{
				{
					Name:    "stage1",
					Type:    StageTypePython,
					Enabled: true,
					Config: map[string]interface{}{
						"udf_name": "test_udf",
					},
				},
			},
			Enabled: true,
		},
		{
			Name:    "document-pipeline-1",
			Version: "1.0.0",
			Type:    PipelineTypeDocument,
			Stages: []StageDefinition{
				{
					Name:    "stage1",
					Type:    StageTypePython,
					Enabled: true,
					Config: map[string]interface{}{
						"udf_name": "test_udf",
					},
				},
			},
			Enabled: true,
		},
	}

	for _, p := range pipelines {
		require.NoError(t, registry.Register(p))
	}

	t.Run("ListAll", func(t *testing.T) {
		result := registry.List("")
		assert.Len(t, result, 3)
	})

	t.Run("FilterByQueryType", func(t *testing.T) {
		result := registry.List(PipelineTypeQuery)
		assert.Len(t, result, 2)
		for _, p := range result {
			assert.Equal(t, PipelineTypeQuery, p.Type)
		}
	})

	t.Run("FilterByDocumentType", func(t *testing.T) {
		result := registry.List(PipelineTypeDocument)
		assert.Len(t, result, 1)
		assert.Equal(t, "document-pipeline-1", result[0].Name)
	})

	t.Run("FilterByResultType", func(t *testing.T) {
		result := registry.List(PipelineTypeResult)
		assert.Len(t, result, 0)
	})
}

func TestRegistry_Unregister(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)

	// Register a pipeline
	def := &PipelineDefinition{
		Name:    "test-unregister",
		Version: "1.0.0",
		Type:    PipelineTypeQuery,
		Stages: []StageDefinition{
			{
				Name:    "stage1",
				Type:    StageTypePython,
				Enabled: true,
				Config: map[string]interface{}{
					"udf_name": "test_udf",
				},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(def))

	t.Run("UnregisterExisting", func(t *testing.T) {
		err := registry.Unregister("test-unregister")
		require.NoError(t, err)

		// Verify it's gone
		_, err = registry.Get("test-unregister")
		assert.Error(t, err)
	})

	t.Run("UnregisterNonExistent", func(t *testing.T) {
		err := registry.Unregister("does-not-exist")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("UnregisterAssociatedPipeline", func(t *testing.T) {
		// Register new pipeline
		def2 := &PipelineDefinition{
			Name:    "associated-pipeline",
			Version: "1.0.0",
			Type:    PipelineTypeQuery,
			Stages: []StageDefinition{
				{
					Name:    "stage1",
					Type:    StageTypePython,
					Enabled: true,
					Config: map[string]interface{}{
						"udf_name": "test_udf",
					},
				},
			},
			Enabled: true,
		}
		require.NoError(t, registry.Register(def2))

		// Associate with index
		err := registry.AssociatePipeline("test-index", PipelineTypeQuery, "associated-pipeline")
		require.NoError(t, err)

		// Try to unregister
		err = registry.Unregister("associated-pipeline")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "still associated")
	})
}

func TestRegistry_AssociatePipeline(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)

	// Register pipelines
	queryDef := &PipelineDefinition{
		Name:    "query-pipeline",
		Version: "1.0.0",
		Type:    PipelineTypeQuery,
		Stages: []StageDefinition{
			{
				Name:    "stage1",
				Type:    StageTypePython,
				Enabled: true,
				Config: map[string]interface{}{
					"udf_name": "test_udf",
				},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(queryDef))

	docDef := &PipelineDefinition{
		Name:    "document-pipeline",
		Version: "1.0.0",
		Type:    PipelineTypeDocument,
		Stages: []StageDefinition{
			{
				Name:    "stage1",
				Type:    StageTypePython,
				Enabled: true,
				Config: map[string]interface{}{
					"udf_name": "test_udf",
				},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(docDef))

	t.Run("AssociateValid", func(t *testing.T) {
		err := registry.AssociatePipeline("test-index", PipelineTypeQuery, "query-pipeline")
		require.NoError(t, err)

		// Verify association
		pipeline, err := registry.GetPipelineForIndex("test-index", PipelineTypeQuery)
		require.NoError(t, err)
		assert.Equal(t, "query-pipeline", pipeline.Name())
	})

	t.Run("AssociateNonExistentPipeline", func(t *testing.T) {
		err := registry.AssociatePipeline("test-index", PipelineTypeQuery, "does-not-exist")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("AssociateTypeMismatch", func(t *testing.T) {
		// Try to associate document pipeline as query type
		err := registry.AssociatePipeline("test-index", PipelineTypeQuery, "document-pipeline")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot associate")
	})

	t.Run("AssociateMultipleTypes", func(t *testing.T) {
		err := registry.AssociatePipeline("multi-index", PipelineTypeQuery, "query-pipeline")
		require.NoError(t, err)

		err = registry.AssociatePipeline("multi-index", PipelineTypeDocument, "document-pipeline")
		require.NoError(t, err)

		// Verify both associations
		queryPipe, err := registry.GetPipelineForIndex("multi-index", PipelineTypeQuery)
		require.NoError(t, err)
		assert.Equal(t, "query-pipeline", queryPipe.Name())

		docPipe, err := registry.GetPipelineForIndex("multi-index", PipelineTypeDocument)
		require.NoError(t, err)
		assert.Equal(t, "document-pipeline", docPipe.Name())
	})
}

func TestRegistry_GetPipelineForIndex(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)

	// Register and associate pipeline
	def := &PipelineDefinition{
		Name:    "test-pipeline",
		Version: "1.0.0",
		Type:    PipelineTypeQuery,
		Stages: []StageDefinition{
			{
				Name:    "stage1",
				Type:    StageTypePython,
				Enabled: true,
				Config: map[string]interface{}{
					"udf_name": "test_udf",
				},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(def))
	require.NoError(t, registry.AssociatePipeline("test-index", PipelineTypeQuery, "test-pipeline"))

	t.Run("GetExisting", func(t *testing.T) {
		pipeline, err := registry.GetPipelineForIndex("test-index", PipelineTypeQuery)
		require.NoError(t, err)
		assert.NotNil(t, pipeline)
		assert.Equal(t, "test-pipeline", pipeline.Name())
	})

	t.Run("GetNonExistentIndex", func(t *testing.T) {
		pipeline, err := registry.GetPipelineForIndex("nonexistent-index", PipelineTypeQuery)
		assert.Error(t, err)
		assert.Nil(t, pipeline)
		assert.Contains(t, err.Error(), "no pipelines configured")
	})

	t.Run("GetNonExistentType", func(t *testing.T) {
		pipeline, err := registry.GetPipelineForIndex("test-index", PipelineTypeDocument)
		assert.Error(t, err)
		assert.Nil(t, pipeline)
		assert.Contains(t, err.Error(), "no pipeline configured")
	})
}

func TestRegistry_Stats(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)

	// Register pipeline
	def := &PipelineDefinition{
		Name:    "stats-pipeline",
		Version: "1.0.0",
		Type:    PipelineTypeQuery,
		Stages: []StageDefinition{
			{
				Name:    "stage1",
				Type:    StageTypePython,
				Enabled: true,
				Config: map[string]interface{}{
					"udf_name": "test_udf",
				},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(def))

	t.Run("InitialStats", func(t *testing.T) {
		stats, err := registry.GetStats("stats-pipeline")
		require.NoError(t, err)
		assert.Equal(t, "stats-pipeline", stats.Name)
		assert.Equal(t, int64(0), stats.TotalExecutions)
		assert.Equal(t, int64(0), stats.SuccessfulExecutions)
		assert.Equal(t, int64(0), stats.FailedExecutions)
	})

	t.Run("UpdateStats", func(t *testing.T) {
		// Simulate execution
		duration := 100 * time.Millisecond
		stageStats := []StageStats{
			{
				Name:                 "stage1",
				TotalExecutions:      1,
				SuccessfulExecutions: 1,
				AverageDuration:      duration,
			},
		}

		registry.UpdateStats("stats-pipeline", duration, true, stageStats)

		stats, err := registry.GetStats("stats-pipeline")
		require.NoError(t, err)
		assert.Equal(t, int64(1), stats.TotalExecutions)
		assert.Equal(t, int64(1), stats.SuccessfulExecutions)
		assert.Equal(t, int64(0), stats.FailedExecutions)
		assert.Equal(t, duration, stats.AverageDuration)
		assert.False(t, stats.LastExecuted.IsZero())
	})

	t.Run("MultipleExecutions", func(t *testing.T) {
		// Execute multiple times
		for i := 0; i < 5; i++ {
			duration := time.Duration(100+i*10) * time.Millisecond
			stageStats := []StageStats{
				{
					Name:                 "stage1",
					TotalExecutions:      1,
					SuccessfulExecutions: 1,
					AverageDuration:      duration,
				},
			}
			registry.UpdateStats("stats-pipeline", duration, true, stageStats)
		}

		stats, err := registry.GetStats("stats-pipeline")
		require.NoError(t, err)
		assert.Equal(t, int64(6), stats.TotalExecutions) // 1 from previous test + 5 new
		assert.Equal(t, int64(6), stats.SuccessfulExecutions)
		assert.Greater(t, stats.AverageDuration, time.Duration(0))
	})

	t.Run("FailedExecution", func(t *testing.T) {
		stageStats := []StageStats{
			{
				Name:             "stage1",
				TotalExecutions:  1,
				FailedExecutions: 1,
				LastError:        "test error",
			},
		}

		registry.UpdateStats("stats-pipeline", 50*time.Millisecond, false, stageStats)

		stats, err := registry.GetStats("stats-pipeline")
		require.NoError(t, err)
		assert.Equal(t, int64(1), stats.FailedExecutions)
	})
}

func TestRegistry_DisassociatePipeline(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)

	// Register and associate pipeline
	def := &PipelineDefinition{
		Name:    "test-pipeline",
		Version: "1.0.0",
		Type:    PipelineTypeQuery,
		Stages: []StageDefinition{
			{
				Name:    "stage1",
				Type:    StageTypePython,
				Enabled: true,
				Config: map[string]interface{}{
					"udf_name": "test_udf",
				},
			},
		},
		Enabled: true,
	}
	require.NoError(t, registry.Register(def))
	require.NoError(t, registry.AssociatePipeline("test-index", PipelineTypeQuery, "test-pipeline"))

	t.Run("DisassociateExisting", func(t *testing.T) {
		err := registry.DisassociatePipeline("test-index", PipelineTypeQuery)
		require.NoError(t, err)

		// Verify it's gone
		_, err = registry.GetPipelineForIndex("test-index", PipelineTypeQuery)
		assert.Error(t, err)
	})

	t.Run("DisassociateNonExistent", func(t *testing.T) {
		err := registry.DisassociatePipeline("nonexistent-index", PipelineTypeQuery)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no pipelines configured")
	})
}
