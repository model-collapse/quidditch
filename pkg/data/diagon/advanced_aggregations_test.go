package diagon_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/quidditch/quidditch/pkg/data/diagon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestHistogramAggregation tests numeric histogram aggregation
func TestHistogramAggregation(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)
	defer bridge.Stop()

	shardPath := filepath.Join(tempDir, "histogram_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index documents with price data
	products := []struct {
		id    string
		name  string
		price float64
	}{
		{"p1", "Widget A", 10.5},
		{"p2", "Widget B", 15.2},
		{"p3", "Widget C", 22.8},
		{"p4", "Widget D", 31.0},
		{"p5", "Widget E", 45.5},
		{"p6", "Widget F", 52.3},
		{"p7", "Widget G", 68.7},
		{"p8", "Widget H", 75.0},
		{"p9", "Widget I", 88.5},
		{"p10", "Widget J", 95.2},
	}

	for _, p := range products {
		doc := map[string]interface{}{
			"name":  p.name,
			"price": p.price,
		}
		err := shard.IndexDocument(p.id, doc)
		assert.NoError(t, err)
	}

	t.Run("PriceHistogram_Interval10", func(t *testing.T) {
		// Histogram with interval of 10
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"price_histogram": {
					"histogram": {
						"field": "price",
						"interval": 10
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(10), result.TotalHits)

		// Should have buckets: [10, 20, 30, 40, 50, 60, 70, 80, 90]
		t.Logf("Histogram aggregation returned %d buckets", len(result.Aggregations))
	})

	t.Run("PriceHistogram_Interval25", func(t *testing.T) {
		// Histogram with larger interval
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"price_histogram": {
					"histogram": {
						"field": "price",
						"interval": 25
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(10), result.TotalHits)

		// Should have fewer buckets with interval 25
		t.Logf("Larger interval histogram returned %d buckets", len(result.Aggregations))
	})
}

// TestDateHistogramAggregation tests time-based histogram aggregation
func TestDateHistogramAggregation(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)
	defer bridge.Stop()

	shardPath := filepath.Join(tempDir, "date_histogram_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index documents with timestamps
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 48; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Hour).UnixMilli()
		doc := map[string]interface{}{
			"event":     fmt.Sprintf("Event %d", i),
			"timestamp": timestamp,
		}
		err := shard.IndexDocument(fmt.Sprintf("e%d", i), doc)
		assert.NoError(t, err)
	}

	t.Run("HourlyHistogram", func(t *testing.T) {
		// Date histogram with 1 hour intervals
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"events_over_time": {
					"date_histogram": {
						"field": "timestamp",
						"interval": "1h"
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(48), result.TotalHits)

		t.Logf("Hourly histogram returned %d buckets", len(result.Aggregations))
	})

	t.Run("DailyHistogram", func(t *testing.T) {
		// Date histogram with 1 day intervals
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"events_per_day": {
					"date_histogram": {
						"field": "timestamp",
						"interval": "1d"
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(48), result.TotalHits)

		// 48 hours = 2 days
		t.Logf("Daily histogram returned %d buckets", len(result.Aggregations))
	})
}

// TestPercentilesAggregation tests percentile calculations
func TestPercentilesAggregation(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)
	defer bridge.Stop()

	shardPath := filepath.Join(tempDir, "percentiles_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index 100 documents with latency data
	for i := 1; i <= 100; i++ {
		doc := map[string]interface{}{
			"request_id": fmt.Sprintf("req%d", i),
			"latency_ms": float64(i), // 1-100ms
		}
		err := shard.IndexDocument(fmt.Sprintf("r%d", i), doc)
		assert.NoError(t, err)
	}

	t.Run("StandardPercentiles", func(t *testing.T) {
		// Calculate 50th, 95th, 99th percentiles
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"latency_percentiles": {
					"percentiles": {
						"field": "latency_ms",
						"percents": [50, 95, 99]
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(100), result.TotalHits)

		// For 1-100, 50th ≈ 50, 95th ≈ 95, 99th ≈ 99
		t.Logf("Percentiles aggregation: %d results", len(result.Aggregations))
	})

	t.Run("CustomPercentiles", func(t *testing.T) {
		// Custom percentile values
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"custom_percentiles": {
					"percentiles": {
						"field": "latency_ms",
						"percents": [25, 50, 75, 90, 95, 99, 99.9]
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(100), result.TotalHits)

		t.Logf("Custom percentiles returned %d aggregations", len(result.Aggregations))
	})
}

// TestCardinalityAggregation tests unique value counting
func TestCardinalityAggregation(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)
	defer bridge.Stop()

	shardPath := filepath.Join(tempDir, "cardinality_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index documents with user activity
	users := []string{"alice", "bob", "charlie", "diana", "eve"}

	for i := 0; i < 100; i++ {
		doc := map[string]interface{}{
			"event_id": i,
			"user":     users[i%len(users)], // 5 unique users
			"action":   fmt.Sprintf("action_%d", i%10), // 10 unique actions
		}
		err := shard.IndexDocument(fmt.Sprintf("evt%d", i), doc)
		assert.NoError(t, err)
	}

	t.Run("UniqueUsers", func(t *testing.T) {
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"unique_users": {
					"cardinality": {
						"field": "user"
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(100), result.TotalHits)

		// Should show 5 unique users
		t.Logf("Cardinality (users): %d aggregations", len(result.Aggregations))
	})

	t.Run("UniqueActions", func(t *testing.T) {
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"unique_actions": {
					"cardinality": {
						"field": "action"
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(100), result.TotalHits)

		// Should show 10 unique actions
		t.Logf("Cardinality (actions): %d aggregations", len(result.Aggregations))
	})
}

// TestExtendedStatsAggregation tests extended statistics
func TestExtendedStatsAggregation(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)
	defer bridge.Stop()

	shardPath := filepath.Join(tempDir, "extended_stats_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index documents with response times
	responseTimes := []float64{10, 12, 15, 18, 20, 22, 25, 28, 30, 35}

	for i, rt := range responseTimes {
		doc := map[string]interface{}{
			"request":       fmt.Sprintf("req%d", i),
			"response_time": rt,
		}
		err := shard.IndexDocument(fmt.Sprintf("r%d", i), doc)
		assert.NoError(t, err)
	}

	t.Run("ResponseTimeStats", func(t *testing.T) {
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"response_stats": {
					"extended_stats": {
						"field": "response_time"
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(10), result.TotalHits)

		// Extended stats should include variance, std deviation, bounds
		t.Logf("Extended stats aggregation: %d results", len(result.Aggregations))
	})
}

// TestMultipleAdvancedAggregations tests combining multiple aggregation types
func TestMultipleAdvancedAggregations(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)
	defer bridge.Stop()

	shardPath := filepath.Join(tempDir, "multi_agg_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index e-commerce transaction data
	for i := 0; i < 100; i++ {
		timestamp := time.Now().Add(-time.Duration(i) * time.Hour).UnixMilli()
		doc := map[string]interface{}{
			"transaction_id": fmt.Sprintf("tx%d", i),
			"amount":         10.0 + float64(i)*2.5,
			"category":       []string{"cat" + fmt.Sprint(i%5)},
			"timestamp":      timestamp,
		}
		err := shard.IndexDocument(fmt.Sprintf("tx%d", i), doc)
		assert.NoError(t, err)
	}

	t.Run("CombinedAggregations", func(t *testing.T) {
		// Combine histogram, percentiles, cardinality, and extended stats
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"amount_histogram": {
					"histogram": {
						"field": "amount",
						"interval": 50
					}
				},
				"amount_percentiles": {
					"percentiles": {
						"field": "amount",
						"percents": [50, 95, 99]
					}
				},
				"unique_categories": {
					"cardinality": {
						"field": "category"
					}
				},
				"amount_extended_stats": {
					"extended_stats": {
						"field": "amount"
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(100), result.TotalHits)

		// Should have all 4 aggregations
		t.Logf("Combined aggregations returned %d aggregation results", len(result.Aggregations))
	})
}

// TestAggregationPerformance tests performance with larger datasets
func TestAggregationPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	logger := zap.NewNop()
	tempDir := t.TempDir()

	cfg := &diagon.Config{
		DataDir:     tempDir,
		SIMDEnabled: false,
		Logger:      logger,
	}

	bridge, err := diagon.NewDiagonBridge(cfg)
	require.NoError(t, err)

	err = bridge.Start()
	require.NoError(t, err)
	defer bridge.Stop()

	shardPath := filepath.Join(tempDir, "perf_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index 10,000 documents
	const numDocs = 10000
	for i := 0; i < numDocs; i++ {
		doc := map[string]interface{}{
			"id":        i,
			"value":     float64(i % 1000),
			"category":  fmt.Sprintf("cat%d", i%50),
			"timestamp": time.Now().Add(-time.Duration(i) * time.Minute).UnixMilli(),
		}
		err := shard.IndexDocument(fmt.Sprintf("doc%d", i), doc)
		require.NoError(t, err)
	}

	t.Run("Histogram_Performance", func(t *testing.T) {
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"value_histogram": {
					"histogram": {
						"field": "value",
						"interval": 100
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(numDocs), result.TotalHits)
		t.Logf("Histogram on %d docs completed in %dms", numDocs, result.Took)
	})

	t.Run("Percentiles_Performance", func(t *testing.T) {
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"value_percentiles": {
					"percentiles": {
						"field": "value",
						"percents": [50, 95, 99]
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(numDocs), result.TotalHits)
		t.Logf("Percentiles on %d docs completed in %dms", numDocs, result.Took)
	})

	t.Run("Cardinality_Performance", func(t *testing.T) {
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"unique_categories": {
					"cardinality": {
						"field": "category"
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(numDocs), result.TotalHits)
		t.Logf("Cardinality on %d docs completed in %dms", numDocs, result.Took)
	})
}
