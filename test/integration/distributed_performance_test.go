package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkDistributedSearchLatency measures query latency on different dataset sizes
func BenchmarkDistributedSearchLatency(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	// Test configurations: dataset sizes
	testCases := []struct {
		name     string
		numDocs  int
		numNodes int
		numShards int
	}{
		{"10K_docs_1_node", 10000, 1, 1},
		{"10K_docs_2_nodes", 10000, 2, 4},
		{"10K_docs_3_nodes", 10000, 3, 6},
		{"50K_docs_3_nodes", 50000, 3, 6},
		{"100K_docs_3_nodes", 100000, 3, 6},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Setup cluster
			cfg := DefaultClusterConfig()
			cfg.NumData = tc.numNodes
			cluster, err := NewTestCluster(b, cfg)
			if err != nil {
				b.Fatalf("Failed to create cluster: %v", err)
			}
			defer cluster.Stop()

			ctx := context.Background()
			if err := cluster.Start(ctx); err != nil {
				b.Fatalf("Failed to start cluster: %v", err)
			}

			if err := cluster.WaitForClusterReady(20 * time.Second); err != nil {
				b.Fatalf("Cluster not ready: %v", err)
			}

			coordNode := cluster.GetCoordNode(0)
			baseURL := fmt.Sprintf("http://127.0.0.1:%d", coordNode.Config.RESTPort)

			// Create index
			indexName := fmt.Sprintf("perf_test_%s", tc.name)
			createIndexReq := map[string]interface{}{
				"settings": map[string]interface{}{
					"index": map[string]interface{}{
						"number_of_shards":   tc.numShards,
						"number_of_replicas": 0,
					},
				},
			}

			if err := createIndex(baseURL, indexName, createIndexReq); err != nil {
				b.Fatalf("Failed to create index: %v", err)
			}

			time.Sleep(2 * time.Second)

			// Index documents
			b.Logf("Indexing %d documents across %d nodes...", tc.numDocs, tc.numNodes)
			startIndex := time.Now()
			for i := 0; i < tc.numDocs; i++ {
				doc := map[string]interface{}{
					"title":    fmt.Sprintf("Document %d", i),
					"value":    rand.Float64() * 1000,
					"category": fmt.Sprintf("cat_%d", i%10),
					"status":   []string{"active", "pending", "completed"}[i%3],
				}
				if err := indexDocument(baseURL, indexName, fmt.Sprintf("%d", i), doc); err != nil {
					b.Fatalf("Failed to index document: %v", err)
				}

				// Log progress every 1000 docs
				if (i+1)%1000 == 0 {
					b.Logf("Indexed %d/%d documents", i+1, tc.numDocs)
				}
			}
			indexDuration := time.Since(startIndex)
			b.Logf("Indexing completed in %v (%.0f docs/sec)", indexDuration, float64(tc.numDocs)/indexDuration.Seconds())

			time.Sleep(2 * time.Second)

			// Prepare query
			query := map[string]interface{}{
				"query": map[string]interface{}{
					"match_all": map[string]interface{}{},
				},
				"size": 10,
			}

			// Reset timer before benchmark
			b.ResetTimer()

			// Run benchmark
			for i := 0; i < b.N; i++ {
				_, err := search(baseURL, indexName, query)
				if err != nil {
					b.Fatalf("Search failed: %v", err)
				}
			}

			b.StopTimer()

			// Report stats
			opsPerSec := float64(b.N) / b.Elapsed().Seconds()
			avgLatency := b.Elapsed() / time.Duration(b.N)
			b.ReportMetric(avgLatency.Seconds()*1000, "ms/op")
			b.ReportMetric(opsPerSec, "ops/sec")
		})
	}
}

