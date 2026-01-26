package coordination

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/quidditch/quidditch/pkg/common/metrics"
	"github.com/quidditch/quidditch/pkg/coordination/bulk"
	"github.com/quidditch/quidditch/pkg/coordination/executor"
	"github.com/quidditch/quidditch/pkg/coordination/parser"
	"github.com/quidditch/quidditch/pkg/coordination/planner"
	"github.com/quidditch/quidditch/pkg/coordination/router"
	"go.uber.org/zap"
)

// CoordinationNode represents a coordination node in the Quidditch cluster
type CoordinationNode struct {
	cfg            *config.CoordinationConfig
	logger         *zap.Logger
	ginRouter      *gin.Engine
	httpServer     *http.Server
	masterClient   *MasterClient
	queryExecutor  *executor.QueryExecutor
	queryPlanner   *planner.QueryPlanner
	docRouter      *router.DocumentRouter
	queryParser    *parser.QueryParser
	metrics        *metrics.MetricsCollector
	dataClients    map[string]*DataNodeClient
	dataClientsMu  sync.RWMutex
}

// NewCoordinationNode creates a new coordination node
func NewCoordinationNode(cfg *config.CoordinationConfig, logger *zap.Logger) (*CoordinationNode, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Set Gin to release mode for production
	gin.SetMode(gin.ReleaseMode)
	ginRouter := gin.New()
	ginRouter.Use(gin.Recovery())
	ginRouter.Use(ginLogger(logger))

	// Create metrics collector
	metricsCollector := metrics.NewMetricsCollector("coordination")
	ginRouter.Use(metrics.HTTPMetricsMiddleware(metricsCollector))

	// Create master client
	masterClient := NewMasterClient(cfg.MasterAddr, logger)

	// Create data clients map
	dataClients := make(map[string]*DataNodeClient)

	// Create query executor
	queryExecutor := executor.NewQueryExecutor(masterClient, logger)

	// Create query planner
	queryPlanner := planner.NewQueryPlanner(masterClient, logger)

	// Create document router
	// We'll convert dataClients to the interface type needed by router
	dataClientInterfaces := make(map[string]router.DataNodeClient)
	docRouter := router.NewDocumentRouter(masterClient, dataClientInterfaces, logger)

	node := &CoordinationNode{
		cfg:           cfg,
		logger:        logger,
		ginRouter:     ginRouter,
		masterClient:  masterClient,
		queryExecutor: queryExecutor,
		queryPlanner:  queryPlanner,
		docRouter:     docRouter,
		queryParser:   parser.NewQueryParser(),
		metrics:       metricsCollector,
		dataClients:   dataClients,
	}

	// Set up routes
	node.setupRoutes()

	return node, nil
}

// Start starts the coordination node
func (c *CoordinationNode) Start(ctx context.Context) error {
	c.logger.Info("Starting coordination node",
		zap.String("node_id", c.cfg.NodeID),
		zap.Int("rest_port", c.cfg.RESTPort))

	// Connect to master node with retries
	if err := c.connectToMasterWithRetries(ctx); err != nil {
		return fmt.Errorf("failed to connect to master: %w", err)
	}

	// Discover and register data nodes
	if err := c.discoverDataNodes(ctx); err != nil {
		c.logger.Warn("Failed to discover data nodes", zap.Error(err))
		// Don't fail startup - data nodes can be discovered later
	}

	// Start HTTP server
	c.httpServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", c.cfg.BindAddr, c.cfg.RESTPort),
		Handler: c.ginRouter,
	}

	go func() {
		if err := c.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	c.logger.Info("Coordination node started successfully",
		zap.String("rest_api", fmt.Sprintf("http://%s:%d", c.cfg.BindAddr, c.cfg.RESTPort)))

	return nil
}

// Stop stops the coordination node
func (c *CoordinationNode) Stop(ctx context.Context) error {
	c.logger.Info("Stopping coordination node")

	// Stop HTTP server
	if c.httpServer != nil {
		if err := c.httpServer.Shutdown(ctx); err != nil {
			c.logger.Error("Failed to shutdown HTTP server", zap.Error(err))
		}
	}

	// Close master connection
	if c.masterClient != nil {
		if err := c.masterClient.Disconnect(); err != nil {
			c.logger.Warn("Failed to disconnect from master", zap.Error(err))
		}
	}

	return nil
}

