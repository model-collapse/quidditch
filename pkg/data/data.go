package data

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/quidditch/quidditch/pkg/data/diagon"
	"github.com/quidditch/quidditch/pkg/wasm"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// DataNode represents a data node in the Quidditch cluster
type DataNode struct {
	cfg          *config.DataNodeConfig
	logger       *zap.Logger
	grpcServer   *grpc.Server
	diagon       *diagon.DiagonBridge
	udfRegistry  *wasm.UDFRegistry
	shards       *ShardManager
	masterClient *MasterClient
	mu           sync.RWMutex
}

// NewDataNode creates a new data node
func NewDataNode(cfg *config.DataNodeConfig, logger *zap.Logger) (*DataNode, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Initialize Diagon bridge
	diagonBridge, err := diagon.NewDiagonBridge(&diagon.Config{
		DataDir:     cfg.DataDir,
		SIMDEnabled: cfg.SIMDEnabled,
		Logger:      logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Diagon: %w", err)
	}

	// Initialize WASM runtime and UDF registry
	wasmRuntime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT:      true,
		EnableDebug:    false,
		MaxMemoryPages: 256,
		Logger:         logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize WASM runtime: %w", err)
	}

	udfRegistry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime:         wasmRuntime,
		DefaultPoolSize: 10,
		EnableStats:     true,
		Logger:          logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize UDF registry: %w", err)
	}

	// Create shard manager
	shardManager := NewShardManager(cfg, logger, diagonBridge, udfRegistry)

	// Create master client
	masterClient := NewMasterClient(cfg.NodeID, cfg.MasterAddr, logger)

	// Create gRPC server
	grpcServer := grpc.NewServer()

	node := &DataNode{
		cfg:          cfg,
		logger:       logger,
		grpcServer:   grpcServer,
		diagon:       diagonBridge,
		udfRegistry:  udfRegistry,
		shards:       shardManager,
		masterClient: masterClient,
	}

	// Register gRPC service
	dataService := NewDataService(node, logger)
	pb.RegisterDataServiceServer(grpcServer, dataService)

	return node, nil
}

// Start starts the data node
func (d *DataNode) Start(ctx context.Context) error {
	d.logger.Info("Starting data node",
		zap.String("node_id", d.cfg.NodeID),
		zap.String("storage_tier", d.cfg.StorageTier),
		zap.Int("max_shards", d.cfg.MaxShards))

	// Start Diagon
	if err := d.diagon.Start(); err != nil {
		return fmt.Errorf("failed to start Diagon: %w", err)
	}

	// Start shard manager
	if err := d.shards.Start(ctx); err != nil {
		return fmt.Errorf("failed to start shard manager: %w", err)
	}

	// Start gRPC server
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", d.cfg.BindAddr, d.cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC port: %w", err)
	}

	go func() {
		d.logger.Info("Starting gRPC server", zap.Int("port", d.cfg.GRPCPort))
		if err := d.grpcServer.Serve(listener); err != nil {
			d.logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	// Register with master node
	go d.registerWithMaster(ctx)

	// Start heartbeat (using master client)
	d.masterClient.StartHeartbeat(ctx, 10*time.Second)

	return nil
}

// Stop stops the data node
func (d *DataNode) Stop(ctx context.Context) error {
	d.logger.Info("Stopping data node")

	// Stop heartbeat
	d.masterClient.StopHeartbeat()

	// Unregister from master
	if err := d.masterClient.Unregister(ctx); err != nil {
		d.logger.Warn("Failed to unregister from master", zap.Error(err))
	}

	// Disconnect from master
	if err := d.masterClient.Disconnect(); err != nil {
		d.logger.Warn("Failed to disconnect from master", zap.Error(err))
	}

	// Stop accepting new requests
	d.grpcServer.GracefulStop()

	// Stop shard manager
	if err := d.shards.Stop(ctx); err != nil {
		d.logger.Error("Error stopping shard manager", zap.Error(err))
	}

	// Stop Diagon
	if err := d.diagon.Stop(); err != nil {
		d.logger.Error("Error stopping Diagon", zap.Error(err))
	}

	return nil
}

// registerWithMaster registers this data node with the master
func (d *DataNode) registerWithMaster(ctx context.Context) {
	d.logger.Info("Registering with master",
		zap.String("master_addr", d.cfg.MasterAddr),
		zap.String("node_id", d.cfg.NodeID))

	// Connect to master with retries
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		if err := d.masterClient.Connect(ctx); err != nil {
			d.logger.Warn("Failed to connect to master, retrying",
				zap.Int("attempt", i+1),
				zap.Error(err))
			time.Sleep(time.Duration(i+1) * 2 * time.Second) // Exponential backoff
			continue
		}

		// Connected successfully, now register
		attributes := &pb.NodeAttributes{
			StorageTier: d.cfg.StorageTier,
			MaxShards:   int32(d.cfg.MaxShards),
			SimdEnabled: d.cfg.SIMDEnabled,
			Version:     "1.0.0", // TODO: Get from build
		}

		if err := d.masterClient.Register(ctx, d.cfg.BindAddr, int32(d.cfg.GRPCPort), attributes); err != nil {
			d.logger.Error("Failed to register with master", zap.Error(err))
			// Continue trying in heartbeat loop
			return
		}

		d.logger.Info("Successfully registered with master")
		return
	}

	d.logger.Error("Failed to connect to master after retries", zap.Int("max_retries", maxRetries))
}

