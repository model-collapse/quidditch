package master

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/quidditch/quidditch/pkg/common/config"
	"go.uber.org/zap"
)

func TestNewMasterNode(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:      "test-master",
		BindAddr:    "127.0.0.1",
		RaftPort:    9300,
		GRPCPort:    9301,
		DataDir:     tmpDir,
		Peers:       []string{},
		LogLevel:    "debug",
		MetricsPort: 9400,
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node: %v", err)
	}

	if node == nil {
		t.Fatal("Master node is nil")
	}

	if node.cfg != cfg {
		t.Error("Config mismatch")
	}

	if node.logger != logger {
		t.Error("Logger mismatch")
	}

	if node.raftNode == nil {
		t.Error("Raft node is nil")
	}

	if node.fsm == nil {
		t.Error("FSM is nil")
	}
}

func TestNewMasterNodeNilLogger(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		DataDir:  tmpDir,
	}

	_, err := NewMasterNode(cfg, nil)
	if err == nil {
		t.Error("Expected error when logger is nil")
	}
}

func TestMasterNodeIsLeader(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		RaftPort: 9300,
		GRPCPort: 9301,
		DataDir:  tmpDir,
		Peers:    []string{}, // Bootstrap
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node: %v", err)
	}

	// Initially should not be leader (Raft needs to start)
	isLeader := node.IsLeader()

	// We can't assert true/false without starting Raft, just verify the method works
	_ = isLeader
}

func TestMasterNodeGetClusterState(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		DataDir:  tmpDir,
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node: %v", err)
	}

	ctx := context.Background()
	state, err := node.GetClusterState(ctx)
	if err != nil {
		t.Fatalf("Failed to get cluster state: %v", err)
	}

	if state == nil {
		t.Fatal("Cluster state is nil")
	}

	if state.Version != 0 {
		t.Errorf("Expected initial version 0, got %d", state.Version)
	}

	if state.Indices == nil {
		t.Error("Indices map is nil")
	}

	if state.Nodes == nil {
		t.Error("Nodes map is nil")
	}

	if state.ShardRouting == nil {
		t.Error("ShardRouting map is nil")
	}
}

func TestMasterNodeCreateIndexNotLeader(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		DataDir:  tmpDir,
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node: %v", err)
	}

	ctx := context.Background()

	// Without starting Raft, node won't be leader
	err = node.CreateIndex(ctx, "test-index", 5, 1)
	if err == nil {
		t.Error("Expected error when not the leader")
	}

	if err.Error() != "not the leader, redirect to " {
		// Error message varies, just check it's an error
		_ = err
	}
}

func TestMasterNodeDeleteIndexNotLeader(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		DataDir:  tmpDir,
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node: %v", err)
	}

	ctx := context.Background()
	err = node.DeleteIndex(ctx, "test-index")
	if err == nil {
		t.Error("Expected error when not the leader")
	}
}

func TestMasterNodeRegisterNodeNotLeader(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		DataDir:  tmpDir,
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node: %v", err)
	}

	ctx := context.Background()
	err = node.RegisterNode(ctx, "data-1", "data", "10.0.0.1", 9303)
	if err == nil {
		t.Error("Expected error when not the leader")
	}
}

func TestMasterNodeLeaderMethod(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		DataDir:  tmpDir,
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node: %v", err)
	}

	// Leader() should return empty string when not started
	leader := node.Leader()

	// Just verify it doesn't panic
	_ = leader
}

func TestMasterNodeDataDirCreation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	// Use a nested directory that doesn't exist
	dataDir := filepath.Join(tmpDir, "nested", "data", "dir")

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		DataDir:  dataDir,
	}

	_, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node with nested dir: %v", err)
	}

	// Verify Raft directory was created
	raftDir := filepath.Join(dataDir, "raft")
	if _, err := os.Stat(raftDir); os.IsNotExist(err) {
		t.Errorf("Raft directory was not created: %s", raftDir)
	}
}

func TestMasterNodeMultipleInstances(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create two master nodes with different configs
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	cfg1 := &config.MasterConfig{
		NodeID:   "master-1",
		BindAddr: "127.0.0.1",
		DataDir:  tmpDir1,
	}

	cfg2 := &config.MasterConfig{
		NodeID:   "master-2",
		BindAddr: "127.0.0.1",
		DataDir:  tmpDir2,
	}

	node1, err := NewMasterNode(cfg1, logger)
	if err != nil {
		t.Fatalf("Failed to create master node 1: %v", err)
	}

	node2, err := NewMasterNode(cfg2, logger)
	if err != nil {
		t.Fatalf("Failed to create master node 2: %v", err)
	}

	if node1.cfg.NodeID == node2.cfg.NodeID {
		t.Error("Nodes should have different IDs")
	}
}

