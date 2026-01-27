// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package pipeline

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"
)

// PipelineType defines when a pipeline executes
type PipelineType string

const (
	// PipelineTypeQuery executes before search (query pre-processing)
	PipelineTypeQuery PipelineType = "query"

	// PipelineTypeDocument executes during indexing (document transformation)
	PipelineTypeDocument PipelineType = "document"

	// PipelineTypeResult executes after search (result post-processing)
	PipelineTypeResult PipelineType = "result"
)

// StageType defines how a stage is implemented
type StageType string

const (
	// StageTypePython executes via WASM UDF
	StageTypePython StageType = "python"

	// StageTypeNative executes pure Go code
	StageTypeNative StageType = "native"

	// StageTypeComposite chains multiple stages
	StageTypeComposite StageType = "composite"
)

// Pipeline represents a sequence of processing stages
type Pipeline interface {
	// Name returns the pipeline identifier
	Name() string

	// Version returns the pipeline version
	Version() string

	// Type returns when the pipeline executes (query, document, or result)
	Type() PipelineType

	// Description returns human-readable description
	Description() string

	// Stages returns the ordered list of stages
	Stages() []Stage

	// Execute runs the pipeline on input data
	// Input type depends on pipeline type:
	//   - Query: *parser.SearchRequest
	//   - Document: map[string]interface{}
	//   - Result: *SearchResult (with original request in context)
	Execute(ctx context.Context, input interface{}) (interface{}, error)

	// Metadata returns custom pipeline metadata
	Metadata() map[string]interface{}
}

// Stage represents a single processing step in a pipeline
type Stage interface {
	// Name returns the stage identifier
	Name() string

	// Type returns the stage implementation type
	Type() StageType

	// Execute processes input data and returns transformed output
	Execute(ctx *StageContext, input interface{}) (interface{}, error)

	// Config returns stage-specific configuration
	Config() map[string]interface{}
}

// StageContext provides execution context for a stage
type StageContext struct {
	// PipelineName is the name of the executing pipeline
	PipelineName string

	// StageName is the name of the current stage
	StageName string

	// StageIndex is the position in the pipeline (0-based)
	StageIndex int

	// TotalStages is the total number of stages
	TotalStages int

	// Metadata contains pipeline and stage metadata
	Metadata map[string]interface{}

	// Logger for structured logging
	Logger *zap.Logger

	// StartTime when stage execution began
	StartTime time.Time

	// Context for cancellation and deadlines
	Context context.Context
}

// PipelineDefinition is a serializable pipeline configuration
type PipelineDefinition struct {
	// Name is the unique pipeline identifier
	Name string `json:"name" binding:"required"`

	// Version for pipeline versioning (e.g., "1.0.0")
	Version string `json:"version" binding:"required"`

	// Type defines when the pipeline executes
	Type PipelineType `json:"type" binding:"required,oneof=query document result"`

	// Description explains what the pipeline does
	Description string `json:"description"`

	// Stages are the ordered processing steps
	Stages []StageDefinition `json:"stages" binding:"required,min=1"`

	// Metadata contains custom pipeline data
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Enabled controls whether the pipeline is active
	Enabled bool `json:"enabled"`

	// OnFailure defines behavior when a stage fails
	OnFailure FailurePolicy `json:"on_failure,omitempty"`

	// Timeout for the entire pipeline execution
	Timeout *time.Duration `json:"timeout,omitempty"`

	// Created timestamp when pipeline was registered
	Created time.Time `json:"created,omitempty"`

	// Updated timestamp when pipeline was last modified
	Updated time.Time `json:"updated,omitempty"`
}

