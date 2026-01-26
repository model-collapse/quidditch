package raft

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/hashicorp/raft"
	"go.uber.org/zap"
)

// CommandType represents the type of command
type CommandType string

const (
	// Index commands
	CommandCreateIndex  CommandType = "create_index"
	CommandDeleteIndex  CommandType = "delete_index"
	CommandUpdateIndex  CommandType = "update_index"

	// Node commands
	CommandRegisterNode   CommandType = "register_node"
	CommandUnregisterNode CommandType = "unregister_node"
	CommandUpdateNode     CommandType = "update_node"
	CommandHeartbeat      CommandType = "heartbeat"

	// Shard commands
	CommandAllocateShard   CommandType = "allocate_shard"
	CommandDeallocateShard CommandType = "deallocate_shard"
	CommandUpdateShard     CommandType = "update_shard"
)

// Command represents a state change command
type Command struct {
	Type    CommandType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ClusterState represents the entire cluster state
type ClusterState struct {
	Version      int64                   `json:"version"`
	ClusterUUID  string                  `json:"cluster_uuid"`
	Indices      map[string]*IndexMeta   `json:"indices"`       // index_name -> metadata
	Nodes        map[string]*NodeMeta    `json:"nodes"`         // node_id -> metadata
	ShardRouting map[string]*ShardRouting `json:"shard_routing"` // "index:shard_id" -> routing
}

// IndexMeta stores index metadata
type IndexMeta struct {
	Name             string            `json:"name"`
	UUID             string            `json:"uuid"`
	Version          int64             `json:"version"`
	NumShards        int32             `json:"num_shards"`
	NumReplicas      int32             `json:"num_replicas"`
	Settings         map[string]string `json:"settings"`
	State            string            `json:"state"` // open, closed, deleting
	CreatedAt        int64             `json:"created_at"`
}

// NodeMeta stores node metadata
type NodeMeta struct {
	NodeID      string            `json:"node_id"`
	NodeType    string            `json:"node_type"` // master, coordination, data
	BindAddr    string            `json:"bind_addr"`
	GRPCPort    int32             `json:"grpc_port"`
	StorageTier string            `json:"storage_tier"`
	MaxShards   int32             `json:"max_shards"`
	Status      string            `json:"status"` // healthy, degraded, offline
	JoinedAt    int64             `json:"joined_at"`
	LastSeen    int64             `json:"last_seen"`
}

// ShardRouting stores shard allocation information
type ShardRouting struct {
	IndexName string `json:"index_name"`
	ShardID   int32  `json:"shard_id"`
	IsPrimary bool   `json:"is_primary"`
	NodeID    string `json:"node_id"`
	State     string `json:"state"` // initializing, started, relocating, unassigned
	Version   int64  `json:"version"`
}

// FSM (Finite State Machine) implements raft.FSM interface
type FSM struct {
	mu     sync.RWMutex
	state  *ClusterState
	logger *zap.Logger
}

// NewFSM creates a new FSM
func NewFSM(logger *zap.Logger) *FSM {
	return &FSM{
		state: &ClusterState{
			Version:      0,
			Indices:      make(map[string]*IndexMeta),
			Nodes:        make(map[string]*NodeMeta),
			ShardRouting: make(map[string]*ShardRouting),
		},
		logger: logger,
	}
}

// Apply applies a Raft log entry to the FSM
func (f *FSM) Apply(log *raft.Log) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		f.logger.Error("Failed to unmarshal command", zap.Error(err))
		return fmt.Errorf("failed to unmarshal command: %w", err)
	}

	f.state.Version++

	switch cmd.Type {
	case CommandCreateIndex:
		return f.applyCreateIndex(cmd.Payload)
	case CommandDeleteIndex:
		return f.applyDeleteIndex(cmd.Payload)
	case CommandUpdateIndex:
		return f.applyUpdateIndex(cmd.Payload)
	case CommandRegisterNode:
		return f.applyRegisterNode(cmd.Payload)
	case CommandUnregisterNode:
		return f.applyUnregisterNode(cmd.Payload)
	case CommandUpdateNode:
		return f.applyUpdateNode(cmd.Payload)
	case CommandHeartbeat:
		return f.applyHeartbeat(cmd.Payload)
	case CommandAllocateShard:
		return f.applyAllocateShard(cmd.Payload)
	case CommandDeallocateShard:
		return f.applyDeallocateShard(cmd.Payload)
	case CommandUpdateShard:
		return f.applyUpdateShard(cmd.Payload)
	default:
		return fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

// Snapshot returns a snapshot of the FSM
func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Deep copy the state
	stateCopy := &ClusterState{
		Version:      f.state.Version,
		ClusterUUID:  f.state.ClusterUUID,
		Indices:      make(map[string]*IndexMeta),
		Nodes:        make(map[string]*NodeMeta),
		ShardRouting: make(map[string]*ShardRouting),
	}

	for k, v := range f.state.Indices {
		stateCopy.Indices[k] = v
	}
	for k, v := range f.state.Nodes {
		stateCopy.Nodes[k] = v
	}
	for k, v := range f.state.ShardRouting {
		stateCopy.ShardRouting[k] = v
	}

	return &fsmSnapshot{state: stateCopy}, nil
}

