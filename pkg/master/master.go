package master

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/quidditch/quidditch/pkg/master/allocation"
	"github.com/quidditch/quidditch/pkg/master/raft"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// MasterNode represents a master node in the Quidditch cluster
type MasterNode struct {
	cfg        *config.MasterConfig
	logger     *zap.Logger
	raftNode   *raft.RaftNode
	grpcServer *grpc.Server
	fsm        *raft.FSM
}

// NewMasterNode creates a new master node
func NewMasterNode(cfg *config.MasterConfig, logger *zap.Logger) (*MasterNode, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Create FSM
	fsm := raft.NewFSM(logger)

	// Create Raft node
	raftCfg := &raft.Config{
		NodeID:    cfg.NodeID,
		RaftAddr:  fmt.Sprintf("%s:%d", cfg.BindAddr, cfg.RaftPort),
		DataDir:   cfg.DataDir,
		Bootstrap: len(cfg.Peers) == 0, // Bootstrap if no peers
		Peers:     cfg.Peers,
		Logger:    logger,
	}

	raftNode, err := raft.NewRaftNode(raftCfg, fsm)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft node: %w", err)
	}

	// Create gRPC server
	grpcServer := grpc.NewServer()

	node := &MasterNode{
		cfg:        cfg,
		logger:     logger,
		raftNode:   raftNode,
		grpcServer: grpcServer,
		fsm:        fsm,
	}

	// Register gRPC service
	masterService := NewMasterService(node, logger)
	pb.RegisterMasterServiceServer(grpcServer, masterService)

	return node, nil
}