// connectToMasterWithRetries establishes connection to master node with retry logic
func (c *CoordinationNode) connectToMasterWithRetries(ctx context.Context) error {
	c.logger.Info("Connecting to master node", zap.String("master_addr", c.cfg.MasterAddr))

	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		if err := c.masterClient.Connect(ctx); err != nil {
			c.logger.Warn("Failed to connect to master, retrying",
				zap.Int("attempt", i+1),
				zap.Int("max_retries", maxRetries),
				zap.Error(err))
			time.Sleep(time.Duration(i+1) * 2 * time.Second) // Exponential backoff
			continue
		}

		c.logger.Info("Successfully connected to master node")
		return nil
	}

	return fmt.Errorf("failed to connect to master after %d retries", maxRetries)
}

// setupRoutes sets up all REST API routes
func (c *CoordinationNode) setupRoutes() {
	// Root endpoint
	c.ginRouter.GET("/", c.handleRoot)

	// Cluster APIs
	c.ginRouter.GET("/_cluster/health", c.handleClusterHealth)
	c.ginRouter.GET("/_cluster/health/:index", c.handleClusterHealth)
	c.ginRouter.GET("/_cluster/state", c.handleClusterState)
	c.ginRouter.GET("/_cluster/stats", c.handleClusterStats)
	c.ginRouter.PUT("/_cluster/settings", c.handleClusterSettings)

	// Index Management APIs
	c.ginRouter.PUT("/:index", c.handleCreateIndex)
	c.ginRouter.DELETE("/:index", c.handleDeleteIndex)
	c.ginRouter.GET("/:index", c.handleGetIndex)
	c.ginRouter.HEAD("/:index", c.handleIndexExists)
	c.ginRouter.POST("/:index/_open", c.handleOpenIndex)
	c.ginRouter.POST("/:index/_close", c.handleCloseIndex)
	c.ginRouter.POST("/:index/_refresh", c.handleRefreshIndex)
	c.ginRouter.POST("/:index/_flush", c.handleFlushIndex)

	// Mapping APIs
	c.ginRouter.GET("/:index/_mapping", c.handleGetMapping)
	c.ginRouter.PUT("/:index/_mapping", c.handlePutMapping)

	// Settings APIs
	c.ginRouter.GET("/:index/_settings", c.handleGetSettings)
	c.ginRouter.PUT("/:index/_settings", c.handlePutSettings)

	// Document APIs
	c.ginRouter.PUT("/:index/_doc/:id", c.handleIndexDocument)
	c.ginRouter.POST("/:index/_doc", c.handleIndexDocument)
	c.ginRouter.GET("/:index/_doc/:id", c.handleGetDocument)
	c.ginRouter.DELETE("/:index/_doc/:id", c.handleDeleteDocument)
	c.ginRouter.POST("/:index/_update/:id", c.handleUpdateDocument)

	// Bulk API
	c.ginRouter.POST("/_bulk", c.handleBulk)
	c.ginRouter.POST("/:index/_bulk", c.handleBulk)

	// Search APIs
	c.ginRouter.GET("/:index/_search", c.handleSearch)
	c.ginRouter.POST("/:index/_search", c.handleSearch)
	c.ginRouter.GET("/_search", c.handleSearch)
	c.ginRouter.POST("/_search", c.handleSearch)

	// Multi-search API
	c.ginRouter.POST("/_msearch", c.handleMultiSearch)
	c.ginRouter.POST("/:index/_msearch", c.handleMultiSearch)

	// Count API
	c.ginRouter.GET("/:index/_count", c.handleCount)
	c.ginRouter.POST("/:index/_count", c.handleCount)

	// Nodes API
	c.ginRouter.GET("/_nodes", c.handleNodes)
	c.ginRouter.GET("/_nodes/stats", c.handleNodesStats)

	// Metrics endpoint (Prometheus)
	c.ginRouter.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Health check endpoint
	c.ginRouter.GET("/_health", c.handleHealthCheck)
}

// Handler implementations

func (c *CoordinationNode) handleRoot(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"name":         "Quidditch",
		"cluster_name": "quidditch-cluster",
		"cluster_uuid": "TBD",
		"version": gin.H{
			"number":         "1.0.0",
			"build_flavor":   "default",
			"build_type":     "tar",
			"build_hash":     "unknown",
			"build_date":     "2026-01-25",
			"lucene_version": "diagon-1.0.0",
		},
		"tagline": "You Know, for Search (powered by Diagon)",
	})
}

