// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package pipeline

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// pipelineImpl implements the Pipeline interface
type pipelineImpl struct {
	def    *PipelineDefinition
	stages []Stage
	logger *zap.Logger
}

// Name returns the pipeline identifier
func (p *pipelineImpl) Name() string {
	return p.def.Name
}

// Version returns the pipeline version
func (p *pipelineImpl) Version() string {
	return p.def.Version
}

// Type returns when the pipeline executes
func (p *pipelineImpl) Type() PipelineType {
	return p.def.Type
}

// Description returns human-readable description
func (p *pipelineImpl) Description() string {
	return p.def.Description
}

// Stages returns the ordered list of stages
func (p *pipelineImpl) Stages() []Stage {
	return p.stages
}

// Metadata returns custom pipeline metadata
func (p *pipelineImpl) Metadata() map[string]interface{} {
	return p.def.Metadata
}

// Execute runs the pipeline on input data
func (p *pipelineImpl) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	startTime := time.Now()

	// Check if pipeline is enabled
	if !p.def.Enabled {
		p.logger.Warn("Pipeline is disabled, skipping execution")
		return input, nil
	}

	// Create parent context with timeout if configured
	if p.def.Timeout != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *p.def.Timeout)
		defer cancel()
	}

	// Validate input type matches pipeline type
	if err := p.validateInput(input); err != nil {
		return nil, &PipelineError{
			PipelineName: p.def.Name,
			Message:      "invalid input type",
			Cause:        err,
			Timestamp:    time.Now(),
		}
	}

	// Track stage statistics
	stageStats := make([]StageStats, 0, len(p.stages))

	// Execute stages sequentially
	output := input
	for i, stage := range p.stages {
		// Skip disabled stages
		stageDef := &p.def.Stages[i]
		if !stageDef.Enabled {
			p.logger.Debug("Stage disabled, skipping",
				zap.String("stage", stageDef.Name),
				zap.Int("index", i))
			continue
		}

		// Create stage context
		stageCtx := &StageContext{
			PipelineName: p.def.Name,
			StageName:    stageDef.Name,
			StageIndex:   i,
			TotalStages:  len(p.stages),
			Metadata:     p.def.Metadata,
			Logger:       p.logger.With(zap.String("stage", stageDef.Name)),
			StartTime:    time.Now(),
			Context:      ctx,
		}

		// Execute stage with error handling
		stageOutput, stageStat, err := p.executeStage(stage, stageDef, stageCtx, output)

		// Record statistics
		stageStats = append(stageStats, *stageStat)

		// Handle stage failure
		if err != nil {
			// Determine failure policy
			failurePolicy := stageDef.OnFailure
			if failurePolicy == "" {
				failurePolicy = p.def.OnFailure
			}
			if failurePolicy == "" {
				failurePolicy = FailurePolicyAbort // Default
			}

			switch failurePolicy {
			case FailurePolicyContinue:
				// Log error and continue with original input
				p.logger.Warn("Stage failed, continuing with original input",
					zap.String("stage", stageDef.Name),
					zap.Int("index", i),
					zap.String("policy", string(failurePolicy)),
					zap.Error(err))
				// Keep output unchanged, continue to next stage
				continue

			case FailurePolicyRetry:
				// Retry logic (simplified - could be enhanced with retry count)
				p.logger.Warn("Stage failed, retry not yet implemented, aborting",
					zap.String("stage", stageDef.Name),
					zap.Int("index", i),
					zap.String("policy", string(failurePolicy)),
					zap.Error(err))
				return nil, err

			case FailurePolicyAbort:
				fallthrough
			default:
				// Abort pipeline execution
				p.logger.Error("Stage failed, aborting pipeline",
					zap.String("stage", stageDef.Name),
					zap.Int("index", i),
					zap.String("policy", string(failurePolicy)),
					zap.Error(err))
				return nil, err
			}
		}

		// Update output for next stage
		output = stageOutput
	}

	// Record pipeline execution time
	duration := time.Since(startTime)

	p.logger.Info("Pipeline execution completed",
		zap.String("pipeline", p.def.Name),
		zap.Duration("duration", duration),
		zap.Int("stages_executed", len(stageStats)))

	return output, nil
}

