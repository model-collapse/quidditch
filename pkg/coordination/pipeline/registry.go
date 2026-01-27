// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package pipeline

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Registry manages pipeline registration and execution
type Registry struct {
	// pipelines maps pipeline name to implementation
	pipelines map[string]*pipelineImpl

	// indexPipelines maps index name -> pipeline type -> pipeline name
	indexPipelines map[string]map[PipelineType]string

	// stats tracks pipeline execution statistics
	stats map[string]*PipelineStats

	// mu protects all maps
	mu sync.RWMutex

	logger *zap.Logger
}

// NewRegistry creates a new pipeline registry
func NewRegistry(logger *zap.Logger) *Registry {
	return &Registry{
		pipelines:      make(map[string]*pipelineImpl),
		indexPipelines: make(map[string]map[PipelineType]string),
		stats:          make(map[string]*PipelineStats),
		logger:         logger,
	}
}

// Register registers a new pipeline
func (r *Registry) Register(def *PipelineDefinition) error {
	if err := r.validatePipeline(def); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if pipeline already exists
	if _, exists := r.pipelines[def.Name]; exists {
		return &ValidationError{
			Field:   "name",
			Message: fmt.Sprintf("pipeline '%s' already exists", def.Name),
		}
	}

	// Create pipeline implementation
	impl := &pipelineImpl{
		def:    def,
		stages: []Stage{}, // Will be populated when stages are created
		logger: r.logger.With(zap.String("pipeline", def.Name)),
	}

	// Store pipeline
	r.pipelines[def.Name] = impl

	// Initialize statistics
	r.stats[def.Name] = &PipelineStats{
		Name:            def.Name,
		StageStats:      make([]StageStats, len(def.Stages)),
		LastExecuted:    time.Time{},
		AverageDuration: 0,
		P50Duration:     0,
		P95Duration:     0,
		P99Duration:     0,
	}

	// Set timestamps
	now := time.Now()
	def.Created = now
	def.Updated = now

	r.logger.Info("Pipeline registered",
		zap.String("name", def.Name),
		zap.String("version", def.Version),
		zap.String("type", string(def.Type)),
		zap.Int("stages", len(def.Stages)))

	return nil
}

// Get retrieves a pipeline by name
func (r *Registry) Get(name string) (Pipeline, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	impl, exists := r.pipelines[name]
	if !exists {
		return nil, fmt.Errorf("pipeline '%s' not found", name)
	}

	return impl, nil
}

// List returns all pipelines, optionally filtered by type
func (r *Registry) List(filterType PipelineType) []*PipelineDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*PipelineDefinition

	for _, impl := range r.pipelines {
		// Apply filter if specified
		if filterType != "" && impl.def.Type != filterType {
			continue
		}

		// Create a copy of the definition
		defCopy := *impl.def
		result = append(result, &defCopy)
	}

	return result
}

// Unregister removes a pipeline from the registry
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if pipeline exists
	if _, exists := r.pipelines[name]; !exists {
		return fmt.Errorf("pipeline '%s' not found", name)
	}

	// Check if pipeline is associated with any indexes
	for indexName, pipelineMap := range r.indexPipelines {
		for pipelineType, pipelineName := range pipelineMap {
			if pipelineName == name {
				return fmt.Errorf("cannot delete pipeline '%s': still associated with index '%s' for type '%s'",
					name, indexName, pipelineType)
			}
		}
	}

	// Remove pipeline
	delete(r.pipelines, name)
	delete(r.stats, name)

	r.logger.Info("Pipeline unregistered", zap.String("name", name))

	return nil
}

// AssociatePipeline associates a pipeline with an index for a specific type
func (r *Registry) AssociatePipeline(indexName string, pipelineType PipelineType, pipelineName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Verify pipeline exists
	impl, exists := r.pipelines[pipelineName]
	if !exists {
		return fmt.Errorf("pipeline '%s' not found", pipelineName)
	}

	// Verify pipeline type matches
	if impl.def.Type != pipelineType {
		return &ValidationError{
			Field: "type",
			Message: fmt.Sprintf("pipeline '%s' is type '%s', cannot associate with type '%s'",
				pipelineName, impl.def.Type, pipelineType),
		}
	}

	// Initialize index pipeline map if needed
	if r.indexPipelines[indexName] == nil {
		r.indexPipelines[indexName] = make(map[PipelineType]string)
	}

	// Associate pipeline
	r.indexPipelines[indexName][pipelineType] = pipelineName

	r.logger.Info("Pipeline associated with index",
		zap.String("pipeline", pipelineName),
		zap.String("index", indexName),
		zap.String("type", string(pipelineType)))

	return nil
}

