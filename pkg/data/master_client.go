package data

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

// MasterClient manages communication with the master node
type MasterClient struct {
	nodeID         string
	masterAddr     string
	logger         *zap.Logger
	conn           *grpc.ClientConn
	client         pb.MasterServiceClient
	mu             sync.RWMutex
	connected      bool
	heartbeatStop  chan struct{}
	heartbeatDone  chan struct{}
}

// NewMasterClient creates a new master client
func NewMasterClient(nodeID, masterAddr string, logger *zap.Logger) *MasterClient {
	return &MasterClient{
		nodeID:        nodeID,
		masterAddr:    masterAddr,
		logger:        logger,
		heartbeatStop: make(chan struct{}),
		heartbeatDone: make(chan struct{}),
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

	// Create gRPC connection
	conn, err := grpc.DialContext(
		ctx,
		mc.masterAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
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

// Register registers this data node with the master
func (mc *MasterClient) Register(ctx context.Context, bindAddr string, grpcPort int32, attributes *pb.NodeAttributes) error {
	mc.mu.RLock()
	if !mc.connected {
		mc.mu.RUnlock()
		return fmt.Errorf("not connected to master")
	}
	client := mc.client
	mc.mu.RUnlock()

	mc.logger.Info("Registering with master",
		zap.String("node_id", mc.nodeID),
		zap.String("bind_addr", bindAddr),
		zap.Int32("grpc_port", grpcPort))

	req := &pb.RegisterNodeRequest{
		NodeId:     mc.nodeID,
		NodeType:   pb.NodeType_NODE_TYPE_DATA,
		BindAddr:   bindAddr,
		GrpcPort:   grpcPort,
		Attributes: attributes,
	}

	// Try to register, handle leader redirection
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		resp, err := client.RegisterNode(ctx, req)
		if err != nil {
			// Check if this is a leader redirection error
			if st, ok := status.FromError(err); ok {
				if st.Code() == codes.FailedPrecondition {
					// Extract leader address from error message and retry
					mc.logger.Info("Not the leader, retrying", zap.String("error", st.Message()))
					time.Sleep(time.Second)
					continue
				}
			}
			return fmt.Errorf("failed to register node: %w", err)
		}

		mc.logger.Info("Successfully registered with master",
			zap.Bool("acknowledged", resp.Acknowledged),
			zap.Int64("cluster_version", resp.ClusterVersion))
		return nil
	}

	return fmt.Errorf("failed to register after %d retries", maxRetries)
}

// StartHeartbeat starts sending periodic heartbeats to the master
func (mc *MasterClient) StartHeartbeat(ctx context.Context, interval time.Duration) {
	mc.logger.Info("Starting heartbeat", zap.Duration("interval", interval))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	go func() {
		defer close(mc.heartbeatDone)

		for {
			select {
			case <-mc.heartbeatStop:
				mc.logger.Info("Stopping heartbeat")
				return
			case <-ticker.C:
				if err := mc.sendHeartbeat(ctx); err != nil {
					mc.logger.Error("Failed to send heartbeat", zap.Error(err))
					// TODO: Implement reconnection logic
				}
			}
		}
	}()
}

// StopHeartbeat stops the heartbeat goroutine
func (mc *MasterClient) StopHeartbeat() {
	close(mc.heartbeatStop)
	<-mc.heartbeatDone
	mc.logger.Info("Heartbeat stopped")
}

// sendHeartbeat sends a single heartbeat to the master
func (mc *MasterClient) sendHeartbeat(ctx context.Context) error {
	mc.mu.RLock()
	if !mc.connected {
		mc.mu.RUnlock()
		return fmt.Errorf("not connected to master")
	}
	client := mc.client
	mc.mu.RUnlock()

	// TODO: Collect actual node statistics
	stats := &pb.NodeStats{
		TotalShards:         0,
		DocsCount:           0,
		StoreSizeBytes:      0,
		CpuUsagePercent:     0.0,
		MemoryUsagePercent:  0.0,
		DiskUsagePercent:    0.0,
		SearchQueriesPerSec: 0,
		IndexingRatePerSec:  0,
	}

	req := &pb.NodeHeartbeatRequest{
		NodeId: mc.nodeID,
		Stats:  stats,
	}

	hbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := client.NodeHeartbeat(hbCtx, req)
	if err != nil {
		return fmt.Errorf("heartbeat failed: %w", err)
	}

	mc.logger.Debug("Heartbeat sent",
		zap.Bool("acknowledged", resp.Acknowledged),
		zap.Int64("cluster_version", resp.ClusterVersion))

	return nil
}

// GetClusterState retrieves the current cluster state from master
func (mc *MasterClient) GetClusterState(ctx context.Context) (*pb.ClusterStateResponse, error) {
	mc.mu.RLock()
	if !mc.connected {
		mc.mu.RUnlock()
		return nil, fmt.Errorf("not connected to master")
	}
	client := mc.client
	mc.mu.RUnlock()

	req := &pb.GetClusterStateRequest{
		IncludeRouting: true,
		IncludeNodes:   true,
		IncludeIndices: true,
	}

	resp, err := client.GetClusterState(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster state: %w", err)
	}

	return resp, nil
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

	req := &pb.GetIndexMetadataRequest{
		IndexName: indexName,
	}

	resp, err := client.GetIndexMetadata(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get index metadata: %w", err)
	}

	return resp, nil
}

// Unregister removes this node from the master
func (mc *MasterClient) Unregister(ctx context.Context) error {
	mc.mu.RLock()
	if !mc.connected {
		mc.mu.RUnlock()
		return fmt.Errorf("not connected to master")
	}
	client := mc.client
	mc.mu.RUnlock()

	mc.logger.Info("Unregistering from master", zap.String("node_id", mc.nodeID))

	req := &pb.UnregisterNodeRequest{
		NodeId: mc.nodeID,
	}

	resp, err := client.UnregisterNode(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to unregister node: %w", err)
	}

	mc.logger.Info("Successfully unregistered from master",
		zap.Bool("acknowledged", resp.Acknowledged))

	return nil
}

// IsConnected returns whether the client is connected
func (mc *MasterClient) IsConnected() bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.connected
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
