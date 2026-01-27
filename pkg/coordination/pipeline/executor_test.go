// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockStage implements Stage interface for testing
type mockStage struct {
	name          string
	stageType     StageType
	executeFunc   func(ctx *StageContext, input interface{}) (interface{}, error)
	config        map[string]interface{}
	executedCount int
}

func (m *mockStage) Name() string {
	return m.name
}

func (m *mockStage) Type() StageType {
	return m.stageType
}

func (m *mockStage) Execute(ctx *StageContext, input interface{}) (interface{}, error) {
	m.executedCount++
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	return input, nil
}

func (m *mockStage) Config() map[string]interface{} {
	return m.config
}

func TestPipelineImpl_Execute(t *testing.T) {
	logger := zap.NewNop()

	t.Run("HappyPath", func(t *testing.T) {
		// Create pipeline with successful stages
		stage1 := &mockStage{
			name:      "stage1",
			stageType: StageTypePython,
			executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
				doc := input.(map[string]interface{})
				doc["stage1_executed"] = true
				return doc, nil
			},
		}

		stage2 := &mockStage{
			name:      "stage2",
			stageType: StageTypePython,
			executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
				doc := input.(map[string]interface{})
				doc["stage2_executed"] = true
				return doc, nil
			},
		}

		pipeline := &pipelineImpl{
			def: &PipelineDefinition{
				Name:    "test-pipeline",
				Version: "1.0.0",
				Type:    PipelineTypeDocument,
				Stages: []StageDefinition{
					{Name: "stage1", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}},
					{Name: "stage2", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}},
				},
				Enabled: true,
			},
			stages: []Stage{stage1, stage2},
			logger: logger,
		}

		input := map[string]interface{}{
			"title": "test document",
		}

		output, err := pipeline.Execute(context.Background(), input)
		require.NoError(t, err)

		result := output.(map[string]interface{})
		assert.Equal(t, "test document", result["title"])
		assert.Equal(t, true, result["stage1_executed"])
		assert.Equal(t, true, result["stage2_executed"])
		assert.Equal(t, 1, stage1.executedCount)
		assert.Equal(t, 1, stage2.executedCount)
	})

	t.Run("DisabledPipeline", func(t *testing.T) {
		stage1 := &mockStage{
			name:      "stage1",
			stageType: StageTypePython,
		}

		pipeline := &pipelineImpl{
			def: &PipelineDefinition{
				Name:    "disabled-pipeline",
				Version: "1.0.0",
				Type:    PipelineTypeDocument,
				Stages: []StageDefinition{
					{Name: "stage1", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}},
				},
				Enabled: false, // Pipeline disabled
			},
			stages: []Stage{stage1},
			logger: logger,
		}

		input := map[string]interface{}{"title": "test"}

		output, err := pipeline.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Equal(t, input, output)
		assert.Equal(t, 0, stage1.executedCount) // Stage not executed
	})

	t.Run("DisabledStage", func(t *testing.T) {
		stage1 := &mockStage{
			name:      "stage1",
			stageType: StageTypePython,
			executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
				doc := input.(map[string]interface{})
				doc["stage1_executed"] = true
				return doc, nil
			},
		}

		stage2 := &mockStage{
			name:      "stage2",
			stageType: StageTypePython,
			executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
				doc := input.(map[string]interface{})
				doc["stage2_executed"] = true
				return doc, nil
			},
		}

		pipeline := &pipelineImpl{
			def: &PipelineDefinition{
				Name:    "test-pipeline",
				Version: "1.0.0",
				Type:    PipelineTypeDocument,
				Stages: []StageDefinition{
					{Name: "stage1", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}},
					{Name: "stage2", Type: StageTypePython, Enabled: false, Config: map[string]interface{}{}}, // Disabled
				},
				Enabled: true,
			},
			stages: []Stage{stage1, stage2},
			logger: logger,
		}

		input := map[string]interface{}{"title": "test"}

		output, err := pipeline.Execute(context.Background(), input)
		require.NoError(t, err)

		result := output.(map[string]interface{})
		assert.Equal(t, true, result["stage1_executed"])
		assert.Nil(t, result["stage2_executed"]) // Stage2 not executed
		assert.Equal(t, 1, stage1.executedCount)
		assert.Equal(t, 0, stage2.executedCount)
	})

	t.Run("StageFailureWithAbortPolicy", func(t *testing.T) {
		stage1 := &mockStage{
			name:      "stage1",
			stageType: StageTypePython,
			executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
				return nil, errors.New("stage1 failed")
			},
		}

		stage2 := &mockStage{
			name:      "stage2",
			stageType: StageTypePython,
		}

		pipeline := &pipelineImpl{
			def: &PipelineDefinition{
				Name:    "test-pipeline",
				Version: "1.0.0",
				Type:    PipelineTypeDocument,
				Stages: []StageDefinition{
					{Name: "stage1", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}, OnFailure: FailurePolicyAbort},
					{Name: "stage2", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}},
				},
				Enabled: true,
			},
			stages: []Stage{stage1, stage2},
			logger: logger,
		}

		input := map[string]interface{}{"title": "test"}

		output, err := pipeline.Execute(context.Background(), input)
		require.Error(t, err)
		assert.Nil(t, output)
		assert.Contains(t, err.Error(), "stage1 failed")
		assert.Equal(t, 1, stage1.executedCount)
		assert.Equal(t, 0, stage2.executedCount) // Stage2 not executed due to abort
	})

	t.Run("StageFailureWithContinuePolicy", func(t *testing.T) {
		stage1 := &mockStage{
			name:      "stage1",
			stageType: StageTypePython,
			executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
				return nil, errors.New("stage1 failed")
			},
		}

		stage2 := &mockStage{
			name:      "stage2",
			stageType: StageTypePython,
			executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
				doc := input.(map[string]interface{})
				doc["stage2_executed"] = true
				return doc, nil
			},
		}

		pipeline := &pipelineImpl{
			def: &PipelineDefinition{
				Name:    "test-pipeline",
				Version: "1.0.0",
				Type:    PipelineTypeDocument,
				Stages: []StageDefinition{
					{Name: "stage1", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}, OnFailure: FailurePolicyContinue},
					{Name: "stage2", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}},
				},
				Enabled: true,
			},
			stages: []Stage{stage1, stage2},
			logger: logger,
		}

		input := map[string]interface{}{"title": "test"}

		output, err := pipeline.Execute(context.Background(), input)
		require.NoError(t, err) // No error despite stage1 failure

		result := output.(map[string]interface{})
		assert.Equal(t, "test", result["title"])
		assert.Equal(t, true, result["stage2_executed"]) // Stage2 executed with original input
		assert.Equal(t, 1, stage1.executedCount)
		assert.Equal(t, 1, stage2.executedCount)
	})

	t.Run("PipelineTimeout", func(t *testing.T) {
		timeout := 50 * time.Millisecond

		stage1 := &mockStage{
			name:      "stage1",
			stageType: StageTypePython,
			executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
				// Simulate slow operation that respects context
				select {
				case <-ctx.Context.Done():
					return nil, ctx.Context.Err()
				case <-time.After(100 * time.Millisecond):
					return input, nil
				}
			},
		}

		pipeline := &pipelineImpl{
			def: &PipelineDefinition{
				Name:    "test-pipeline",
				Version: "1.0.0",
				Type:    PipelineTypeDocument,
				Stages: []StageDefinition{
					{Name: "stage1", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}},
				},
				Enabled: true,
				Timeout: &timeout,
			},
			stages: []Stage{stage1},
			logger: logger,
		}

		input := map[string]interface{}{"title": "test"}

		output, err := pipeline.Execute(context.Background(), input)
		require.Error(t, err)
		assert.Nil(t, output)
	})

	t.Run("StageTimeout", func(t *testing.T) {
		timeout := 50 * time.Millisecond

		stage1 := &mockStage{
			name:      "stage1",
			stageType: StageTypePython,
			executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
				// Check context cancellation
				select {
				case <-ctx.Context.Done():
					return nil, ctx.Context.Err()
				case <-time.After(100 * time.Millisecond):
					return input, nil
				}
			},
		}

		pipeline := &pipelineImpl{
			def: &PipelineDefinition{
				Name:    "test-pipeline",
				Version: "1.0.0",
				Type:    PipelineTypeDocument,
				Stages: []StageDefinition{
					{Name: "stage1", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}, Timeout: &timeout},
				},
				Enabled: true,
			},
			stages: []Stage{stage1},
			logger: logger,
		}

		input := map[string]interface{}{"title": "test"}

		output, err := pipeline.Execute(context.Background(), input)
		require.Error(t, err)
		assert.Nil(t, output)
	})

	t.Run("InvalidInputType", func(t *testing.T) {
		pipeline := &pipelineImpl{
			def: &PipelineDefinition{
				Name:    "test-pipeline",
				Version: "1.0.0",
				Type:    PipelineTypeDocument,
				Stages: []StageDefinition{
					{Name: "stage1", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}},
				},
				Enabled: true,
			},
			stages: []Stage{},
			logger: logger,
		}

		// Pass wrong input type (string instead of map)
		output, err := pipeline.Execute(context.Background(), "invalid input")
		require.Error(t, err)
		assert.Nil(t, output)
		assert.Contains(t, err.Error(), "invalid input type")
	})

	t.Run("NilInput", func(t *testing.T) {
		pipeline := &pipelineImpl{
			def: &PipelineDefinition{
				Name:    "test-pipeline",
				Version: "1.0.0",
				Type:    PipelineTypeDocument,
				Stages: []StageDefinition{
					{Name: "stage1", Type: StageTypePython, Enabled: true, Config: map[string]interface{}{}},
				},
				Enabled: true,
			},
			stages: []Stage{},
			logger: logger,
		}

		output, err := pipeline.Execute(context.Background(), nil)
		require.Error(t, err)
		assert.Nil(t, output)
		assert.Contains(t, err.Error(), "input cannot be nil")
	})
}

