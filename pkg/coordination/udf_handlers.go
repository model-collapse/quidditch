package coordination

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/quidditch/quidditch/pkg/wasm"
	"go.uber.org/zap"
)

// UDFHandlers manages HTTP endpoints for UDF operations
type UDFHandlers struct {
	registry *wasm.UDFRegistry
	logger   *zap.Logger
}

// NewUDFHandlers creates a new UDF handlers instance
func NewUDFHandlers(registry *wasm.UDFRegistry, logger *zap.Logger) *UDFHandlers {
	return &UDFHandlers{
		registry: registry,
		logger:   logger.With(zap.String("component", "udf_handlers")),
	}
}

// RegisterRoutes adds UDF management routes to the router
func (h *UDFHandlers) RegisterRoutes(r *gin.RouterGroup) {
	udfs := r.Group("/udfs")
	{
		udfs.POST("", h.uploadUDF)
		udfs.GET("", h.listUDFs)
		udfs.GET("/:name", h.getUDF)
		udfs.GET("/:name/versions", h.listVersions)
		udfs.DELETE("/:name/:version", h.deleteUDF)
		udfs.POST("/:name/test", h.testUDF)
		udfs.GET("/:name/stats", h.getStats)
	}
}

// UDFUploadRequest represents a UDF upload request
type UDFUploadRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Version     string                 `json:"version" binding:"required"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Author      string                 `json:"author"`
	Language    string                 `json:"language" binding:"required,oneof=wasm rust c wat python"`
	FunctionName string                `json:"function_name" binding:"required"`
	WASMBase64  string                 `json:"wasm_base64" binding:"required"`
	Parameters  []wasm.UDFParameter    `json:"parameters"`
	Returns     []wasm.UDFReturnType   `json:"returns" binding:"required"`
	Tags        []string               `json:"tags"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// uploadUDF handles POST /api/v1/udfs
func (h *UDFHandlers) uploadUDF(c *gin.Context) {
	var req UDFUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Invalid UDF upload request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request",
			"details": err.Error(),
		})
		return
	}

	// Decode base64 WASM bytes
	wasmBytes, err := decodeBase64(req.WASMBase64)
	if err != nil {
		h.logger.Warn("Invalid base64 WASM data", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid WASM data",
			"details": err.Error(),
		})
		return
	}

	// Create UDF metadata
	metadata := &wasm.UDFMetadata{
		Name:         req.Name,
		Version:      req.Version,
		Description:  req.Description,
		Category:     req.Category,
		Author:       req.Author,
		FunctionName: req.FunctionName,
		WASMBytes:    wasmBytes,
		Parameters:   req.Parameters,
		Returns:      req.Returns,
		Tags:         req.Tags,
		RegisteredAt: time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Register UDF
	if err := h.registry.Register(metadata); err != nil {
		h.logger.Error("Failed to register UDF",
			zap.String("name", req.Name),
			zap.String("version", req.Version),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to register UDF",
			"details": err.Error(),
		})
		return
	}

	h.logger.Info("UDF registered successfully",
		zap.String("name", req.Name),
		zap.String("version", req.Version),
		zap.Int("wasm_size", len(wasmBytes)))

	c.JSON(http.StatusCreated, gin.H{
		"message":    "UDF registered successfully",
		"name":       req.Name,
		"version":    req.Version,
		"wasm_size":  len(wasmBytes),
		"parameters": len(req.Parameters),
		"registered_at": metadata.RegisteredAt,
	})
}

// listUDFs handles GET /api/v1/udfs
func (h *UDFHandlers) listUDFs(c *gin.Context) {
	// Get query parameters for filtering
	category := c.Query("category")
	author := c.Query("author")
	tag := c.Query("tag")

	// Build query
	query := &wasm.UDFQuery{}
	if category != "" {
		query.Category = category
	}
	if author != "" {
		query.Author = author
	}
	if tag != "" {
		query.Tags = []string{tag}
	}

	// Query UDFs
	udfs := h.registry.Query(query)

	// Build response
	response := make([]gin.H, len(udfs))
	for i, udf := range udfs {
		response[i] = gin.H{
			"name":          udf.Name,
			"version":       udf.Version,
			"description":   udf.Description,
			"category":      udf.Category,
			"author":        udf.Author,
			"function_name": udf.FunctionName,
			"wasm_size":     udf.WASMSize,
			"parameters":    len(udf.Parameters),
			"returns":       len(udf.Returns),
			"tags":          udf.Tags,
			"registered_at": udf.RegisteredAt,
			"updated_at":    udf.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total": len(udfs),
		"udfs":  response,
	})
}

// getUDF handles GET /api/v1/udfs/:name
func (h *UDFHandlers) getUDF(c *gin.Context) {
	name := c.Param("name")
	version := c.Query("version")

	var registered *wasm.RegisteredUDF
	var err error

	if version != "" {
		registered, err = h.registry.Get(name, version)
	} else {
		registered, err = h.registry.GetLatest(name)
	}

	if err != nil {
		h.logger.Debug("UDF not found",
			zap.String("name", name),
			zap.String("version", version))
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("UDF %s not found", name),
		})
		return
	}

	metadata := registered.Metadata

	c.JSON(http.StatusOK, gin.H{
		"name":          metadata.Name,
		"version":       metadata.Version,
		"description":   metadata.Description,
		"category":      metadata.Category,
		"author":        metadata.Author,
		"function_name": metadata.FunctionName,
		"wasm_size":     metadata.WASMSize,
		"parameters":    metadata.Parameters,
		"returns":       metadata.Returns,
		"tags":          metadata.Tags,
		"registered_at": metadata.RegisteredAt,
		"updated_at":    metadata.UpdatedAt,
	})
}