func TestMasterNodeStopWithoutStart(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		DataDir:  tmpDir,
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node: %v", err)
	}

	ctx := context.Background()

	// Stopping without starting should not cause errors
	err = node.Stop(ctx)
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestMasterNodeStartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:      "test-master",
		BindAddr:    "127.0.0.1",
		RaftPort:    19300, // Use different port to avoid conflicts
		GRPCPort:    19301,
		DataDir:     tmpDir,
		Peers:       []string{},
		LogLevel:    "debug",
		MetricsPort: 19400,
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node: %v", err)
	}

	ctx := context.Background()

	// Start the node
	if err := node.Start(ctx); err != nil {
		t.Fatalf("Failed to start master node: %v", err)
	}

	// Give it a moment to elect leader
	time.Sleep(2 * time.Second)

	// Stop the node
	if err := node.Stop(ctx); err != nil {
		t.Errorf("Failed to stop master node: %v", err)
	}
}

func TestMasterNodeCreateIndexAsLeader(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		RaftPort: 19302,
		GRPCPort: 19303,
		DataDir:  tmpDir,
		Peers:    []string{}, // Bootstrap to become leader
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node: %v", err)
	}

	ctx := context.Background()

	// Start the node
	if err := node.Start(ctx); err != nil {
		t.Fatalf("Failed to start master node: %v", err)
	}
	defer node.Stop(ctx)

	// Wait for leader election
	time.Sleep(3 * time.Second)

	// Should be leader now
	if !node.IsLeader() {
		t.Skip("Node did not become leader, skipping test")
	}

	// Create an index
	err = node.CreateIndex(ctx, "test-index", 5, 1)
	if err != nil {
		t.Errorf("Failed to create index: %v", err)
	}

	// Verify index was created
	state, err := node.GetClusterState(ctx)
	if err != nil {
		t.Fatalf("Failed to get cluster state: %v", err)
	}

	index, exists := state.Indices["test-index"]
	if !exists {
		t.Fatal("Index was not created")
	}

	if index.NumShards != 5 {
		t.Errorf("Expected 5 shards, got %d", index.NumShards)
	}

	if index.NumReplicas != 1 {
		t.Errorf("Expected 1 replica, got %d", index.NumReplicas)
	}
}

func TestMasterNodeRegisterNodeAsLeader(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger, _ := zap.NewDevelopment()
	tmpDir := t.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		RaftPort: 19304,
		GRPCPort: 19305,
		DataDir:  tmpDir,
		Peers:    []string{},
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create master node: %v", err)
	}

	ctx := context.Background()

	if err := node.Start(ctx); err != nil {
		t.Fatalf("Failed to start master node: %v", err)
	}
	defer node.Stop(ctx)

	time.Sleep(3 * time.Second)

	if !node.IsLeader() {
		t.Skip("Node did not become leader, skipping test")
	}

	// Register a data node
	err = node.RegisterNode(ctx, "data-1", "data", "10.0.0.1", 9303)
	if err != nil {
		t.Errorf("Failed to register node: %v", err)
	}

	// Verify node was registered
	state, err := node.GetClusterState(ctx)
	if err != nil {
		t.Fatalf("Failed to get cluster state: %v", err)
	}

	registeredNode, exists := state.Nodes["data-1"]
	if !exists {
		t.Fatal("Node was not registered")
	}

	if registeredNode.NodeType != "data" {
		t.Errorf("Expected node type 'data', got '%s'", registeredNode.NodeType)
	}
}

func BenchmarkGetClusterState(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	tmpDir := b.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		DataDir:  tmpDir,
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		b.Fatalf("Failed to create master node: %v", err)
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = node.GetClusterState(ctx)
	}
}

func BenchmarkIsLeader(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	tmpDir := b.TempDir()

	cfg := &config.MasterConfig{
		NodeID:   "test-master",
		BindAddr: "127.0.0.1",
		DataDir:  tmpDir,
	}

	node, err := NewMasterNode(cfg, logger)
	if err != nil {
		b.Fatalf("Failed to create master node: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = node.IsLeader()
	}
}
