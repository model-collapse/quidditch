// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package stages

import (
	"context"
	"testing"

	"github.com/quidditch/quidditch/pkg/coordination/pipeline"
	"github.com/quidditch/quidditch/pkg/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockUDFRegistry implements a mock UDF registry for testing
type mockUDFRegistry struct {
	callFunc func(ctx context.Context, name, version string, docCtx *wasm.DocumentContext, params map[string]wasm.Value) ([]wasm.Value, error)
}

func (m *mockUDFRegistry) Call(ctx context.Context, name, version string, docCtx *wasm.DocumentContext, params map[string]wasm.Value) ([]wasm.Value, error) {
	if m.callFunc != nil {
		return m.callFunc(ctx, name, version, docCtx, params)
	}
	return nil, nil
}

func (m *mockUDFRegistry) Get(name, version string) (*wasm.RegisteredUDF, error) {
	return nil, nil
}

func (m *mockUDFRegistry) Register(metadata *wasm.UDFMetadata) error {
	return nil
}

func (m *mockUDFRegistry) List() []*wasm.RegisteredUDF {
	return nil
}

func (m *mockUDFRegistry) Unregister(name, version string) error {
	return nil
}

func TestNewPythonStage(t *testing.T) {
	logger := zap.NewNop()
	registry := &mockUDFRegistry{}

	t.Run("ValidConfig", func(t *testing.T) {
		config := map[string]interface{}{
			"udf_name":    "test_udf",
			"udf_version": "1.0.0",
			"parameters": map[string]interface{}{
				"threshold": 10,
				"boost":     1.5,
			},
		}

		stage, err := NewPythonStage("test-stage", config, registry, logger)
		require.NoError(t, err)
		assert.NotNil(t, stage)
		assert.Equal(t, "test-stage", stage.Name())
		assert.Equal(t, pipeline.StageTypePython, stage.Type())
		assert.Equal(t, "test_udf", stage.udfName)
		assert.Equal(t, "1.0.0", stage.udfVersion)
		assert.Len(t, stage.parameters, 2)
	})

	t.Run("MissingUDFName", func(t *testing.T) {
		config := map[string]interface{}{
			"udf_version": "1.0.0",
		}

		stage, err := NewPythonStage("test-stage", config, registry, logger)
		assert.Error(t, err)
		assert.Nil(t, stage)
		assert.Contains(t, err.Error(), "udf_name is required")
	})

	t.Run("MissingVersion", func(t *testing.T) {
		config := map[string]interface{}{
			"udf_name": "test_udf",
		}

		stage, err := NewPythonStage("test-stage", config, registry, logger)
		require.NoError(t, err)
		assert.NotNil(t, stage)
		assert.Equal(t, "", stage.udfVersion) // Default to empty (latest)
	})

	t.Run("MissingParameters", func(t *testing.T) {
		config := map[string]interface{}{
			"udf_name": "test_udf",
		}

		stage, err := NewPythonStage("test-stage", config, registry, logger)
		require.NoError(t, err)
		assert.NotNil(t, stage)
		assert.NotNil(t, stage.parameters)
		assert.Len(t, stage.parameters, 0)
	})

	t.Run("InvalidParameters", func(t *testing.T) {
		config := map[string]interface{}{
			"udf_name":   "test_udf",
			"parameters": "not a map",
		}

		stage, err := NewPythonStage("test-stage", config, registry, logger)
		assert.Error(t, err)
		assert.Nil(t, stage)
		assert.Contains(t, err.Error(), "parameters must be an object")
	})
}

func TestPythonStage_Execute(t *testing.T) {
	logger := zap.NewNop()

	t.Run("BooleanResult_True", func(t *testing.T) {
		registry := &mockUDFRegistry{
			callFunc: func(ctx context.Context, name, version string, docCtx *wasm.DocumentContext, params map[string]wasm.Value) ([]wasm.Value, error) {
				// Return true (document passes filter)
				return []wasm.Value{wasm.NewBoolValue(true)}, nil
			},
		}

		stage, err := NewPythonStage("test-stage", map[string]interface{}{
			"udf_name": "test_udf",
		}, registry, logger)
		require.NoError(t, err)

		input := map[string]interface{}{
			"title": "test document",
			"score": 10.5,
		}

		ctx := &pipeline.StageContext{
			Context: context.Background(),
		}

		output, err := stage.Execute(ctx, input)
		require.NoError(t, err)
		assert.Equal(t, input, output) // Original input returned
	})

	t.Run("BooleanResult_False", func(t *testing.T) {
		registry := &mockUDFRegistry{
			callFunc: func(ctx context.Context, name, version string, docCtx *wasm.DocumentContext, params map[string]wasm.Value) ([]wasm.Value, error) {
				// Return false (document filtered out)
				return []wasm.Value{wasm.NewBoolValue(false)}, nil
			},
		}

		stage, err := NewPythonStage("test-stage", map[string]interface{}{
			"udf_name": "test_udf",
		}, registry, logger)
		require.NoError(t, err)

		input := map[string]interface{}{
			"title": "test document",
		}

		ctx := &pipeline.StageContext{
			Context: context.Background(),
		}

		output, err := stage.Execute(ctx, input)
		assert.Error(t, err)
		assert.Nil(t, output)
		assert.Contains(t, err.Error(), "filtered out")
	})

	t.Run("StringResult_JSON", func(t *testing.T) {
		registry := &mockUDFRegistry{
			callFunc: func(ctx context.Context, name, version string, docCtx *wasm.DocumentContext, params map[string]wasm.Value) ([]wasm.Value, error) {
				// Return JSON string with modifications
				jsonResult := `{"title": "modified title", "new_field": "new value"}`
				return []wasm.Value{wasm.NewStringValue(jsonResult)}, nil
			},
		}

		stage, err := NewPythonStage("test-stage", map[string]interface{}{
			"udf_name": "test_udf",
		}, registry, logger)
		require.NoError(t, err)

		input := map[string]interface{}{
			"title":    "original title",
			"existing": "value",
		}

		ctx := &pipeline.StageContext{
			Context: context.Background(),
		}

		output, err := stage.Execute(ctx, input)
		require.NoError(t, err)

		result := output.(map[string]interface{})
		assert.Equal(t, "modified title", result["title"])       // Overwritten
		assert.Equal(t, "new value", result["new_field"])        // Added
		assert.Equal(t, "value", result["existing"])             // Preserved
	})

	t.Run("NumericResult", func(t *testing.T) {
		registry := &mockUDFRegistry{
			callFunc: func(ctx context.Context, name, version string, docCtx *wasm.DocumentContext, params map[string]wasm.Value) ([]wasm.Value, error) {
				// Return numeric score
				return []wasm.Value{wasm.NewF64Value(42.5)}, nil
			},
		}

		stage, err := NewPythonStage("test-stage", map[string]interface{}{
			"udf_name": "test_udf",
		}, registry, logger)
		require.NoError(t, err)

		input := map[string]interface{}{
			"title": "test document",
		}

		ctx := &pipeline.StageContext{
			Context: context.Background(),
		}

		output, err := stage.Execute(ctx, input)
		require.NoError(t, err)
		assert.Equal(t, float64(42.5), output)
	})

	t.Run("NoResult", func(t *testing.T) {
		registry := &mockUDFRegistry{
			callFunc: func(ctx context.Context, name, version string, docCtx *wasm.DocumentContext, params map[string]wasm.Value) ([]wasm.Value, error) {
				// Return no results
				return []wasm.Value{}, nil
			},
		}

		stage, err := NewPythonStage("test-stage", map[string]interface{}{
			"udf_name": "test_udf",
		}, registry, logger)
		require.NoError(t, err)

		input := map[string]interface{}{
			"title": "test document",
		}

		ctx := &pipeline.StageContext{
			Context: context.Background(),
		}

		output, err := stage.Execute(ctx, input)
		require.NoError(t, err)
		assert.Equal(t, input, output) // Original input returned
	})
}

func TestPythonStage_ParameterConversion(t *testing.T) {
	logger := zap.NewNop()

	registry := &mockUDFRegistry{
		callFunc: func(ctx context.Context, name, version string, docCtx *wasm.DocumentContext, params map[string]wasm.Value) ([]wasm.Value, error) {
			// Verify parameters were converted correctly
			intVal, _ := params["int_param"].AsInt64()
			assert.Equal(t, int64(10), intVal)

			floatVal, _ := params["float_param"].AsFloat64()
			assert.Equal(t, float64(1.5), floatVal)

			strVal, _ := params["string_param"].AsString()
			assert.Equal(t, "test", strVal)

			boolVal, _ := params["bool_param"].AsBool()
			assert.Equal(t, true, boolVal)

			return []wasm.Value{wasm.NewBoolValue(true)}, nil
		},
	}

	stage, err := NewPythonStage("test-stage", map[string]interface{}{
		"udf_name": "test_udf",
		"parameters": map[string]interface{}{
			"int_param":    10,
			"float_param":  1.5,
			"string_param": "test",
			"bool_param":   true,
		},
	}, registry, logger)
	require.NoError(t, err)

	input := map[string]interface{}{
		"title": "test",
	}

	ctx := &pipeline.StageContext{
		Context: context.Background(),
	}

	_, err = stage.Execute(ctx, input)
	require.NoError(t, err)
}

func TestPythonStage_InputConversion(t *testing.T) {
	logger := zap.NewNop()

	registry := &mockUDFRegistry{
		callFunc: func(ctx context.Context, name, version string, docCtx *wasm.DocumentContext, params map[string]wasm.Value) ([]wasm.Value, error) {
			// Verify document context
			title, exists := docCtx.GetFieldString("title")
			assert.True(t, exists)
			assert.Equal(t, "test document", title)

			return []wasm.Value{wasm.NewBoolValue(true)}, nil
		},
	}

	stage, err := NewPythonStage("test-stage", map[string]interface{}{
		"udf_name": "test_udf",
	}, registry, logger)
	require.NoError(t, err)

	t.Run("MapInput", func(t *testing.T) {
		input := map[string]interface{}{
			"title": "test document",
			"_id":   "doc123",
			"_score": 10.5,
		}

		ctx := &pipeline.StageContext{
			Context: context.Background(),
		}

		_, err := stage.Execute(ctx, input)
		require.NoError(t, err)
	})

	t.Run("JSONBytesInput", func(t *testing.T) {
		input := []byte(`{"title": "test document"}`)

		ctx := &pipeline.StageContext{
			Context: context.Background(),
		}

		_, err := stage.Execute(ctx, input)
		require.NoError(t, err)
	})
}

func TestStageBuilder(t *testing.T) {
	logger := zap.NewNop()
	registry := &mockUDFRegistry{}
	builder := NewStageBuilder(registry, logger)

	t.Run("BuildPythonStage", func(t *testing.T) {
		def := &pipeline.StageDefinition{
			Name:    "python-stage",
			Type:    pipeline.StageTypePython,
			Enabled: true,
			Config: map[string]interface{}{
				"udf_name": "test_udf",
			},
		}

		stage, err := builder.BuildStage(def)
		require.NoError(t, err)
		assert.NotNil(t, stage)
		assert.Equal(t, "python-stage", stage.Name())
		assert.Equal(t, pipeline.StageTypePython, stage.Type())
	})

	t.Run("BuildNativeStage", func(t *testing.T) {
		def := &pipeline.StageDefinition{
			Name:    "native-stage",
			Type:    pipeline.StageTypeNative,
			Enabled: true,
			Config: map[string]interface{}{
				"function": "test_func",
			},
		}

		stage, err := builder.BuildStage(def)
		assert.Error(t, err)
		assert.Nil(t, stage)
		assert.Contains(t, err.Error(), "not yet implemented")
	})

	t.Run("BuildMultipleStages", func(t *testing.T) {
		defs := []pipeline.StageDefinition{
			{
				Name:    "stage1",
				Type:    pipeline.StageTypePython,
				Enabled: true,
				Config: map[string]interface{}{
					"udf_name": "udf1",
				},
			},
			{
				Name:    "stage2",
				Type:    pipeline.StageTypePython,
				Enabled: true,
				Config: map[string]interface{}{
					"udf_name": "udf2",
				},
			},
		}

		stages, err := builder.BuildStages(defs)
		require.NoError(t, err)
		assert.Len(t, stages, 2)
		assert.Equal(t, "stage1", stages[0].Name())
		assert.Equal(t, "stage2", stages[1].Name())
	})

	t.Run("BuildStagesWithError", func(t *testing.T) {
		defs := []pipeline.StageDefinition{
			{
				Name:    "stage1",
				Type:    pipeline.StageTypePython,
				Enabled: true,
				Config: map[string]interface{}{
					// Missing udf_name
				},
			},
		}

		stages, err := builder.BuildStages(defs)
		assert.Error(t, err)
		assert.Nil(t, stages)
		assert.Contains(t, err.Error(), "failed to build stage")
	})
}
