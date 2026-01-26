package raft

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/raft"
	"go.uber.org/zap"
)

func TestFSMApplyCreateIndex(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fsm := NewFSM(logger)

	// Create index metadata
	index := &IndexMeta{
		Name:        "test-index",
		UUID:        "test-uuid-123",
		Version:     1,
		NumShards:   5,
		NumReplicas: 1,
		Settings:    map[string]string{"codec": "lz4"},
		State:       "open",
		CreatedAt:   1234567890,
	}

	payload, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("Failed to marshal index: %v", err)
	}

	cmd := Command{
		Type:    CommandCreateIndex,
		Payload: payload,
	}

	cmdData, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("Failed to marshal command: %v", err)
	}

	// Apply the command
	log := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  cmdData,
	}

	result := fsm.Apply(log)
	if result != nil {
		if err, ok := result.(error); ok {
			t.Fatalf("Apply returned error: %v", err)
		}
	}

	// Verify the index was created
	state := fsm.GetState()
	if state.Version != 1 {
		t.Errorf("Expected version 1, got %d", state.Version)
	}

	createdIndex, exists := state.Indices["test-index"]
	if !exists {
		t.Fatal("Index was not created")
	}

	if createdIndex.Name != "test-index" {
		t.Errorf("Expected name 'test-index', got '%s'", createdIndex.Name)
	}

	if createdIndex.NumShards != 5 {
		t.Errorf("Expected 5 shards, got %d", createdIndex.NumShards)
	}
}

func TestFSMApplyDeleteIndex(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fsm := NewFSM(logger)

	// First create an index
	index := &IndexMeta{
		Name:        "test-index",
		UUID:        "test-uuid-123",
		Version:     1,
		NumShards:   5,
		NumReplicas: 1,
		Settings:    map[string]string{},
		State:       "open",
		CreatedAt:   1234567890,
	}

	fsm.state.Indices["test-index"] = index
	fsm.state.Version = 1

	// Now delete it
	req := struct {
		IndexName string `json:"index_name"`
	}{
		IndexName: "test-index",
	}

	payload, _ := json.Marshal(req)
	cmd := Command{
		Type:    CommandDeleteIndex,
		Payload: payload,
	}

	cmdData, _ := json.Marshal(cmd)
	log := &raft.Log{
		Index: 2,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  cmdData,
	}

	result := fsm.Apply(log)
	if result != nil {
		if err, ok := result.(error); ok {
			t.Fatalf("Apply returned error: %v", err)
		}
	}

	// Verify the index was deleted
	state := fsm.GetState()
	if state.Version != 2 {
		t.Errorf("Expected version 2, got %d", state.Version)
	}

	_, exists := state.Indices["test-index"]
	if exists {
		t.Error("Index should have been deleted")
	}
}

func TestFSMApplyRegisterNode(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fsm := NewFSM(logger)

	node := &NodeMeta{
		NodeID:      "node-1",
		NodeType:    "data",
		BindAddr:    "10.0.0.1",
		GRPCPort:    9303,
		StorageTier: "hot",
		MaxShards:   100,
		Status:      "healthy",
		JoinedAt:    1234567890,
		LastSeen:    1234567890,
	}

	payload, _ := json.Marshal(node)
	cmd := Command{
		Type:    CommandRegisterNode,
		Payload: payload,
	}

	cmdData, _ := json.Marshal(cmd)
	log := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  cmdData,
	}

	result := fsm.Apply(log)
	if result != nil {
		if err, ok := result.(error); ok {
			t.Fatalf("Apply returned error: %v", err)
		}
	}

	// Verify the node was registered
	state := fsm.GetState()
	registeredNode, exists := state.Nodes["node-1"]
	if !exists {
		t.Fatal("Node was not registered")
	}

	if registeredNode.NodeType != "data" {
		t.Errorf("Expected node type 'data', got '%s'", registeredNode.NodeType)
	}

	if registeredNode.StorageTier != "hot" {
		t.Errorf("Expected storage tier 'hot', got '%s'", registeredNode.StorageTier)
	}
}

func TestFSMApplyAllocateShard(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fsm := NewFSM(logger)

	shard := &ShardRouting{
		IndexName: "test-index",
		ShardID:   0,
		IsPrimary: true,
		NodeID:    "node-1",
		State:     "started",
		Version:   1,
	}

	payload, _ := json.Marshal(shard)
	cmd := Command{
		Type:    CommandAllocateShard,
		Payload: payload,
	}

	cmdData, _ := json.Marshal(cmd)
	log := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  cmdData,
	}

	result := fsm.Apply(log)
	if result != nil {
		if err, ok := result.(error); ok {
			t.Fatalf("Apply returned error: %v", err)
		}
	}

	// Verify the shard was allocated
	state := fsm.GetState()
	key := "test-index:0"
	allocatedShard, exists := state.ShardRouting[key]
	if !exists {
		t.Fatal("Shard was not allocated")
	}

	if allocatedShard.NodeID != "node-1" {
		t.Errorf("Expected node ID 'node-1', got '%s'", allocatedShard.NodeID)
	}

	if !allocatedShard.IsPrimary {
		t.Error("Shard should be primary")
	}
}

