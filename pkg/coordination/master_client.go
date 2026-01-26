package coordination

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// MasterClient manages communication with the master node from a coordination node
type MasterClient struct {
	masterAddr string
	logger     *zap.Logger
	conn       *grpc.ClientConn
	client     pb.MasterServiceClient
	mu         sync.RWMutex
	connected  bool
}

// NewMasterClient creates a new master client for coordination nodes
func NewMasterClient(masterAddr string, logger *zap.Logger) *MasterClient {
	return &MasterClient{
		masterAddr: masterAddr,
		logger:     logger,
	}
}

// Connect establishes connection to the master node
func (mc *MasterClient) Connect(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.connected {
		return nil
	}

	mc.logger.Info("Connecting to master node", zap.String("address", mc.masterAddr))

	// Create gRPC connection with timeout
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		mc.masterAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to master: %w", err)
	}

	mc.conn = conn
	mc.client = pb.NewMasterServiceClient(conn)
	mc.connected = true

	mc.logger.Info("Connected to master node")
	return nil
}

// Disconnect closes the connection to the master
func (mc *MasterClient) Disconnect() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if !mc.connected {
		return nil
	}

	mc.logger.Info("Disconnecting from master node")

	if mc.conn != nil {
		if err := mc.conn.Close(); err != nil {
			mc.logger.Error("Error closing connection", zap.Error(err))
			return err
		}
	}

	mc.connected = false
	mc.logger.Info("Disconnected from master node")
	return nil
}

// IsConnected returns whether the client is connected
func (mc *MasterClient) IsConnected() bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.connected
}

// CreateIndex creates a new index
func (mc *MasterClient) CreateIndex(ctx context.Context, indexName string, settings *pb.IndexSettings, mappings map[string]*pb.FieldMapping) (*pb.CreateIndexResponse, error) {
	mc.mu.RLock()
	if !mc.connected {
		mc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to master")
	}
	client := mc.client
	mc.mu.RUnlock()

	mc.logger.Info("Creating index", zap.String("index", indexName))

	req := &pb.CreateIndexRequest{
		IndexName: indexName,
		Settings:  settings,
		Mappings:  mappings,
	}

	// Try to create index, handle leader redirection
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		resp, err := client.CreateIndex(ctx, req)
		if err != nil {
			// Check if this is a leader redirection error
			if st, ok := status.FromError(err); ok {
				if st.Code() == codes.FailedPrecondition {
					// Not the leader, retry after delay
					mc.logger.Info("Master not leader, retrying", zap.String("error", st.Message()))
					time.Sleep(time.Second)
					continue
				}
			}
			return nil, fmt.Errorf("failed to create index: %w", err)
		}

		mc.logger.Info("Successfully created index",
			zap.String("index", indexName),
			zap.Bool("acknowledged", resp.Acknowledged))
		return resp, nil
	}

	return nil, fmt.Errorf("failed to create index after %d retries", maxRetries)
}

// DeleteIndex deletes an index
func (mc *MasterClient) DeleteIndex(ctx context.Context, indexName string) (*pb.DeleteIndexResponse, error) {
	mc.mu.RLock()
	if !mc.connected {
		mc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to master")
	}
	client := mc.client
	mc.mu.RUnlock()

	mc.logger.Info("Deleting index", zap.String("index", indexName))

	req := &pb.DeleteIndexRequest{
		IndexName: indexName,
	}

	// Try to delete index, handle leader redirection
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		resp, err := client.DeleteIndex(ctx, req)
		if err != nil {
			if st, ok := status.FromError(err); ok {
				if st.Code() == codes.FailedPrecondition {
					mc.logger.Info("Master not leader, retrying", zap.String("error", st.Message()))
					time.Sleep(time.Second)
					continue
				}
			}
			return nil, fmt.Errorf("failed to delete index: %w", err)
		}

		mc.logger.Info("Successfully deleted index",
			zap.String("index", indexName),
			zap.Bool("acknowledged", resp.Acknowledged))
		return resp, nil
	}

	return nil, fmt.Errorf("failed to delete index after %d retries", maxRetries)
}

