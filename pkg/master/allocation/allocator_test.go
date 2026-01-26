package allocation

import (
	"testing"

	"github.com/quidditch/quidditch/pkg/master/raft"
	"go.uber.org/zap"
)

func TestAllocateShards(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	allocator := NewAllocator(logger)

	// Create cluster state with 3 data nodes
	state := &raft.ClusterState{
		Version:     1,
		ClusterUUID: "test-cluster",
		Indices:     make(map[string]*raft.IndexMeta),
		Nodes: map[string]*raft.NodeMeta{
			"node-1": {
				NodeID:      "node-1",
				NodeType:    "data",
				Status:      "healthy",
				MaxShards:   100,
				StorageTier: "hot",
			},
			"node-2": {
				NodeID:      "node-2",
				NodeType:    "data",
				Status:      "healthy",
				MaxShards:   100,
				StorageTier: "hot",
			},
			"node-3": {
				NodeID:      "node-3",
				NodeType:    "data",
				Status:      "healthy",
				MaxShards:   100,
				StorageTier: "hot",
			},
		},
		ShardRouting: make(map[string]*raft.ShardRouting),
	}

	// Allocate shards for a new index (5 shards, 1 replica)
	decisions, err := allocator.AllocateShards(state, "test-index", 5, 1)
	if err != nil {
		t.Fatalf("AllocateShards failed: %v", err)
	}

	// Should have 5 primary + 5 replica = 10 total allocations
	if len(decisions) != 10 {
		t.Errorf("Expected 10 allocation decisions, got %d", len(decisions))
	}

	// Count primary and replica allocations
	primaryCount := 0
	replicaCount := 0
	for _, decision := range decisions {
		if decision.IsPrimary {
			primaryCount++
		} else {
			replicaCount++
		}
	}

	if primaryCount != 5 {
		t.Errorf("Expected 5 primary shards, got %d", primaryCount)
	}

	if replicaCount != 5 {
		t.Errorf("Expected 5 replica shards, got %d", replicaCount)
	}

	// Verify all shards are allocated to valid nodes
	validNodes := map[string]bool{"node-1": true, "node-2": true, "node-3": true}
	for _, decision := range decisions {
		if !validNodes[decision.NodeID] {
			t.Errorf("Shard allocated to invalid node: %s", decision.NodeID)
		}
	}

	// Verify no shard's primary and replica are on the same node
	shardAllocations := make(map[int32][]string)
	for _, decision := range decisions {
		shardAllocations[decision.ShardID] = append(shardAllocations[decision.ShardID], decision.NodeID)
	}

	for shardID, nodes := range shardAllocations {
		if len(nodes) == 2 && nodes[0] == nodes[1] {
			t.Errorf("Shard %d has primary and replica on same node: %s", shardID, nodes[0])
		}
	}
}

func TestAllocateShardsNoNodes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	allocator := NewAllocator(logger)

	// Create cluster state with no data nodes
	state := &raft.ClusterState{
		Version:      1,
		ClusterUUID:  "test-cluster",
		Indices:      make(map[string]*raft.IndexMeta),
		Nodes:        make(map[string]*raft.NodeMeta),
		ShardRouting: make(map[string]*raft.ShardRouting),
	}

	// Should fail with no nodes available
	_, err := allocator.AllocateShards(state, "test-index", 5, 1)
	if err == nil {
		t.Error("Expected error when no data nodes available")
	}
}

func TestAllocateShardsUnhealthyNodes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	allocator := NewAllocator(logger)

	// Create cluster state with only unhealthy nodes
	state := &raft.ClusterState{
		Version:     1,
		ClusterUUID: "test-cluster",
		Indices:     make(map[string]*raft.IndexMeta),
		Nodes: map[string]*raft.NodeMeta{
			"node-1": {
				NodeID:   "node-1",
				NodeType: "data",
				Status:   "unhealthy",
			},
			"node-2": {
				NodeID:   "node-2",
				NodeType: "coordination", // Wrong type
				Status:   "healthy",
			},
		},
		ShardRouting: make(map[string]*raft.ShardRouting),
	}

	// Should fail with no healthy data nodes
	_, err := allocator.AllocateShards(state, "test-index", 5, 1)
	if err == nil {
		t.Error("Expected error when no healthy data nodes available")
	}
}

func TestAllocateShardsLoadBalancing(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	allocator := NewAllocator(logger)

	// Create cluster state with 3 data nodes
	state := &raft.ClusterState{
		Version:     1,
		ClusterUUID: "test-cluster",
		Indices:     make(map[string]*raft.IndexMeta),
		Nodes: map[string]*raft.NodeMeta{
			"node-1": {NodeID: "node-1", NodeType: "data", Status: "healthy"},
			"node-2": {NodeID: "node-2", NodeType: "data", Status: "healthy"},
			"node-3": {NodeID: "node-3", NodeType: "data", Status: "healthy"},
		},
		ShardRouting: make(map[string]*raft.ShardRouting),
	}

	// Allocate 6 shards (2 per node ideally)
	decisions, err := allocator.AllocateShards(state, "test-index", 6, 0)
	if err != nil {
		t.Fatalf("AllocateShards failed: %v", err)
	}

	// Count allocations per node
	nodeAllocations := make(map[string]int)
	for _, decision := range decisions {
		nodeAllocations[decision.NodeID]++
	}

	// Each node should have 2 shards (balanced)
	for nodeID, count := range nodeAllocations {
		if count != 2 {
			t.Errorf("Node %s has %d shards, expected 2", nodeID, count)
		}
	}
}