// GetPipelineForIndex retrieves the pipeline associated with an index for a specific type
func (r *Registry) GetPipelineForIndex(indexName string, pipelineType PipelineType) (Pipeline, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get index pipeline map
	pipelineMap, exists := r.indexPipelines[indexName]
	if !exists {
		return nil, fmt.Errorf("no pipelines configured for index '%s'", indexName)
	}

	// Get pipeline name for type
	pipelineName, exists := pipelineMap[pipelineType]
	if !exists {
		return nil, fmt.Errorf("no pipeline configured for index '%s' and type '%s'",
			indexName, pipelineType)
	}

	// Get pipeline implementation
	impl, exists := r.pipelines[pipelineName]
	if !exists {
		// This should not happen, but handle it gracefully
		return nil, fmt.Errorf("pipeline '%s' not found (inconsistent state)", pipelineName)
	}

	return impl, nil
}

// GetStats retrieves execution statistics for a pipeline
func (r *Registry) GetStats(name string) (*PipelineStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats, exists := r.stats[name]
	if !exists {
		return nil, fmt.Errorf("statistics for pipeline '%s' not found", name)
	}

	// Return a copy
	statsCopy := *stats
	statsCopy.StageStats = make([]StageStats, len(stats.StageStats))
	copy(statsCopy.StageStats, stats.StageStats)

	return &statsCopy, nil
}

// UpdateStats updates statistics after pipeline execution
func (r *Registry) UpdateStats(name string, duration time.Duration, success bool, stageStats []StageStats) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stats, exists := r.stats[name]
	if !exists {
		return
	}

	// Update totals
	stats.TotalExecutions++
	if success {
		stats.SuccessfulExecutions++
	} else {
		stats.FailedExecutions++
	}

	// Update last executed
	stats.LastExecuted = time.Now()

	// Update average duration (simple moving average)
	if stats.TotalExecutions == 1 {
		stats.AverageDuration = duration
	} else {
		// Weighted moving average
		stats.AverageDuration = (stats.AverageDuration*time.Duration(stats.TotalExecutions-1) + duration) /
			time.Duration(stats.TotalExecutions)
	}

	// Update percentiles (simplified - in production, use histogram)
	// For now, use simple approximations
	stats.P50Duration = stats.AverageDuration
	stats.P95Duration = time.Duration(float64(stats.AverageDuration) * 1.5)
	stats.P99Duration = time.Duration(float64(stats.AverageDuration) * 2.0)

	// Update stage statistics
	for i, stageStat := range stageStats {
		if i >= len(stats.StageStats) {
			stats.StageStats = append(stats.StageStats, stageStat)
		} else {
			// Merge statistics
			existing := &stats.StageStats[i]
			existing.Name = stageStat.Name
			existing.TotalExecutions++
			if stageStat.SuccessfulExecutions > 0 {
				existing.SuccessfulExecutions++
			} else {
				existing.FailedExecutions++
				existing.LastError = stageStat.LastError
			}

			// Update average duration
			if existing.TotalExecutions == 1 {
				existing.AverageDuration = stageStat.AverageDuration
			} else {
				existing.AverageDuration = (existing.AverageDuration*time.Duration(existing.TotalExecutions-1) +
					stageStat.AverageDuration) / time.Duration(existing.TotalExecutions)
			}
		}
	}
}

// validatePipeline validates a pipeline definition
func (r *Registry) validatePipeline(def *PipelineDefinition) error {
	// Validate required fields
	if def.Name == "" {
		return &ValidationError{Field: "name", Message: "pipeline name is required"}
	}

	if def.Version == "" {
		return &ValidationError{Field: "version", Message: "pipeline version is required"}
	}

	if def.Type == "" {
		return &ValidationError{Field: "type", Message: "pipeline type is required"}
	}

	// Validate pipeline type
	validTypes := map[PipelineType]bool{
		PipelineTypeQuery:    true,
		PipelineTypeDocument: true,
		PipelineTypeResult:   true,
	}
	if !validTypes[def.Type] {
		return &ValidationError{
			Field:   "type",
			Message: fmt.Sprintf("invalid pipeline type '%s', must be one of: query, document, result", def.Type),
		}
	}

	// Validate stages
	if len(def.Stages) == 0 {
		return &ValidationError{Field: "stages", Message: "pipeline must have at least one stage"}
	}

	// Track stage names for uniqueness check
	stageNames := make(map[string]bool)

	for i, stage := range def.Stages {
		if err := r.validateStage(&stage, i, stageNames); err != nil {
			return err
		}
		stageNames[stage.Name] = true
	}

	return nil
}