// Start starts the master node
func (m *MasterNode) Start(ctx context.Context) error {
	// Start Raft
	if err := m.raftNode.Start(ctx); err != nil {
		return fmt.Errorf("failed to start raft: %w", err)
	}

	// Wait for leader election
	if err := m.raftNode.WaitForLeader(30 * time.Second); err != nil {
		return fmt.Errorf("failed to elect leader: %w", err)
	}

	if m.raftNode.IsLeader() {
		m.logger.Info("This node is the Raft leader")
		// Initialize cluster UUID if this is a new cluster
		if err := m.initializeCluster(); err != nil {
			return fmt.Errorf("failed to initialize cluster: %w", err)
		}
	} else {
		m.logger.Info("This node is a Raft follower", zap.String("leader", m.raftNode.Leader()))
	}

	// Start gRPC server
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", m.cfg.BindAddr, m.cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC port: %w", err)
	}

	go func() {
		m.logger.Info("Starting gRPC server", zap.Int("port", m.cfg.GRPCPort))
		if err := m.grpcServer.Serve(listener); err != nil {
			m.logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop stops the master node
func (m *MasterNode) Stop(ctx context.Context) error {
	m.logger.Info("Stopping master node")

	// Stop gRPC server
	m.grpcServer.GracefulStop()

	// Stop Raft
	if err := m.raftNode.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop raft: %w", err)
	}

	return nil
}

// initializeCluster initializes a new cluster with a UUID
func (m *MasterNode) initializeCluster() error {
	state := m.fsm.GetState()
	if state.ClusterUUID != "" {
		return nil // Already initialized
	}

	// Generate cluster UUID
	clusterUUID := uuid.New().String()

	// This would normally be applied through Raft
	m.logger.Info("Initializing cluster", zap.String("cluster_uuid", clusterUUID))

	// TODO: Apply initialization command through Raft
	// cmd := raft.Command{
	//     Type: "init_cluster",
	//     Payload: ...
	// }
	// return m.raftNode.Apply(cmd, 5*time.Second)

	return nil
}

// CreateIndex creates a new index in the cluster
func (m *MasterNode) CreateIndex(ctx context.Context, indexName string, numShards, numReplicas int32) error {
	if !m.raftNode.IsLeader() {
		return fmt.Errorf("not the leader, redirect to %s", m.raftNode.Leader())
	}

	// Create index metadata
	index := &raft.IndexMeta{
		Name:        indexName,
		UUID:        uuid.New().String(),
		Version:     1,
		NumShards:   numShards,
		NumReplicas: numReplicas,
		Settings:    make(map[string]string),
		State:       "open",
		CreatedAt:   time.Now().Unix(),
	}

	// Marshal payload
	payload, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	// Apply command through Raft
	cmd := raft.Command{
		Type:    raft.CommandCreateIndex,
		Payload: payload,
	}

	if err := m.raftNode.Apply(cmd, 5*time.Second); err != nil {
		return fmt.Errorf("failed to apply create index command: %w", err)
	}

	m.logger.Info("Created index", zap.String("index", indexName))

	// Allocate shards for the newly created index
	if err := m.allocateShards(ctx, indexName, numShards, numReplicas); err != nil {
		m.logger.Error("Failed to allocate shards",
			zap.String("index", indexName),
			zap.Error(err))
		// Don't fail index creation if allocation fails - allocation can be retried
		// The index exists, just without shard assignments yet
	}

	return nil
}

// DeleteIndex deletes an index from the cluster
func (m *MasterNode) DeleteIndex(ctx context.Context, indexName string) error {
	if !m.raftNode.IsLeader() {
		return fmt.Errorf("not the leader, redirect to %s", m.raftNode.Leader())
	}

	// Create delete request
	req := struct {
		IndexName string `json:"index_name"`
	}{
		IndexName: indexName,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Apply command through Raft
	cmd := raft.Command{
		Type:    raft.CommandDeleteIndex,
		Payload: payload,
	}

	if err := m.raftNode.Apply(cmd, 5*time.Second); err != nil {
		return fmt.Errorf("failed to apply delete index command: %w", err)
	}

	m.logger.Info("Deleted index", zap.String("index", indexName))

	return nil
}

// allocateShards allocates shards for an index across data nodes
func (m *MasterNode) allocateShards(ctx context.Context, indexName string, numShards, numReplicas int32) error {
	if !m.raftNode.IsLeader() {
		return fmt.Errorf("not the leader")
	}

	// Get current cluster state
	state := m.fsm.GetState()

	m.logger.Info("Attempting shard allocation",
		zap.String("index", indexName),
		zap.Int32("num_shards", numShards),
		zap.Int32("num_replicas", numReplicas),
		zap.Int("total_nodes", len(state.Nodes)))

	// Count data nodes for debugging
	dataNodeCount := 0
	for _, node := range state.Nodes {
		m.logger.Debug("Node in cluster",
			zap.String("node_id", node.NodeID),
			zap.String("node_type", node.NodeType),
			zap.String("status", node.Status))
		if node.NodeType == "data" && node.Status == "healthy" {
			dataNodeCount++
		}
	}

	m.logger.Info("Data nodes available for allocation",
		zap.Int("count", dataNodeCount))

	// Create allocator and get allocation decisions
	allocator := allocation.NewAllocator(m.logger)
	decisions, err := allocator.AllocateShards(state, indexName, numShards, numReplicas)
	if err != nil {
		return fmt.Errorf("failed to allocate shards: %w", err)
	}

	m.logger.Info("Allocator returned decisions",
		zap.Int("decision_count", len(decisions)))

	// Apply each shard allocation through Raft
	for _, decision := range decisions {
		shardRouting := raft.ShardRouting{
			IndexName: decision.IndexName,
			ShardID:   decision.ShardID,
			IsPrimary: decision.IsPrimary,
			NodeID:    decision.NodeID,
			State:     "initializing",
			Version:   1,
		}

		payload, err := json.Marshal(shardRouting)
		if err != nil {
			return fmt.Errorf("failed to marshal shard routing: %w", err)
		}

		cmd := raft.Command{
			Type:    raft.CommandAllocateShard,
			Payload: payload,
		}

		if err := m.raftNode.Apply(cmd, 5*time.Second); err != nil {
			m.logger.Error("Failed to apply shard allocation",
				zap.String("index", indexName),
				zap.Int32("shard_id", decision.ShardID),
				zap.String("node", decision.NodeID),
				zap.Error(err))
			// Continue with other shards even if one fails
			continue
		}

		m.logger.Info("Allocated shard",
			zap.String("index", indexName),
			zap.Int32("shard_id", decision.ShardID),
			zap.Bool("is_primary", decision.IsPrimary),
			zap.String("node", decision.NodeID))

		// After allocation in Raft, tell the data node to actually create the shard
		go m.createShardOnDataNode(ctx, decision.NodeID, indexName, decision.ShardID)
	}

	return nil
}

// RegisterNode registers a new node in the cluster
func (m *MasterNode) RegisterNode(ctx context.Context, nodeID, nodeType, bindAddr string, grpcPort int32) error {
	if !m.raftNode.IsLeader() {
		return fmt.Errorf("not the leader, redirect to %s", m.raftNode.Leader())
	}

	node := &raft.NodeMeta{
		NodeID:   nodeID,
		NodeType: nodeType,
		BindAddr: bindAddr,
		GRPCPort: grpcPort,
		Status:   "healthy",
		JoinedAt: time.Now().Unix(),
		LastSeen: time.Now().Unix(),
	}

	payload, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to marshal node: %w", err)
	}

	cmd := raft.Command{
		Type:    raft.CommandRegisterNode,
		Payload: payload,
	}

	if err := m.raftNode.Apply(cmd, 5*time.Second); err != nil {
		return fmt.Errorf("failed to apply register node command: %w", err)
	}

	m.logger.Info("Registered node", zap.String("node_id", nodeID))

	return nil
}

// GetClusterState returns the current cluster state
func (m *MasterNode) GetClusterState(ctx context.Context) (*raft.ClusterState, error) {
	return m.fsm.GetState(), nil
}

// createShardOnDataNode creates a shard on the specified data node
func (m *MasterNode) createShardOnDataNode(ctx context.Context, nodeID, indexName string, shardID int32) {
	// Get node information from cluster state
	state := m.fsm.GetState()
	node, exists := state.Nodes[nodeID]
	if !exists {
		m.logger.Error("Node not found in cluster state",
			zap.String("node_id", nodeID))
		return
	}

	// Connect to data node
	addr := fmt.Sprintf("%s:%d", node.BindAddr, node.GRPCPort)
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		m.logger.Error("Failed to connect to data node",
			zap.String("node_id", nodeID),
			zap.String("address", addr),
			zap.Error(err))
		return
	}
	defer conn.Close()

	client := pb.NewDataServiceClient(conn)

	// Create shard on data node
	req := &pb.CreateShardRequest{
		IndexName: indexName,
		ShardId:   shardID,
	}

	m.logger.Info("Creating shard on data node",
		zap.String("node_id", nodeID),
		zap.String("index", indexName),
		zap.Int32("shard_id", shardID))

	// Use a new context with timeout for the RPC call
	rpcCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.CreateShard(rpcCtx, req)
	if err != nil {
		m.logger.Error("Failed to create shard on data node",
			zap.String("node_id", nodeID),
			zap.String("index", indexName),
			zap.Int32("shard_id", shardID),
			zap.Error(err))
		return
	}

	if resp.Acknowledged {
		m.logger.Info("Successfully created shard on data node",
			zap.String("node_id", nodeID),
			zap.String("index", indexName),
			zap.Int32("shard_id", shardID),
			zap.String("shard_key", resp.ShardKey))

		// CRITICAL FIX: Update shard state to STARTED so executor can query it
		m.logger.Info("Updating shard state to STARTED",
			zap.String("index", indexName),
			zap.Int32("shard_id", shardID),
			zap.String("node_id", nodeID))

		// Get current shard routing to preserve IsPrimary field
		state := m.fsm.GetState()
		key := fmt.Sprintf("%s:%d", indexName, shardID)
		currentShard, exists := state.ShardRouting[key]
		if !exists {
			m.logger.Error("Shard not found in routing table during state update",
				zap.String("index", indexName),
				zap.Int32("shard_id", shardID))
			return
		}

		// Update shard state to "started" through Raft
		// CRITICAL: Preserve IsPrimary field from current shard!
		updateRouting := raft.ShardRouting{
			IndexName: indexName,
			ShardID:   shardID,
			IsPrimary: currentShard.IsPrimary, // Preserve IsPrimary!
			NodeID:    nodeID,
			State:     "started",
			Version:   currentShard.Version + 1, // Increment from current version
		}

		payload, err := json.Marshal(updateRouting)
		if err != nil {
			m.logger.Error("Failed to marshal shard state update",
				zap.String("index", indexName),
				zap.Int32("shard_id", shardID),
				zap.Error(err))
			return
		}

		cmd := raft.Command{
			Type:    raft.CommandUpdateShard,
			Payload: payload,
		}

		if err := m.raftNode.Apply(cmd, 5*time.Second); err != nil {
			m.logger.Error("Failed to update shard state to STARTED",
				zap.String("index", indexName),
				zap.Int32("shard_id", shardID),
				zap.Error(err))
			return
		}

		m.logger.Info("Shard state updated to STARTED - now searchable",
			zap.String("index", indexName),
			zap.Int32("shard_id", shardID),
			zap.String("node_id", nodeID))
	} else {
		m.logger.Error("Data node did not acknowledge shard creation",
			zap.String("node_id", nodeID),
			zap.String("index", indexName),
			zap.Int32("shard_id", shardID))
	}
}

// IsLeader returns whether this node is the Raft leader
func (m *MasterNode) IsLeader() bool {
	return m.raftNode.IsLeader()
}

// Leader returns the current leader address
func (m *MasterNode) Leader() string {
	return m.raftNode.Leader()
}