func TestRebalanceShards(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	allocator := NewAllocator(logger)

	// Create imbalanced cluster state
	state := &raft.ClusterState{
		Version:     1,
		ClusterUUID: "test-cluster",
		Indices:     make(map[string]*raft.IndexMeta),
		Nodes: map[string]*raft.NodeMeta{
			"node-1": {NodeID: "node-1", NodeType: "data", Status: "healthy"},
			"node-2": {NodeID: "node-2", NodeType: "data", Status: "healthy"},
			"node-3": {NodeID: "node-3", NodeType: "data", Status: "healthy"},
		},
		ShardRouting: map[string]*raft.ShardRouting{
			// Node-1 has 4 shards (overloaded)
			"index-1:0": {IndexName: "index-1", ShardID: 0, IsPrimary: true, NodeID: "node-1"},
			"index-1:1": {IndexName: "index-1", ShardID: 1, IsPrimary: true, NodeID: "node-1"},
			"index-1:2": {IndexName: "index-1", ShardID: 2, IsPrimary: true, NodeID: "node-1"},
			"index-1:3": {IndexName: "index-1", ShardID: 3, IsPrimary: true, NodeID: "node-1"},
			// Node-2 has 1 shard
			"index-1:4": {IndexName: "index-1", ShardID: 4, IsPrimary: true, NodeID: "node-2"},
			// Node-3 has 1 shard
			"index-1:5": {IndexName: "index-1", ShardID: 5, IsPrimary: true, NodeID: "node-3"},
		},
	}

	decisions, err := allocator.RebalanceShards(state)
	if err != nil {
		t.Fatalf("RebalanceShards failed: %v", err)
	}

	// Should produce rebalancing decisions
	if len(decisions) == 0 {
		t.Error("Expected rebalancing decisions for imbalanced cluster")
	}

	// Verify decisions move shards from overloaded node
	for _, decision := range decisions {
		if decision.FromNode != "node-1" {
			t.Errorf("Expected rebalancing from node-1, got %s", decision.FromNode)
		}
		if decision.ToNode == "node-1" {
			t.Error("Should not rebalance back to overloaded node")
		}
	}
}

func TestRebalanceShardsBalancedCluster(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	allocator := NewAllocator(logger)

	// Create balanced cluster state
	state := &raft.ClusterState{
		Version:     1,
		ClusterUUID: "test-cluster",
		Indices:     make(map[string]*raft.IndexMeta),
		Nodes: map[string]*raft.NodeMeta{
			"node-1": {NodeID: "node-1", NodeType: "data", Status: "healthy"},
			"node-2": {NodeID: "node-2", NodeType: "data", Status: "healthy"},
		},
		ShardRouting: map[string]*raft.ShardRouting{
			"index-1:0": {IndexName: "index-1", ShardID: 0, IsPrimary: true, NodeID: "node-1"},
			"index-1:1": {IndexName: "index-1", ShardID: 1, IsPrimary: true, NodeID: "node-2"},
		},
	}

	decisions, err := allocator.RebalanceShards(state)
	if err != nil {
		t.Fatalf("RebalanceShards failed: %v", err)
	}

	// Should not produce any rebalancing decisions for balanced cluster
	if len(decisions) != 0 {
		t.Errorf("Expected no rebalancing for balanced cluster, got %d decisions", len(decisions))
	}
}

func TestRebalanceShardsInsufficientNodes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	allocator := NewAllocator(logger)

	// Create cluster state with only 1 node
	state := &raft.ClusterState{
		Version:     1,
		ClusterUUID: "test-cluster",
		Indices:     make(map[string]*raft.IndexMeta),
		Nodes: map[string]*raft.NodeMeta{
			"node-1": {NodeID: "node-1", NodeType: "data", Status: "healthy"},
		},
		ShardRouting: map[string]*raft.ShardRouting{
			"index-1:0": {IndexName: "index-1", ShardID: 0, IsPrimary: true, NodeID: "node-1"},
		},
	}

	decisions, err := allocator.RebalanceShards(state)
	if err != nil {
		t.Fatalf("RebalanceShards failed: %v", err)
	}

	// Should not produce rebalancing decisions with only 1 node
	if len(decisions) != 0 {
		t.Error("Expected no rebalancing with single node")
	}
}