// BenchmarkDistributedSearchThroughput measures throughput under concurrent load
func BenchmarkDistributedSearchThroughput(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	// Test with different concurrency levels
	concurrencyLevels := []int{1, 5, 10, 25, 50}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			// Setup cluster with 3 nodes
			cfg := DefaultClusterConfig()
			cfg.NumData = 3
			cluster, err := NewTestCluster(b, cfg)
			if err != nil {
				b.Fatalf("Failed to create cluster: %v", err)
			}
			defer cluster.Stop()

			ctx := context.Background()
			if err := cluster.Start(ctx); err != nil {
				b.Fatalf("Failed to start cluster: %v", err)
			}

			if err := cluster.WaitForClusterReady(20 * time.Second); err != nil {
				b.Fatalf("Cluster not ready: %v", err)
			}

			coordNode := cluster.GetCoordNode(0)
			baseURL := fmt.Sprintf("http://127.0.0.1:%d", coordNode.Config.RESTPort)

			// Create and populate index
			indexName := fmt.Sprintf("throughput_test_%d", concurrency)
			createIndexReq := map[string]interface{}{
				"settings": map[string]interface{}{
					"index": map[string]interface{}{
						"number_of_shards":   6,
						"number_of_replicas": 0,
					},
				},
			}

			if err := createIndex(baseURL, indexName, createIndexReq); err != nil {
				b.Fatalf("Failed to create index: %v", err)
			}

			time.Sleep(2 * time.Second)

			// Index 10K documents
			numDocs := 10000
			b.Logf("Indexing %d documents...", numDocs)
			for i := 0; i < numDocs; i++ {
				doc := map[string]interface{}{
					"value": rand.Float64() * 1000,
				}
				if err := indexDocument(baseURL, indexName, fmt.Sprintf("%d", i), doc); err != nil {
					b.Fatalf("Failed to index: %v", err)
				}
			}

			time.Sleep(2 * time.Second)

			query := map[string]interface{}{
				"query": map[string]interface{}{
					"match_all": map[string]interface{}{},
				},
				"size": 10,
			}

			// Reset timer
			b.ResetTimer()

			// Run concurrent queries
			var wg sync.WaitGroup
			queriesPerWorker := b.N / concurrency
			if queriesPerWorker == 0 {
				queriesPerWorker = 1
			}

			var successCount int64
			var errorCount int64

			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < queriesPerWorker; j++ {
						_, err := search(baseURL, indexName, query)
						if err != nil {
							atomic.AddInt64(&errorCount, 1)
						} else {
							atomic.AddInt64(&successCount, 1)
						}
					}
				}()
			}

			wg.Wait()
			b.StopTimer()

			// Report metrics
			totalOps := atomic.LoadInt64(&successCount) + atomic.LoadInt64(&errorCount)
			opsPerSec := float64(totalOps) / b.Elapsed().Seconds()
			errorRate := float64(errorCount) / float64(totalOps) * 100

			b.ReportMetric(opsPerSec, "qps")
			b.ReportMetric(errorRate, "%errors")
			b.Logf("Throughput: %.0f QPS, Errors: %.2f%%", opsPerSec, errorRate)
		})
	}
}

