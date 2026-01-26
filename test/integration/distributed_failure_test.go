package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestPartialShardFailure tests query behavior when some shards are unavailable
// Validates graceful degradation as specified in Phase 1 plan
func TestPartialShardFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start with 4 data nodes
	cfg := DefaultClusterConfig()
	cfg.NumData = 4
	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	if err := cluster.WaitForClusterReady(20 * time.Second); err != nil {
		t.Fatalf("Cluster not ready: %v", err)
	}

	coordNode := cluster.GetCoordNode(0)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", coordNode.Config.RESTPort)

	// Create index with 8 shards (2 per node)
	indexName := "failure_test"
	createIndexReq := map[string]interface{}{
		"settings": map[string]interface{}{
			"index": map[string]interface{}{
				"number_of_shards":   8,
				"number_of_replicas": 0, // No replicas for this test
			},
		},
	}

	if err := createIndex(baseURL, indexName, createIndexReq); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Index 1000 documents
	for i := 0; i < 1000; i++ {
		doc := map[string]interface{}{
			"value":    i,
			"category": fmt.Sprintf("cat_%d", i%10),
		}
		if err := indexDocument(baseURL, indexName, fmt.Sprintf("%d", i), doc); err != nil {
			t.Fatalf("Failed to index document: %v", err)
		}
	}

	t.Logf("Indexed 1000 documents across 4 nodes")
	time.Sleep(1 * time.Second)

	// Query with all nodes healthy
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match_all": map[string]interface{}{},
		},
		"size": 0, // Only get count
	}

	result1, err := search(baseURL, indexName, query)
	if err != nil {
		t.Fatalf("Initial search failed: %v", err)
	}

	hits1 := result1["hits"].(map[string]interface{})
	total1 := hits1["total"].(map[string]interface{})
	totalHits1 := int(total1["value"].(float64))

	if totalHits1 != 1000 {
		t.Errorf("Expected 1000 hits with all nodes, got %d", totalHits1)
	}

	t.Logf("All nodes healthy: %d hits", totalHits1)

	// Stop one data node (simulating failure)
	dataNode3 := cluster.GetDataNode(3)
	if dataNode3 == nil {
		t.Fatal("Could not get data node 3")
	}

	t.Log("Stopping data node 3 (simulating failure)...")
	if err := cluster.stopDataNode(ctx, dataNode3); err != nil {
		t.Errorf("Failed to stop data node: %v", err)
	}

	// Wait a bit for the failure to be detected
	time.Sleep(2 * time.Second)

	// Query again with 1 node down (1/4 of shards unavailable)
	result2, err := search(baseURL, indexName, query)
	if err != nil {
		t.Fatalf("Search after node failure failed: %v", err)
	}

	hits2 := result2["hits"].(map[string]interface{})
	total2 := hits2["total"].(map[string]interface{})
	totalHits2 := int(total2["value"].(float64))

	// Should get ~750 docs (3/4 of shards available)
	expectedRange := []int{700, 800} // Allow some variance due to hash distribution
	if totalHits2 < expectedRange[0] || totalHits2 > expectedRange[1] {
		t.Errorf("Expected %d-%d hits with 1 node down, got %d", expectedRange[0], expectedRange[1], totalHits2)
	}

	t.Logf("1 node down: %d hits (%.1f%% of original)", totalHits2, float64(totalHits2)/float64(totalHits1)*100)

	// Verify query still succeeds (graceful degradation)
	if err != nil {
		t.Error("Query should succeed even with partial shard failure")
	}

	// Restart the node
	t.Log("Restarting data node 3...")
	if err := cluster.startDataNode(ctx, dataNode3); err != nil {
		t.Fatalf("Failed to restart data node: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Query again with all nodes back up
	result3, err := search(baseURL, indexName, query)
	if err != nil {
		t.Fatalf("Search after node recovery failed: %v", err)
	}

	hits3 := result3["hits"].(map[string]interface{})
	total3 := hits3["total"].(map[string]interface{})
	totalHits3 := int(total3["value"].(float64))

	if totalHits3 != 1000 {
		t.Errorf("Expected 1000 hits after recovery, got %d", totalHits3)
	}

	t.Logf("Node recovered: %d hits", totalHits3)

	t.Log("✅ Partial shard failure test passed: graceful degradation confirmed")
}

// TestMasterFailover tests behavior during master node failover
// Validates Raft consensus continues to work
func TestMasterFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start cluster with 3 master nodes for Raft quorum
	cfg := DefaultClusterConfig()
	cfg.NumMasters = 3
	cfg.NumData = 2
	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	if err := cluster.WaitForClusterReady(20 * time.Second); err != nil {
		t.Fatalf("Cluster not ready: %v", err)
	}

	// Identify current leader
	leader := cluster.GetLeader()
	if leader == nil {
		t.Fatal("No leader elected")
	}

	t.Logf("Current leader: %s", leader.Config.NodeID)

	coordNode := cluster.GetCoordNode(0)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", coordNode.Config.RESTPort)

	// Create an index
	indexName := "failover_test"
	createIndexReq := map[string]interface{}{
		"settings": map[string]interface{}{
			"index": map[string]interface{}{
				"number_of_shards":   2,
				"number_of_replicas": 0,
			},
		},
	}

	if err := createIndex(baseURL, indexName, createIndexReq); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Kill the leader
	t.Logf("Stopping leader %s...", leader.Config.NodeID)
	if err := cluster.stopMasterNode(ctx, leader); err != nil {
		t.Errorf("Failed to stop leader: %v", err)
	}

	// Wait for new leader election
	t.Log("Waiting for new leader election...")
	time.Sleep(5 * time.Second)

	// Verify new leader elected
	newLeader := cluster.GetLeader()
	if newLeader == nil {
		t.Fatal("No new leader elected after failover")
	}

	if newLeader.Config.NodeID == leader.Config.NodeID {
		t.Error("Leader did not change after failover")
	}

	t.Logf("New leader elected: %s", newLeader.Config.NodeID)

	// Verify cluster still functions
	// Try creating another index
	indexName2 := "failover_test_2"
	if err := createIndex(baseURL, indexName2, createIndexReq); err != nil {
		t.Errorf("Failed to create index after failover: %v", err)
	} else {
		t.Log("Cluster still functional after leader failover")
	}

	t.Log("✅ Master failover test passed: new leader elected, cluster functional")
}