// StageDefinition is a serializable stage configuration
type StageDefinition struct {
	// Name is the stage identifier within the pipeline
	Name string `json:"name" binding:"required"`

	// Type defines the stage implementation (python, native, composite)
	Type StageType `json:"type" binding:"required,oneof=python native composite"`

	// Config contains stage-specific configuration
	//
	// For python stages:
	//   - udf_name: Name of the UDF to execute
	//   - udf_version: Version of the UDF (optional, uses latest if omitted)
	//   - parameters: Parameters passed to the UDF
	//
	// For native stages:
	//   - function: Name of the built-in function to execute
	//   - parameters: Parameters for the function
	//
	// For composite stages:
	//   - stages: Nested list of stage definitions
	Config map[string]interface{} `json:"config" binding:"required"`

	// OnFailure defines behavior when this stage fails
	OnFailure FailurePolicy `json:"on_failure,omitempty"`

	// Timeout for this stage execution
	Timeout *time.Duration `json:"timeout,omitempty"`

	// Enabled controls whether this stage is active
	Enabled bool `json:"enabled"`
}

// FailurePolicy defines behavior when a pipeline or stage fails
type FailurePolicy string

const (
	// FailurePolicyContinue logs the error and continues with original input
	FailurePolicyContinue FailurePolicy = "continue"

	// FailurePolicyAbort stops pipeline execution and returns an error
	FailurePolicyAbort FailurePolicy = "abort"

	// FailurePolicyRetry attempts to re-execute the stage
	FailurePolicyRetry FailurePolicy = "retry"
)

// PipelineStats tracks pipeline execution metrics
type PipelineStats struct {
	// Name of the pipeline
	Name string `json:"name"`

	// TotalExecutions is the total number of times the pipeline has run
	TotalExecutions int64 `json:"total_executions"`

	// SuccessfulExecutions is the number of successful runs
	SuccessfulExecutions int64 `json:"successful_executions"`

	// FailedExecutions is the number of failed runs
	FailedExecutions int64 `json:"failed_executions"`

	// AverageDuration is the mean execution time
	AverageDuration time.Duration `json:"average_duration"`

	// P50Duration is the 50th percentile (median) execution time
	P50Duration time.Duration `json:"p50_duration"`

	// P95Duration is the 95th percentile execution time
	P95Duration time.Duration `json:"p95_duration"`

	// P99Duration is the 99th percentile execution time
	P99Duration time.Duration `json:"p99_duration"`

	// LastExecuted is the timestamp of the last execution
	LastExecuted time.Time `json:"last_executed"`

	// StageStats contains per-stage execution metrics
	StageStats []StageStats `json:"stage_stats"`
}

// StageStats tracks individual stage execution metrics
type StageStats struct {
	// Name of the stage
	Name string `json:"name"`

	// TotalExecutions is the total number of times the stage has run
	TotalExecutions int64 `json:"total_executions"`

	// SuccessfulExecutions is the number of successful runs
	SuccessfulExecutions int64 `json:"successful_executions"`

	// FailedExecutions is the number of failed runs
	FailedExecutions int64 `json:"failed_executions"`

	// AverageDuration is the mean execution time
	AverageDuration time.Duration `json:"average_duration"`

	// LastError is the most recent error message
	LastError string `json:"last_error,omitempty"`
}

// PipelineError represents a pipeline execution error
type PipelineError struct {
	// PipelineName is the name of the failed pipeline
	PipelineName string

	// StageName is the name of the failed stage (if applicable)
	StageName string

	// StageIndex is the index of the failed stage
	StageIndex int

	// Message is the error message
	Message string

	// Cause is the underlying error
	Cause error

	// Timestamp when the error occurred
	Timestamp time.Time
}

// Error implements the error interface
func (e *PipelineError) Error() string {
	if e.StageName != "" {
		data, _ := json.Marshal(map[string]interface{}{
			"pipeline": e.PipelineName,
			"stage":    e.StageName,
			"index":    e.StageIndex,
			"message":  e.Message,
			"cause":    e.Cause.Error(),
			"time":     e.Timestamp,
		})
		return string(data)
	}
	data, _ := json.Marshal(map[string]interface{}{
		"pipeline": e.PipelineName,
		"message":  e.Message,
		"cause":    e.Cause.Error(),
		"time":     e.Timestamp,
	})
	return string(data)
}

// Unwrap returns the underlying error for error wrapping
func (e *PipelineError) Unwrap() error {
	return e.Cause
}

// ValidationError represents a pipeline configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	data, _ := json.Marshal(map[string]string{
		"field":   e.Field,
		"message": e.Message,
	})
	return string(data)
}