// GetIndexMetadata retrieves metadata for a specific index
func (mc *MasterClient) GetIndexMetadata(ctx context.Context, indexName string) (*pb.IndexMetadataResponse, error) {
	mc.mu.RLock()
	if !mc.connected {
		mc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to master")
	}
	client := mc.client
	mc.mu.RUnlock()

	mc.logger.Debug("Getting index metadata", zap.String("index", indexName))

	req := &pb.GetIndexMetadataRequest{
		IndexName: indexName,
	}

	resp, err := client.GetIndexMetadata(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get index metadata: %w", err)
	}

	return resp, nil
}

// GetClusterState retrieves the current cluster state from master
func (mc *MasterClient) GetClusterState(ctx context.Context, includeRouting, includeNodes, includeIndices bool) (*pb.ClusterStateResponse, error) {
	mc.mu.RLock()
	if !mc.connected {
		mc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to master")
	}
	client := mc.client
	mc.mu.RUnlock()

	mc.logger.Debug("Getting cluster state")

	req := &pb.GetClusterStateRequest{
		IncludeRouting: includeRouting,
		IncludeNodes:   includeNodes,
		IncludeIndices: includeIndices,
	}

	resp, err := client.GetClusterState(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster state: %w", err)
	}

	return resp, nil
}

// GetShardRouting retrieves shard routing information for an index
func (mc *MasterClient) GetShardRouting(ctx context.Context, indexName string) (map[int32]*pb.ShardRouting, error) {
	// Get cluster state with routing information
	state, err := mc.GetClusterState(ctx, true, false, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster state: %w", err)
	}

	// Extract shard routing for the specific index
	if state.RoutingTable == nil || state.RoutingTable.Indices == nil {
		return nil, fmt.Errorf("no routing information available")
	}

	indexRouting, exists := state.RoutingTable.Indices[indexName]
	if !exists {
		return nil, fmt.Errorf("index %s not found in routing table", indexName)
	}

	return indexRouting.Shards, nil
}

// UpdateIndexSettings updates settings for an index
func (mc *MasterClient) UpdateIndexSettings(ctx context.Context, indexName string, settings *pb.IndexSettings) (*pb.UpdateIndexSettingsResponse, error) {
	mc.mu.RLock()
	if !mc.connected {
		mc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to master")
	}
	client := mc.client
	mc.mu.RUnlock()

	mc.logger.Info("Updating index settings", zap.String("index", indexName))

	req := &pb.UpdateIndexSettingsRequest{
		IndexName: indexName,
		Settings:  settings,
	}

	// Try to update settings, handle leader redirection
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		resp, err := client.UpdateIndexSettings(ctx, req)
		if err != nil {
			if st, ok := status.FromError(err); ok {
				if st.Code() == codes.FailedPrecondition {
					mc.logger.Info("Master not leader, retrying", zap.String("error", st.Message()))
					time.Sleep(time.Second)
					continue
				}
			}
			return nil, fmt.Errorf("failed to update index settings: %w", err)
		}

		mc.logger.Info("Successfully updated index settings",
			zap.String("index", indexName),
			zap.Bool("acknowledged", resp.Acknowledged))
		return resp, nil
	}

	return nil, fmt.Errorf("failed to update index settings after %d retries", maxRetries)
}

// GetClusterHealth retrieves cluster health information
func (mc *MasterClient) GetClusterHealth(ctx context.Context) (*pb.ClusterStateResponse, error) {
	// Cluster health is derived from cluster state
	return mc.GetClusterState(ctx, false, true, true)
}

// Reconnect attempts to reconnect to the master
func (mc *MasterClient) Reconnect(ctx context.Context) error {
	mc.logger.Info("Attempting to reconnect to master")

	// Disconnect first
	if err := mc.Disconnect(); err != nil {
		mc.logger.Warn("Error during disconnect", zap.Error(err))
	}

	// Wait a bit before reconnecting
	time.Sleep(2 * time.Second)

	// Reconnect
	if err := mc.Connect(ctx); err != nil {
		return fmt.Errorf("reconnection failed: %w", err)
	}

	mc.logger.Info("Successfully reconnected to master")
	return nil
}