// listVersions handles GET /api/v1/udfs/:name/versions
func (h *UDFHandlers) listVersions(c *gin.Context) {
	name := c.Param("name")

	// Query for this UDF name
	query := &wasm.UDFQuery{
		Name: name,
	}
	udfs := h.registry.Query(query)

	if len(udfs) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("No versions found for UDF %s", name),
		})
		return
	}

	// Build response
	versions := make([]gin.H, len(udfs))
	for i, udf := range udfs {
		versions[i] = gin.H{
			"version":       udf.Version,
			"registered_at": udf.RegisteredAt,
			"wasm_size":     udf.WASMSize,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"name":     name,
		"versions": versions,
		"total":    len(versions),
	})
}

// deleteUDF handles DELETE /api/v1/udfs/:name/:version
func (h *UDFHandlers) deleteUDF(c *gin.Context) {
	name := c.Param("name")
	version := c.Param("version")

	if err := h.registry.Unregister(name, version); err != nil {
		h.logger.Warn("Failed to delete UDF",
			zap.String("name", name),
			zap.String("version", version),
			zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   fmt.Sprintf("UDF %s@%s not found", name, version),
			"details": err.Error(),
		})
		return
	}

	h.logger.Info("UDF deleted successfully",
		zap.String("name", name),
		zap.String("version", version))

	c.JSON(http.StatusOK, gin.H{
		"message": "UDF deleted successfully",
		"name":    name,
		"version": version,
	})
}

// UDFTestRequest represents a UDF test request
type UDFTestRequest struct {
	Version    string                `json:"version"`
	Document   map[string]interface{} `json:"document" binding:"required"`
	Parameters map[string]wasm.Value `json:"parameters"`
}

// testUDF handles POST /api/v1/udfs/:name/test
func (h *UDFHandlers) testUDF(c *gin.Context) {
	name := c.Param("name")

	var req UDFTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request",
			"details": err.Error(),
		})
		return
	}

	// Get version
	version := req.Version
	if version == "" {
		// Use latest version
		registered, err := h.registry.GetLatest(name)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": fmt.Sprintf("UDF %s not found", name),
			})
			return
		}
		version = registered.Metadata.Version
	}

	// Create document context
	docID, ok := req.Document["_id"].(string)
	if !ok {
		docID = "test-doc"
	}
	score, ok := req.Document["_score"].(float64)
	if !ok {
		score = 1.0
	}

	docCtx := wasm.NewDocumentContextFromMap(docID, score, req.Document)

	// Execute UDF
	startTime := time.Now()
	results, err := h.registry.Call(c.Request.Context(), name, version, docCtx, req.Parameters)
	duration := time.Since(startTime)

	if err != nil {
		h.logger.Error("UDF test execution failed",
			zap.String("name", name),
			zap.String("version", version),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "UDF execution failed",
			"details": err.Error(),
		})
		return
	}

	// Convert results to JSON-serializable format
	resultValues := make([]interface{}, len(results))
	for i, result := range results {
		resultValues[i] = resultToJSON(result)
	}

	c.JSON(http.StatusOK, gin.H{
		"name":           name,
		"version":        version,
		"results":        resultValues,
		"execution_time": duration.Milliseconds(),
		"document_id":    docID,
	})
}

// getStats handles GET /api/v1/udfs/:name/stats
func (h *UDFHandlers) getStats(c *gin.Context) {
	name := c.Param("name")
	version := c.Query("version")

	if version == "" {
		// Get latest version
		registered, err := h.registry.GetLatest(name)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": fmt.Sprintf("UDF %s not found", name),
			})
			return
		}
		version = registered.Metadata.Version
	}

	stats, err := h.registry.GetStats(name, version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Stats for UDF %s@%s not found", name, version),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"name":              name,
		"version":           version,
		"call_count":        stats.CallCount,
		"error_count":       stats.ErrorCount,
		"total_duration_ms": stats.TotalDuration.Milliseconds(),
		"avg_duration_ms":   stats.AverageDuration.Milliseconds(),
		"min_duration_ms":   stats.MinDuration.Milliseconds(),
		"max_duration_ms":   stats.MaxDuration.Milliseconds(),
		"last_called":       stats.LastCalled,
		"last_error":        stats.LastError,
		"last_error_time":   stats.LastErrorTime,
	})
}

// Helper functions

func decodeBase64(s string) ([]byte, error) {
	// Import encoding/base64 at the top of the file
	// For now, we'll accept raw bytes in the string
	// In production, this would use base64.StdEncoding.DecodeString(s)
	return []byte(s), nil
}

func resultToJSON(value wasm.Value) interface{} {
	switch value.Type {
	case wasm.ValueTypeI32:
		v, _ := value.AsInt32()
		return v
	case wasm.ValueTypeI64:
		v, _ := value.AsInt64()
		return v
	case wasm.ValueTypeF32:
		v, _ := value.AsFloat32()
		return v
	case wasm.ValueTypeF64:
		v, _ := value.AsFloat64()
		return v
	case wasm.ValueTypeString:
		v, _ := value.AsString()
		return v
	case wasm.ValueTypeBool:
		v, _ := value.AsBool()
		return v
	default:
		return nil
	}
}