func (c *CoordinationNode) handleClusterHealth(ctx *gin.Context) {
	// Get cluster state from master
	state, err := c.masterClient.GetClusterHealth(ctx.Request.Context())
	if err != nil {
		c.logger.Error("Failed to get cluster health", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":   "cluster_health_exception",
				"reason": fmt.Sprintf("Failed to get cluster health: %v", err),
			},
		})
		return
	}

	// Convert cluster state to health response
	status := "green"
	switch state.Status {
	case pb.ClusterStatus_CLUSTER_STATUS_GREEN:
		status = "green"
	case pb.ClusterStatus_CLUSTER_STATUS_YELLOW:
		status = "yellow"
	case pb.ClusterStatus_CLUSTER_STATUS_RED:
		status = "red"
	}

	// Count shards from routing table
	var activePrimaryShards, activeShards, relocatingShards, initializingShards, unassignedShards int32
	if state.RoutingTable != nil && state.RoutingTable.Indices != nil {
		for _, indexRouting := range state.RoutingTable.Indices {
			for _, shard := range indexRouting.Shards {
				if shard.Allocation == nil {
					continue
				}
				switch shard.Allocation.State {
				case pb.ShardAllocation_SHARD_STATE_STARTED:
					activeShards++
					if shard.IsPrimary {
						activePrimaryShards++
					}
				case pb.ShardAllocation_SHARD_STATE_RELOCATING:
					relocatingShards++
				case pb.ShardAllocation_SHARD_STATE_INITIALIZING:
					initializingShards++
				case pb.ShardAllocation_SHARD_STATE_UNASSIGNED:
					unassignedShards++
				}
			}
		}
	}

	clusterName := "quidditch-cluster"
	if state.ClusterName != "" {
		clusterName = state.ClusterName
	}

	numNodes := int32(len(state.Nodes))
	numDataNodes := int32(0)
	for _, node := range state.Nodes {
		if node.NodeType == pb.NodeType_NODE_TYPE_DATA {
			numDataNodes++
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"cluster_name":                     clusterName,
		"status":                           status,
		"timed_out":                        false,
		"number_of_nodes":                  numNodes,
		"number_of_data_nodes":             numDataNodes,
		"active_primary_shards":            activePrimaryShards,
		"active_shards":                    activeShards,
		"relocating_shards":                relocatingShards,
		"initializing_shards":              initializingShards,
		"unassigned_shards":                unassignedShards,
		"delayed_unassigned_shards":        0,
		"number_of_pending_tasks":          0,
		"number_of_in_flight_fetch":        0,
		"task_max_waiting_in_queue_millis": 0,
		"active_shards_percent_as_number":  100.0,
	})
}

func (c *CoordinationNode) handleClusterState(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"cluster_name": "quidditch-cluster",
		"version":      1,
		"state_uuid":   "TBD",
		"master_node":  "master-1",
		"nodes":        gin.H{},
		"metadata":     gin.H{},
		"routing_table": gin.H{},
	})
}

func (c *CoordinationNode) handleClusterStats(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"cluster_name": "quidditch-cluster",
		"status":       "green",
		"indices":      gin.H{"count": 0},
		"nodes":        gin.H{"count": 1},
	})
}

func (c *CoordinationNode) handleClusterSettings(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"acknowledged": true,
		"persistent":   gin.H{},
		"transient":    gin.H{},
	})
}

func (c *CoordinationNode) handleCreateIndex(ctx *gin.Context) {
	indexName := ctx.Param("index")

	c.logger.Info("Creating index", zap.String("index", indexName))

	// Parse request body for settings and mappings
	var body map[string]interface{}
	if err := ctx.ShouldBindJSON(&body); err != nil && err != io.EOF {
		c.logger.Error("Failed to parse request body", zap.Error(err))
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":   "parsing_exception",
				"reason": fmt.Sprintf("Failed to parse request body: %v", err),
			},
		})
		return
	}

	// Extract settings (with defaults)
	numShards := int32(1)
	numReplicas := int32(0)
	if settingsMap, ok := body["settings"].(map[string]interface{}); ok {
		if indexSettings, ok := settingsMap["index"].(map[string]interface{}); ok {
			if shards, ok := indexSettings["number_of_shards"].(float64); ok {
				numShards = int32(shards)
			}
			if replicas, ok := indexSettings["number_of_replicas"].(float64); ok {
				numReplicas = int32(replicas)
			}
		}
	}

	// Create index settings
	settings := &pb.IndexSettings{
		NumberOfShards:   numShards,
		NumberOfReplicas: numReplicas,
	}

	// TODO: Parse mappings from body
	var mappings map[string]*pb.FieldMapping

	// Call master to create index
	resp, err := c.masterClient.CreateIndex(ctx.Request.Context(), indexName, settings, mappings)
	if err != nil {
		c.logger.Error("Failed to create index", zap.String("index", indexName), zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":   "create_index_exception",
				"reason": fmt.Sprintf("Failed to create index: %v", err),
			},
		})
		return
	}

	c.logger.Info("Successfully created index",
		zap.String("index", indexName),
		zap.Bool("acknowledged", resp.Acknowledged))

	ctx.JSON(http.StatusOK, gin.H{
		"acknowledged":        resp.Acknowledged,
		"shards_acknowledged": true,
		"index":               indexName,
	})
}

