package integration

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestRESTAPIClusterHealth(t *testing.T) {
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

	// Create HTTP client
	coordURL := GetCoordNodeURL(cluster, 0)
	client := NewHTTPClient(t, coordURL)

	// Get cluster health
	health, err := GetClusterHealth(t, client)
	if err != nil {
		t.Fatalf("Failed to get cluster health: %v", err)
	}

	t.Logf("Cluster health: %s", health.Status)
	t.Logf("Number of nodes: %d", health.NumberOfNodes)
	t.Logf("Number of data nodes: %d", health.NumberOfDataNodes)

	// Verify health response
	if health.ClusterName != "quidditch-cluster" {
		t.Errorf("Expected cluster name 'quidditch-cluster', got %s", health.ClusterName)
	}
}

func TestRESTAPIRootEndpoint(t *testing.T) {
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

	// Create HTTP client
	coordURL := GetCoordNodeURL(cluster, 0)
	client := NewHTTPClient(t, coordURL)

	// Test root endpoint
	resp, err := client.Get("/")
	if err != nil {
		t.Fatalf("Failed to get root endpoint: %v", err)
	}
	defer resp.Body.Close()

	AssertHTTPStatus(t, resp, http.StatusOK)

	var data map[string]interface{}
	if err := client.DecodeJSON(resp, &data); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	AssertJSONField(t, data, "name", "Quidditch")
	AssertJSONField(t, data, "cluster_name", "quidditch-cluster")
	AssertJSONField(t, data, "tagline", "You Know, for Search (powered by Diagon)")

	t.Logf("Root endpoint response: %v", data)
}

func TestRESTAPIIndexCRUD(t *testing.T) {
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

	// Create HTTP client
	coordURL := GetCoordNodeURL(cluster, 0)
	client := NewHTTPClient(t, coordURL)

	indexName := "test-api-index"

	// Create index
	if err := CreateTestIndex(t, client, indexName, 5, 1); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	t.Logf("Created index: %s", indexName)

	// Get index
	resp, err := client.Get("/" + indexName)
	if err != nil {
		t.Fatalf("Failed to get index: %v", err)
	}
	defer resp.Body.Close()

	AssertHTTPStatus(t, resp, http.StatusOK)

	var indexData map[string]interface{}
	if err := client.DecodeJSON(resp, &indexData); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, exists := indexData[indexName]; !exists {
		t.Error("Index not found in response")
	}

	// Delete index
	resp, err = client.Delete("/" + indexName)
	if err != nil {
		t.Fatalf("Failed to delete index: %v", err)
	}
	defer resp.Body.Close()

	AssertHTTPStatus(t, resp, http.StatusOK)

	t.Logf("Deleted index: %s", indexName)
}

func TestRESTAPISearch(t *testing.T) {
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

	// Create HTTP client
	coordURL := GetCoordNodeURL(cluster, 0)
	client := NewHTTPClient(t, coordURL)

	indexName := "test-search-index"

	// Create index
	if err := CreateTestIndex(t, client, indexName, 5, 1); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Perform search with match_all query
	query := map[string]interface{}{
		"match_all": map[string]interface{}{},
	}

	searchResp, err := SearchIndex(t, client, indexName, query)
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	t.Logf("Search took: %d ms", searchResp.Took)
	t.Logf("Total hits: %d", searchResp.Hits.Total.Value)

	if searchResp.TimedOut {
		t.Error("Search timed out")
	}

	if searchResp.Shards.Failed > 0 {
		t.Errorf("Search had %d failed shards", searchResp.Shards.Failed)
	}
}

func TestRESTAPISearchWithQuery(t *testing.T) {
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

	// Create HTTP client
	coordURL := GetCoordNodeURL(cluster, 0)
	client := NewHTTPClient(t, coordURL)

	indexName := "test-query-index"

	// Create index
	if err := CreateTestIndex(t, client, indexName, 5, 1); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Test match query
	query := map[string]interface{}{
		"match": map[string]interface{}{
			"title": "test document",
		},
	}

	searchResp, err := SearchIndex(t, client, indexName, query)
	if err != nil {
		t.Fatalf("Failed to search with match query: %v", err)
	}

	t.Logf("Match query took: %d ms", searchResp.Took)

	// Test bool query
	boolQuery := map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []interface{}{
				map[string]interface{}{
					"match": map[string]interface{}{
						"title": "test",
					},
				},
			},
			"filter": []interface{}{
				map[string]interface{}{
					"term": map[string]interface{}{
						"status": "published",
					},
				},
			},
		},
	}

	searchResp, err = SearchIndex(t, client, indexName, boolQuery)
	if err != nil {
		t.Fatalf("Failed to search with bool query: %v", err)
	}

	t.Logf("Bool query took: %d ms", searchResp.Took)
}

func TestRESTAPIClusterState(t *testing.T) {
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

	// Create HTTP client
	coordURL := GetCoordNodeURL(cluster, 0)
	client := NewHTTPClient(t, coordURL)

	// Get cluster state
	resp, err := client.Get("/_cluster/state")
	if err != nil {
		t.Fatalf("Failed to get cluster state: %v", err)
	}
	defer resp.Body.Close()

	AssertHTTPStatus(t, resp, http.StatusOK)

	var state map[string]interface{}
	if err := client.DecodeJSON(resp, &state); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	AssertJSONField(t, state, "cluster_name", "quidditch-cluster")

	t.Logf("Cluster state version: %v", state["version"])
}

func TestRESTAPINodes(t *testing.T) {
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

	// Create HTTP client
	coordURL := GetCoordNodeURL(cluster, 0)
	client := NewHTTPClient(t, coordURL)

	// Get nodes info
	resp, err := client.Get("/_nodes")
	if err != nil {
		t.Fatalf("Failed to get nodes: %v", err)
	}
	defer resp.Body.Close()

	AssertHTTPStatus(t, resp, http.StatusOK)

	var nodes map[string]interface{}
	if err := client.DecodeJSON(resp, &nodes); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	AssertJSONField(t, nodes, "cluster_name", "quidditch-cluster")

	// Get nodes stats
	resp, err = client.Get("/_nodes/stats")
	if err != nil {
		t.Fatalf("Failed to get nodes stats: %v", err)
	}
	defer resp.Body.Close()

	AssertHTTPStatus(t, resp, http.StatusOK)

	t.Logf("Nodes API working correctly")
}

func TestRESTAPICount(t *testing.T) {
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

	// Create HTTP client
	coordURL := GetCoordNodeURL(cluster, 0)
	client := NewHTTPClient(t, coordURL)

	indexName := "test-count-index"

	// Create index
	if err := CreateTestIndex(t, client, indexName, 5, 1); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Test count API
	resp, err := client.Get("/" + indexName + "/_count")
	if err != nil {
		t.Fatalf("Failed to get count: %v", err)
	}
	defer resp.Body.Close()

	AssertHTTPStatus(t, resp, http.StatusOK)

	var countResp map[string]interface{}
	if err := client.DecodeJSON(resp, &countResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, exists := countResp["count"]; !exists {
		t.Error("Count field not found in response")
	}

	t.Logf("Count: %v", countResp["count"])
}
