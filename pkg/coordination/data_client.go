package coordination

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

// DataNodeClient manages communication with a data node
type DataNodeClient struct {
	nodeID   string
	address  string
	logger   *zap.Logger
	conn     *grpc.ClientConn
	client   pb.DataServiceClient
	mu       sync.RWMutex
	connected bool
}

// NewDataNodeClient creates a new data node client
func NewDataNodeClient(nodeID, address string, logger *zap.Logger) *DataNodeClient {
	return &DataNodeClient{
		nodeID:  nodeID,
		address: address,
		logger:  logger,
	}
}

// Connect establishes connection to the data node
func (dc *DataNodeClient) Connect(ctx context.Context) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.connected {
		return nil
	}

	dc.logger.Debug("Connecting to data node",
		zap.String("node_id", dc.nodeID),
		zap.String("address", dc.address))

	// Create gRPC connection with timeout
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		dc.address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to data node %s: %w", dc.nodeID, err)
	}

	dc.conn = conn
	dc.client = pb.NewDataServiceClient(conn)
	dc.connected = true

	dc.logger.Debug("Connected to data node", zap.String("node_id", dc.nodeID))
	return nil
}

// Disconnect closes the connection to the data node
func (dc *DataNodeClient) Disconnect() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if !dc.connected {
		return nil
	}

	if dc.conn != nil {
		if err := dc.conn.Close(); err != nil {
			dc.logger.Error("Error closing connection", zap.String("node_id", dc.nodeID), zap.Error(err))
			return err
		}
	}

	dc.connected = false
	dc.logger.Debug("Disconnected from data node", zap.String("node_id", dc.nodeID))
	return nil
}

// IsConnected returns whether the client is connected
func (dc *DataNodeClient) IsConnected() bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.connected
}

// Search executes a search query on a specific shard
func (dc *DataNodeClient) Search(ctx context.Context, indexName string, shardID int32, query []byte, filterExpression []byte) (*pb.SearchResponse, error) {
	dc.mu.RLock()
	if !dc.connected {
		dc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to data node %s", dc.nodeID)
	}
	client := dc.client
	dc.mu.RUnlock()

	req := &pb.SearchRequest{
		IndexName:        indexName,
		ShardId:          shardID,
		Query:            query,
		FilterExpression: filterExpression,
	}

	resp, err := client.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("search failed on node %s shard %d: %w", dc.nodeID, shardID, err)
	}

	return resp, nil
}

// Count returns the document count for a specific shard
func (dc *DataNodeClient) Count(ctx context.Context, indexName string, shardID int32, query []byte, filterExpression []byte) (*pb.CountResponse, error) {
	dc.mu.RLock()
	if !dc.connected {
		dc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to data node %s", dc.nodeID)
	}
	client := dc.client
	dc.mu.RUnlock()

	req := &pb.CountRequest{
		IndexName:        indexName,
		ShardId:          shardID,
		Query:            query,
		FilterExpression: filterExpression,
	}

	resp, err := client.Count(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("count failed on node %s shard %d: %w", dc.nodeID, shardID, err)
	}

	return resp, nil
}

// IndexDocument indexes a document on a specific shard
func (dc *DataNodeClient) IndexDocument(ctx context.Context, indexName string, shardID int32, docID string, document map[string]interface{}) (*pb.IndexDocumentResponse, error) {
	dc.mu.RLock()
	if !dc.connected {
		dc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to data node %s", dc.nodeID)
	}
	client := dc.client
	dc.mu.RUnlock()

	// Convert document to protobuf Struct
	docStruct, err := convertMapToStruct(document)
	if err != nil {
		return nil, fmt.Errorf("failed to convert document: %w", err)
	}

	req := &pb.IndexDocumentRequest{
		IndexName: indexName,
		ShardId:   shardID,
		DocId:     docID,
		Document:  docStruct,
	}

	resp, err := client.IndexDocument(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("index document failed on node %s shard %d: %w", dc.nodeID, shardID, err)
	}

	return resp, nil
}

// GetDocument retrieves a document by ID from a specific shard
func (dc *DataNodeClient) GetDocument(ctx context.Context, indexName string, shardID int32, docID string) (*pb.GetDocumentResponse, error) {
	dc.mu.RLock()
	if !dc.connected {
		dc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to data node %s", dc.nodeID)
	}
	client := dc.client
	dc.mu.RUnlock()

	req := &pb.GetDocumentRequest{
		IndexName: indexName,
		ShardId:   shardID,
		DocId:     docID,
	}

	resp, err := client.GetDocument(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get document failed on node %s shard %d: %w", dc.nodeID, shardID, err)
	}

	return resp, nil
}

// DeleteDocument deletes a document by ID from a specific shard
func (dc *DataNodeClient) DeleteDocument(ctx context.Context, indexName string, shardID int32, docID string) (*pb.DeleteDocumentResponse, error) {
	dc.mu.RLock()
	if !dc.connected {
		dc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to data node %s", dc.nodeID)
	}
	client := dc.client
	dc.mu.RUnlock()

	req := &pb.DeleteDocumentRequest{
		IndexName: indexName,
		ShardId:   shardID,
		DocId:     docID,
	}

	resp, err := client.DeleteDocument(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("delete document failed on node %s shard %d: %w", dc.nodeID, shardID, err)
	}

	return resp, nil
}

// GetShardStats retrieves statistics for a specific shard
func (dc *DataNodeClient) GetShardStats(ctx context.Context, indexName string, shardID int32) (*pb.ShardStats, error) {
	dc.mu.RLock()
	if !dc.connected {
		dc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to data node %s", dc.nodeID)
	}
	client := dc.client
	dc.mu.RUnlock()

	req := &pb.GetShardStatsRequest{
		IndexName: indexName,
		ShardId:   shardID,
	}

	resp, err := client.GetShardStats(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get shard stats failed on node %s shard %d: %w", dc.nodeID, shardID, err)
	}

	return resp, nil
}

// NodeID returns the node ID
func (dc *DataNodeClient) NodeID() string {
	return dc.nodeID
}

// Address returns the node address
func (dc *DataNodeClient) Address() string {
	return dc.address
}

// Helper function to convert map to protobuf Struct
func convertMapToStruct(m map[string]interface{}) (*structpb.Struct, error) {
	return structpb.NewStruct(m)
}