// executeStage executes a single stage with timeout and error handling
func (p *pipelineImpl) executeStage(stage Stage, stageDef *StageDefinition,
	ctx *StageContext, input interface{}) (interface{}, *StageStats, error) {

	stageStat := &StageStats{
		Name:                 stageDef.Name,
		TotalExecutions:      1,
		SuccessfulExecutions: 0,
		FailedExecutions:     0,
		AverageDuration:      0,
		LastError:            "",
	}

	// Apply stage timeout if configured
	stageCtx := ctx.Context
	if stageDef.Timeout != nil {
		var cancel context.CancelFunc
		stageCtx, cancel = context.WithTimeout(ctx.Context, *stageDef.Timeout)
		defer cancel()
		ctx.Context = stageCtx
	}

	// Execute stage
	startTime := time.Now()
	output, err := stage.Execute(ctx, input)
	duration := time.Since(startTime)

	// Update statistics
	stageStat.AverageDuration = duration

	if err != nil {
		// Stage failed
		stageStat.FailedExecutions = 1
		stageStat.LastError = err.Error()

		p.logger.Warn("Stage execution failed",
			zap.String("stage", stageDef.Name),
			zap.Int("index", ctx.StageIndex),
			zap.Duration("duration", duration),
			zap.Error(err))

		return nil, stageStat, &PipelineError{
			PipelineName: p.def.Name,
			StageName:    stageDef.Name,
			StageIndex:   ctx.StageIndex,
			Message:      "stage execution failed",
			Cause:        err,
			Timestamp:    time.Now(),
		}
	}

	// Stage succeeded
	stageStat.SuccessfulExecutions = 1

	p.logger.Debug("Stage execution completed",
		zap.String("stage", stageDef.Name),
		zap.Int("index", ctx.StageIndex),
		zap.Duration("duration", duration))

	return output, stageStat, nil
}

// handleStageFailure handles stage failure based on the failure policy
func (p *pipelineImpl) handleStageFailure(stageDef *StageDefinition, stageIndex int,
	err error, originalInput interface{}, stageStats []StageStats) (interface{}, error) {

	// Determine failure policy
	failurePolicy := stageDef.OnFailure
	if failurePolicy == "" {
		failurePolicy = p.def.OnFailure
	}
	if failurePolicy == "" {
		failurePolicy = FailurePolicyAbort // Default
	}

	switch failurePolicy {
	case FailurePolicyContinue:
		// Log error and continue with original input
		p.logger.Warn("Stage failed, continuing with original input",
			zap.String("stage", stageDef.Name),
			zap.Int("index", stageIndex),
			zap.String("policy", string(failurePolicy)),
			zap.Error(err))
		return originalInput, nil

	case FailurePolicyRetry:
		// Retry logic (simplified - could be enhanced with retry count)
		p.logger.Warn("Stage failed, retry not yet implemented",
			zap.String("stage", stageDef.Name),
			zap.Int("index", stageIndex),
			zap.String("policy", string(failurePolicy)),
			zap.Error(err))
		// For now, treat as abort
		return nil, err

	case FailurePolicyAbort:
		fallthrough
	default:
		// Abort pipeline execution
		p.logger.Error("Stage failed, aborting pipeline",
			zap.String("stage", stageDef.Name),
			zap.Int("index", stageIndex),
			zap.String("policy", string(failurePolicy)),
			zap.Error(err))
		return nil, err
	}
}

// validateInput validates that input type matches pipeline type
func (p *pipelineImpl) validateInput(input interface{}) error {
	if input == nil {
		return fmt.Errorf("input cannot be nil")
	}

	// Type validation based on pipeline type
	switch p.def.Type {
	case PipelineTypeQuery:
		// For query pipelines, input should be a SearchRequest or map
		// We'll accept map[string]interface{} for flexibility
		if _, ok := input.(map[string]interface{}); !ok {
			// Also check if it's a pointer to struct (SearchRequest)
			return fmt.Errorf("query pipeline expects map[string]interface{} or SearchRequest, got %T", input)
		}

	case PipelineTypeDocument:
		// For document pipelines, input should be a document (map)
		if _, ok := input.(map[string]interface{}); !ok {
			return fmt.Errorf("document pipeline expects map[string]interface{}, got %T", input)
		}

	case PipelineTypeResult:
		// For result pipelines, input should be a SearchResult or map
		if _, ok := input.(map[string]interface{}); !ok {
			return fmt.Errorf("result pipeline expects map[string]interface{} or SearchResult, got %T", input)
		}

	default:
		return fmt.Errorf("unknown pipeline type: %s", p.def.Type)
	}

	return nil
}