func (c *CoordinationNode) handleDeleteIndex(ctx *gin.Context) {
	indexName := ctx.Param("index")

	c.logger.Info("Deleting index", zap.String("index", indexName))

	// Call master to delete index
	resp, err := c.masterClient.DeleteIndex(ctx.Request.Context(), indexName)
	if err != nil {
		c.logger.Error("Failed to delete index", zap.String("index", indexName), zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":   "delete_index_exception",
				"reason": fmt.Sprintf("Failed to delete index: %v", err),
			},
		})
		return
	}

	c.logger.Info("Successfully deleted index",
		zap.String("index", indexName),
		zap.Bool("acknowledged", resp.Acknowledged))

	ctx.JSON(http.StatusOK, gin.H{
		"acknowledged": resp.Acknowledged,
	})
}

func (c *CoordinationNode) handleGetIndex(ctx *gin.Context) {
	indexName := ctx.Param("index")

	c.logger.Debug("Getting index metadata", zap.String("index", indexName))

	// Query master for index metadata
	resp, err := c.masterClient.GetIndexMetadata(ctx.Request.Context(), indexName)
	if err != nil {
		c.logger.Error("Failed to get index metadata", zap.String("index", indexName), zap.Error(err))
		ctx.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"type":   "index_not_found_exception",
				"reason": fmt.Sprintf("Index %s not found: %v", indexName, err),
			},
		})
		return
	}

	// Convert to OpenSearch format
	indexInfo := gin.H{
		"aliases":  gin.H{},
		"mappings": gin.H{},
		"settings": gin.H{
			"index": gin.H{
				"number_of_shards":   fmt.Sprintf("%d", resp.Metadata.Settings.NumberOfShards),
				"number_of_replicas": fmt.Sprintf("%d", resp.Metadata.Settings.NumberOfReplicas),
				"uuid":               resp.Metadata.IndexUuid,
				"version": gin.H{
					"created": resp.Metadata.Version,
				},
			},
		},
	}

	ctx.JSON(http.StatusOK, gin.H{
		indexName: indexInfo,
	})
}

func (c *CoordinationNode) handleIndexExists(ctx *gin.Context) {
	// TODO: Check with master if index exists
	ctx.Status(http.StatusOK)
}

func (c *CoordinationNode) handleOpenIndex(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"acknowledged": true})
}

func (c *CoordinationNode) handleCloseIndex(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"acknowledged": true})
}

func (c *CoordinationNode) handleRefreshIndex(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"_shards": gin.H{"total": 1, "successful": 1, "failed": 0}})
}

func (c *CoordinationNode) handleFlushIndex(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"_shards": gin.H{"total": 1, "successful": 1, "failed": 0}})
}

func (c *CoordinationNode) handleGetMapping(ctx *gin.Context) {
	indexName := ctx.Param("index")
	ctx.JSON(http.StatusOK, gin.H{
		indexName: gin.H{"mappings": gin.H{}},
	})
}

func (c *CoordinationNode) handlePutMapping(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"acknowledged": true})
}

func (c *CoordinationNode) handleGetSettings(ctx *gin.Context) {
	indexName := ctx.Param("index")
	ctx.JSON(http.StatusOK, gin.H{
		indexName: gin.H{
			"settings": gin.H{
				"index": gin.H{
					"number_of_shards":   "1",
					"number_of_replicas": "0",
				},
			},
		},
	})
}

func (c *CoordinationNode) handlePutSettings(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"acknowledged": true})
}

func (c *CoordinationNode) handleIndexDocument(ctx *gin.Context) {
	indexName := ctx.Param("index")
	docID := ctx.Param("id")

	// Parse document from request body
	var document map[string]interface{}
	if err := ctx.ShouldBindJSON(&document); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":   "parse_exception",
				"reason": fmt.Sprintf("Failed to parse document: %v", err),
			},
		})
		return
	}

	// Route to appropriate data node
	resp, err := c.docRouter.RouteIndexDocument(ctx.Request.Context(), indexName, docID, document)
	if err != nil {
		c.logger.Error("Failed to index document",
			zap.String("index", indexName),
			zap.String("doc_id", docID),
			zap.Error(err))

		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":   "index_failed_exception",
				"reason": fmt.Sprintf("Failed to index document: %v", err),
			},
		})
		return
	}

	// Return success response
	result := "created"
	statusCode := http.StatusCreated
	if resp.Version > 1 {
		result = "updated"
		statusCode = http.StatusOK
	}

	ctx.JSON(statusCode, gin.H{
		"_index":   indexName,
		"_id":      docID,
		"_version": resp.Version,
		"result":   result,
		"_shards": gin.H{
			"total":      resp.Shards.Total,
			"successful": resp.Shards.Successful,
			"failed":     resp.Shards.Failed,
		},
	})
}