func TestFSMSnapshot(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fsm := NewFSM(logger)

	// Populate some state
	fsm.state.Version = 10
	fsm.state.ClusterUUID = "test-cluster-uuid"
	fsm.state.Indices["test-index"] = &IndexMeta{
		Name:      "test-index",
		UUID:      "index-uuid",
		NumShards: 5,
	}
	fsm.state.Nodes["node-1"] = &NodeMeta{
		NodeID:   "node-1",
		NodeType: "data",
	}

	// Create snapshot
	snapshot, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	// Verify snapshot
	fsmSnapshot, ok := snapshot.(*fsmSnapshot)
	if !ok {
		t.Fatal("Snapshot is not of correct type")
	}

	if fsmSnapshot.state.Version != 10 {
		t.Errorf("Expected version 10, got %d", fsmSnapshot.state.Version)
	}

	if fsmSnapshot.state.ClusterUUID != "test-cluster-uuid" {
		t.Errorf("Expected cluster UUID 'test-cluster-uuid', got '%s'", fsmSnapshot.state.ClusterUUID)
	}

	if len(fsmSnapshot.state.Indices) != 1 {
		t.Errorf("Expected 1 index, got %d", len(fsmSnapshot.state.Indices))
	}

	if len(fsmSnapshot.state.Nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(fsmSnapshot.state.Nodes))
	}
}

func TestFSMRestore(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fsm := NewFSM(logger)

	// Create state to restore
	state := &ClusterState{
		Version:     20,
		ClusterUUID: "restored-cluster-uuid",
		Indices: map[string]*IndexMeta{
			"restored-index": {
				Name:      "restored-index",
				UUID:      "restored-uuid",
				NumShards: 3,
			},
		},
		Nodes: map[string]*NodeMeta{
			"restored-node": {
				NodeID:   "restored-node",
				NodeType: "coordination",
			},
		},
		ShardRouting: map[string]*ShardRouting{},
	}

	// Marshal state to JSON
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Failed to marshal state: %v", err)
	}

	// Create a mock ReadCloser
	rc := &mockReadCloser{data: data}

	// Restore from snapshot
	if err := fsm.Restore(rc); err != nil {
		t.Fatalf("Failed to restore: %v", err)
	}

	// Verify restored state
	restoredState := fsm.GetState()

	if restoredState.Version != 20 {
		t.Errorf("Expected version 20, got %d", restoredState.Version)
	}

	if restoredState.ClusterUUID != "restored-cluster-uuid" {
		t.Errorf("Expected cluster UUID 'restored-cluster-uuid', got '%s'", restoredState.ClusterUUID)
	}

	if len(restoredState.Indices) != 1 {
		t.Errorf("Expected 1 index, got %d", len(restoredState.Indices))
	}

	if len(restoredState.Nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(restoredState.Nodes))
	}

	restoredIndex, exists := restoredState.Indices["restored-index"]
	if !exists {
		t.Fatal("Restored index not found")
	}

	if restoredIndex.NumShards != 3 {
		t.Errorf("Expected 3 shards, got %d", restoredIndex.NumShards)
	}
}

func TestFSMGetStateConcurrency(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fsm := NewFSM(logger)

	// Populate state
	fsm.state.Indices["test-index"] = &IndexMeta{Name: "test-index"}

	// Get state concurrently from multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			state := fsm.GetState()
			if len(state.Indices) != 1 {
				t.Error("Concurrent GetState failed")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestFSMApplyInvalidCommand(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fsm := NewFSM(logger)

	// Create command with unknown type
	cmd := Command{
		Type:    "unknown_command",
		Payload: json.RawMessage(`{}`),
	}

	cmdData, _ := json.Marshal(cmd)
	log := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  cmdData,
	}

	result := fsm.Apply(log)
	if result == nil {
		t.Fatal("Expected error for unknown command type")
	}

	if err, ok := result.(error); ok {
		if err.Error() == "" {
			t.Error("Expected non-empty error message")
		}
	} else {
		t.Error("Result should be an error")
	}
}

func TestFSMApplyMalformedJSON(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fsm := NewFSM(logger)

	// Create log with malformed JSON
	log := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  []byte("not valid json"),
	}

	result := fsm.Apply(log)
	if result == nil {
		t.Fatal("Expected error for malformed JSON")
	}
}

func TestFSMStateVersionIncrement(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fsm := NewFSM(logger)

	if fsm.state.Version != 0 {
		t.Errorf("Initial version should be 0, got %d", fsm.state.Version)
	}

	// Apply a command
	node := &NodeMeta{NodeID: "node-1", NodeType: "data"}
	payload, _ := json.Marshal(node)
	cmd := Command{Type: CommandRegisterNode, Payload: payload}
	cmdData, _ := json.Marshal(cmd)

	log := &raft.Log{Index: 1, Term: 1, Type: raft.LogCommand, Data: cmdData}
	fsm.Apply(log)

	if fsm.state.Version != 1 {
		t.Errorf("Version should be 1 after first command, got %d", fsm.state.Version)
	}

	// Apply another command
	log2 := &raft.Log{Index: 2, Term: 1, Type: raft.LogCommand, Data: cmdData}
	fsm.Apply(log2)

	if fsm.state.Version != 2 {
		t.Errorf("Version should be 2 after second command, got %d", fsm.state.Version)
	}
}

// Mock ReadCloser for testing
type mockReadCloser struct {
	data []byte
	pos  int
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.pos >= len(m.data) {
		return 0, nil
	}
	n = copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	return nil
}