// SetStages sets the pipeline stages (called during registration after stages are created)
func (p *pipelineImpl) SetStages(stages []Stage) {
	p.stages = stages
}

// ExecutorMetrics tracks pipeline executor performance
type ExecutorMetrics struct {
	PipelinesExecuted  int64
	PipelinesFailed    int64
	TotalDuration      time.Duration
	AverageDuration    time.Duration
	StagesExecuted     int64
	StagesFailed       int64
	LastExecutionTime  time.Time
	ExecutionHistogram map[string]int64 // Duration bucket -> count
}

// Executor manages pipeline execution and provides metrics
type Executor struct {
	registry *Registry
	metrics  *ExecutorMetrics
	logger   *zap.Logger
}

// NewExecutor creates a new pipeline executor
func NewExecutor(registry *Registry, logger *zap.Logger) *Executor {
	return &Executor{
		registry: registry,
		metrics: &ExecutorMetrics{
			ExecutionHistogram: make(map[string]int64),
		},
		logger: logger,
	}
}

// ExecutePipeline executes a pipeline by name
func (e *Executor) ExecutePipeline(ctx context.Context, pipelineName string, input interface{}) (interface{}, error) {
	// Get pipeline
	pipeline, err := e.registry.Get(pipelineName)
	if err != nil {
		return nil, err
	}

	// Execute
	startTime := time.Now()
	output, err := pipeline.Execute(ctx, input)
	duration := time.Since(startTime)

	// Update metrics
	e.updateMetrics(pipelineName, duration, err == nil, pipeline.Stages())

	// Update registry statistics
	success := err == nil
	var stageStats []StageStats
	if impl, ok := pipeline.(*pipelineImpl); ok {
		stageStats = make([]StageStats, len(impl.stages))
		for i, stage := range impl.stages {
			stageStats[i] = StageStats{
				Name:                 stage.Name(),
				TotalExecutions:      1,
				SuccessfulExecutions: 0,
				FailedExecutions:     0,
			}
			if success {
				stageStats[i].SuccessfulExecutions = 1
			} else {
				stageStats[i].FailedExecutions = 1
			}
		}
	}
	e.registry.UpdateStats(pipelineName, duration, success, stageStats)

	return output, err
}

// ExecutePipelineForIndex executes the pipeline associated with an index
func (e *Executor) ExecutePipelineForIndex(ctx context.Context, indexName string,
	pipelineType PipelineType, input interface{}) (interface{}, error) {

	// Get pipeline for index
	pipeline, err := e.registry.GetPipelineForIndex(indexName, pipelineType)
	if err != nil {
		return nil, err
	}

	return e.ExecutePipeline(ctx, pipeline.Name(), input)
}

// updateMetrics updates executor metrics
func (e *Executor) updateMetrics(pipelineName string, duration time.Duration, success bool, stages []Stage) {
	e.metrics.PipelinesExecuted++
	if !success {
		e.metrics.PipelinesFailed++
	}

	e.metrics.TotalDuration += duration
	e.metrics.AverageDuration = e.metrics.TotalDuration / time.Duration(e.metrics.PipelinesExecuted)
	e.metrics.LastExecutionTime = time.Now()

	e.metrics.StagesExecuted += int64(len(stages))

	// Update histogram (bucket by 10ms intervals)
	bucket := fmt.Sprintf("%dms", duration.Milliseconds()/10*10)
	e.metrics.ExecutionHistogram[bucket]++
}

// GetMetrics returns current executor metrics
func (e *Executor) GetMetrics() *ExecutorMetrics {
	// Return a copy
	metricsCopy := *e.metrics
	metricsCopy.ExecutionHistogram = make(map[string]int64)
	for k, v := range e.metrics.ExecutionHistogram {
		metricsCopy.ExecutionHistogram[k] = v
	}
	return &metricsCopy
}