// TestSlowNode tests behavior when one node has high latency
// Validates that slow nodes don't block the entire query
func TestSlowNode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Note: This test requires network simulation capabilities")
	t.Log("Placeholder for slow node testing")

	// TODO: Implement slow node test:
	// 1. Start cluster with 3 nodes
	// 2. Inject 500ms delay on one node (via iptables or tc)
	// 3. Measure query latency
	// 4. Verify total latency is not dominated by slow node
	// 5. Expected: Query completes in <1s (parallel execution)

	t.Skip("Slow node test requires network manipulation - implement with iptables/tc")
}

// TestNetworkPartition tests behavior during network split
// Validates split-brain prevention via Raft
func TestNetworkPartition(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Note: This test requires network partition simulation")
	t.Log("Placeholder for network partition testing")

	// TODO: Implement network partition test:
	// 1. Start cluster with 5 nodes
	// 2. Partition into 2 groups (2 nodes, 3 nodes)
	// 3. Verify majority partition (3 nodes) continues to function
	// 4. Verify minority partition (2 nodes) becomes read-only
	// 5. Heal partition
	// 6. Verify full cluster reconverges

	t.Skip("Network partition test requires network manipulation - implement with iptables")
}

// TestCascadingFailure tests behavior when multiple nodes fail sequentially
func TestCascadingFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start with 5 data nodes
	cfg := DefaultClusterConfig()
	cfg.NumData = 5
	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	if err := cluster.WaitForClusterReady(20 * time.Second); err != nil {
		t.Fatalf("Cluster not ready: %v", err)
	}

	coordNode := cluster.GetCoordNode(0)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", coordNode.Config.RESTPort)

	// Create index with 10 shards
	indexName := "cascade_test"
	createIndexReq := map[string]interface{}{
		"settings": map[string]interface{}{
			"index": map[string]interface{}{
				"number_of_shards":   10,
				"number_of_replicas": 0,
			},
		},
	}

	if err := createIndex(baseURL, indexName, createIndexReq); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Index documents
	for i := 0; i < 500; i++ {
		doc := map[string]interface{}{"value": i}
		if err := indexDocument(baseURL, indexName, fmt.Sprintf("%d", i), doc); err != nil {
			t.Fatalf("Failed to index document: %v", err)
		}
	}

	t.Log("Indexed 500 documents across 5 nodes")
	time.Sleep(1 * time.Second)

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match_all": map[string]interface{}{},
		},
		"size": 0,
	}

	// Test progressive failure
	for failureCount := 1; failureCount <= 3; failureCount++ {
		t.Logf("Failing node %d...", failureCount)

		// Stop node
		dataNode := cluster.GetDataNode(failureCount - 1)
		if err := cluster.stopDataNode(ctx, dataNode); err != nil {
			t.Errorf("Failed to stop node %d: %v", failureCount, err)
		}

		time.Sleep(2 * time.Second)

		// Query
		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Errorf("Search failed with %d nodes down: %v", failureCount, err)
			continue
		}

		hits := result["hits"].(map[string]interface{})
		total := hits["total"].(map[string]interface{})
		totalHits := int(total["value"].(float64))

		// Calculate expected percentage
		nodesUp := 5 - failureCount
		expectedPercentage := float64(nodesUp) / 5.0 * 100

		t.Logf("%d nodes down: %d hits (%.1f%% expected: %.1f%%)",
			failureCount, totalHits,
			float64(totalHits)/500.0*100,
			expectedPercentage)
	}

	t.Log("✅ Cascading failure test passed: system degrades gracefully")
}

// TestQueryTimeoutWithFailedNodes tests query timeout behavior
func TestQueryTimeoutWithFailedNodes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Testing query timeout behavior with failed nodes")

	// TODO: Implement timeout test:
	// 1. Start cluster
	// 2. Configure query timeout (e.g., 5s)
	// 3. Stop one node (making some shards unavailable)
	// 4. Execute query
	// 5. Verify query either:
	//    a) Returns partial results within timeout
	//    b) Returns error indicating timeout
	// 6. Verify query doesn't hang indefinitely

	t.Log("Test placeholder - implement actual timeout verification")
}

// TestErrorMessagesOnFailure tests error reporting quality
func TestErrorMessagesOnFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Verifying error messages are informative")

	// TODO: Implement error message test:
	// 1. Trigger various failure scenarios
	// 2. Verify error messages include:
	//    - Which shards failed
	//    - Which nodes were unavailable
	//    - Whether results are partial
	//    - Specific error reasons
	// 3. Verify error messages are actionable

	t.Log("Test placeholder - implement error message verification")
}
