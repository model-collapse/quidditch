// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package stages

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/quidditch/quidditch/pkg/coordination/pipeline"
	"github.com/quidditch/quidditch/pkg/wasm"
	"go.uber.org/zap"
)

// UDFCaller is an interface for calling UDFs
type UDFCaller interface {
	Call(ctx context.Context, name, version string, docCtx *wasm.DocumentContext, params map[string]wasm.Value) ([]wasm.Value, error)
}

// PythonStage executes Python UDFs via WASM runtime
type PythonStage struct {
	name       string
	udfName    string
	udfVersion string
	parameters map[string]interface{}
	udfCaller  UDFCaller
	logger     *zap.Logger
}

// NewPythonStage creates a new Python stage
func NewPythonStage(name string, config map[string]interface{}, udfCaller UDFCaller, logger *zap.Logger) (*PythonStage, error) {
	// Extract udf_name (required)
	udfName, ok := config["udf_name"].(string)
	if !ok {
		return nil, fmt.Errorf("udf_name is required and must be a string")
	}

	// Extract udf_version (optional, defaults to empty = latest)
	udfVersion := ""
	if v, ok := config["udf_version"]; ok {
		if vStr, ok := v.(string); ok {
			udfVersion = vStr
		}
	}

	// Extract parameters (optional)
	var parameters map[string]interface{}
	if params, ok := config["parameters"]; ok {
		if paramMap, ok := params.(map[string]interface{}); ok {
			parameters = paramMap
		} else {
			return nil, fmt.Errorf("parameters must be an object")
		}
	} else {
		parameters = make(map[string]interface{})
	}

	return &PythonStage{
		name:       name,
		udfName:    udfName,
		udfVersion: udfVersion,
		parameters: parameters,
		udfCaller:  udfCaller,
		logger:     logger.With(zap.String("stage", name), zap.String("udf", udfName)),
	}, nil
}

// Name returns the stage identifier
func (s *PythonStage) Name() string {
	return s.name
}

// Type returns the stage implementation type
func (s *PythonStage) Type() pipeline.StageType {
	return pipeline.StageTypePython
}

// Config returns stage-specific configuration
func (s *PythonStage) Config() map[string]interface{} {
	return map[string]interface{}{
		"udf_name":    s.udfName,
		"udf_version": s.udfVersion,
		"parameters":  s.parameters,
	}
}

// Execute processes input data and returns transformed output
func (s *PythonStage) Execute(ctx *pipeline.StageContext, input interface{}) (interface{}, error) {
	s.logger.Debug("Executing Python stage",
		zap.String("pipeline", ctx.PipelineName),
		zap.Int("stage_index", ctx.StageIndex))

	// Convert input to DocumentContext
	docCtx, err := s.inputToDocumentContext(input)
	if err != nil {
		return nil, fmt.Errorf("failed to convert input to document context: %w", err)
	}

	// Convert parameters to WASM values
	wasmParams := s.parametersToWasmValues()

	// Execute UDF via caller
	results, err := s.udfCaller.Call(ctx.Context, s.udfName, s.udfVersion, docCtx, wasmParams)
	if err != nil {
		s.logger.Error("UDF execution failed",
			zap.String("udf", s.udfName),
			zap.String("version", s.udfVersion),
			zap.Error(err))
		return nil, fmt.Errorf("UDF '%s' execution failed: %w", s.udfName, err)
	}

	// Convert results back to expected output type
	output, err := s.wasmResultToOutput(results, input)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WASM result: %w", err)
	}

	s.logger.Debug("Python stage execution completed",
		zap.String("udf", s.udfName))

	return output, nil
}

// inputToDocumentContext converts pipeline input to DocumentContext
func (s *PythonStage) inputToDocumentContext(input interface{}) (*wasm.DocumentContext, error) {
	// Convert input to map
	var dataMap map[string]interface{}

	switch v := input.(type) {
	case map[string]interface{}:
		dataMap = v
	case []byte:
		// Try to unmarshal JSON
		if err := json.Unmarshal(v, &dataMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON input: %w", err)
		}
	default:
		// Try to marshal to JSON then unmarshal to map
		jsonBytes, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal input to JSON: %w", err)
		}
		if err := json.Unmarshal(jsonBytes, &dataMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal input: %w", err)
		}
	}

	// Extract document ID and score if present
	documentID := ""
	if id, ok := dataMap["_id"]; ok {
		if idStr, ok := id.(string); ok {
			documentID = idStr
		}
	}

	score := 0.0
	if s, ok := dataMap["_score"]; ok {
		switch v := s.(type) {
		case float64:
			score = v
		case float32:
			score = float64(v)
		case int:
			score = float64(v)
		case int64:
			score = float64(v)
		}
	}

	// Create DocumentContext
	return wasm.NewDocumentContextFromMap(documentID, score, dataMap), nil
}

