package master

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"github.com/quidditch/quidditch/pkg/master/raft"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MasterService implements the gRPC MasterService
type MasterService struct {
	pb.UnimplementedMasterServiceServer
	node      *MasterNode
	logger    *zap.Logger
}

// NewMasterService creates a new master service
func NewMasterService(node *MasterNode, logger *zap.Logger) *MasterService {
	return &MasterService{
		node:      node,
		logger:    logger,
	}
}

// GetClusterState returns the current cluster state
func (s *MasterService) GetClusterState(ctx context.Context, req *pb.GetClusterStateRequest) (*pb.ClusterStateResponse, error) {
	s.logger.Debug("GetClusterState request",
		zap.Bool("include_routing", req.IncludeRouting),
		zap.Bool("include_nodes", req.IncludeNodes),
		zap.Bool("include_indices", req.IncludeIndices))

	state, err := s.node.GetClusterState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get cluster state: %v", err)
	}

	resp := &pb.ClusterStateResponse{
		Version:     state.Version,
		ClusterName: "quidditch-cluster", // Default cluster name
		ClusterUuid: state.ClusterUUID,
		Status:      s.calculateClusterStatus(state),
	}

	// Include indices if requested
	if req.IncludeIndices {
		resp.Indices = s.convertIndicesToProto(state.Indices)
	}

	// Include routing table if requested
	if req.IncludeRouting {
		resp.RoutingTable = s.convertRoutingTableToProto(state.ShardRouting)
	}

	// Include nodes if requested
	if req.IncludeNodes {
		resp.Nodes = s.convertNodesToProto(state.Nodes)
	}

	// Add master node info
	if s.node.IsLeader() {
		resp.MasterNode = &pb.MasterNode{
			NodeId:     s.node.cfg.NodeID,
			NodeName:   s.node.cfg.NodeID,
			ElectedAt:  timestamppb.Now(),
			Term:       1, // TODO: Get actual term from Raft
		}
	}

	return resp, nil
}

// CreateIndex creates a new index
func (s *MasterService) CreateIndex(ctx context.Context, req *pb.CreateIndexRequest) (*pb.CreateIndexResponse, error) {
	s.logger.Info("CreateIndex request", zap.String("index", req.IndexName))

	// Validate request
	if req.IndexName == "" {
		return nil, status.Error(codes.InvalidArgument, "index name is required")
	}
	if req.Settings == nil {
		return nil, status.Error(codes.InvalidArgument, "index settings are required")
	}

	// Check if not leader
	if !s.node.IsLeader() {
		return nil, status.Errorf(codes.FailedPrecondition, "not the leader, redirect to %s", s.node.Leader())
	}

	// Create index metadata
	index := &raft.IndexMeta{
		Name:        req.IndexName,
		UUID:        uuid.New().String(),
		Version:     1,
		NumShards:   req.Settings.NumberOfShards,
		NumReplicas: req.Settings.NumberOfReplicas,
		Settings:    make(map[string]string),
		State:       "open",
		CreatedAt:   time.Now().Unix(),
	}

	// Marshal payload
	payload, err := json.Marshal(index)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal index: %v", err)
	}

	// Apply command through Raft
	cmd := raft.Command{
		Type:    raft.CommandCreateIndex,
		Payload: payload,
	}

	if err := s.node.raftNode.Apply(cmd, 5*time.Second); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create index: %v", err)
	}

	// Get updated state version
	state, _ := s.node.GetClusterState(ctx)

	return &pb.CreateIndexResponse{
		Acknowledged: true,
		IndexName:    req.IndexName,
		Version:      state.Version,
	}, nil
}

// DeleteIndex deletes an index
func (s *MasterService) DeleteIndex(ctx context.Context, req *pb.DeleteIndexRequest) (*pb.DeleteIndexResponse, error) {
	s.logger.Info("DeleteIndex request", zap.String("index", req.IndexName))

	// Validate request
	if req.IndexName == "" {
		return nil, status.Error(codes.InvalidArgument, "index name is required")
	}

	// Check if not leader
	if !s.node.IsLeader() {
		return nil, status.Errorf(codes.FailedPrecondition, "not the leader, redirect to %s", s.node.Leader())
	}

	// Create delete request
	deleteReq := struct {
		IndexName string `json:"index_name"`
	}{
		IndexName: req.IndexName,
	}

	payload, err := json.Marshal(deleteReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal request: %v", err)
	}

	// Apply command through Raft
	cmd := raft.Command{
		Type:    raft.CommandDeleteIndex,
		Payload: payload,
	}

	if err := s.node.raftNode.Apply(cmd, 5*time.Second); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete index: %v", err)
	}

	return &pb.DeleteIndexResponse{
		Acknowledged: true,
	}, nil
}