func TestPipelineImpl_Getters(t *testing.T) {
	logger := zap.NewNop()

	def := &PipelineDefinition{
		Name:        "test-pipeline",
		Version:     "1.0.0",
		Type:        PipelineTypeQuery,
		Description: "Test pipeline",
		Metadata: map[string]interface{}{
			"author": "test",
		},
		Stages:  []StageDefinition{},
		Enabled: true,
	}

	pipeline := &pipelineImpl{
		def:    def,
		stages: []Stage{},
		logger: logger,
	}

	assert.Equal(t, "test-pipeline", pipeline.Name())
	assert.Equal(t, "1.0.0", pipeline.Version())
	assert.Equal(t, PipelineTypeQuery, pipeline.Type())
	assert.Equal(t, "Test pipeline", pipeline.Description())
	assert.Equal(t, "test", pipeline.Metadata()["author"])
	assert.Equal(t, 0, len(pipeline.Stages()))
}

func TestExecutor_ExecutePipeline(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)
	executor := NewExecutor(registry, logger)

	// Register a pipeline
	stage1 := &mockStage{
		name:      "stage1",
		stageType: StageTypeNative,
		executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
			doc := input.(map[string]interface{})
			doc["processed"] = true
			return doc, nil
		},
	}

	def := &PipelineDefinition{
		Name:    "test-pipeline",
		Version: "1.0.0",
		Type:    PipelineTypeDocument,
		Stages: []StageDefinition{
			{Name: "stage1", Type: StageTypeNative, Enabled: true, Config: map[string]interface{}{"function": "test_func"}},
		},
		Enabled: true,
	}

	require.NoError(t, registry.Register(def))

	// Get pipeline and set stages
	pipeline, err := registry.Get("test-pipeline")
	require.NoError(t, err)
	impl := pipeline.(*pipelineImpl)
	impl.SetStages([]Stage{stage1})

	t.Run("ExecuteSuccess", func(t *testing.T) {
		input := map[string]interface{}{
			"title": "test document",
		}

		output, err := executor.ExecutePipeline(context.Background(), "test-pipeline", input)
		require.NoError(t, err)

		result := output.(map[string]interface{})
		assert.Equal(t, "test document", result["title"])
		assert.Equal(t, true, result["processed"])

		// Check metrics
		metrics := executor.GetMetrics()
		assert.Equal(t, int64(1), metrics.PipelinesExecuted)
		assert.Equal(t, int64(0), metrics.PipelinesFailed)
		assert.Equal(t, int64(1), metrics.StagesExecuted)
	})

	t.Run("ExecuteNonExistentPipeline", func(t *testing.T) {
		input := map[string]interface{}{"title": "test"}

		output, err := executor.ExecutePipeline(context.Background(), "nonexistent", input)
		assert.Error(t, err)
		assert.Nil(t, output)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestExecutor_ExecutePipelineForIndex(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)
	executor := NewExecutor(registry, logger)

	// Register pipeline
	stage1 := &mockStage{
		name:      "stage1",
		stageType: StageTypeNative,
		executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
			doc := input.(map[string]interface{})
			doc["processed"] = true
			return doc, nil
		},
	}

	def := &PipelineDefinition{
		Name:    "test-pipeline",
		Version: "1.0.0",
		Type:    PipelineTypeDocument,
		Stages: []StageDefinition{
			{Name: "stage1", Type: StageTypeNative, Enabled: true, Config: map[string]interface{}{"function": "test_func"}},
		},
		Enabled: true,
	}

	require.NoError(t, registry.Register(def))

	// Set stages
	pipeline, err := registry.Get("test-pipeline")
	require.NoError(t, err)
	impl := pipeline.(*pipelineImpl)
	impl.SetStages([]Stage{stage1})

	// Associate with index
	require.NoError(t, registry.AssociatePipeline("test-index", PipelineTypeDocument, "test-pipeline"))

	t.Run("ExecuteForIndexSuccess", func(t *testing.T) {
		input := map[string]interface{}{
			"title": "test document",
		}

		output, err := executor.ExecutePipelineForIndex(context.Background(), "test-index", PipelineTypeDocument, input)
		require.NoError(t, err)

		result := output.(map[string]interface{})
		assert.Equal(t, true, result["processed"])
	})

	t.Run("ExecuteForNonExistentIndex", func(t *testing.T) {
		input := map[string]interface{}{"title": "test"}

		output, err := executor.ExecutePipelineForIndex(context.Background(), "nonexistent-index", PipelineTypeDocument, input)
		assert.Error(t, err)
		assert.Nil(t, output)
		assert.Contains(t, err.Error(), "no pipelines configured")
	})
}