// Restore restores the FSM from a snapshot
func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()

	var state ClusterState
	if err := json.NewDecoder(rc).Decode(&state); err != nil {
		return fmt.Errorf("failed to decode snapshot: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.state = &state
	f.logger.Info("Restored FSM from snapshot", zap.Int64("version", state.Version))

	return nil
}

// GetState returns a copy of the current state
func (f *FSM) GetState() *ClusterState {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Return a copy to prevent external modifications
	stateCopy := &ClusterState{
		Version:      f.state.Version,
		ClusterUUID:  f.state.ClusterUUID,
		Indices:      make(map[string]*IndexMeta),
		Nodes:        make(map[string]*NodeMeta),
		ShardRouting: make(map[string]*ShardRouting),
	}

	for k, v := range f.state.Indices {
		stateCopy.Indices[k] = v
	}
	for k, v := range f.state.Nodes {
		stateCopy.Nodes[k] = v
	}
	for k, v := range f.state.ShardRouting {
		stateCopy.ShardRouting[k] = v
	}

	return stateCopy
}

// Command application methods

func (f *FSM) applyCreateIndex(payload json.RawMessage) error {
	var index IndexMeta
	if err := json.Unmarshal(payload, &index); err != nil {
		return fmt.Errorf("failed to unmarshal index: %w", err)
	}

	if _, exists := f.state.Indices[index.Name]; exists {
		return fmt.Errorf("index %s already exists", index.Name)
	}

	f.state.Indices[index.Name] = &index
	f.logger.Info("Created index", zap.String("index", index.Name))

	return nil
}

func (f *FSM) applyDeleteIndex(payload json.RawMessage) error {
	var req struct {
		IndexName string `json:"index_name"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}

	delete(f.state.Indices, req.IndexName)
	f.logger.Info("Deleted index", zap.String("index", req.IndexName))

	return nil
}

func (f *FSM) applyUpdateIndex(payload json.RawMessage) error {
	var index IndexMeta
	if err := json.Unmarshal(payload, &index); err != nil {
		return fmt.Errorf("failed to unmarshal index: %w", err)
	}

	if _, exists := f.state.Indices[index.Name]; !exists {
		return fmt.Errorf("index %s does not exist", index.Name)
	}

	f.state.Indices[index.Name] = &index
	f.logger.Info("Updated index", zap.String("index", index.Name))

	return nil
}

func (f *FSM) applyRegisterNode(payload json.RawMessage) error {
	var node NodeMeta
	if err := json.Unmarshal(payload, &node); err != nil {
		return fmt.Errorf("failed to unmarshal node: %w", err)
	}

	f.state.Nodes[node.NodeID] = &node
	f.logger.Info("Registered node", zap.String("node_id", node.NodeID))

	return nil
}

func (f *FSM) applyUnregisterNode(payload json.RawMessage) error {
	var req struct {
		NodeID string `json:"node_id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}

	delete(f.state.Nodes, req.NodeID)
	f.logger.Info("Unregistered node", zap.String("node_id", req.NodeID))

	return nil
}

func (f *FSM) applyUpdateNode(payload json.RawMessage) error {
	var node NodeMeta
	if err := json.Unmarshal(payload, &node); err != nil {
		return fmt.Errorf("failed to unmarshal node: %w", err)
	}

	if _, exists := f.state.Nodes[node.NodeID]; !exists {
		return fmt.Errorf("node %s does not exist", node.NodeID)
	}

	f.state.Nodes[node.NodeID] = &node
	f.logger.Info("Updated node", zap.String("node_id", node.NodeID))

	return nil
}

func (f *FSM) applyHeartbeat(payload json.RawMessage) error {
	var heartbeat struct {
		NodeID   string `json:"node_id"`
		LastSeen int64  `json:"last_seen"`
	}
	if err := json.Unmarshal(payload, &heartbeat); err != nil {
		return fmt.Errorf("failed to unmarshal heartbeat: %w", err)
	}

	if node, exists := f.state.Nodes[heartbeat.NodeID]; exists {
		node.LastSeen = heartbeat.LastSeen
		f.logger.Debug("Heartbeat received", zap.String("node_id", heartbeat.NodeID))
	} else {
		return fmt.Errorf("node %s does not exist", heartbeat.NodeID)
	}

	return nil
}

func (f *FSM) applyAllocateShard(payload json.RawMessage) error {
	var shard ShardRouting
	if err := json.Unmarshal(payload, &shard); err != nil {
		return fmt.Errorf("failed to unmarshal shard: %w", err)
	}

	key := fmt.Sprintf("%s:%d", shard.IndexName, shard.ShardID)
	f.state.ShardRouting[key] = &shard
	f.logger.Info("Allocated shard",
		zap.String("index", shard.IndexName),
		zap.Int32("shard_id", shard.ShardID),
		zap.String("node", shard.NodeID))

	return nil
}

func (f *FSM) applyDeallocateShard(payload json.RawMessage) error {
	var req struct {
		IndexName string `json:"index_name"`
		ShardID   int32  `json:"shard_id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}

	key := fmt.Sprintf("%s:%d", req.IndexName, req.ShardID)
	delete(f.state.ShardRouting, key)
	f.logger.Info("Deallocated shard",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardID))

	return nil
}

func (f *FSM) applyUpdateShard(payload json.RawMessage) error {
	var shard ShardRouting
	if err := json.Unmarshal(payload, &shard); err != nil {
		return fmt.Errorf("failed to unmarshal shard: %w", err)
	}

	key := fmt.Sprintf("%s:%d", shard.IndexName, shard.ShardID)
	f.state.ShardRouting[key] = &shard
	f.logger.Info("Updated shard",
		zap.String("index", shard.IndexName),
		zap.Int32("shard_id", shard.ShardID))

	return nil
}

// fsmSnapshot implements raft.FSMSnapshot
type fsmSnapshot struct {
	state *ClusterState
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		// Encode state as JSON
		data, err := json.Marshal(s.state)
		if err != nil {
			return fmt.Errorf("failed to marshal state: %w", err)
		}

		// Write to sink
		if _, err := sink.Write(data); err != nil {
			return fmt.Errorf("failed to write snapshot: %w", err)
		}

		return sink.Close()
	}()

	if err != nil {
		sink.Cancel()
		return err
	}

	return nil
}

func (s *fsmSnapshot) Release() {}