// UpdateIndexSettings updates index settings
func (s *MasterService) UpdateIndexSettings(ctx context.Context, req *pb.UpdateIndexSettingsRequest) (*pb.UpdateIndexSettingsResponse, error) {
	s.logger.Info("UpdateIndexSettings request", zap.String("index", req.IndexName))

	// Check if not leader
	if !s.node.IsLeader() {
		return nil, status.Errorf(codes.FailedPrecondition, "not the leader, redirect to %s", s.node.Leader())
	}

	// TODO: Implement update index settings
	return nil, status.Error(codes.Unimplemented, "UpdateIndexSettings not yet implemented")
}

// GetIndexMetadata returns metadata for an index
func (s *MasterService) GetIndexMetadata(ctx context.Context, req *pb.GetIndexMetadataRequest) (*pb.IndexMetadataResponse, error) {
	s.logger.Debug("GetIndexMetadata request", zap.String("index", req.IndexName))

	state, err := s.node.GetClusterState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get cluster state: %v", err)
	}

	// Find the index
	var indexMeta *raft.IndexMeta
	for _, idx := range state.Indices {
		if idx.Name == req.IndexName {
			indexMeta = idx
			break
		}
	}

	if indexMeta == nil {
		return nil, status.Errorf(codes.NotFound, "index not found: %s", req.IndexName)
	}

	// Convert to proto
	metadata := &pb.IndexMetadata{
		IndexName: indexMeta.Name,
		IndexUuid: indexMeta.UUID,
		Version:   indexMeta.Version,
		Settings: &pb.IndexSettings{
			NumberOfShards:   indexMeta.NumShards,
			NumberOfReplicas: indexMeta.NumReplicas,
		},
		State:     s.convertIndexStateToProto(indexMeta.State),
		CreatedAt: timestamppb.New(time.Unix(indexMeta.CreatedAt, 0)),
	}

	return &pb.IndexMetadataResponse{
		Metadata: metadata,
	}, nil
}

// AllocateShard allocates a shard to a node
func (s *MasterService) AllocateShard(ctx context.Context, req *pb.AllocateShardRequest) (*pb.AllocateShardResponse, error) {
	s.logger.Info("AllocateShard request",
		zap.String("index", req.IndexName),
		zap.Int32("shard", req.ShardId),
		zap.Bool("primary", req.IsPrimary))

	// Check if not leader
	if !s.node.IsLeader() {
		return nil, status.Errorf(codes.FailedPrecondition, "not the leader, redirect to %s", s.node.Leader())
	}

	// Get current state
	state, err := s.node.GetClusterState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get cluster state: %v", err)
	}

	// TODO: Implement shard allocation logic
	_ = state

	return nil, status.Error(codes.Unimplemented, "AllocateShard not yet implemented")
}

// RebalanceShards triggers shard rebalancing
func (s *MasterService) RebalanceShards(ctx context.Context, req *pb.RebalanceShardsRequest) (*pb.RebalanceShardsResponse, error) {
	s.logger.Info("RebalanceShards request", zap.Bool("dry_run", req.DryRun))

	// Check if not leader
	if !s.node.IsLeader() {
		return nil, status.Errorf(codes.FailedPrecondition, "not the leader, redirect to %s", s.node.Leader())
	}

	// Get current state
	state, err := s.node.GetClusterState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get cluster state: %v", err)
	}

	// TODO: Implement rebalancing logic
	_ = state

	// Convert to proto
	relocations := make([]*pb.ShardRelocation, 0)

	return &pb.RebalanceShardsResponse{
		Relocations: relocations,
	}, nil
}

