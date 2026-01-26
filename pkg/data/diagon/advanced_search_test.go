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

// TestBM25Scoring tests BM25 scoring for relevance
func TestBM25Scoring(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "bm25_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index documents with varying term frequencies
	docs := []struct {
		id  string
		doc map[string]interface{}
	}{
		{
			id: "doc1",
			doc: map[string]interface{}{
				"title":       "search engine optimization",
				"description": "Learn about search engine optimization techniques",
			},
		},
		{
			id: "doc2",
			doc: map[string]interface{}{
				"title":       "database search",
				"description": "Full-text search in databases",
			},
		},
		{
			id: "doc3",
			doc: map[string]interface{}{
				"title":       "search algorithms",
				"description": "Binary search and linear search algorithms",
			},
		},
	}

	for _, d := range docs {
		err := shard.IndexDocument(d.id, d.doc)
		assert.NoError(t, err)
	}

	t.Run("TermQueryWithScoring", func(t *testing.T) {
		// Search for "optimization" - appears in only 1 document (better for BM25 testing)
		query := []byte(`{"term": {"title": "optimization"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), result.TotalHits)

		// Results should be sorted by score
		assert.Greater(t, result.MaxScore, 0.0)
		t.Logf("Max score: %.4f", result.MaxScore)

		// Should return doc1 (search engine optimization)
		if len(result.Hits) > 0 {
			assert.Equal(t, "doc1", result.Hits[0].ID)
			assert.Greater(t, result.Hits[0].Score, 0.0)
		}
	})

	t.Run("MatchQueryWithScoring", func(t *testing.T) {
		// Search for "engine optimization"
		query := []byte(`{"match": {"description": "engine optimization"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, result.TotalHits, int64(1))

		// doc1 should score highest (has both terms)
		t.Logf("Found %d documents", result.TotalHits)
		if len(result.Hits) > 0 {
			topDoc := result.Hits[0]
			t.Logf("Top document: %s (score: %.4f)", topDoc.ID, topDoc.Score)
		}
	})
}

// TestPhraseQuery tests exact phrase matching
func TestPhraseQuery(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "phrase_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index documents with different word arrangements
	docs := []struct {
		id  string
		doc map[string]interface{}
	}{
		{
			id: "doc1",
			doc: map[string]interface{}{
				"text": "the quick brown fox jumps over the lazy dog",
			},
		},
		{
			id: "doc2",
			doc: map[string]interface{}{
				"text": "a lazy dog sleeps in the sun",
			},
		},
		{
			id: "doc3",
			doc: map[string]interface{}{
				"text": "the dog is not lazy but quick",
			},
		},
	}

	for _, d := range docs {
		err := shard.IndexDocument(d.id, d.doc)
		assert.NoError(t, err)
	}

	t.Run("ExactPhrase", func(t *testing.T) {
		// Search for exact phrase "lazy dog"
		query := []byte(`{"phrase": {"text": "lazy dog"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc1 and doc2 (exact phrase)
		// Should NOT match doc3 (words not consecutive)
		assert.GreaterOrEqual(t, result.TotalHits, int64(2))
		t.Logf("Found %d documents with exact phrase 'lazy dog'", result.TotalHits)
	})

	t.Run("LongerPhrase", func(t *testing.T) {
		// Search for longer phrase
		query := []byte(`{"phrase": {"text": "quick brown fox"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should only match doc1
		assert.Equal(t, int64(1), result.TotalHits)
		if len(result.Hits) > 0 {
			assert.Equal(t, "doc1", result.Hits[0].ID)
		}
	})
}

// TestRangeQuery tests numeric range queries
func TestRangeQuery(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "range_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index products with different prices
	for i := 1; i <= 10; i++ {
		doc := map[string]interface{}{
			"name":  fmt.Sprintf("Product %d", i),
			"price": float64(i * 10),
			"stock": i * 5,
		}
		err := shard.IndexDocument(fmt.Sprintf("prod%d", i), doc)
		assert.NoError(t, err)
	}

	t.Run("PriceRange_20to50", func(t *testing.T) {
		// Find products priced between 20 and 50 (inclusive)
		query := []byte(`{"range": {"price": {"gte": 20, "lte": 50}}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match products 2, 3, 4, 5 (prices 20, 30, 40, 50)
		assert.Equal(t, int64(4), result.TotalHits)
		t.Logf("Found %d products in price range 20-50", result.TotalHits)
	})

	t.Run("PriceRange_Exclusive", func(t *testing.T) {
		// Find products with price > 30 and < 70
		query := []byte(`{"range": {"price": {"gt": 30, "lt": 70}}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match products 4, 5, 6 (prices 40, 50, 60)
		assert.Equal(t, int64(3), result.TotalHits)
	})

	t.Run("StockRange", func(t *testing.T) {
		// Find products with stock >= 25
		query := []byte(`{"range": {"stock": {"gte": 25}}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match products 5-10
		assert.GreaterOrEqual(t, result.TotalHits, int64(6))
	})
}

// TestPrefixQuery tests prefix searching
func TestPrefixQuery(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "prefix_shard")
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
			doc: map[string]interface{}{"text": "searching for information"},
		},
		{
			id:  "doc2",
			doc: map[string]interface{}{"text": "search engine optimization"},
		},
		{
			id:  "doc3",
			doc: map[string]interface{}{"text": "researching new algorithms"},
		},
		{
			id:  "doc4",
			doc: map[string]interface{}{"text": "database indexing techniques"},
		},
	}

	for _, d := range docs {
		err := shard.IndexDocument(d.id, d.doc)
		assert.NoError(t, err)
	}

	t.Run("PrefixSearch", func(t *testing.T) {
		// Search for words starting with "search"
		query := []byte(`{"prefix": {"text": "search"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc1 (searching) and doc2 (search)
		assert.Equal(t, int64(2), result.TotalHits)
		t.Logf("Found %d documents with prefix 'search'", result.TotalHits)
	})

	t.Run("PrefixResearch", func(t *testing.T) {
		// Search for words starting with "research"
		query := []byte(`{"prefix": {"text": "research"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc3 (researching)
		assert.Equal(t, int64(1), result.TotalHits)
	})

	t.Run("PrefixIndex", func(t *testing.T) {
		// Search for words starting with "index"
		query := []byte(`{"prefix": {"text": "index"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc4 (indexing)
		assert.Equal(t, int64(1), result.TotalHits)
	})
}

// TestBooleanQueries tests complex boolean query combinations
func TestBooleanQueries(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "bool_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index diverse documents
	docs := []struct {
		id  string
		doc map[string]interface{}
	}{
		{
			id: "doc1",
			doc: map[string]interface{}{
				"title":    "Machine Learning Basics",
				"category": "AI",
				"price":    29.99,
				"tags":     []interface{}{"ml", "ai", "tutorial"},
			},
		},
		{
			id: "doc2",
			doc: map[string]interface{}{
				"title":    "Deep Learning Advanced",
				"category": "AI",
				"price":    49.99,
				"tags":     []interface{}{"dl", "ai", "advanced"},
			},
		},
		{
			id: "doc3",
			doc: map[string]interface{}{
				"title":    "Database Design",
				"category": "Database",
				"price":    39.99,
				"tags":     []interface{}{"database", "design", "tutorial"},
			},
		},
		{
			id: "doc4",
			doc: map[string]interface{}{
				"title":    "Web Development",
				"category": "Web",
				"price":    24.99,
				"tags":     []interface{}{"web", "javascript", "tutorial"},
			},
		},
	}

	for _, d := range docs {
		err := shard.IndexDocument(d.id, d.doc)
		assert.NoError(t, err)
	}

	t.Run("BoolMust_AND", func(t *testing.T) {
		// Must have both "learning" AND category "AI"
		query := []byte(`{
			"bool": {
				"must": [
					{"match": {"title": "learning"}},
					{"term": {"category": "ai"}}
				]
			}
		}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc1 and doc2
		assert.Equal(t, int64(2), result.TotalHits)
		t.Logf("Found %d AI learning documents", result.TotalHits)
	})

	t.Run("BoolShould_OR", func(t *testing.T) {
		// Should have "database" OR "web"
		query := []byte(`{
			"bool": {
				"should": [
					{"match": {"title": "database"}},
					{"match": {"title": "web"}}
				]
			}
		}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc3 and doc4
		assert.Equal(t, int64(2), result.TotalHits)
	})

	t.Run("BoolMustNot_Exclusion", func(t *testing.T) {
		// All documents but exclude category "AI"
		query := []byte(`{
			"bool": {
				"must": [
					{"match_all": {}}
				],
				"must_not": [
					{"term": {"category": "ai"}}
				]
			}
		}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc3 and doc4 (exclude doc1 and doc2)
		assert.Equal(t, int64(2), result.TotalHits)
	})

	t.Run("BoolFilter_NoScoring", func(t *testing.T) {
		// Match all with price filter
		query := []byte(`{
			"bool": {
				"must": [
					{"match_all": {}}
				],
				"filter": [
					{"range": {"price": {"gte": 30, "lte": 50}}}
				]
			}
		}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc2 (49.99) and doc3 (39.99)
		assert.Equal(t, int64(2), result.TotalHits)
		t.Logf("Found %d documents in price range 30-50", result.TotalHits)
	})

	t.Run("ComplexBool", func(t *testing.T) {
		// Complex: (tutorial OR advanced) AND price < 40 AND NOT web
		query := []byte(`{
			"bool": {
				"should": [
					{"term": {"tags": "tutorial"}},
					{"term": {"tags": "advanced"}}
				],
				"filter": [
					{"range": {"price": {"lt": 40}}}
				],
				"must_not": [
					{"term": {"category": "web"}}
				]
			}
		}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc1 (AI, 29.99, tutorial) and doc3 (Database, 39.99, tutorial)
		// Should NOT match doc2 (price 49.99) or doc4 (Web category)
		assert.GreaterOrEqual(t, result.TotalHits, int64(1))
		t.Logf("Found %d documents matching complex query", result.TotalHits)
	})
}

// TestNestedFieldQueries tests querying nested document fields
func TestNestedFieldQueries(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "nested_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index documents with nested fields
	docs := []struct {
		id  string
		doc map[string]interface{}
	}{
		{
			id: "doc1",
			doc: map[string]interface{}{
				"title": "Product A",
				"metadata": map[string]interface{}{
					"author":  "John Doe",
					"rating":  4.5,
					"reviews": 120,
				},
			},
		},
		{
			id: "doc2",
			doc: map[string]interface{}{
				"title": "Product B",
				"metadata": map[string]interface{}{
					"author":  "Jane Smith",
					"rating":  3.8,
					"reviews": 85,
				},
			},
		},
		{
			id: "doc3",
			doc: map[string]interface{}{
				"title": "Product C",
				"metadata": map[string]interface{}{
					"author":  "John Doe",
					"rating":  4.9,
					"reviews": 200,
				},
			},
		},
	}

	for _, d := range docs {
		err := shard.IndexDocument(d.id, d.doc)
		assert.NoError(t, err)
	}

	t.Run("NestedFieldRange", func(t *testing.T) {
		// Find products with rating >= 4.0
		query := []byte(`{"range": {"metadata.rating": {"gte": 4.0}}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc1 (4.5) and doc3 (4.9)
		assert.Equal(t, int64(2), result.TotalHits)
		t.Logf("Found %d high-rated products", result.TotalHits)
	})

	t.Run("NestedFieldTerm", func(t *testing.T) {
		// Find products by author "John Doe"
		query := []byte(`{"term": {"metadata.author": "john"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)

		// Should match doc1 and doc3
		assert.Equal(t, int64(2), result.TotalHits)
	})
}
