package integration

import (
	"context"
	"testing"
	"time"
)

func TestClusterFormation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultClusterConfig()
	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()

	// Start the cluster
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	// Verify cluster topology
	if cluster.NumMasterNodes() != 3 {
		t.Errorf("Expected 3 master nodes, got %d", cluster.NumMasterNodes())
	}

	if cluster.NumCoordNodes() != 1 {
		t.Errorf("Expected 1 coordination node, got %d", cluster.NumCoordNodes())
	}

	if cluster.NumDataNodes() != 2 {
		t.Errorf("Expected 2 data nodes, got %d", cluster.NumDataNodes())
	}

	// Verify leader exists
	leader := cluster.GetLeader()
	if leader == nil {
		t.Fatal("No leader elected")
	}

	t.Logf("Leader: %s", leader.Config.NodeID)
	t.Logf("Cluster uptime: %v", cluster.Uptime())
}

func TestLeaderElection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultClusterConfig()
	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()

	// Start the cluster
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	// Wait for leader election
	if err := cluster.WaitForLeader(10 * time.Second); err != nil {
		t.Fatalf("Leader election failed: %v", err)
	}

	// Get the leader
	leader := cluster.GetLeader()
	if leader == nil {
		t.Fatal("No leader elected")
	}

	// Verify only one leader
	leaderCount := 0
	for i := 0; i < cluster.NumMasterNodes(); i++ {
		node := cluster.GetMasterNode(i)
		if node.Node.IsLeader() {
			leaderCount++
		}
	}

	if leaderCount != 1 {
		t.Errorf("Expected exactly 1 leader, got %d", leaderCount)
	}

	t.Logf("Leader elected: %s", leader.Config.NodeID)
}

func TestCreateIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultClusterConfig()
	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()

	// Start the cluster
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	// Wait for cluster to be ready
	if err := cluster.WaitForClusterReady(10 * time.Second); err != nil {
		t.Fatalf("Cluster not ready: %v", err)
	}

	// Get the leader
	leader := cluster.GetLeader()
	if leader == nil {
		t.Fatal("No leader elected")
	}

	// Create an index through the leader
	indexName := "test-index"
	numShards := int32(5)
	numReplicas := int32(1)

	if err := leader.Node.CreateIndex(ctx, indexName, numShards, numReplicas); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Verify the index was created
	state, err := leader.Node.GetClusterState(ctx)
	if err != nil {
		t.Fatalf("Failed to get cluster state: %v", err)
	}

	index, exists := state.Indices[indexName]
	if !exists {
		t.Fatal("Index was not created")
	}

	if index.NumShards != numShards {
		t.Errorf("Expected %d shards, got %d", numShards, index.NumShards)
	}

	if index.NumReplicas != numReplicas {
		t.Errorf("Expected %d replicas, got %d", numReplicas, index.NumReplicas)
	}

	t.Logf("Created index: %s with %d shards and %d replicas", indexName, numShards, numReplicas)
}

func TestRegisterDataNode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultClusterConfig()
	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()

	// Start the cluster
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	// Wait for cluster to be ready
	if err := cluster.WaitForClusterReady(10 * time.Second); err != nil {
		t.Fatalf("Cluster not ready: %v", err)
	}

	// Get the leader
	leader := cluster.GetLeader()
	if leader == nil {
		t.Fatal("No leader elected")
	}

	// Register a data node
	dataNode := cluster.GetDataNode(0)
	if dataNode == nil {
		t.Fatal("No data node available")
	}

	nodeID := dataNode.Config.NodeID
	nodeType := dataNode.Config.NodeType
	bindAddr := dataNode.Config.BindAddr
	grpcPort := dataNode.Config.GRPCPort

	if err := leader.Node.RegisterNode(ctx, nodeID, nodeType, bindAddr, grpcPort); err != nil {
		t.Fatalf("Failed to register node: %v", err)
	}

	// Verify the node was registered
	state, err := leader.Node.GetClusterState(ctx)
	if err != nil {
		t.Fatalf("Failed to get cluster state: %v", err)
	}

	registeredNode, exists := state.Nodes[nodeID]
	if !exists {
		t.Fatal("Node was not registered")
	}

	if registeredNode.NodeType != nodeType {
		t.Errorf("Expected node type %s, got %s", nodeType, registeredNode.NodeType)
	}

	t.Logf("Registered data node: %s", nodeID)
}