// RegisterNode registers a new node in the cluster
func (s *MasterService) RegisterNode(ctx context.Context, req *pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error) {
	s.logger.Info("RegisterNode request",
		zap.String("node_id", req.NodeId),
		zap.String("type", req.NodeType.String()))

	// Validate request
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	// Check if not leader
	if !s.node.IsLeader() {
		return nil, status.Errorf(codes.FailedPrecondition, "not the leader, redirect to %s", s.node.Leader())
	}

	node := &raft.NodeMeta{
		NodeID:   req.NodeId,
		NodeType: s.convertNodeTypeFromProto(req.NodeType),
		BindAddr: req.BindAddr,
		GRPCPort: req.GrpcPort,
		Status:   "healthy",
		JoinedAt: time.Now().Unix(),
		LastSeen: time.Now().Unix(),
	}

	payload, err := json.Marshal(node)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal node: %v", err)
	}

	cmd := raft.Command{
		Type:    raft.CommandRegisterNode,
		Payload: payload,
	}

	if err := s.node.raftNode.Apply(cmd, 5*time.Second); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to register node: %v", err)
	}

	// Get updated cluster version
	state, _ := s.node.GetClusterState(ctx)

	return &pb.RegisterNodeResponse{
		Acknowledged:   true,
		ClusterVersion: state.Version,
	}, nil
}

// UnregisterNode removes a node from the cluster
func (s *MasterService) UnregisterNode(ctx context.Context, req *pb.UnregisterNodeRequest) (*pb.UnregisterNodeResponse, error) {
	s.logger.Info("UnregisterNode request", zap.String("node_id", req.NodeId))

	// Check if not leader
	if !s.node.IsLeader() {
		return nil, status.Errorf(codes.FailedPrecondition, "not the leader, redirect to %s", s.node.Leader())
	}

	unregisterReq := struct {
		NodeID string `json:"node_id"`
	}{
		NodeID: req.NodeId,
	}

	payload, err := json.Marshal(unregisterReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal request: %v", err)
	}

	cmd := raft.Command{
		Type:    raft.CommandUnregisterNode,
		Payload: payload,
	}

	if err := s.node.raftNode.Apply(cmd, 5*time.Second); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unregister node: %v", err)
	}

	return &pb.UnregisterNodeResponse{
		Acknowledged: true,
	}, nil
}

// NodeHeartbeat processes node heartbeats
func (s *MasterService) NodeHeartbeat(ctx context.Context, req *pb.NodeHeartbeatRequest) (*pb.NodeHeartbeatResponse, error) {
	s.logger.Debug("NodeHeartbeat from", zap.String("node_id", req.NodeId))

	// Check if not leader
	if !s.node.IsLeader() {
		return nil, status.Errorf(codes.FailedPrecondition, "not the leader, redirect to %s", s.node.Leader())
	}

	// Update node's last seen timestamp
	heartbeat := struct {
		NodeID   string `json:"node_id"`
		LastSeen int64  `json:"last_seen"`
	}{
		NodeID:   req.NodeId,
		LastSeen: time.Now().Unix(),
	}

	payload, err := json.Marshal(heartbeat)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal heartbeat: %v", err)
	}

	cmd := raft.Command{
		Type:    raft.CommandHeartbeat,
		Payload: payload,
	}

	if err := s.node.raftNode.Apply(cmd, 5*time.Second); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to process heartbeat: %v", err)
	}

	// Get current cluster version
	state, _ := s.node.GetClusterState(ctx)

	return &pb.NodeHeartbeatResponse{
		Acknowledged:   true,
		ClusterVersion: state.Version,
	}, nil
}

// WatchClusterState streams cluster state changes
func (s *MasterService) WatchClusterState(req *pb.WatchClusterStateRequest, stream pb.MasterService_WatchClusterStateServer) error {
	s.logger.Info("WatchClusterState request", zap.Int64("from_version", req.FromVersion))

	// TODO: Implement cluster state watching
	// This would involve subscribing to FSM updates and streaming changes
	return status.Error(codes.Unimplemented, "WatchClusterState not yet implemented")
}

// Helper functions for conversions

