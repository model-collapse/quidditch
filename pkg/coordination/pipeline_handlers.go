// Copyright 2026 Quidditch Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package coordination

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/quidditch/quidditch/pkg/coordination/pipeline"
	"go.uber.org/zap"
)

// PipelineHandlers manages HTTP endpoints for pipeline operations
type PipelineHandlers struct {
	registry *pipeline.Registry
	executor *pipeline.Executor
	logger   *zap.Logger
}

// NewPipelineHandlers creates a new pipeline handlers instance
func NewPipelineHandlers(registry *pipeline.Registry, executor *pipeline.Executor, logger *zap.Logger) *PipelineHandlers {
	return &PipelineHandlers{
		registry: registry,
		executor: executor,
		logger:   logger.With(zap.String("component", "pipeline_handlers")),
	}
}

// RegisterRoutes adds pipeline management routes to the router
func (h *PipelineHandlers) RegisterRoutes(r *gin.RouterGroup) {
	pipelines := r.Group("/pipelines")
	{
		pipelines.POST("/:name", h.createPipeline)
		pipelines.GET("/:name", h.getPipeline)
		pipelines.DELETE("/:name", h.deletePipeline)
		pipelines.GET("", h.listPipelines)
		pipelines.POST("/:name/_execute", h.executePipeline)
		pipelines.GET("/:name/_stats", h.getStats)
	}
}

// PipelineCreateRequest represents a pipeline creation request
type PipelineCreateRequest struct {
	Name        string                      `json:"name" binding:"required"`
	Version     string                      `json:"version" binding:"required"`
	Type        pipeline.PipelineType       `json:"type" binding:"required,oneof=query document result"`
	Description string                      `json:"description"`
	Stages      []pipeline.StageDefinition  `json:"stages" binding:"required,min=1"`
	Metadata    map[string]interface{}      `json:"metadata"`
	Enabled     bool                        `json:"enabled"`
	OnFailure   pipeline.FailurePolicy      `json:"on_failure,omitempty"`
	Timeout     *time.Duration              `json:"timeout,omitempty"`
}

// PipelineResponse represents a pipeline response
type PipelineResponse struct {
	Name        string                      `json:"name"`
	Version     string                      `json:"version"`
	Type        pipeline.PipelineType       `json:"type"`
	Description string                      `json:"description"`
	Stages      []pipeline.StageDefinition  `json:"stages"`
	Metadata    map[string]interface{}      `json:"metadata,omitempty"`
	Enabled     bool                        `json:"enabled"`
	OnFailure   pipeline.FailurePolicy      `json:"on_failure,omitempty"`
	Timeout     *time.Duration              `json:"timeout,omitempty"`
	Created     time.Time                   `json:"created"`
	Updated     time.Time                   `json:"updated"`
}

// PipelineExecuteRequest represents a pipeline test execution request
type PipelineExecuteRequest struct {
	Input interface{} `json:"input" binding:"required"`
}

// PipelineExecuteResponse represents a pipeline test execution response
type PipelineExecuteResponse struct {
	Output   interface{}    `json:"output"`
	Duration time.Duration  `json:"duration_ms"`
	Success  bool           `json:"success"`
	Error    string         `json:"error,omitempty"`
}

// createPipeline handles POST /_pipelines/{name}
func (h *PipelineHandlers) createPipeline(c *gin.Context) {
	pipelineName := c.Param("name")

	var req PipelineCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid pipeline creation request",
			zap.String("name", pipelineName),
			zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request",
			"details": err.Error(),
		})
		return
	}

	// Validate that name in path matches name in body
	if req.Name != pipelineName {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Name mismatch",
			"details": "Pipeline name in URL must match name in request body",
		})
		return
	}

	// Create pipeline definition
	def := &pipeline.PipelineDefinition{
		Name:        req.Name,
		Version:     req.Version,
		Type:        req.Type,
		Description: req.Description,
		Stages:      req.Stages,
		Metadata:    req.Metadata,
		Enabled:     req.Enabled,
		OnFailure:   req.OnFailure,
		Timeout:     req.Timeout,
	}

	// Register pipeline
	if err := h.registry.Register(def); err != nil {
		h.logger.Error("Failed to register pipeline",
			zap.String("name", req.Name),
			zap.String("version", req.Version),
			zap.Error(err))

		// Check if it's a validation error
		if _, ok := err.(*pipeline.ValidationError); ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Validation failed",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to register pipeline",
			"details": err.Error(),
		})
		return
	}

	h.logger.Info("Pipeline created successfully",
		zap.String("name", req.Name),
		zap.String("version", req.Version),
		zap.String("type", string(req.Type)))

	c.JSON(http.StatusCreated, gin.H{
		"acknowledged": true,
		"name":         req.Name,
		"version":      req.Version,
		"type":         req.Type,
	})
}