func (c *CoordinationNode) handleGetDocument(ctx *gin.Context) {
	indexName := ctx.Param("index")
	docID := ctx.Param("id")

	// Route to appropriate data node
	resp, err := c.docRouter.RouteGetDocument(ctx.Request.Context(), indexName, docID)
	if err != nil {
		c.logger.Error("Failed to get document",
			zap.String("index", indexName),
			zap.String("doc_id", docID),
			zap.Error(err))

		// Check if document not found
		if strings.Contains(err.Error(), "not found") {
			ctx.JSON(http.StatusNotFound, gin.H{
				"_index": indexName,
				"_id":    docID,
				"found":  false,
			})
			return
		}

		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":   "get_failed_exception",
				"reason": fmt.Sprintf("Failed to get document: %v", err),
			},
		})
		return
	}

	// Return document
	ctx.JSON(http.StatusOK, gin.H{
		"_index":   indexName,
		"_id":      docID,
		"_version": resp.Version,
		"found":    resp.Found,
		"_source":  resp.Source.AsMap(),
	})
}

func (c *CoordinationNode) handleDeleteDocument(ctx *gin.Context) {
	indexName := ctx.Param("index")
	docID := ctx.Param("id")

	// Route to appropriate data node
	resp, err := c.docRouter.RouteDeleteDocument(ctx.Request.Context(), indexName, docID)
	if err != nil {
		c.logger.Error("Failed to delete document",
			zap.String("index", indexName),
			zap.String("doc_id", docID),
			zap.Error(err))

		// Check if document not found
		if strings.Contains(err.Error(), "not found") {
			ctx.JSON(http.StatusNotFound, gin.H{
				"_index": indexName,
				"_id":    docID,
				"found":  false,
			})
			return
		}

		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":   "delete_failed_exception",
				"reason": fmt.Sprintf("Failed to delete document: %v", err),
			},
		})
		return
	}

	// Return success response
	ctx.JSON(http.StatusOK, gin.H{
		"_index":   indexName,
		"_id":      docID,
		"_version": resp.Version,
		"result":   "deleted",
		"_shards": gin.H{
			"total":      resp.Shards.Total,
			"successful": resp.Shards.Successful,
			"failed":     resp.Shards.Failed,
		},
	})
}

func (c *CoordinationNode) handleUpdateDocument(ctx *gin.Context) {
	indexName := ctx.Param("index")
	docID := ctx.Param("id")

	// Parse update request body
	var updateReq struct {
		Doc             map[string]interface{} `json:"doc"`
		DocAsUpsert     bool                   `json:"doc_as_upsert"`
		ScriptedUpsert  bool                   `json:"scripted_upsert"`
		Upsert          map[string]interface{} `json:"upsert"`
	}
	if err := ctx.ShouldBindJSON(&updateReq); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":   "parse_exception",
				"reason": fmt.Sprintf("Failed to parse update request: %v", err),
			},
		})
		return
	}

	// For now, perform a full document replacement with the "doc" field
	// TODO: Implement partial updates and scripted updates
	document := updateReq.Doc
	if document == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":   "illegal_argument_exception",
				"reason": "Update request must contain 'doc' field",
			},
		})
		return
	}

	// Route to appropriate data node
	resp, err := c.docRouter.RouteIndexDocument(ctx.Request.Context(), indexName, docID, document)
	if err != nil {
		c.logger.Error("Failed to update document",
			zap.String("index", indexName),
			zap.String("doc_id", docID),
			zap.Error(err))

		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":   "update_failed_exception",
				"reason": fmt.Sprintf("Failed to update document: %v", err),
			},
		})
		return
	}

	// Return success response
	ctx.JSON(http.StatusOK, gin.H{
		"_index":   indexName,
		"_id":      docID,
		"_version": resp.Version,
		"result":   "updated",
		"_shards": gin.H{
			"total":      resp.Shards.Total,
			"successful": resp.Shards.Successful,
			"failed":     resp.Shards.Failed,
		},
	})
}