// heartbeatLoop sends periodic heartbeats to the master
func (d *DataNode) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.sendHeartbeat(ctx)
		}
	}
}

// sendHeartbeat sends a heartbeat to the master
func (d *DataNode) sendHeartbeat(ctx context.Context) {
	// TODO: Implement heartbeat via gRPC
	stats := d.collectStats()

	d.logger.Debug("Sending heartbeat",
		zap.Int("active_shards", stats.ActiveShards),
		zap.Int64("docs_count", stats.DocsCount),
		zap.Int64("store_size", stats.StoreSizeBytes))

	// In a real implementation:
	// 1. Collect node stats (CPU, memory, disk)
	// 2. Collect shard states
	// 3. Send NodeHeartbeat request to master
	// 4. Process any allocation commands from master
}

// collectStats collects node statistics
func (d *DataNode) collectStats() *NodeStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := &NodeStats{
		NodeID:       d.cfg.NodeID,
		ActiveShards: d.shards.Count(),
		DocsCount:    0,
		StoreSizeBytes: 0,
	}

	// Aggregate stats from all shards
	for _, shard := range d.shards.List() {
		stats.DocsCount += shard.DocsCount
		stats.StoreSizeBytes += shard.SizeBytes
	}

	return stats
}

// CreateShard creates a new shard on this node
func (d *DataNode) CreateShard(ctx context.Context, indexName string, shardID int32, isPrimary bool) error {
	d.logger.Info("Creating shard",
		zap.String("index", indexName),
		zap.Int32("shard_id", shardID),
		zap.Bool("is_primary", isPrimary))

	return d.shards.CreateShard(ctx, indexName, shardID, isPrimary)
}

// DeleteShard deletes a shard from this node
func (d *DataNode) DeleteShard(ctx context.Context, indexName string, shardID int32) error {
	d.logger.Info("Deleting shard",
		zap.String("index", indexName),
		zap.Int32("shard_id", shardID))

	return d.shards.DeleteShard(ctx, indexName, shardID)
}

// IndexDocument indexes a document in a shard
func (d *DataNode) IndexDocument(ctx context.Context, indexName string, shardID int32, docID string, doc map[string]interface{}) error {
	shard, err := d.shards.GetShard(indexName, shardID)
	if err != nil {
		return err
	}

	return shard.IndexDocument(ctx, docID, doc)
}

// SearchShard executes a search query on a shard
func (d *DataNode) SearchShard(ctx context.Context, indexName string, shardID int32, query []byte) (*diagon.SearchResult, error) {
	shard, err := d.shards.GetShard(indexName, shardID)
	if err != nil {
		return nil, err
	}

	return shard.Search(ctx, query)
}

// NodeStats represents node statistics
type NodeStats struct {
	NodeID         string
	ActiveShards   int
	DocsCount      int64
	StoreSizeBytes int64
	CPUPercent     float64
	MemoryPercent  float64
	DiskPercent    float64
}

// SearchResult represents search results from a shard
type SearchResult struct {
	Took       int64
	TotalHits  int64
	MaxScore   float64
	Hits       []*Hit
}

// Hit represents a search hit
type Hit struct {
	ID     string
	Score  float64
	Source map[string]interface{}
}