func (s *MasterService) calculateClusterStatus(state *raft.ClusterState) pb.ClusterStatus {
	// Simple logic: if we have indices and nodes, cluster is green
	if len(state.Indices) > 0 && len(state.Nodes) > 0 {
		return pb.ClusterStatus_CLUSTER_STATUS_GREEN
	}
	if len(state.Indices) > 0 {
		return pb.ClusterStatus_CLUSTER_STATUS_YELLOW
	}
	return pb.ClusterStatus_CLUSTER_STATUS_RED
}

func (s *MasterService) convertIndicesToProto(indices map[string]*raft.IndexMeta) []*pb.IndexMetadata {
	result := make([]*pb.IndexMetadata, 0, len(indices))
	for _, idx := range indices {
		result = append(result, &pb.IndexMetadata{
			IndexName: idx.Name,
			IndexUuid: idx.UUID,
			Version:   idx.Version,
			Settings: &pb.IndexSettings{
				NumberOfShards:   idx.NumShards,
				NumberOfReplicas: idx.NumReplicas,
			},
			State:     s.convertIndexStateToProto(idx.State),
			CreatedAt: timestamppb.New(time.Unix(idx.CreatedAt, 0)),
		})
	}
	return result
}

func (s *MasterService) convertRoutingTableToProto(routing map[string]*raft.ShardRouting) *pb.RoutingTable {
	// TODO: Implement routing table conversion
	_ = routing
	return &pb.RoutingTable{
		Version: 1,
		Indices: make(map[string]*pb.IndexRoutingTable),
	}
}

func (s *MasterService) convertNodesToProto(nodes map[string]*raft.NodeMeta) []*pb.NodeInfo {
	result := make([]*pb.NodeInfo, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, &pb.NodeInfo{
			NodeId:    node.NodeID,
			NodeName:  node.NodeID,
			NodeType:  s.convertNodeTypeToProto(node.NodeType),
			BindAddr:  node.BindAddr,
			GrpcPort:  node.GRPCPort,
			Status:    s.convertNodeStatusToProto(node.Status),
			JoinedAt:  timestamppb.New(time.Unix(node.JoinedAt, 0)),
			LastSeen:  timestamppb.New(time.Unix(node.LastSeen, 0)),
		})
	}
	return result
}

func (s *MasterService) convertIndexStateToProto(state string) pb.IndexMetadata_IndexState {
	switch state {
	case "creating":
		return pb.IndexMetadata_INDEX_STATE_CREATING
	case "open":
		return pb.IndexMetadata_INDEX_STATE_OPEN
	case "closed":
		return pb.IndexMetadata_INDEX_STATE_CLOSED
	case "deleting":
		return pb.IndexMetadata_INDEX_STATE_DELETING
	default:
		return pb.IndexMetadata_INDEX_STATE_UNKNOWN
	}
}

func (s *MasterService) convertNodeTypeToProto(nodeType string) pb.NodeType {
	switch nodeType {
	case "master":
		return pb.NodeType_NODE_TYPE_MASTER
	case "coordination":
		return pb.NodeType_NODE_TYPE_COORDINATION
	case "data":
		return pb.NodeType_NODE_TYPE_DATA
	case "ingest":
		return pb.NodeType_NODE_TYPE_INGEST
	default:
		return pb.NodeType_NODE_TYPE_UNKNOWN
	}
}

func (s *MasterService) convertNodeTypeFromProto(nodeType pb.NodeType) string {
	switch nodeType {
	case pb.NodeType_NODE_TYPE_MASTER:
		return "master"
	case pb.NodeType_NODE_TYPE_COORDINATION:
		return "coordination"
	case pb.NodeType_NODE_TYPE_DATA:
		return "data"
	case pb.NodeType_NODE_TYPE_INGEST:
		return "ingest"
	default:
		return "unknown"
	}
}

func (s *MasterService) convertNodeStatusToProto(status string) pb.NodeStatus {
	switch status {
	case "healthy":
		return pb.NodeStatus_NODE_STATUS_HEALTHY
	case "degraded":
		return pb.NodeStatus_NODE_STATUS_DEGRADED
	case "unhealthy":
		return pb.NodeStatus_NODE_STATUS_UNHEALTHY
	case "offline":
		return pb.NodeStatus_NODE_STATUS_OFFLINE
	default:
		return pb.NodeStatus_NODE_STATUS_UNKNOWN
	}
}
