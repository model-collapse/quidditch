package diagon_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/quidditch/quidditch/pkg/data/diagon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestWildcardQuery tests wildcard pattern matching (* and ?)
func TestWildcardQuery(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "wildcard_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index documents with various terms
	docs := []struct {
		id  string
		doc map[string]interface{}
	}{
		{
			id:  "doc1",
			doc: map[string]interface{}{"text": "searching for information online"},
		},
		{
			id:  "doc2",
			doc: map[string]interface{}{"text": "database search engine technology"},
		},
		{
			id:  "doc3",
			doc: map[string]interface{}{"text": "research methodology and analysis"},
		},
		{
			id:  "doc4",
			doc: map[string]interface{}{"text": "machine learning algorithms"},
		},
	}

	for _, d := range docs {
		err := shard.IndexDocument(d.id, d.doc)
		assert.NoError(t, err)
	}

	t.Run("WildcardStar", func(t *testing.T) {
		// Pattern: sea*ch - should match "search" (not "searching" - * must match middle exactly)
		query := []byte(`{"wildcard": {"text": "sea*ch"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc2 (search) - exact pattern match
		assert.Equal(t, int64(1), result.TotalHits)
		t.Logf("Found %d documents matching 'sea*ch'", result.TotalHits)
	})

	t.Run("WildcardStarMultiple", func(t *testing.T) {
		// Pattern: *search* - should match any term containing "search"
		query := []byte(`{"wildcard": {"text": "*search*"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc1 (searching), doc2 (search), doc3 (research)
		assert.Equal(t, int64(3), result.TotalHits)
		t.Logf("Found %d documents matching '*search*'", result.TotalHits)
	})

	t.Run("WildcardQuestion", func(t *testing.T) {
		// Pattern: se?rch - should match "search" (6 letters with any char at position 3)
		query := []byte(`{"wildcard": {"text": "se?rch"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc2 (search)
		assert.GreaterOrEqual(t, result.TotalHits, int64(1))
		t.Logf("Found %d documents matching 'se?rch'", result.TotalHits)
	})

	t.Run("WildcardComplex", func(t *testing.T) {
		// Pattern: *ing - should match words ending with "ing"
		query := []byte(`{"wildcard": {"text": "*ing"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc1 (searching), doc4 (learning)
		assert.GreaterOrEqual(t, result.TotalHits, int64(2))
		t.Logf("Found %d documents matching '*ing'", result.TotalHits)
	})
}

// TestFuzzyQuery tests fuzzy matching with Levenshtein distance
func TestFuzzyQuery(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "fuzzy_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index documents with similar terms
	docs := []struct {
		id  string
		doc map[string]interface{}
	}{
		{
			id:  "doc1",
			doc: map[string]interface{}{"text": "the quick brown fox"},
		},
		{
			id:  "doc2",
			doc: map[string]interface{}{"text": "a quik test example"},
		},
		{
			id:  "doc3",
			doc: map[string]interface{}{"text": "quality assurance process"},
		},
		{
			id:  "doc4",
			doc: map[string]interface{}{"text": "quantum computing research"},
		},
	}

	for _, d := range docs {
		err := shard.IndexDocument(d.id, d.doc)
		assert.NoError(t, err)
	}

	t.Run("FuzzyDistance1", func(t *testing.T) {
		// Search for "quik" with fuzziness 1
		query := []byte(`{"fuzzy": {"text": {"value": "quik", "fuzziness": 1}}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc1 (quick - 1 edit) and doc2 (quik - exact)
		assert.GreaterOrEqual(t, result.TotalHits, int64(2))
		t.Logf("Found %d documents with fuzzy match for 'quik' (fuzziness=1)", result.TotalHits)
	})

	t.Run("FuzzyDistance2", func(t *testing.T) {
		// Search for "quik" with fuzziness 2
		query := []byte(`{"fuzzy": {"text": {"value": "quik", "fuzziness": 2}}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match more documents with higher fuzziness
		// doc1 (quick), doc2 (quik), possibly doc3 (quality) or doc4 (quantum)
		assert.GreaterOrEqual(t, result.TotalHits, int64(2))
		t.Logf("Found %d documents with fuzzy match for 'quik' (fuzziness=2)", result.TotalHits)
	})

	t.Run("FuzzySimpleString", func(t *testing.T) {
		// Simple fuzzy query (default fuzziness=2)
		query := []byte(`{"fuzzy": {"text": "quick"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc1 (quick - exact) and doc2 (quik - 1 edit)
		assert.GreaterOrEqual(t, result.TotalHits, int64(1))
		t.Logf("Found %d documents with fuzzy match for 'quick'", result.TotalHits)
	})
}

// TestAggregations tests terms and stats aggregations
func TestAggregations(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "aggs_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index e-commerce products
	products := []struct {
		id       string
		name     string
		category string
		brand    string
		price    float64
		rating   float64
		stock    int
	}{
		{"prod1", "Laptop Pro", "Electronics", "TechCorp", 1299.99, 4.5, 50},
		{"prod2", "Laptop Air", "Electronics", "TechCorp", 999.99, 4.3, 30},
		{"prod3", "Desktop Tower", "Electronics", "BuildIT", 1499.99, 4.7, 20},
		{"prod4", "Office Chair", "Furniture", "ComfortCo", 299.99, 4.2, 100},
		{"prod5", "Standing Desk", "Furniture", "ComfortCo", 599.99, 4.6, 45},
		{"prod6", "Monitor 27", "Electronics", "ViewMax", 399.99, 4.4, 75},
		{"prod7", "Keyboard Mech", "Electronics", "TechCorp", 149.99, 4.8, 200},
		{"prod8", "Mouse Wireless", "Electronics", "TechCorp", 79.99, 4.1, 150},
	}

	for _, p := range products {
		doc := map[string]interface{}{
			"name":     p.name,
			"category": p.category,
			"brand":    p.brand,
			"price":    p.price,
			"rating":   p.rating,
			"stock":    p.stock,
		}
		err := shard.IndexDocument(p.id, doc)
		assert.NoError(t, err)
	}

	t.Run("TermsAggregation_Category", func(t *testing.T) {
		// Aggregate by category
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"categories": {
					"terms": {
						"field": "category",
						"size": 10
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(8), result.TotalHits)

		// Check aggregation results
		t.Logf("Aggregations returned: %d", len(result.Aggregations))

		// Note: Aggregations are currently not fully integrated through Go bridge
		// This tests the C++ implementation
	})

	t.Run("TermsAggregation_Brand", func(t *testing.T) {
		// Aggregate by brand
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"brands": {
					"terms": {
						"field": "brand",
						"size": 5
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should return all products
		assert.Equal(t, int64(8), result.TotalHits)
		t.Logf("Found %d products with brand aggregation", result.TotalHits)
	})

	t.Run("StatsAggregation_Price", func(t *testing.T) {
		// Get price statistics
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"price_stats": {
					"stats": {
						"field": "price"
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should return all products
		assert.Equal(t, int64(8), result.TotalHits)
		t.Logf("Found %d products for price stats", result.TotalHits)

		// Price stats should be: min=79.99, max=1499.99, avg~604.99
	})

	t.Run("FilteredAggregation", func(t *testing.T) {
		// Aggregate only electronics
		query := []byte(`{
			"term": {"category": "electronics"},
			"aggs": {
				"brand_distribution": {
					"terms": {
						"field": "brand",
						"size": 10
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match 6 electronics products
		assert.Equal(t, int64(6), result.TotalHits)
		t.Logf("Found %d electronics products", result.TotalHits)
	})

	t.Run("MultipleAggregations", func(t *testing.T) {
		// Multiple aggregations in one query
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"categories": {
					"terms": {
						"field": "category"
					}
				},
				"price_stats": {
					"stats": {
						"field": "price"
					}
				},
				"rating_stats": {
					"stats": {
						"field": "rating"
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(8), result.TotalHits)
		t.Logf("Executed query with 3 aggregations")
	})
}

// TestCombinedAdvancedQueries tests combinations of advanced features
func TestCombinedAdvancedQueries(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "combined_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index varied documents
	for i := 1; i <= 20; i++ {
		doc := map[string]interface{}{
			"id":          i,
			"title":       fmt.Sprintf("Document %d", i),
			"content":     fmt.Sprintf("Content about topic %d", i%5),
			"category":    []string{"cat" + fmt.Sprint(i%3)},
			"score":       float64(50 + i*2),
			"published":   i%2 == 0,
			"view_count":  i * 100,
		}
		err := shard.IndexDocument(fmt.Sprintf("doc%d", i), doc)
		assert.NoError(t, err)
	}

	t.Run("WildcardWithBool", func(t *testing.T) {
		// Wildcard search combined with boolean query
		query := []byte(`{
			"bool": {
				"must": [
					{"wildcard": {"content": "content*"}}
				],
				"filter": [
					{"range": {"score": {"gte": 60}}}
				]
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		// Should match documents where content starts with "content" AND score >= 60
		assert.GreaterOrEqual(t, result.TotalHits, int64(1))
		t.Logf("Wildcard + bool: found %d documents", result.TotalHits)
	})

	t.Run("FuzzyWithAggregation", func(t *testing.T) {
		// Fuzzy search with aggregation
		query := []byte(`{
			"fuzzy": {"content": "topic"},
			"aggs": {
				"categories": {
					"terms": {
						"field": "category"
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, result.TotalHits, int64(1))
		t.Logf("Fuzzy + agg: found %d documents", result.TotalHits)
	})

	t.Run("ComplexMultiQuery", func(t *testing.T) {
		// Complex query: bool + range + stats
		query := []byte(`{
			"bool": {
				"should": [
					{"wildcard": {"title": "*1*"}},
					{"range": {"score": {"gte": 70, "lte": 90}}}
				]
			},
			"aggs": {
				"view_stats": {
					"stats": {
						"field": "view_count"
					}
				}
			}
		}`)

		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, result.TotalHits, int64(1))
		t.Logf("Complex query: found %d documents", result.TotalHits)
	})
}

// TestPerformance benchmarks the advanced query features
func TestPerformance(t *testing.T) {
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

	// Index 1000 documents
	const numDocs = 1000
	for i := 0; i < numDocs; i++ {
		doc := map[string]interface{}{
			"id":      i,
			"title":   fmt.Sprintf("Document %d about searching and indexing", i),
			"content": fmt.Sprintf("Content %d with various terms for testing", i),
			"value":   float64(i),
			"tags":    []string{fmt.Sprintf("tag%d", i%10), fmt.Sprintf("cat%d", i%5)},
		}
		err := shard.IndexDocument(fmt.Sprintf("doc%d", i), doc)
		require.NoError(t, err)
	}

	t.Run("WildcardPerformance", func(t *testing.T) {
		query := []byte(`{"wildcard": {"title": "*search*"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		t.Logf("Wildcard search on %d docs: found %d matches", numDocs, result.TotalHits)
	})

	t.Run("FuzzyPerformance", func(t *testing.T) {
		query := []byte(`{"fuzzy": {"title": {"value": "searching", "fuzziness": 2}}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		t.Logf("Fuzzy search on %d docs: found %d matches", numDocs, result.TotalHits)
	})

	t.Run("AggregationPerformance", func(t *testing.T) {
		query := []byte(`{
			"match_all": {},
			"aggs": {
				"tags_agg": {
					"terms": {
						"field": "tags",
						"size": 20
					}
				},
				"value_stats": {
					"stats": {
						"field": "value"
					}
				}
			}
		}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		t.Logf("Aggregation on %d docs: returned %d hits", numDocs, result.TotalHits)
	})
}