func TestMultipleIndices(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultClusterConfig()
	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()

	// Start the cluster
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	// Wait for cluster to be ready
	if err := cluster.WaitForClusterReady(10 * time.Second); err != nil {
		t.Fatalf("Cluster not ready: %v", err)
	}

	// Get the leader
	leader := cluster.GetLeader()
	if leader == nil {
		t.Fatal("No leader elected")
	}

	// Create multiple indices
	indices := []struct {
		name     string
		shards   int32
		replicas int32
	}{
		{"index-1", 3, 1},
		{"index-2", 5, 0},
		{"index-3", 2, 2},
	}

	for _, idx := range indices {
		if err := leader.Node.CreateIndex(ctx, idx.name, idx.shards, idx.replicas); err != nil {
			t.Fatalf("Failed to create index %s: %v", idx.name, err)
		}
		t.Logf("Created index: %s", idx.name)
	}

	// Verify all indices were created
	state, err := leader.Node.GetClusterState(ctx)
	if err != nil {
		t.Fatalf("Failed to get cluster state: %v", err)
	}

	if len(state.Indices) != len(indices) {
		t.Errorf("Expected %d indices, got %d", len(indices), len(state.Indices))
	}

	for _, idx := range indices {
		index, exists := state.Indices[idx.name]
		if !exists {
			t.Errorf("Index %s was not created", idx.name)
			continue
		}

		if index.NumShards != idx.shards {
			t.Errorf("Index %s: expected %d shards, got %d", idx.name, idx.shards, index.NumShards)
		}

		if index.NumReplicas != idx.replicas {
			t.Errorf("Index %s: expected %d replicas, got %d", idx.name, idx.replicas, index.NumReplicas)
		}
	}
}

func TestDeleteIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultClusterConfig()
	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()

	// Start the cluster
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	// Wait for cluster to be ready
	if err := cluster.WaitForClusterReady(10 * time.Second); err != nil {
		t.Fatalf("Cluster not ready: %v", err)
	}

	// Get the leader
	leader := cluster.GetLeader()
	if leader == nil {
		t.Fatal("No leader elected")
	}

	// Create an index
	indexName := "test-index"
	if err := leader.Node.CreateIndex(ctx, indexName, 5, 1); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Verify it exists
	state, err := leader.Node.GetClusterState(ctx)
	if err != nil {
		t.Fatalf("Failed to get cluster state: %v", err)
	}

	if _, exists := state.Indices[indexName]; !exists {
		t.Fatal("Index was not created")
	}

	// Delete the index
	if err := leader.Node.DeleteIndex(ctx, indexName); err != nil {
		t.Fatalf("Failed to delete index: %v", err)
	}

	// Verify it was deleted
	state, err = leader.Node.GetClusterState(ctx)
	if err != nil {
		t.Fatalf("Failed to get cluster state: %v", err)
	}

	if _, exists := state.Indices[indexName]; exists {
		t.Error("Index should have been deleted")
	}

	t.Logf("Successfully deleted index: %s", indexName)
}

func TestClusterStateConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultClusterConfig()
	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()

	// Start the cluster
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	// Wait for cluster to be ready
	if err := cluster.WaitForClusterReady(10 * time.Second); err != nil {
		t.Fatalf("Cluster not ready: %v", err)
	}

	// Get the leader
	leader := cluster.GetLeader()
	if leader == nil {
		t.Fatal("No leader elected")
	}

	// Create an index
	indexName := "test-index"
	if err := leader.Node.CreateIndex(ctx, indexName, 5, 1); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Wait for state replication
	time.Sleep(500 * time.Millisecond)

	// Query state from all master nodes
	states := make(map[string]int64)
	for i := 0; i < cluster.NumMasterNodes(); i++ {
		node := cluster.GetMasterNode(i)
		state, err := node.Node.GetClusterState(ctx)
		if err != nil {
			t.Fatalf("Failed to get cluster state from node %d: %v", i, err)
		}
		states[node.Config.NodeID] = state.Version
	}

	// Verify all nodes have the same version
	var expectedVersion int64
	first := true
	for nodeID, version := range states {
		if first {
			expectedVersion = version
			first = false
		} else {
			if version != expectedVersion {
				t.Errorf("Node %s has version %d, expected %d", nodeID, version, expectedVersion)
			}
		}
		t.Logf("Node %s: version %d", nodeID, version)
	}
}

func TestSmallCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a minimal cluster
	cfg := &ClusterConfig{
		NumMasters:      1,
		NumCoordination: 1,
		NumData:         1,
		StartPorts: PortRange{
			MasterRaftBase: 20300,
			MasterGRPCBase: 20400,
			CoordRESTBase:  20500,
			DataGRPCBase:   20600,
		},
	}

	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()

	// Start the cluster
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	// Verify single-node operation
	leader := cluster.GetLeader()
	if leader == nil {
		t.Fatal("No leader elected")
	}

	// Single master should immediately become leader
	if !leader.Node.IsLeader() {
		t.Error("Single master node should be leader")
	}

	t.Logf("Small cluster (1-1-1) started successfully")
}