// validateStage validates a stage definition
func (r *Registry) validateStage(stage *StageDefinition, index int, stageNames map[string]bool) error {
	// Validate stage name
	if stage.Name == "" {
		return &ValidationError{
			Field:   fmt.Sprintf("stages[%d].name", index),
			Message: "stage name is required",
		}
	}

	// Check stage name uniqueness
	if stageNames[stage.Name] {
		return &ValidationError{
			Field:   fmt.Sprintf("stages[%d].name", index),
			Message: fmt.Sprintf("duplicate stage name '%s'", stage.Name),
		}
	}

	// Validate stage type
	if stage.Type == "" {
		return &ValidationError{
			Field:   fmt.Sprintf("stages[%d].type", index),
			Message: "stage type is required",
		}
	}

	validStageTypes := map[StageType]bool{
		StageTypePython:    true,
		StageTypeNative:    true,
		StageTypeComposite: true,
	}
	if !validStageTypes[stage.Type] {
		return &ValidationError{
			Field: fmt.Sprintf("stages[%d].type", index),
			Message: fmt.Sprintf("invalid stage type '%s', must be one of: python, native, composite",
				stage.Type),
		}
	}

	// Validate config
	if stage.Config == nil {
		return &ValidationError{
			Field:   fmt.Sprintf("stages[%d].config", index),
			Message: "stage config is required",
		}
	}

	// Type-specific validation
	switch stage.Type {
	case StageTypePython:
		if err := r.validatePythonStageConfig(stage.Config, index); err != nil {
			return err
		}
	case StageTypeNative:
		if err := r.validateNativeStageConfig(stage.Config, index); err != nil {
			return err
		}
	case StageTypeComposite:
		if err := r.validateCompositeStageConfig(stage.Config, index); err != nil {
			return err
		}
	}

	return nil
}

// validatePythonStageConfig validates Python stage configuration
func (r *Registry) validatePythonStageConfig(config map[string]interface{}, index int) error {
	// udf_name is required
	udfName, ok := config["udf_name"]
	if !ok {
		return &ValidationError{
			Field:   fmt.Sprintf("stages[%d].config.udf_name", index),
			Message: "udf_name is required for python stages",
		}
	}

	// Verify it's a string
	if _, ok := udfName.(string); !ok {
		return &ValidationError{
			Field:   fmt.Sprintf("stages[%d].config.udf_name", index),
			Message: "udf_name must be a string",
		}
	}

	// udf_version is optional but must be string if present
	if udfVersion, ok := config["udf_version"]; ok {
		if _, ok := udfVersion.(string); !ok {
			return &ValidationError{
				Field:   fmt.Sprintf("stages[%d].config.udf_version", index),
				Message: "udf_version must be a string",
			}
		}
	}

	// parameters is optional but must be map if present
	if params, ok := config["parameters"]; ok {
		if _, ok := params.(map[string]interface{}); !ok {
			return &ValidationError{
				Field:   fmt.Sprintf("stages[%d].config.parameters", index),
				Message: "parameters must be an object",
			}
		}
	}

	return nil
}

// validateNativeStageConfig validates native stage configuration
func (r *Registry) validateNativeStageConfig(config map[string]interface{}, index int) error {
	// function is required
	function, ok := config["function"]
	if !ok {
		return &ValidationError{
			Field:   fmt.Sprintf("stages[%d].config.function", index),
			Message: "function is required for native stages",
		}
	}

	// Verify it's a string
	if _, ok := function.(string); !ok {
		return &ValidationError{
			Field:   fmt.Sprintf("stages[%d].config.function", index),
			Message: "function must be a string",
		}
	}

	return nil
}

// validateCompositeStageConfig validates composite stage configuration
func (r *Registry) validateCompositeStageConfig(config map[string]interface{}, index int) error {
	// stages is required
	stages, ok := config["stages"]
	if !ok {
		return &ValidationError{
			Field:   fmt.Sprintf("stages[%d].config.stages", index),
			Message: "stages is required for composite stages",
		}
	}

	// Verify it's an array
	stageArray, ok := stages.([]interface{})
	if !ok {
		return &ValidationError{
			Field:   fmt.Sprintf("stages[%d].config.stages", index),
			Message: "stages must be an array",
		}
	}

	if len(stageArray) == 0 {
		return &ValidationError{
			Field:   fmt.Sprintf("stages[%d].config.stages", index),
			Message: "composite stage must have at least one nested stage",
		}
	}

	return nil
}

// DisassociatePipeline removes a pipeline association from an index
func (r *Registry) DisassociatePipeline(indexName string, pipelineType PipelineType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	pipelineMap, exists := r.indexPipelines[indexName]
	if !exists {
		return fmt.Errorf("no pipelines configured for index '%s'", indexName)
	}

	if _, exists := pipelineMap[pipelineType]; !exists {
		return fmt.Errorf("no pipeline configured for index '%s' and type '%s'",
			indexName, pipelineType)
	}

	delete(pipelineMap, pipelineType)

	// Clean up empty map
	if len(pipelineMap) == 0 {
		delete(r.indexPipelines, indexName)
	}

	r.logger.Info("Pipeline disassociated from index",
		zap.String("index", indexName),
		zap.String("type", string(pipelineType)))

	return nil
}