// getPipeline handles GET /_pipelines/{name}
func (h *PipelineHandlers) getPipeline(c *gin.Context) {
	pipelineName := c.Param("name")

	// Get pipeline from registry
	pipe, err := h.registry.Get(pipelineName)
	if err != nil {
		h.logger.Debug("Pipeline not found",
			zap.String("name", pipelineName),
			zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Pipeline not found",
			"details": err.Error(),
		})
		return
	}

	// Convert to response format
	response := PipelineResponse{
		Name:        pipe.Name(),
		Version:     pipe.Version(),
		Type:        pipe.Type(),
		Description: pipe.Description(),
		Metadata:    pipe.Metadata(),
	}

	// Get the full definition for additional fields
	pipelines := h.registry.List("")
	for _, def := range pipelines {
		if def.Name == pipelineName {
			response.Stages = def.Stages
			response.Enabled = def.Enabled
			response.OnFailure = def.OnFailure
			response.Timeout = def.Timeout
			response.Created = def.Created
			response.Updated = def.Updated
			break
		}
	}

	c.JSON(http.StatusOK, response)
}

// deletePipeline handles DELETE /_pipelines/{name}
func (h *PipelineHandlers) deletePipeline(c *gin.Context) {
	pipelineName := c.Param("name")

	// Unregister pipeline
	if err := h.registry.Unregister(pipelineName); err != nil {
		h.logger.Warn("Failed to delete pipeline",
			zap.String("name", pipelineName),
			zap.Error(err))

		// Check if it's a not found error
		if err.Error() == "pipeline '"+pipelineName+"' not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Pipeline not found",
				"details": err.Error(),
			})
			return
		}

		// Check if it's still associated with indexes
		if err.Error() != "" {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Cannot delete pipeline",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to delete pipeline",
			"details": err.Error(),
		})
		return
	}

	h.logger.Info("Pipeline deleted successfully",
		zap.String("name", pipelineName))

	c.JSON(http.StatusOK, gin.H{
		"acknowledged": true,
		"message":      "Pipeline deleted successfully",
	})
}

// listPipelines handles GET /_pipelines
func (h *PipelineHandlers) listPipelines(c *gin.Context) {
	// Get optional type filter
	typeFilter := c.Query("type")
	var pipelineType pipeline.PipelineType
	if typeFilter != "" {
		pipelineType = pipeline.PipelineType(typeFilter)
	}

	// List pipelines
	pipelines := h.registry.List(pipelineType)

	// Convert to response format
	responses := make([]PipelineResponse, 0, len(pipelines))
	for _, def := range pipelines {
		responses = append(responses, PipelineResponse{
			Name:        def.Name,
			Version:     def.Version,
			Type:        def.Type,
			Description: def.Description,
			Stages:      def.Stages,
			Metadata:    def.Metadata,
			Enabled:     def.Enabled,
			OnFailure:   def.OnFailure,
			Timeout:     def.Timeout,
			Created:     def.Created,
			Updated:     def.Updated,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     len(responses),
		"pipelines": responses,
	})
}

// executePipeline handles POST /_pipelines/{name}/_execute
func (h *PipelineHandlers) executePipeline(c *gin.Context) {
	pipelineName := c.Param("name")

	var req PipelineExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid pipeline execution request",
			zap.String("name", pipelineName),
			zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request",
			"details": err.Error(),
		})
		return
	}

	// Execute pipeline
	startTime := time.Now()
	output, err := h.executor.ExecutePipeline(c.Request.Context(), pipelineName, req.Input)
	duration := time.Since(startTime)

	if err != nil {
		h.logger.Warn("Pipeline execution failed",
			zap.String("name", pipelineName),
			zap.Duration("duration", duration),
			zap.Error(err))

		// Return error but with 200 OK since this is a test execution
		c.JSON(http.StatusOK, PipelineExecuteResponse{
			Output:   nil,
			Duration: duration,
			Success:  false,
			Error:    err.Error(),
		})
		return
	}

	h.logger.Debug("Pipeline executed successfully",
		zap.String("name", pipelineName),
		zap.Duration("duration", duration))

	c.JSON(http.StatusOK, PipelineExecuteResponse{
		Output:   output,
		Duration: duration,
		Success:  true,
	})
}

// getStats handles GET /_pipelines/{name}/_stats
func (h *PipelineHandlers) getStats(c *gin.Context) {
	pipelineName := c.Param("name")

	// Get statistics
	stats, err := h.registry.GetStats(pipelineName)
	if err != nil {
		h.logger.Debug("Pipeline statistics not found",
			zap.String("name", pipelineName),
			zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Pipeline not found",
			"details": err.Error(),
		})
		return
	}

	// Convert durations to milliseconds for JSON response
	c.JSON(http.StatusOK, gin.H{
		"name":                  stats.Name,
		"total_executions":      stats.TotalExecutions,
		"successful_executions": stats.SuccessfulExecutions,
		"failed_executions":     stats.FailedExecutions,
		"average_duration_ms":   stats.AverageDuration.Milliseconds(),
		"p50_duration_ms":       stats.P50Duration.Milliseconds(),
		"p95_duration_ms":       stats.P95Duration.Milliseconds(),
		"p99_duration_ms":       stats.P99Duration.Milliseconds(),
		"last_executed":         stats.LastExecuted,
		"stage_stats":           stats.StageStats,
	})
}