// parametersToWasmValues converts parameters to WASM Value map
func (s *PythonStage) parametersToWasmValues() map[string]wasm.Value {
	wasmParams := make(map[string]wasm.Value)

	for name, value := range s.parameters {
		wasmValue := s.convertToWasmValue(value)
		wasmParams[name] = wasmValue
	}

	return wasmParams
}

// convertToWasmValue converts a Go value to WASM Value
func (s *PythonStage) convertToWasmValue(value interface{}) wasm.Value {
	switch v := value.(type) {
	case int:
		return wasm.NewI64Value(int64(v))
	case int32:
		return wasm.NewI32Value(v)
	case int64:
		return wasm.NewI64Value(v)
	case float32:
		return wasm.NewF32Value(v)
	case float64:
		return wasm.NewF64Value(v)
	case string:
		return wasm.NewStringValue(v)
	case bool:
		return wasm.NewBoolValue(v)
	default:
		// Try to marshal to JSON string
		if jsonBytes, err := json.Marshal(value); err == nil {
			return wasm.NewStringValue(string(jsonBytes))
		}
		// Fallback to string representation
		return wasm.NewStringValue(fmt.Sprintf("%v", value))
	}
}

// wasmResultToOutput converts WASM result to pipeline output
func (s *PythonStage) wasmResultToOutput(results []wasm.Value, originalInput interface{}) (interface{}, error) {
	if len(results) == 0 {
		// No return value, return original input unchanged
		return originalInput, nil
	}

	// Get first result (most common case)
	result := results[0]

	// Convert based on type
	switch result.Type {
	case wasm.ValueTypeBool:
		// Boolean result - typically used for filters
		boolVal, err := result.AsBool()
		if err != nil {
			return nil, fmt.Errorf("failed to convert result to bool: %w", err)
		}
		if !boolVal {
			// Filter rejected the document
			return nil, fmt.Errorf("document filtered out by UDF")
		}
		// Filter accepted - return original input
		return originalInput, nil

	case wasm.ValueTypeString:
		// String result - typically JSON
		strVal, err := result.AsString()
		if err != nil {
			return nil, fmt.Errorf("failed to convert result to string: %w", err)
		}

		// Try to parse as JSON
		var outputMap map[string]interface{}
		if err := json.Unmarshal([]byte(strVal), &outputMap); err != nil {
			// Not JSON, return as is
			return strVal, nil
		}

		// Merge with original input if it was a map
		if origMap, ok := originalInput.(map[string]interface{}); ok {
			// Copy original
			merged := make(map[string]interface{})
			for k, v := range origMap {
				merged[k] = v
			}
			// Merge output (overwrites existing keys)
			for k, v := range outputMap {
				merged[k] = v
			}
			return merged, nil
		}

		return outputMap, nil

	case wasm.ValueTypeI32:
		val, err := result.AsInt32()
		if err != nil {
			return nil, err
		}
		return val, nil

	case wasm.ValueTypeI64:
		val, err := result.AsInt64()
		if err != nil {
			return nil, err
		}
		return val, nil

	case wasm.ValueTypeF32:
		val, err := result.AsFloat32()
		if err != nil {
			return nil, err
		}
		return val, nil

	case wasm.ValueTypeF64:
		val, err := result.AsFloat64()
		if err != nil {
			return nil, err
		}
		return val, nil

	default:
		return nil, fmt.Errorf("unsupported result type: %v", result.Type)
	}
}

// StageBuilder is a helper for creating stages from definitions
type StageBuilder struct {
	udfCaller UDFCaller
	logger    *zap.Logger
}

// NewStageBuilder creates a new stage builder
func NewStageBuilder(udfCaller UDFCaller, logger *zap.Logger) *StageBuilder {
	return &StageBuilder{
		udfCaller: udfCaller,
		logger:    logger,
	}
}

// BuildStage creates a stage from a stage definition
func (b *StageBuilder) BuildStage(def *pipeline.StageDefinition) (pipeline.Stage, error) {
	switch def.Type {
	case pipeline.StageTypePython:
		return NewPythonStage(def.Name, def.Config, b.udfCaller, b.logger)

	case pipeline.StageTypeNative:
		// TODO: Implement native stages
		return nil, fmt.Errorf("native stages not yet implemented")

	case pipeline.StageTypeComposite:
		// TODO: Implement composite stages
		return nil, fmt.Errorf("composite stages not yet implemented")

	default:
		return nil, fmt.Errorf("unknown stage type: %s", def.Type)
	}
}

// BuildStages creates all stages from stage definitions
func (b *StageBuilder) BuildStages(defs []pipeline.StageDefinition) ([]pipeline.Stage, error) {
	stages := make([]pipeline.Stage, 0, len(defs))

	for i, def := range defs {
		stage, err := b.BuildStage(&def)
		if err != nil {
			return nil, fmt.Errorf("failed to build stage %d (%s): %w", i, def.Name, err)
		}
		stages = append(stages, stage)
	}

	return stages, nil
}
