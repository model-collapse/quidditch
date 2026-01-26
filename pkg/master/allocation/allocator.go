package allocation

import (
	"fmt"
	"sort"

	"github.com/quidditch/quidditch/pkg/master/raft"
	"go.uber.org/zap"
)

// Allocator handles shard allocation across data nodes
type Allocator struct {
	logger *zap.Logger
}

// NewAllocator creates a new shard allocator
func NewAllocator(logger *zap.Logger) *Allocator {
	return &Allocator{
		logger: logger,
	}
}

// AllocationDecision represents a shard allocation decision
type AllocationDecision struct {
	IndexName string
	ShardID   int32
	IsPrimary bool
	NodeID    string
	Reason    string
}

// AllocateShards allocates shards for a new index across available data nodes
func (a *Allocator) AllocateShards(state *raft.ClusterState, indexName string, numShards, numReplicas int32) ([]AllocationDecision, error) {
	// Get all healthy data nodes
	dataNodes := a.getHealthyDataNodes(state)
	if len(dataNodes) == 0 {
		return nil, fmt.Errorf("no healthy data nodes available")
	}

	decisions := make([]AllocationDecision, 0)

	// Allocate primary shards
	for shardID := int32(0); shardID < numShards; shardID++ {
		node := a.selectNodeForShard(dataNodes, state, indexName, shardID, true)
		if node == nil {
			return nil, fmt.Errorf("failed to allocate primary shard %d", shardID)
		}

		decisions = append(decisions, AllocationDecision{
			IndexName: indexName,
			ShardID:   shardID,
			IsPrimary: true,
			NodeID:    node.NodeID,
			Reason:    "primary_allocation",
		})

		a.logger.Debug("Allocated primary shard",
			zap.String("index", indexName),
			zap.Int32("shard_id", shardID),
			zap.String("node", node.NodeID))
	}

	// Allocate replica shards
	for replica := int32(0); replica < numReplicas; replica++ {
		for shardID := int32(0); shardID < numShards; shardID++ {
			// Find primary shard allocation
			var primaryNode string
			for _, decision := range decisions {
				if decision.ShardID == shardID && decision.IsPrimary {
					primaryNode = decision.NodeID
					break
				}
			}

			// Select different node for replica
			node := a.selectNodeForReplica(dataNodes, state, indexName, shardID, primaryNode)
			if node == nil {
				a.logger.Warn("Failed to allocate replica shard",
					zap.String("index", indexName),
					zap.Int32("shard_id", shardID),
					zap.Int32("replica", replica))
				continue
			}

			decisions = append(decisions, AllocationDecision{
				IndexName: indexName,
				ShardID:   shardID,
				IsPrimary: false,
				NodeID:    node.NodeID,
				Reason:    fmt.Sprintf("replica_%d_allocation", replica),
			})

			a.logger.Debug("Allocated replica shard",
				zap.String("index", indexName),
				zap.Int32("shard_id", shardID),
				zap.Int32("replica", replica),
				zap.String("node", node.NodeID))
		}
	}

	return decisions, nil
}

// RebalanceShards rebalances shards across nodes
func (a *Allocator) RebalanceShards(state *raft.ClusterState) ([]RebalanceDecision, error) {
	dataNodes := a.getHealthyDataNodes(state)
	if len(dataNodes) < 2 {
		return nil, nil // No rebalancing needed
	}

	// Calculate current shard distribution
	nodeShardCounts := make(map[string]int)
	for _, node := range dataNodes {
		nodeShardCounts[node.NodeID] = 0
	}

	for _, shard := range state.ShardRouting {
		if count, exists := nodeShardCounts[shard.NodeID]; exists {
			nodeShardCounts[shard.NodeID] = count + 1
		}
	}

	// Find imbalance
	avgShards := 0
	for _, count := range nodeShardCounts {
		avgShards += count
	}
	if len(nodeShardCounts) > 0 {
		avgShards = avgShards / len(nodeShardCounts)
	}

	decisions := make([]RebalanceDecision, 0)

	// Move shards from overloaded nodes to underloaded nodes
	for {
		overloadedNode := a.findOverloadedNode(nodeShardCounts, avgShards)
		underloadedNode := a.findUnderloadedNode(nodeShardCounts, avgShards)

		if overloadedNode == "" || underloadedNode == "" {
			break // No more rebalancing needed
		}

		// Find a shard to move
		shardToMove := a.findShardToMove(state, overloadedNode)
		if shardToMove == nil {
			break
		}

		decisions = append(decisions, RebalanceDecision{
			IndexName: shardToMove.IndexName,
			ShardID:   shardToMove.ShardID,
			IsPrimary: shardToMove.IsPrimary,
			FromNode:  overloadedNode,
			ToNode:    underloadedNode,
			Reason:    "rebalance",
		})

		// Update counts
		nodeShardCounts[overloadedNode]--
		nodeShardCounts[underloadedNode]++

		a.logger.Info("Rebalancing shard",
			zap.String("index", shardToMove.IndexName),
			zap.Int32("shard_id", shardToMove.ShardID),
			zap.String("from", overloadedNode),
			zap.String("to", underloadedNode))
	}

	return decisions, nil
}