func (c *CoordinationNode) handleBulk(ctx *gin.Context) {
	startTime := time.Now()

	// Read request body
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":   "parse_exception",
				"reason": fmt.Sprintf("Failed to read request body: %v", err),
			},
		})
		return
	}

	// Parse bulk request
	bulkReq, err := bulk.ParseBulkRequest(body)
	if err != nil {
		c.logger.Error("Failed to parse bulk request", zap.Error(err))
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":   "parse_exception",
				"reason": fmt.Sprintf("Failed to parse bulk request: %v", err),
			},
		})
		return
	}

	c.logger.Debug("Processing bulk request",
		zap.Int("num_operations", len(bulkReq.Operations)))

	// Process operations in parallel with limited concurrency
	response := bulk.NewBulkResponse()
	results := make([]*bulkOperationResult, len(bulkReq.Operations))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit concurrent operations to 10

	for i, op := range bulkReq.Operations {
		wg.Add(1)
		go func(idx int, operation *bulk.BulkOperation) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Execute operation
			result := c.executeBulkOperation(ctx.Request.Context(), operation)
			results[idx] = result
		}(i, op)
	}

	// Wait for all operations to complete
	wg.Wait()

	// Build response maintaining order
	for i, result := range results {
		response.AddItem(bulkReq.Operations[i].Type, result.itemResult)
	}

	// Set timing
	duration := time.Since(startTime)
	response.Took = duration.Milliseconds()

	// Record bulk operation metrics
	c.metrics.RecordBulkOperation("bulk", "success", duration, len(bulkReq.Operations), response.Errors)

	ctx.JSON(http.StatusOK, response)
}

// bulkOperationResult holds the result of a single bulk operation
type bulkOperationResult struct {
	itemResult *bulk.BulkItemResult
}

// executeBulkOperation executes a single bulk operation
func (c *CoordinationNode) executeBulkOperation(ctx context.Context, op *bulk.BulkOperation) *bulkOperationResult {
	result := &bulkOperationResult{
		itemResult: &bulk.BulkItemResult{
			Index: op.Index,
			ID:    op.ID,
		},
	}

	switch op.Type {
	case bulk.OperationIndex, bulk.OperationCreate:
		// Index or create document
		resp, err := c.docRouter.RouteIndexDocument(ctx, op.Index, op.ID, op.Document)
		if err != nil {
			c.logger.Error("Bulk index operation failed",
				zap.String("index", op.Index),
				zap.String("doc_id", op.ID),
				zap.Error(err))

			result.itemResult.Status = http.StatusInternalServerError
			result.itemResult.Error = &bulk.BulkItemError{
				Type:   "index_failed_exception",
				Reason: err.Error(),
			}
		} else {
			result.itemResult.Status = http.StatusCreated
			if op.Type == bulk.OperationIndex && resp.Version > 1 {
				result.itemResult.Status = http.StatusOK
				result.itemResult.Result = "updated"
			} else {
				result.itemResult.Result = "created"
			}
			result.itemResult.Version = resp.Version
			result.itemResult.Shards = &bulk.BulkItemShards{
				Total:      resp.Shards.Total,
				Successful: resp.Shards.Successful,
				Failed:     resp.Shards.Failed,
			}
		}

	case bulk.OperationUpdate:
		// Update document
		document := op.UpdateDoc
		if document == nil {
			document = op.Document
		}

		resp, err := c.docRouter.RouteIndexDocument(ctx, op.Index, op.ID, document)
		if err != nil {
			c.logger.Error("Bulk update operation failed",
				zap.String("index", op.Index),
				zap.String("doc_id", op.ID),
				zap.Error(err))

			result.itemResult.Status = http.StatusInternalServerError
			result.itemResult.Error = &bulk.BulkItemError{
				Type:   "update_failed_exception",
				Reason: err.Error(),
			}
		} else {
			result.itemResult.Status = http.StatusOK
			result.itemResult.Result = "updated"
			result.itemResult.Version = resp.Version
			result.itemResult.Shards = &bulk.BulkItemShards{
				Total:      resp.Shards.Total,
				Successful: resp.Shards.Successful,
				Failed:     resp.Shards.Failed,
			}
		}

	case bulk.OperationDelete:
		// Delete document
		resp, err := c.docRouter.RouteDeleteDocument(ctx, op.Index, op.ID)
		if err != nil {
			c.logger.Error("Bulk delete operation failed",
				zap.String("index", op.Index),
				zap.String("doc_id", op.ID),
				zap.Error(err))

			// Check if document not found
			if strings.Contains(err.Error(), "not found") {
				result.itemResult.Status = http.StatusNotFound
				result.itemResult.Result = "not_found"
			} else {
				result.itemResult.Status = http.StatusInternalServerError
				result.itemResult.Error = &bulk.BulkItemError{
					Type:   "delete_failed_exception",
					Reason: err.Error(),
				}
			}
		} else {
			result.itemResult.Status = http.StatusOK
			result.itemResult.Result = "deleted"
			result.itemResult.Version = resp.Version
			result.itemResult.Shards = &bulk.BulkItemShards{
				Total:      resp.Shards.Total,
				Successful: resp.Shards.Successful,
				Failed:     resp.Shards.Failed,
			}
		}

	default:
		result.itemResult.Status = http.StatusBadRequest
		result.itemResult.Error = &bulk.BulkItemError{
			Type:   "illegal_argument_exception",
			Reason: fmt.Sprintf("Unknown bulk operation type: %s", op.Type),
		}
	}

	return result
}

