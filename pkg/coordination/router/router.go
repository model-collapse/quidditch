package router

import (
	"context"
	"fmt"
	"hash/fnv"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"go.uber.org/zap"
)

// DataNodeClient interface for communication with data nodes
type DataNodeClient interface {
	IndexDocument(ctx context.Context, indexName string, shardID int32, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error)
	GetDocument(ctx context.Context, indexName string, shardID int32, docID string) (*pb.GetDocumentResponse, error)
	DeleteDocument(ctx context.Context, indexName string, shardID int32, docID string) (*pb.DeleteDocumentResponse, error)
	IsConnected() bool
	Connect(ctx context.Context) error
	NodeID() string
}

// MasterClient interface for getting cluster state
type MasterClient interface {
	GetShardRouting(ctx context.Context, indexName string) (map[int32]*pb.ShardRouting, error)
	GetIndexMetadata(ctx context.Context, indexName string) (*pb.IndexMetadataResponse, error)
}

// DocumentRouter routes document operations to the appropriate shards
type DocumentRouter struct {
	logger      *zap.Logger
	masterClient MasterClient
	dataClients map[string]DataNodeClient // nodeID -> client
}

// NewDocumentRouter creates a new document router
func NewDocumentRouter(masterClient MasterClient, dataClients map[string]DataNodeClient, logger *zap.Logger) *DocumentRouter {
	return &DocumentRouter{
		logger:      logger,
		masterClient: masterClient,
		dataClients: dataClients,
	}
}

// RouteIndexDocument routes an index document operation to the correct shard
func (dr *DocumentRouter) RouteIndexDocument(ctx context.Context, indexName, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error) {
	// Get index metadata to determine number of shards
	metadata, err := dr.masterClient.GetIndexMetadata(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to get index metadata: %w", err)
	}

	numShards := metadata.Metadata.Settings.NumberOfShards
	if numShards == 0 {
		return nil, fmt.Errorf("index has no shards configured")
	}

	// Calculate which shard this document belongs to
	shardID := dr.calculateShardID(docID, numShards)

	// Get shard routing information
	routing, err := dr.masterClient.GetShardRouting(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to get shard routing: %w", err)
	}

	shard, exists := routing[shardID]
	if !exists {
		return nil, fmt.Errorf("shard %d not found for index %s", shardID, indexName)
	}

	// Find primary shard for writes
	if shard.Allocation == nil || shard.Allocation.State != pb.ShardAllocation_SHARD_STATE_STARTED {
		return nil, fmt.Errorf("shard %d is not available (state: %v)", shardID, shard.Allocation.State)
	}

	// Only write to primary shard
	if !shard.IsPrimary {
		return nil, fmt.Errorf("shard %d is not a primary shard", shardID)
	}

	nodeID := shard.Allocation.NodeId
	if nodeID == "" {
		return nil, fmt.Errorf("shard %d has no node assignment", shardID)
	}

	// Get data node client
	client, exists := dr.dataClients[nodeID]
	if !exists {
		return nil, fmt.Errorf("data node %s not found", nodeID)
	}

	// Ensure client is connected
	if !client.IsConnected() {
		if err := client.Connect(ctx); err != nil {
			return nil, fmt.Errorf("failed to connect to node %s: %w", nodeID, err)
		}
	}

	// Route to data node
	dr.logger.Info("Routing index document to data node",
		zap.String("index", indexName),
		zap.String("doc_id", docID),
		zap.Int32("shard_id", shardID),
		zap.String("node_id", nodeID))

	resp, err := client.IndexDocument(ctx, indexName, shardID, docID, document)
	if err != nil {
		dr.logger.Error("IndexDocument call failed", zap.Error(err))
		return nil, err
	}

	dr.logger.Info("IndexDocument succeeded",
		zap.String("doc_id", docID),
		zap.Int64("version", resp.Version))

	return resp, nil
}