func TestExecutor_Metrics(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry(logger)
	executor := NewExecutor(registry, logger)

	// Register pipeline
	stage1 := &mockStage{
		name:      "stage1",
		stageType: StageTypeNative,
		executeFunc: func(ctx *StageContext, input interface{}) (interface{}, error) {
			time.Sleep(10 * time.Millisecond)
			return input, nil
		},
	}

	def := &PipelineDefinition{
		Name:    "test-pipeline",
		Version: "1.0.0",
		Type:    PipelineTypeDocument,
		Stages: []StageDefinition{
			{Name: "stage1", Type: StageTypeNative, Enabled: true, Config: map[string]interface{}{"function": "test_func"}},
		},
		Enabled: true,
	}

	require.NoError(t, registry.Register(def))

	pipeline, err := registry.Get("test-pipeline")
	require.NoError(t, err)
	impl := pipeline.(*pipelineImpl)
	impl.SetStages([]Stage{stage1})

	// Execute multiple times
	input := map[string]interface{}{"title": "test"}
	for i := 0; i < 5; i++ {
		_, err := executor.ExecutePipeline(context.Background(), "test-pipeline", input)
		require.NoError(t, err)
	}

	// Check metrics
	metrics := executor.GetMetrics()
	assert.Equal(t, int64(5), metrics.PipelinesExecuted)
	assert.Equal(t, int64(0), metrics.PipelinesFailed)
	assert.Equal(t, int64(5), metrics.StagesExecuted)
	assert.Greater(t, metrics.AverageDuration, time.Duration(0))
	assert.Greater(t, metrics.TotalDuration, time.Duration(0))
	assert.False(t, metrics.LastExecutionTime.IsZero())
}