// BenchmarkAggregationOverhead measures aggregation processing overhead
func BenchmarkAggregationOverhead(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	// Setup cluster
	cfg := DefaultClusterConfig()
	cfg.NumData = 3
	cluster, err := NewTestCluster(b, cfg)
	if err != nil {
		b.Fatalf("Failed to create cluster: %v", err)
	}
	defer cluster.Stop()

	ctx := context.Background()
	if err := cluster.Start(ctx); err != nil {
		b.Fatalf("Failed to start cluster: %v", err)
	}

	if err := cluster.WaitForClusterReady(20 * time.Second); err != nil {
		b.Fatalf("Cluster not ready: %v", err)
	}

	coordNode := cluster.GetCoordNode(0)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", coordNode.Config.RESTPort)

	// Create index
	indexName := "agg_overhead_test"
	createIndexReq := map[string]interface{}{
		"settings": map[string]interface{}{
			"index": map[string]interface{}{
				"number_of_shards":   6,
				"number_of_replicas": 0,
			},
		},
	}

	if err := createIndex(baseURL, indexName, createIndexReq); err != nil {
		b.Fatalf("Failed to create index: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Index documents
	numDocs := 10000
	b.Logf("Indexing %d documents...", numDocs)
	for i := 0; i < numDocs; i++ {
		doc := map[string]interface{}{
			"category": fmt.Sprintf("cat_%d", i%50),
			"value":    rand.Float64() * 1000,
			"amount":   float64(rand.Intn(1000)),
		}
		if err := indexDocument(baseURL, indexName, fmt.Sprintf("%d", i), doc); err != nil {
			b.Fatalf("Failed to index: %v", err)
		}
	}

	time.Sleep(2 * time.Second)

	// Test cases: different aggregation complexities
	testCases := []struct {
		name  string
		query map[string]interface{}
	}{
		{
			name: "NoAggregations",
			query: map[string]interface{}{
				"query": map[string]interface{}{"match_all": map[string]interface{}{}},
				"size":  10,
			},
		},
		{
			name: "SingleTermsAggregation",
			query: map[string]interface{}{
				"query": map[string]interface{}{"match_all": map[string]interface{}{}},
				"size":  0,
				"aggs": map[string]interface{}{
					"categories": map[string]interface{}{
						"terms": map[string]interface{}{
							"field": "category",
							"size":  50,
						},
					},
				},
			},
		},
		{
			name: "SingleStatsAggregation",
			query: map[string]interface{}{
				"query": map[string]interface{}{"match_all": map[string]interface{}{}},
				"size":  0,
				"aggs": map[string]interface{}{
					"value_stats": map[string]interface{}{
						"stats": map[string]interface{}{
							"field": "value",
						},
					},
				},
			},
		},
		{
			name: "MultipleAggregations",
			query: map[string]interface{}{
				"query": map[string]interface{}{"match_all": map[string]interface{}{}},
				"size":  0,
				"aggs": map[string]interface{}{
					"categories": map[string]interface{}{
						"terms": map[string]interface{}{
							"field": "category",
							"size":  50,
						},
					},
					"value_stats": map[string]interface{}{
						"stats": map[string]interface{}{
							"field": "value",
						},
					},
					"amount_extended_stats": map[string]interface{}{
						"extended_stats": map[string]interface{}{
							"field": "amount",
						},
					},
					"unique_categories": map[string]interface{}{
						"cardinality": map[string]interface{}{
							"field": "category",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := search(baseURL, indexName, tc.query)
				if err != nil {
					b.Fatalf("Search failed: %v", err)
				}
			}

			b.StopTimer()

			// Report metrics
			avgLatency := b.Elapsed() / time.Duration(b.N)
			b.ReportMetric(avgLatency.Seconds()*1000, "ms/op")
		})
	}
}

// TestScalability measures linear scalability by comparing performance across different node counts
func TestScalability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping scalability test in short mode")
	}

	numDocs := 30000
	nodeConfigs := []struct {
		numNodes  int
		numShards int
	}{
		{1, 2},
		{2, 4},
		{3, 6},
	}

	results := make(map[int]time.Duration)

	for _, cfg := range nodeConfigs {
		t.Run(fmt.Sprintf("%d_nodes", cfg.numNodes), func(t *testing.T) {
			// Setup cluster
			clusterCfg := DefaultClusterConfig()
			clusterCfg.NumData = cfg.numNodes
			cluster, err := NewTestCluster(t, clusterCfg)
			if err != nil {
				t.Fatalf("Failed to create cluster: %v", err)
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

			// Create index
			indexName := fmt.Sprintf("scale_test_%d_nodes", cfg.numNodes)
			createIndexReq := map[string]interface{}{
				"settings": map[string]interface{}{
					"index": map[string]interface{}{
						"number_of_shards":   cfg.numShards,
						"number_of_replicas": 0,
					},
				},
			}

			if err := createIndex(baseURL, indexName, createIndexReq); err != nil {
				t.Fatalf("Failed to create index: %v", err)
			}

			time.Sleep(2 * time.Second)

			// Index documents
			t.Logf("Indexing %d documents across %d nodes...", numDocs, cfg.numNodes)
			for i := 0; i < numDocs; i++ {
				doc := map[string]interface{}{
					"value": rand.Float64() * 1000,
				}
				if err := indexDocument(baseURL, indexName, fmt.Sprintf("%d", i), doc); err != nil {
					t.Fatalf("Failed to index: %v", err)
				}
			}

			time.Sleep(2 * time.Second)

			// Run queries and measure average latency
			query := map[string]interface{}{
				"query": map[string]interface{}{
					"match_all": map[string]interface{}{},
				},
				"size": 10,
				"aggs": map[string]interface{}{
					"value_stats": map[string]interface{}{
						"stats": map[string]interface{}{
							"field": "value",
						},
					},
				},
			}

			numQueries := 100
			start := time.Now()

			for i := 0; i < numQueries; i++ {
				if _, err := search(baseURL, indexName, query); err != nil {
					t.Fatalf("Search failed: %v", err)
				}
			}

			avgLatency := time.Since(start) / time.Duration(numQueries)
			results[cfg.numNodes] = avgLatency

			t.Logf("%d nodes: avg latency = %v, throughput = %.0f QPS",
				cfg.numNodes, avgLatency, float64(numQueries)/time.Since(start).Seconds())
		})
	}

	// Analyze scalability
	t.Run("ScalabilityAnalysis", func(t *testing.T) {
		if len(results) < 2 {
			t.Skip("Need at least 2 node configs for scalability analysis")
		}

		baseline := results[1]
		t.Logf("\nScalability Analysis (baseline: 1 node = %v):", baseline)

		for nodes := 2; nodes <= 3; nodes++ {
			if latency, ok := results[nodes]; ok {
				speedup := baseline.Seconds() / latency.Seconds()
				efficiency := speedup / float64(nodes) * 100
				t.Logf("%d nodes: latency=%v, speedup=%.2fx, efficiency=%.1f%%",
					nodes, latency, speedup, efficiency)
			}
		}
	})
}

// Helper to create HTTP client with timeout
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}
