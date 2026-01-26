package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestDistributedSearchBasic tests basic distributed search across multiple data nodes
func TestDistributedSearchBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultClusterConfig()
	cfg.NumData = 3 // Use 3 data nodes for distributed testing
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
	if err := cluster.WaitForClusterReady(15 * time.Second); err != nil {
		t.Fatalf("Cluster not ready: %v", err)
	}

	// Get coordination node for API calls
	coordNode := cluster.GetCoordNode(0)
	if coordNode == nil {
		t.Fatal("No coordination node available")
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", coordNode.Config.RESTPort)

	// Create index with 6 shards (2 per data node)
	indexName := "products"
	createIndexReq := map[string]interface{}{
		"settings": map[string]interface{}{
			"index": map[string]interface{}{
				"number_of_shards":   6,
				"number_of_replicas": 0,
			},
		},
	}

	if err := createIndex(baseURL, indexName, createIndexReq); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	t.Logf("Created index %s with 6 shards", indexName)

	// Wait for shards to be allocated
	time.Sleep(2 * time.Second)

	// Index documents distributed across shards
	docs := []map[string]interface{}{
		{"id": "1", "name": "Laptop", "category": "electronics", "price": 999.99, "stock": 50},
		{"id": "2", "name": "Phone", "category": "electronics", "price": 599.99, "stock": 100},
		{"id": "3", "name": "Tablet", "category": "electronics", "price": 399.99, "stock": 75},
		{"id": "4", "name": "Monitor", "category": "electronics", "price": 299.99, "stock": 30},
		{"id": "5", "name": "Keyboard", "category": "accessories", "price": 79.99, "stock": 200},
		{"id": "6", "name": "Mouse", "category": "accessories", "price": 49.99, "stock": 300},
		{"id": "7", "name": "Headphones", "category": "accessories", "price": 149.99, "stock": 150},
		{"id": "8", "name": "Webcam", "category": "accessories", "price": 89.99, "stock": 80},
		{"id": "9", "name": "Speaker", "category": "electronics", "price": 129.99, "stock": 60},
		{"id": "10", "name": "Printer", "category": "electronics", "price": 199.99, "stock": 40},
	}

	for _, doc := range docs {
		docID := doc["id"].(string)
		delete(doc, "id") // Remove id from document body
		if err := indexDocument(baseURL, indexName, docID, doc); err != nil {
			t.Fatalf("Failed to index document %s: %v", docID, err)
		}
	}

	t.Logf("Indexed %d documents", len(docs))

	// Wait for documents to be searchable
	time.Sleep(1 * time.Second)

	// Test 1: Match all query - should return all documents
	t.Run("MatchAll_AcrossNodes", func(t *testing.T) {
		query := map[string]interface{}{
			"query": map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
			"size": 100,
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		hits := result["hits"].(map[string]interface{})
		total := hits["total"].(map[string]interface{})
		totalHits := int(total["value"].(float64))

		if totalHits != len(docs) {
			t.Errorf("Expected %d total hits, got %d", len(docs), totalHits)
		}

		hitsArray := hits["hits"].([]interface{})
		if len(hitsArray) != len(docs) {
			t.Errorf("Expected %d returned hits, got %d", len(docs), len(hitsArray))
		}

		t.Logf("MatchAll query returned %d hits from distributed shards", totalHits)
	})

	// Test 2: Term query - filter by category
	t.Run("TermQuery_Electronics", func(t *testing.T) {
		query := map[string]interface{}{
			"query": map[string]interface{}{
				"term": map[string]interface{}{
					"category": "electronics",
				},
			},
			"size": 100,
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		hits := result["hits"].(map[string]interface{})
		hitsArray := hits["hits"].([]interface{})

		// Count electronics docs
		electronicsCount := 0
		for _, doc := range docs {
			if doc["category"] == "electronics" {
				electronicsCount++
			}
		}

		if len(hitsArray) != electronicsCount {
			t.Errorf("Expected %d electronics hits, got %d", electronicsCount, len(hitsArray))
		}

		t.Logf("Term query for 'electronics' returned %d hits", len(hitsArray))
	})

	// Test 3: Pagination across nodes
	t.Run("Pagination_AcrossNodes", func(t *testing.T) {
		// Get first page
		query1 := map[string]interface{}{
			"query": map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
			"from": 0,
			"size": 5,
		}

		result1, err := search(baseURL, indexName, query1)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		hits1 := result1["hits"].(map[string]interface{})
		hitsArray1 := hits1["hits"].([]interface{})

		if len(hitsArray1) != 5 {
			t.Errorf("Expected 5 hits in first page, got %d", len(hitsArray1))
		}

		// Get second page
		query2 := map[string]interface{}{
			"query": map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
			"from": 5,
			"size": 5,
		}

		result2, err := search(baseURL, indexName, query2)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		hits2 := result2["hits"].(map[string]interface{})
		hitsArray2 := hits2["hits"].([]interface{})

		if len(hitsArray2) != 5 {
			t.Errorf("Expected 5 hits in second page, got %d", len(hitsArray2))
		}

		// Verify no duplicates between pages
		ids1 := make(map[string]bool)
		for _, hit := range hitsArray1 {
			hitMap := hit.(map[string]interface{})
			ids1[hitMap["_id"].(string)] = true
		}

		for _, hit := range hitsArray2 {
			hitMap := hit.(map[string]interface{})
			id := hitMap["_id"].(string)
			if ids1[id] {
				t.Errorf("Duplicate document ID %s found across pages", id)
			}
		}

		t.Logf("Pagination test passed: no duplicates across pages")
	})
}

// TestDistributedSearchWithAggregations tests aggregations across multiple data nodes
func TestDistributedSearchWithAggregations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultClusterConfig()
	cfg.NumData = 3 // Use 3 data nodes
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
	if err := cluster.WaitForClusterReady(15 * time.Second); err != nil {
		t.Fatalf("Cluster not ready: %v", err)
	}

	// Get coordination node
	coordNode := cluster.GetCoordNode(0)
	if coordNode == nil {
		t.Fatal("No coordination node available")
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", coordNode.Config.RESTPort)

	// Create index
	indexName := "orders"
	createIndexReq := map[string]interface{}{
		"settings": map[string]interface{}{
			"index": map[string]interface{}{
				"number_of_shards":   6,
				"number_of_replicas": 0,
			},
		},
	}

	if err := createIndex(baseURL, indexName, createIndexReq); err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Wait for allocation
	time.Sleep(2 * time.Second)

	// Index sample orders
	orders := []map[string]interface{}{
		{"id": "1", "customer": "Alice", "amount": 100.0, "status": "completed"},
		{"id": "2", "customer": "Bob", "amount": 200.0, "status": "completed"},
		{"id": "3", "customer": "Charlie", "amount": 150.0, "status": "pending"},
		{"id": "4", "customer": "Alice", "amount": 300.0, "status": "completed"},
		{"id": "5", "customer": "Bob", "amount": 50.0, "status": "cancelled"},
		{"id": "6", "customer": "Charlie", "amount": 250.0, "status": "completed"},
		{"id": "7", "customer": "Alice", "amount": 180.0, "status": "pending"},
		{"id": "8", "customer": "Bob", "amount": 120.0, "status": "completed"},
		{"id": "9", "customer": "Charlie", "amount": 90.0, "status": "completed"},
		{"id": "10", "customer": "Alice", "amount": 210.0, "status": "completed"},
	}

	for _, order := range orders {
		orderID := order["id"].(string)
		delete(order, "id")
		if err := indexDocument(baseURL, indexName, orderID, order); err != nil {
			t.Fatalf("Failed to index order %s: %v", orderID, err)
		}
	}

	t.Logf("Indexed %d orders", len(orders))
	time.Sleep(1 * time.Second)

	// Test 1: Terms aggregation across nodes
	t.Run("TermsAggregation_CustomerCounts", func(t *testing.T) {
		query := map[string]interface{}{
			"query": map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
			"size": 0, // Only get aggregations
			"aggs": map[string]interface{}{
				"customers": map[string]interface{}{
					"terms": map[string]interface{}{
						"field": "customer",
						"size":  10,
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs, ok := result["aggregations"].(map[string]interface{})
		if !ok {
			t.Fatal("No aggregations in response")
		}

		customers, ok := aggs["customers"].(map[string]interface{})
		if !ok {
			t.Fatal("No 'customers' aggregation")
		}

		buckets := customers["buckets"].([]interface{})
		if len(buckets) != 3 { // Alice, Bob, Charlie
			t.Errorf("Expected 3 customer buckets, got %d", len(buckets))
		}

		// Verify bucket counts
		customerCounts := make(map[string]int)
		for _, order := range orders {
			customerCounts[order["customer"].(string)]++
		}

		for _, bucket := range buckets {
			b := bucket.(map[string]interface{})
			customer := b["key"].(string)
			docCount := int(b["doc_count"].(float64))
			expectedCount := customerCounts[customer]

			if docCount != expectedCount {
				t.Errorf("Customer %s: expected %d orders, got %d", customer, expectedCount, docCount)
			}
		}

		t.Logf("Terms aggregation across nodes: %d unique customers", len(buckets))
	})

	// Test 2: Stats aggregation across nodes
	t.Run("StatsAggregation_OrderAmounts", func(t *testing.T) {
		query := map[string]interface{}{
			"query": map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
			"size": 0,
			"aggs": map[string]interface{}{
				"amount_stats": map[string]interface{}{
					"stats": map[string]interface{}{
						"field": "amount",
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})
		stats := aggs["amount_stats"].(map[string]interface{})

		count := int(stats["count"].(float64))
		min := stats["min"].(float64)
		max := stats["max"].(float64)
		avg := stats["avg"].(float64)
		sum := stats["sum"].(float64)

		if count != len(orders) {
			t.Errorf("Expected count %d, got %d", len(orders), count)
		}

		// Calculate expected stats
		var expectedSum float64
		expectedMin := 1000000.0
		expectedMax := 0.0

		for _, order := range orders {
			amount := order["amount"].(float64)
			expectedSum += amount
			if amount < expectedMin {
				expectedMin = amount
			}
			if amount > expectedMax {
				expectedMax = amount
			}
		}
		expectedAvg := expectedSum / float64(len(orders))

		if min != expectedMin {
			t.Errorf("Expected min %f, got %f", expectedMin, min)
		}
		if max != expectedMax {
			t.Errorf("Expected max %f, got %f", expectedMax, max)
		}
		if sum != expectedSum {
			t.Errorf("Expected sum %f, got %f", expectedSum, sum)
		}
		if avg != expectedAvg {
			t.Errorf("Expected avg %f, got %f", expectedAvg, avg)
		}

		t.Logf("Stats aggregation: count=%d, min=%.2f, max=%.2f, avg=%.2f, sum=%.2f",
			count, min, max, avg, sum)
	})

	// Test 3: Multiple aggregations in one query
	t.Run("MultipleAggregations", func(t *testing.T) {
		query := map[string]interface{}{
			"query": map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
			"size": 0,
			"aggs": map[string]interface{}{
				"by_status": map[string]interface{}{
					"terms": map[string]interface{}{
						"field": "status",
						"size":  10,
					},
				},
				"amount_stats": map[string]interface{}{
					"stats": map[string]interface{}{
						"field": "amount",
					},
				},
				"unique_customers": map[string]interface{}{
					"cardinality": map[string]interface{}{
						"field": "customer",
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})

		// Verify all aggregations present
		if _, ok := aggs["by_status"]; !ok {
			t.Error("Missing 'by_status' aggregation")
		}
		if _, ok := aggs["amount_stats"]; !ok {
			t.Error("Missing 'amount_stats' aggregation")
		}
		if _, ok := aggs["unique_customers"]; !ok {
			t.Error("Missing 'unique_customers' aggregation")
		}

		// Verify cardinality
		cardinality := aggs["unique_customers"].(map[string]interface{})
		uniqueCount := int(cardinality["value"].(float64))

		uniqueCustomers := make(map[string]bool)
		for _, order := range orders {
			uniqueCustomers[order["customer"].(string)] = true
		}

		if uniqueCount != len(uniqueCustomers) {
			t.Errorf("Expected %d unique customers, got %d", len(uniqueCustomers), uniqueCount)
		}

		t.Logf("Multiple aggregations test passed: %d aggregations returned", len(aggs))
	})
}

// Helper functions

func createIndex(baseURL, indexName string, body map[string]interface{}) error {
	url := fmt.Sprintf("%s/%s", baseURL, indexName)
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create index failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func indexDocument(baseURL, indexName, docID string, doc map[string]interface{}) error {
	url := fmt.Sprintf("%s/%s/_doc/%s", baseURL, indexName, docID)
	jsonBody, _ := json.Marshal(doc)

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("index document failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func search(baseURL, indexName string, query map[string]interface{}) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/%s/_search", baseURL, indexName)
	jsonBody, _ := json.Marshal(query)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result, nil
}