// RouteGetDocument routes a get document operation to the correct shard
func (dr *DocumentRouter) RouteGetDocument(ctx context.Context, indexName, docID string) (*pb.GetDocumentResponse, error) {
	// Get index metadata to determine number of shards
	metadata, err := dr.masterClient.GetIndexMetadata(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to get index metadata: %w", err)
	}

	numShards := metadata.Metadata.Settings.NumberOfShards
	if numShards == 0 {
		return nil, fmt.Errorf("index has no shards configured")
	}

	// Calculate which shard this document belongs to
	shardID := dr.calculateShardID(docID, numShards)

	// Get shard routing information
	routing, err := dr.masterClient.GetShardRouting(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to get shard routing: %w", err)
	}

	shard, exists := routing[shardID]
	if !exists {
		return nil, fmt.Errorf("shard %d not found for index %s", shardID, indexName)
	}

	// For reads, we can use primary or replica
	if shard.Allocation == nil || shard.Allocation.State != pb.ShardAllocation_SHARD_STATE_STARTED {
		return nil, fmt.Errorf("shard %d is not available", shardID)
	}

	nodeID := shard.Allocation.NodeId
	if nodeID == "" {
		return nil, fmt.Errorf("shard %d has no node assignment", shardID)
	}

	// Get data node client
	client, exists := dr.dataClients[nodeID]
	if !exists {
		return nil, fmt.Errorf("data node %s not found", nodeID)
	}

	// Ensure client is connected
	if !client.IsConnected() {
		if err := client.Connect(ctx); err != nil {
			return nil, fmt.Errorf("failed to connect to node %s: %w", nodeID, err)
		}
	}

	// Route to data node
	dr.logger.Debug("Routing get document",
		zap.String("index", indexName),
		zap.String("doc_id", docID),
		zap.Int32("shard_id", shardID),
		zap.String("node_id", nodeID))

	return client.GetDocument(ctx, indexName, shardID, docID)
}

// RouteDeleteDocument routes a delete document operation to the correct shard
func (dr *DocumentRouter) RouteDeleteDocument(ctx context.Context, indexName, docID string) (*pb.DeleteDocumentResponse, error) {
	// Get index metadata to determine number of shards
	metadata, err := dr.masterClient.GetIndexMetadata(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to get index metadata: %w", err)
	}

	numShards := metadata.Metadata.Settings.NumberOfShards
	if numShards == 0 {
		return nil, fmt.Errorf("index has no shards configured")
	}

	// Calculate which shard this document belongs to
	shardID := dr.calculateShardID(docID, numShards)

	// Get shard routing information
	routing, err := dr.masterClient.GetShardRouting(ctx, indexName)
	if err != nil {
		return nil, fmt.Errorf("failed to get shard routing: %w", err)
	}

	shard, exists := routing[shardID]
	if !exists {
		return nil, fmt.Errorf("shard %d not found for index %s", shardID, indexName)
	}

	// Only delete from primary shard
	if shard.Allocation == nil || shard.Allocation.State != pb.ShardAllocation_SHARD_STATE_STARTED {
		return nil, fmt.Errorf("shard %d is not available", shardID)
	}

	if !shard.IsPrimary {
		return nil, fmt.Errorf("shard %d is not a primary shard", shardID)
	}

	nodeID := shard.Allocation.NodeId
	if nodeID == "" {
		return nil, fmt.Errorf("shard %d has no node assignment", shardID)
	}

	// Get data node client
	client, exists := dr.dataClients[nodeID]
	if !exists {
		return nil, fmt.Errorf("data node %s not found", nodeID)
	}

	// Ensure client is connected
	if !client.IsConnected() {
		if err := client.Connect(ctx); err != nil {
			return nil, fmt.Errorf("failed to connect to node %s: %w", nodeID, err)
		}
	}

	// Route to data node
	dr.logger.Debug("Routing delete document",
		zap.String("index", indexName),
		zap.String("doc_id", docID),
		zap.Int32("shard_id", shardID),
		zap.String("node_id", nodeID))

	return client.DeleteDocument(ctx, indexName, shardID, docID)
}

// calculateShardID uses consistent hashing to determine which shard a document belongs to
func (dr *DocumentRouter) calculateShardID(docID string, numShards int32) int32 {
	// Use FNV-1a hash (fast, good distribution)
	h := fnv.New32a()
	h.Write([]byte(docID))
	hash := h.Sum32()

	// Modulo to get shard ID
	return int32(hash % uint32(numShards))
}

// SetDataClients updates the data node clients
func (dr *DocumentRouter) SetDataClients(clients map[string]DataNodeClient) {
	dr.dataClients = clients
}
