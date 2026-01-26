package integration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestAllAggregationTypesDistributed tests all 14 aggregation types across multiple nodes
// This test validates that Phase 1's aggregation merge logic works correctly
func TestAllAggregationTypesDistributed(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start cluster with 3 data nodes
	cfg := DefaultClusterConfig()
	cfg.NumData = 3
	cluster, err := NewTestCluster(t, cfg)
	if err != nil {
		t.Fatalf("Failed to create test cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()
	if err := cluster.Start(ctx); err != nil {
		t.Fatalf("Failed to start cluster: %v", err)
	}

	if err := cluster.WaitForClusterReady(15 * time.Second); err != nil {
		t.Fatalf("Cluster not ready: %v", err)
	}

	coordNode := cluster.GetCoordNode(0)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", coordNode.Config.RESTPort)

	// Create index with 6 shards (2 per node)
	indexName := "agg_complete_test"
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

	time.Sleep(2 * time.Second)

	// Index test documents with various fields for different aggregation types
	testDocs := []map[string]interface{}{
		{"id": "1", "category": "A", "price": 100.0, "quantity": 5, "status": "active", "timestamp": "2024-01-01T00:00:00Z"},
		{"id": "2", "category": "B", "price": 200.0, "quantity": 10, "status": "active", "timestamp": "2024-01-01T12:00:00Z"},
		{"id": "3", "category": "A", "price": 150.0, "quantity": 7, "status": "inactive", "timestamp": "2024-01-02T00:00:00Z"},
		{"id": "4", "category": "C", "price": 300.0, "quantity": 3, "status": "active", "timestamp": "2024-01-02T12:00:00Z"},
		{"id": "5", "category": "A", "price": 250.0, "quantity": 8, "status": "active", "timestamp": "2024-01-03T00:00:00Z"},
		{"id": "6", "category": "B", "price": 180.0, "quantity": 12, "status": "inactive", "timestamp": "2024-01-03T12:00:00Z"},
		{"id": "7", "category": "A", "price": 220.0, "quantity": 6, "status": "active", "timestamp": "2024-01-04T00:00:00Z"},
		{"id": "8", "category": "C", "price": 350.0, "quantity": 4, "status": "active", "timestamp": "2024-01-04T12:00:00Z"},
		{"id": "9", "category": "B", "price": 190.0, "quantity": 9, "status": "inactive", "timestamp": "2024-01-05T00:00:00Z"},
		{"id": "10", "category": "A", "price": 280.0, "quantity": 11, "status": "active", "timestamp": "2024-01-05T12:00:00Z"},
	}

	for _, doc := range testDocs {
		docID := doc["id"].(string)
		delete(doc, "id")
		if err := indexDocument(baseURL, indexName, docID, doc); err != nil {
			t.Fatalf("Failed to index document %s: %v", docID, err)
		}
	}

	t.Logf("Indexed %d test documents across 3 nodes", len(testDocs))
	time.Sleep(1 * time.Second)

	// Test 1: Terms Aggregation
	t.Run("TermsAggregation", func(t *testing.T) {
		query := map[string]interface{}{
			"size": 0,
			"aggs": map[string]interface{}{
				"by_category": map[string]interface{}{
					"terms": map[string]interface{}{
						"field": "category",
						"size":  10,
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})
		termsAgg := aggs["by_category"].(map[string]interface{})
		buckets := termsAgg["buckets"].([]interface{})

		// Should have 3 categories: A, B, C
		if len(buckets) != 3 {
			t.Errorf("Expected 3 category buckets, got %d", len(buckets))
		}

		t.Logf("Terms aggregation: %d buckets merged across nodes", len(buckets))
	})

	// Test 2: Stats Aggregation
	t.Run("StatsAggregation", func(t *testing.T) {
		query := map[string]interface{}{
			"size": 0,
			"aggs": map[string]interface{}{
				"price_stats": map[string]interface{}{
					"stats": map[string]interface{}{
						"field": "price",
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})
		stats := aggs["price_stats"].(map[string]interface{})

		count := int(stats["count"].(float64))
		min := stats["min"].(float64)
		max := stats["max"].(float64)
		avg := stats["avg"].(float64)
		sum := stats["sum"].(float64)

		if count != len(testDocs) {
			t.Errorf("Expected count %d, got %d", len(testDocs), count)
		}

		// Calculate expected values
		expectedMin := 100.0
		expectedMax := 350.0
		expectedSum := 2220.0
		expectedAvg := expectedSum / float64(len(testDocs))

		if min != expectedMin || max != expectedMax || sum != expectedSum {
			t.Errorf("Stats mismatch: min=%.2f (expected %.2f), max=%.2f (expected %.2f), sum=%.2f (expected %.2f)",
				min, expectedMin, max, expectedMax, sum, expectedSum)
		}

		if avg != expectedAvg {
			t.Errorf("Average mismatch: %.2f (expected %.2f)", avg, expectedAvg)
		}

		t.Logf("Stats aggregation: count=%d, min=%.2f, max=%.2f, avg=%.2f, sum=%.2f", count, min, max, avg, sum)
	})

	// Test 3: Extended Stats Aggregation
	t.Run("ExtendedStatsAggregation", func(t *testing.T) {
		query := map[string]interface{}{
			"size": 0,
			"aggs": map[string]interface{}{
				"price_ext_stats": map[string]interface{}{
					"extended_stats": map[string]interface{}{
						"field": "price",
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})
		extStats := aggs["price_ext_stats"].(map[string]interface{})

		// Verify all extended stats fields are present
		requiredFields := []string{"count", "min", "max", "avg", "sum",
			"sum_of_squares", "variance", "std_deviation",
			"std_deviation_bounds_upper", "std_deviation_bounds_lower"}

		for _, field := range requiredFields {
			if _, ok := extStats[field]; !ok {
				t.Errorf("Missing extended stats field: %s", field)
			}
		}

		stdDev := extStats["std_deviation"].(float64)
		variance := extStats["variance"].(float64)

		if stdDev <= 0 || variance <= 0 {
			t.Errorf("Invalid std deviation or variance: stdDev=%.2f, variance=%.2f", stdDev, variance)
		}

		t.Logf("Extended stats aggregation: std_dev=%.2f, variance=%.2f", stdDev, variance)
	})

	// Test 4: Histogram Aggregation
	t.Run("HistogramAggregation", func(t *testing.T) {
		query := map[string]interface{}{
			"size": 0,
			"aggs": map[string]interface{}{
				"price_histogram": map[string]interface{}{
					"histogram": map[string]interface{}{
						"field":    "price",
						"interval": 100.0,
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})
		histogram := aggs["price_histogram"].(map[string]interface{})
		buckets := histogram["buckets"].([]interface{})

		if len(buckets) == 0 {
			t.Error("Expected histogram buckets, got none")
		}

		t.Logf("Histogram aggregation: %d buckets", len(buckets))
	})

	// Test 5: Date Histogram Aggregation
	t.Run("DateHistogramAggregation", func(t *testing.T) {
		query := map[string]interface{}{
			"size": 0,
			"aggs": map[string]interface{}{
				"sales_over_time": map[string]interface{}{
					"date_histogram": map[string]interface{}{
						"field":    "timestamp",
						"interval": "1d",
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})
		dateHist := aggs["sales_over_time"].(map[string]interface{})
		buckets := dateHist["buckets"].([]interface{})

		if len(buckets) == 0 {
			t.Error("Expected date histogram buckets, got none")
		}

		t.Logf("Date histogram aggregation: %d time buckets", len(buckets))
	})

	// Test 6: Range Aggregation (NEW in Phase 1)
	t.Run("RangeAggregation", func(t *testing.T) {
		query := map[string]interface{}{
			"size": 0,
			"aggs": map[string]interface{}{
				"price_ranges": map[string]interface{}{
					"range": map[string]interface{}{
						"field": "price",
						"ranges": []map[string]interface{}{
							{"key": "low", "to": 150.0},
							{"key": "medium", "from": 150.0, "to": 250.0},
							{"key": "high", "from": 250.0},
						},
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})
		rangeAgg := aggs["price_ranges"].(map[string]interface{})
		buckets := rangeAgg["buckets"].([]interface{})

		// Should have 3 buckets: low, medium, high
		if len(buckets) != 3 {
			t.Errorf("Expected 3 range buckets, got %d", len(buckets))
		}

		// Verify buckets are in order
		expectedKeys := []string{"low", "medium", "high"}
		for i, bucket := range buckets {
			b := bucket.(map[string]interface{})
			key := b["key"].(string)
			if key != expectedKeys[i] {
				t.Errorf("Bucket %d: expected key '%s', got '%s'", i, expectedKeys[i], key)
			}
		}

		t.Logf("Range aggregation: %d range buckets merged", len(buckets))
	})

	// Test 7: Filters Aggregation (NEW in Phase 1)
	t.Run("FiltersAggregation", func(t *testing.T) {
		query := map[string]interface{}{
			"size": 0,
			"aggs": map[string]interface{}{
				"status_filters": map[string]interface{}{
					"filters": map[string]interface{}{
						"filters": map[string]interface{}{
							"active_items": map[string]interface{}{
								"term": map[string]interface{}{
									"status": "active",
								},
							},
							"inactive_items": map[string]interface{}{
								"term": map[string]interface{}{
									"status": "inactive",
								},
							},
						},
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})
		filtersAgg := aggs["status_filters"].(map[string]interface{})
		buckets := filtersAgg["buckets"].(map[string]interface{})

		// Should have 2 named buckets
		if len(buckets) != 2 {
			t.Errorf("Expected 2 filter buckets, got %d", len(buckets))
		}

		// Verify named buckets
		if _, ok := buckets["active_items"]; !ok {
			t.Error("Missing 'active_items' filter bucket")
		}
		if _, ok := buckets["inactive_items"]; !ok {
			t.Error("Missing 'inactive_items' filter bucket")
		}

		t.Logf("Filters aggregation: %d named filters merged", len(buckets))
	})

	// Test 8-12: Simple Metric Aggregations (NEW in Phase 1)
	t.Run("SimpleMetricAggregations", func(t *testing.T) {
		query := map[string]interface{}{
			"size": 0,
			"aggs": map[string]interface{}{
				"avg_price": map[string]interface{}{
					"avg": map[string]interface{}{"field": "price"},
				},
				"min_price": map[string]interface{}{
					"min": map[string]interface{}{"field": "price"},
				},
				"max_price": map[string]interface{}{
					"max": map[string]interface{}{"field": "price"},
				},
				"sum_price": map[string]interface{}{
					"sum": map[string]interface{}{"field": "price"},
				},
				"count_price": map[string]interface{}{
					"value_count": map[string]interface{}{"field": "price"},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})

		// Test 8: Avg
		avgAgg := aggs["avg_price"].(map[string]interface{})
		avgValue := avgAgg["value"].(float64)
		expectedAvg := 222.0 // Sum: 2220, Count: 10
		if avgValue != expectedAvg {
			t.Errorf("Avg: expected %.2f, got %.2f", expectedAvg, avgValue)
		}
		t.Logf("Avg aggregation: %.2f", avgValue)

		// Test 9: Min
		minAgg := aggs["min_price"].(map[string]interface{})
		minValue := minAgg["value"].(float64)
		if minValue != 100.0 {
			t.Errorf("Min: expected 100.0, got %.2f", minValue)
		}
		t.Logf("Min aggregation: %.2f", minValue)

		// Test 10: Max
		maxAgg := aggs["max_price"].(map[string]interface{})
		maxValue := maxAgg["value"].(float64)
		if maxValue != 350.0 {
			t.Errorf("Max: expected 350.0, got %.2f", maxValue)
		}
		t.Logf("Max aggregation: %.2f", maxValue)

		// Test 11: Sum
		sumAgg := aggs["sum_price"].(map[string]interface{})
		sumValue := sumAgg["value"].(float64)
		if sumValue != 2220.0 {
			t.Errorf("Sum: expected 2220.0, got %.2f", sumValue)
		}
		t.Logf("Sum aggregation: %.2f", sumValue)

		// Test 12: Value Count
		countAgg := aggs["count_price"].(map[string]interface{})
		countValue := int(countAgg["value"].(float64))
		if countValue != len(testDocs) {
			t.Errorf("Value count: expected %d, got %d", len(testDocs), countValue)
		}
		t.Logf("Value count aggregation: %d", countValue)
	})

	// Test 13: Percentiles Aggregation
	t.Run("PercentilesAggregation", func(t *testing.T) {
		query := map[string]interface{}{
			"size": 0,
			"aggs": map[string]interface{}{
				"price_percentiles": map[string]interface{}{
					"percentiles": map[string]interface{}{
						"field":      "price",
						"percents":   []float64{25, 50, 75, 95, 99},
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})
		percentiles := aggs["price_percentiles"].(map[string]interface{})
		values := percentiles["values"].(map[string]interface{})

		// Verify all requested percentiles are present
		expectedPercentiles := []string{"25.0", "50.0", "75.0", "95.0", "99.0"}
		for _, p := range expectedPercentiles {
			if _, ok := values[p]; !ok {
				t.Errorf("Missing percentile: %s", p)
			}
		}

		t.Logf("Percentiles aggregation: %d percentiles calculated", len(values))
	})

	// Test 14: Cardinality Aggregation
	t.Run("CardinalityAggregation", func(t *testing.T) {
		query := map[string]interface{}{
			"size": 0,
			"aggs": map[string]interface{}{
				"unique_categories": map[string]interface{}{
					"cardinality": map[string]interface{}{
						"field": "category",
					},
				},
			},
		}

		result, err := search(baseURL, indexName, query)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		aggs := result["aggregations"].(map[string]interface{})
		cardinality := aggs["unique_categories"].(map[string]interface{})
		uniqueCount := int(cardinality["value"].(float64))

		// Should have 3 unique categories: A, B, C
		expectedUnique := 3
		if uniqueCount != expectedUnique {
			t.Errorf("Expected %d unique categories, got %d", expectedUnique, uniqueCount)
		}

		t.Logf("Cardinality aggregation: %d unique values", uniqueCount)
	})

	t.Log("✅ All 14 aggregation types tested successfully across distributed nodes!")
}

// TestAggregationMergeAccuracy tests the accuracy of aggregation merging
func TestAggregationMergeAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test verifies that aggregations merged from multiple shards
	// produce the same results as if computed on a single shard

	t.Log("Comparing single-node vs multi-node aggregation results")

	// TODO: Implement comparison test:
	// 1. Create index with 1 shard on 1 node
	// 2. Index documents
	// 3. Run aggregation, record results
	// 4. Create index with 6 shards on 3 nodes
	// 5. Index same documents
	// 6. Run same aggregation
	// 7. Compare results - should be identical (for exact aggregations)

	t.Log("Test placeholder - implement actual comparison")
}

// TestAggregationPerformanceOverhead measures the overhead of aggregation merging
func TestAggregationPerformanceOverhead(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	t.Log("Measuring aggregation merge overhead")

	// TODO: Implement performance test:
	// 1. Measure query time with aggregations on single node
	// 2. Measure query time with aggregations on multiple nodes
	// 3. Calculate overhead percentage
	// 4. Verify overhead is <10% as per plan target

	t.Log("Test placeholder - implement actual benchmark")
}