func TestAllocateShardsWithReplicas(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	allocator := NewAllocator(logger)

	// Create cluster state with 2 nodes
	state := &raft.ClusterState{
		Version:     1,
		ClusterUUID: "test-cluster",
		Indices:     make(map[string]*raft.IndexMeta),
		Nodes: map[string]*raft.NodeMeta{
			"node-1": {NodeID: "node-1", NodeType: "data", Status: "healthy"},
			"node-2": {NodeID: "node-2", NodeType: "data", Status: "healthy"},
		},
		ShardRouting: make(map[string]*raft.ShardRouting),
	}

	// Allocate 2 shards with 1 replica each
	decisions, err := allocator.AllocateShards(state, "test-index", 2, 1)
	if err != nil {
		t.Fatalf("AllocateShards failed: %v", err)
	}

	// Should have 2 primary + 2 replica = 4 total
	if len(decisions) != 4 {
		t.Errorf("Expected 4 allocation decisions, got %d", len(decisions))
	}

	// Verify primary and replica are on different nodes for each shard
	shardNodes := make(map[int32]map[string]bool)
	for _, decision := range decisions {
		if shardNodes[decision.ShardID] == nil {
			shardNodes[decision.ShardID] = make(map[string]bool)
		}
		shardNodes[decision.ShardID][decision.NodeID] = true
	}

	for shardID, nodes := range shardNodes {
		if len(nodes) == 1 {
			t.Errorf("Shard %d has primary and replica on same node", shardID)
		}
		if len(nodes) != 2 {
			t.Errorf("Shard %d should be on 2 nodes, got %d", shardID, len(nodes))
		}
	}
}

func TestAllocateShardsInsufficientNodesForReplicas(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	allocator := NewAllocator(logger)

	// Create cluster state with only 1 node
	state := &raft.ClusterState{
		Version:     1,
		ClusterUUID: "test-cluster",
		Indices:     make(map[string]*raft.IndexMeta),
		Nodes: map[string]*raft.NodeMeta{
			"node-1": {NodeID: "node-1", NodeType: "data", Status: "healthy"},
		},
		ShardRouting: make(map[string]*raft.ShardRouting),
	}

	// Try to allocate with replicas (should succeed for primaries, skip replicas)
	decisions, err := allocator.AllocateShards(state, "test-index", 2, 1)
	if err != nil {
		t.Fatalf("AllocateShards failed: %v", err)
	}

	// Should only have 2 primary shards (no replicas possible)
	primaryCount := 0
	replicaCount := 0
	for _, decision := range decisions {
		if decision.IsPrimary {
			primaryCount++
		} else {
			replicaCount++
		}
	}

	if primaryCount != 2 {
		t.Errorf("Expected 2 primary shards, got %d", primaryCount)
	}

	if replicaCount != 0 {
		t.Errorf("Expected 0 replica shards (insufficient nodes), got %d", replicaCount)
	}
}

func TestGetHealthyDataNodes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	allocator := NewAllocator(logger)

	state := &raft.ClusterState{
		Nodes: map[string]*raft.NodeMeta{
			"node-1": {NodeID: "node-1", NodeType: "data", Status: "healthy"},
			"node-2": {NodeID: "node-2", NodeType: "data", Status: "unhealthy"},
			"node-3": {NodeID: "node-3", NodeType: "coordination", Status: "healthy"},
			"node-4": {NodeID: "node-4", NodeType: "data", Status: "healthy"},
		},
	}

	nodes := allocator.getHealthyDataNodes(state)

	// Should only return healthy data nodes (node-1 and node-4)
	if len(nodes) != 2 {
		t.Errorf("Expected 2 healthy data nodes, got %d", len(nodes))
	}

	healthyNodeIDs := make(map[string]bool)
	for _, node := range nodes {
		healthyNodeIDs[node.NodeID] = true
	}

	if !healthyNodeIDs["node-1"] || !healthyNodeIDs["node-4"] {
		t.Error("Wrong nodes returned")
	}
}

func TestAllocateShardsLargeNumberOfShards(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	allocator := NewAllocator(logger)

	// Create cluster state with 5 nodes
	state := &raft.ClusterState{
		Version:     1,
		ClusterUUID: "test-cluster",
		Indices:     make(map[string]*raft.IndexMeta),
		Nodes:       make(map[string]*raft.NodeMeta),
		ShardRouting: make(map[string]*raft.ShardRouting),
	}

	for i := 1; i <= 5; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		state.Nodes[nodeID] = &raft.NodeMeta{
			NodeID:   nodeID,
			NodeType: "data",
			Status:   "healthy",
		}
	}

	// Allocate 50 shards with 2 replicas
	decisions, err := allocator.AllocateShards(state, "large-index", 50, 2)
	if err != nil {
		t.Fatalf("AllocateShards failed: %v", err)
	}

	// Should have 50 primary + 100 replica = 150 total
	if len(decisions) != 150 {
		t.Errorf("Expected 150 allocation decisions, got %d", len(decisions))
	}

	// Verify distribution is reasonable (roughly balanced)
	nodeAllocations := make(map[string]int)
	for _, decision := range decisions {
		nodeAllocations[decision.NodeID]++
	}

	// Each node should have approximately 30 shards (150 / 5)
	for nodeID, count := range nodeAllocations {
		if count < 25 || count > 35 {
			t.Errorf("Node %s has %d shards, expected around 30", nodeID, count)
		}
	}
}

// Add fmt import at the top
import "fmt"