// RebalanceDecision represents a shard rebalancing decision
type RebalanceDecision struct {
	IndexName string
	ShardID   int32
	IsPrimary bool
	FromNode  string
	ToNode    string
	Reason    string
}

// Helper methods

func (a *Allocator) getHealthyDataNodes(state *raft.ClusterState) []*raft.NodeMeta {
	nodes := make([]*raft.NodeMeta, 0)
	for _, node := range state.Nodes {
		if node.NodeType == "data" && node.Status == "healthy" {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func (a *Allocator) selectNodeForShard(nodes []*raft.NodeMeta, state *raft.ClusterState, indexName string, shardID int32, isPrimary bool) *raft.NodeMeta {
	// Count shards per node
	shardCounts := make(map[string]int)
	for _, node := range nodes {
		shardCounts[node.NodeID] = 0
	}

	for _, shard := range state.ShardRouting {
		if count, exists := shardCounts[shard.NodeID]; exists {
			shardCounts[shard.NodeID] = count + 1
		}
	}

	// Sort nodes by shard count (ascending)
	sort.Slice(nodes, func(i, j int) bool {
		return shardCounts[nodes[i].NodeID] < shardCounts[nodes[j].NodeID]
	})

	// Return node with fewest shards
	if len(nodes) > 0 {
		return nodes[0]
	}

	return nil
}

func (a *Allocator) selectNodeForReplica(nodes []*raft.NodeMeta, state *raft.ClusterState, indexName string, shardID int32, primaryNode string) *raft.NodeMeta {
	// Count shards per node
	shardCounts := make(map[string]int)
	for _, node := range nodes {
		shardCounts[node.NodeID] = 0
	}

	for _, shard := range state.ShardRouting {
		if count, exists := shardCounts[shard.NodeID]; exists {
			shardCounts[shard.NodeID] = count + 1
		}
	}

	// Filter out primary node
	candidateNodes := make([]*raft.NodeMeta, 0)
	for _, node := range nodes {
		if node.NodeID != primaryNode {
			candidateNodes = append(candidateNodes, node)
		}
	}

	if len(candidateNodes) == 0 {
		return nil
	}

	// Sort candidates by shard count
	sort.Slice(candidateNodes, func(i, j int) bool {
		return shardCounts[candidateNodes[i].NodeID] < shardCounts[candidateNodes[j].NodeID]
	})

	return candidateNodes[0]
}

func (a *Allocator) findOverloadedNode(shardCounts map[string]int, avgShards int) string {
	var overloadedNode string
	maxShards := avgShards

	for nodeID, count := range shardCounts {
		if count > maxShards+1 { // Allow 1 shard difference
			overloadedNode = nodeID
			maxShards = count
		}
	}

	return overloadedNode
}

func (a *Allocator) findUnderloadedNode(shardCounts map[string]int, avgShards int) string {
	var underloadedNode string
	minShards := avgShards

	for nodeID, count := range shardCounts {
		if count < minShards-1 { // Allow 1 shard difference
			underloadedNode = nodeID
			minShards = count
		}
	}

	return underloadedNode
}

func (a *Allocator) findShardToMove(state *raft.ClusterState, fromNode string) *raft.ShardRouting {
	// Find a non-primary shard to move (replicas are safer to move)
	for _, shard := range state.ShardRouting {
		if shard.NodeID == fromNode && !shard.IsPrimary {
			return shard
		}
	}

	// If no replicas, move a primary
	for _, shard := range state.ShardRouting {
		if shard.NodeID == fromNode {
			return shard
		}
	}

	return nil
}