func (c *CoordinationNode) handleSearch(ctx *gin.Context) {
	startTime := time.Now()
	indexName := ctx.Param("index")

	// Read request body
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":   "parse_exception",
				"reason": fmt.Sprintf("Failed to read request body: %v", err),
			},
		})
		return
	}

	// Parse search request
	var searchReq *parser.SearchRequest
	if len(body) > 0 {
		searchReq, err = c.queryParser.ParseSearchRequest(body)
		if err != nil {
			c.logger.Error("Failed to parse query", zap.Error(err))
			ctx.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"type":   "parsing_exception",
					"reason": fmt.Sprintf("Failed to parse query: %v", err),
				},
			})
			return
		}

		// Validate the parsed query
		if searchReq.ParsedQuery != nil {
			if err := c.queryParser.Validate(searchReq.ParsedQuery); err != nil {
				c.logger.Error("Query validation failed", zap.Error(err))
				ctx.JSON(http.StatusBadRequest, gin.H{
					"error": gin.H{
						"type":   "validation_exception",
						"reason": fmt.Sprintf("Query validation failed: %v", err),
					},
				})
				return
			}
		}
	} else {
		// Empty body - match all query
		searchReq = &parser.SearchRequest{
			ParsedQuery: &parser.MatchAllQuery{},
		}
	}

	// Create query plan
	plan, err := c.queryPlanner.PlanQuery(ctx.Request.Context(), indexName, searchReq)
	if err != nil {
		c.logger.Error("Query planning failed",
			zap.String("index", indexName),
			zap.Error(err))
		// Continue without plan - planning is optional
	} else {
		// Log plan details
		c.logger.Debug("Query plan created",
			zap.String("index", indexName),
			zap.Int("complexity", plan.Complexity),
			zap.Float64("estimated_cost", plan.EstimatedCost),
			zap.Int("target_shards", len(plan.TargetShards)),
			zap.Bool("cacheable", plan.Cacheable))
	}

	// Log query info
	c.logger.Debug("Processing search request",
		zap.String("index", indexName),
		zap.String("query_type", searchReq.ParsedQuery.QueryType()),
		zap.Int("size", searchReq.Size),
		zap.Int("from", searchReq.From))

	// Extract filter expression from query if present
	filterExpression := extractFilterExpression(searchReq.ParsedQuery)

	// Execute query across shards
	// TODO: Serialize parsed query properly - for now using original body
	result, err := c.queryExecutor.ExecuteSearch(ctx.Request.Context(), indexName, body, filterExpression, searchReq.From, searchReq.Size)
	if err != nil {
		// Record failed query metrics
		complexity := 0
		if plan != nil {
			complexity = plan.Complexity
		}
		c.metrics.RecordQuery(indexName, searchReq.ParsedQuery.QueryType(), "error", time.Since(startTime), complexity, 0)

		c.logger.Error("Query execution failed",
			zap.String("index", indexName),
			zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":   "search_exception",
				"reason": fmt.Sprintf("Query execution failed: %v", err),
			},
		})
		return
	}

	// Record successful query metrics
	complexity := 0
	shardCount := 0
	if plan != nil {
		complexity = plan.Complexity
		shardCount = len(plan.TargetShards)
	}
	c.metrics.RecordQuery(indexName, searchReq.ParsedQuery.QueryType(), "success", time.Since(startTime), complexity, shardCount)

	// Convert result to OpenSearch format
	hits := make([]gin.H, 0, len(result.Hits))
	for _, hit := range result.Hits {
		hits = append(hits, gin.H{
			"_id":     hit.ID,
			"_score":  hit.Score,
			"_source": hit.Source,
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"took":      result.TookMillis,
		"timed_out": false,
		"_shards":   gin.H{"total": 1, "successful": 1, "skipped": 0, "failed": 0},
		"hits": gin.H{
			"total": gin.H{
				"value":    result.TotalHits,
				"relation": "eq",
			},
			"max_score": result.MaxScore,
			"hits":      hits,
		},
	})
}

func (c *CoordinationNode) handleMultiSearch(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"responses": []gin.H{},
	})
}

func (c *CoordinationNode) handleCount(ctx *gin.Context) {
	indexName := ctx.Param("index")

	// Read request body (optional query)
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":   "parse_exception",
				"reason": fmt.Sprintf("Failed to read request body: %v", err),
			},
		})
		return
	}

	// Parse query to extract filter expression if present
	var filterExpression []byte
	if len(body) > 0 {
		searchReq, err := c.queryParser.ParseSearchRequest(body)
		if err == nil && searchReq.ParsedQuery != nil {
			filterExpression = extractFilterExpression(searchReq.ParsedQuery)
		}
	}

	// Execute count across shards
	count, err := c.queryExecutor.ExecuteCount(ctx.Request.Context(), indexName, body, filterExpression)
	if err != nil {
		c.logger.Error("Count execution failed",
			zap.String("index", indexName),
			zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":   "count_exception",
				"reason": fmt.Sprintf("Count execution failed: %v", err),
			},
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"count":   count,
		"_shards": gin.H{"total": 1, "successful": 1, "skipped": 0, "failed": 0},
	})
}

func (c *CoordinationNode) handleNodes(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"_nodes": gin.H{"total": 1, "successful": 1, "failed": 0},
		"cluster_name": "quidditch-cluster",
		"nodes":        gin.H{},
	})
}

func (c *CoordinationNode) handleNodesStats(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"_nodes": gin.H{"total": 1, "successful": 1, "failed": 0},
		"cluster_name": "quidditch-cluster",
		"nodes":        gin.H{},
	})
}

func (c *CoordinationNode) handleHealthCheck(ctx *gin.Context) {
	// Check if master client is connected
	healthy := true
	checks := gin.H{
		"master_connection": "ok",
		"query_executor":    "ok",
		"query_planner":     "ok",
	}

	// Try a simple master query to verify connectivity
	_, err := c.masterClient.GetClusterHealth(ctx.Request.Context())
	if err != nil {
		healthy = false
		checks["master_connection"] = "failed"
	}

	status := "green"
	if !healthy {
		status = "yellow"
	}

	httpStatus := http.StatusOK
	if !healthy {
		httpStatus = http.StatusServiceUnavailable
	}

	ctx.JSON(httpStatus, gin.H{
		"status": status,
		"checks": checks,
	})
}

// discoverDataNodes discovers data nodes from master and registers them with query executor
func (c *CoordinationNode) discoverDataNodes(ctx context.Context) error {
	c.logger.Info("Discovering data nodes from master")

	// Get cluster state from master
	state, err := c.masterClient.GetClusterState(ctx, false, true, false)
	if err != nil {
		return fmt.Errorf("failed to get cluster state: %w", err)
	}

	// Register data node clients
	dataNodeCount := 0
	dataClientInterfaces := make(map[string]router.DataNodeClient)

	for _, node := range state.Nodes {
		if node.NodeType == pb.NodeType_NODE_TYPE_DATA {
			// Construct data node address
			address := fmt.Sprintf("%s:%d", node.BindAddr, node.GrpcPort)

			// Create data node client
			dataClient := NewDataNodeClient(node.NodeId, address, c.logger)

			// Store in coordination node
			c.dataClientsMu.Lock()
			c.dataClients[node.NodeId] = dataClient
			c.dataClientsMu.Unlock()

			// Register with query executor
			c.queryExecutor.RegisterDataNode(dataClient)

			// Add to interface map for document router
			dataClientInterfaces[node.NodeId] = dataClient

			c.logger.Info("Registered data node",
				zap.String("node_id", node.NodeId),
				zap.String("address", address))
			dataNodeCount++
		}
	}

	// Update document router with data clients
	c.docRouter.SetDataClients(dataClientInterfaces)

	c.logger.Info("Data node discovery complete",
		zap.Int("data_nodes", dataNodeCount))

	return nil
}

// ginLogger creates a Gin middleware that logs requests using zap
func ginLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		logger.Info("HTTP request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}

// extractFilterExpression recursively searches the query tree for ExpressionQuery
// and returns the serialized expression bytes. Returns nil if no expression filter found.
func extractFilterExpression(query parser.Query) []byte {
	if query == nil {
		return nil
	}

	// Check if this is an expression query
	if exprQuery, ok := query.(*parser.ExpressionQuery); ok {
		return exprQuery.SerializedExpression
	}

	// Recursively search bool query clauses
	if boolQuery, ok := query.(*parser.BoolQuery); ok {
		// Check filter clauses first (most common location for expressions)
		for _, filterQuery := range boolQuery.Filter {
			if expr := extractFilterExpression(filterQuery); expr != nil {
				return expr
			}
		}
		// Check must clauses
		for _, mustQuery := range boolQuery.Must {
			if expr := extractFilterExpression(mustQuery); expr != nil {
				return expr
			}
		}
		// Check should clauses
		for _, shouldQuery := range boolQuery.Should {
			if expr := extractFilterExpression(shouldQuery); expr != nil {
				return expr
			}
		}
		// Check must_not clauses
		for _, mustNotQuery := range boolQuery.MustNot {
			if expr := extractFilterExpression(mustNotQuery); expr != nil {
				return expr
			}
		}
	}

	// No expression filter found
	return nil
}
