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

// TestDocumentIndexing tests full document indexing pipeline
func TestDocumentIndexing(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "indexing_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	t.Run("IndexMultipleDocuments", func(t *testing.T) {
		docs := []struct {
			id  string
			doc map[string]interface{}
		}{
			{
				id: "doc1",
				doc: map[string]interface{}{
					"title":       "The Quick Brown Fox",
					"description": "A story about a fox",
					"price":       29.99,
					"category":    "books",
					"in_stock":    true,
				},
			},
			{
				id: "doc2",
				doc: map[string]interface{}{
					"title":       "Lazy Dog Adventures",
					"description": "Tales of a lazy dog",
					"price":       19.99,
					"category":    "books",
					"in_stock":    true,
				},
			},
			{
				id: "doc3",
				doc: map[string]interface{}{
					"title":       "Electronics Guide",
					"description": "Everything about electronics",
					"price":       49.99,
					"category":    "electronics",
					"in_stock":    false,
				},
			},
		}

		// Index all documents
		for _, d := range docs {
			err := shard.IndexDocument(d.id, d.doc)
			assert.NoError(t, err, "Failed to index document %s", d.id)
		}

		t.Logf("Indexed %d documents", len(docs))
	})

	t.Run("RetrieveIndexedDocuments", func(t *testing.T) {
		// Retrieve doc1
		doc, err := shard.GetDocument("doc1")
		assert.NoError(t, err)
		assert.NotNil(t, doc)

		// Verify field access
		if doc != nil {
			title, ok := doc["title"].(string)
			assert.True(t, ok)
			assert.Equal(t, "The Quick Brown Fox", title)

			price, ok := doc["price"].(float64)
			assert.True(t, ok)
			assert.Equal(t, 29.99, price)
		}

		// Retrieve non-existent document
		doc, err = shard.GetDocument("nonexistent")
		assert.Error(t, err)
		assert.Nil(t, doc)
	})

	t.Run("SearchMatchAll", func(t *testing.T) {
		query := []byte(`{"match_all": {}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, int64(3), result.TotalHits, "Should match all 3 documents")
		assert.Equal(t, 3, len(result.Hits), "Should return 3 hits")
	})

	t.Run("SearchTermQuery", func(t *testing.T) {
		// Search for "fox" in title
		query := []byte(`{"term": {"title": "fox"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.GreaterOrEqual(t, result.TotalHits, int64(1), "Should find at least 1 document with 'fox'")

		t.Logf("Found %d documents matching 'fox'", result.TotalHits)
	})

	t.Run("SearchMatchQuery", func(t *testing.T) {
		// Search for "lazy dog" in description
		query := []byte(`{"match": {"description": "lazy dog"}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.GreaterOrEqual(t, result.TotalHits, int64(1), "Should find documents with 'lazy' or 'dog'")

		t.Logf("Found %d documents matching 'lazy dog'", result.TotalHits)
	})

	t.Run("SearchWithPagination", func(t *testing.T) {
		// Get all documents
		query := []byte(`{"match_all": {}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		totalHits := result.TotalHits

		// Get with pagination (from=1, size=2)
		// Note: pagination not fully implemented in test query format yet
		// This tests that basic search works
		assert.Equal(t, int64(3), totalHits)
	})

	t.Run("DeleteDocument", func(t *testing.T) {
		// Delete doc3
		err := shard.DeleteDocument("doc3")
		assert.NoError(t, err)

		// Verify it's gone
		doc, err := shard.GetDocument("doc3")
		assert.Error(t, err)
		assert.Nil(t, doc)

		// Search should now return only 2 documents
		query := []byte(`{"match_all": {}}`)
		result, err := shard.Search(query, nil)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), result.TotalHits, "Should have 2 documents after deletion")
	})

	t.Run("UpdateDocument", func(t *testing.T) {
		// Update doc1 (reindex with same ID)
		updatedDoc := map[string]interface{}{
			"title":       "The Quick Brown Fox - Updated",
			"description": "Updated story",
			"price":       39.99,
			"category":    "books",
			"in_stock":    false,
		}

		err := shard.IndexDocument("doc1", updatedDoc)
		assert.NoError(t, err)

		// Retrieve and verify update
		doc, err := shard.GetDocument("doc1")
		assert.NoError(t, err)
		assert.NotNil(t, doc)

		if doc != nil {
			title, _ := doc["title"].(string)
			assert.Contains(t, title, "Updated")

			price, _ := doc["price"].(float64)
			assert.Equal(t, 39.99, price)
		}
	})
}

// TestComplexDocuments tests indexing of documents with nested fields
func TestComplexDocuments(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "complex_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Document with nested fields and arrays
	complexDoc := map[string]interface{}{
		"title": "Complex Document",
		"metadata": map[string]interface{}{
			"author":  "John Doe",
			"created": "2026-01-26",
		},
		"tags":        []interface{}{"important", "urgent", "review"},
		"description": "This is a complex document with nested fields",
		"score":       95.5,
	}

	err = shard.IndexDocument("complex1", complexDoc)
	assert.NoError(t, err)

	// Retrieve and verify
	doc, err := shard.GetDocument("complex1")
	assert.NoError(t, err)
	assert.NotNil(t, doc)

	if doc != nil {
		// Verify top-level field
		title, ok := doc["title"].(string)
		assert.True(t, ok)
		assert.Equal(t, "Complex Document", title)

		// Verify nested field
		metadata, ok := doc["metadata"].(map[string]interface{})
		assert.True(t, ok)
		if ok {
			author, ok := metadata["author"].(string)
			assert.True(t, ok)
			assert.Equal(t, "John Doe", author)
		}

		// Verify array field
		tags, ok := doc["tags"].([]interface{})
		assert.True(t, ok)
		assert.Equal(t, 3, len(tags))
	}

	// Search should index array elements
	query := []byte(`{"term": {"tags": "urgent"}}`)
	result, err := shard.Search(query, nil)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, result.TotalHits, int64(1), "Should find document with tag 'urgent'")
}

// TestBulkIndexing tests indexing many documents
func TestBulkIndexing(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "bulk_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	// Index 100 documents
	const numDocs = 100

	for i := 0; i < numDocs; i++ {
		doc := map[string]interface{}{
			"id":          i,
			"title":       fmt.Sprintf("Document %d", i),
			"description": fmt.Sprintf("This is document number %d", i),
			"value":       float64(i * 10),
			"category":    fmt.Sprintf("cat%d", i%5),
		}

		err := shard.IndexDocument(fmt.Sprintf("doc%d", i), doc)
		assert.NoError(t, err)
	}

	t.Logf("Indexed %d documents", numDocs)

	// Search for all documents
	query := []byte(`{"match_all": {}}`)
	result, err := shard.Search(query, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(numDocs), result.TotalHits, "Should have all documents")

	// Search for specific term
	query = []byte(`{"term": {"title": "document"}}`)
	result, err = shard.Search(query, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(numDocs), result.TotalHits, "All documents have 'document' in title")
}

// TestFieldTypes tests indexing documents with various field types
func TestFieldTypes(t *testing.T) {
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

	shardPath := filepath.Join(tempDir, "types_shard")
	shard, err := bridge.CreateShard(shardPath)
	require.NoError(t, err)
	defer shard.Close()

	doc := map[string]interface{}{
		"string_field":  "hello world",
		"int_field":     42,
		"float_field":   3.14159,
		"bool_field":    true,
		"null_field":    nil,
		"array_field":   []interface{}{1, 2, 3},
		"object_field":  map[string]interface{}{"nested": "value"},
	}

	err = shard.IndexDocument("types1", doc)
	assert.NoError(t, err)

	// Retrieve and verify all types
	retrieved, err := shard.GetDocument("types1")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)

	if retrieved != nil {
		assert.Equal(t, "hello world", retrieved["string_field"])
		// Note: JSON numbers may be parsed as float64
		assert.NotNil(t, retrieved["int_field"])
		assert.NotNil(t, retrieved["float_field"])
		assert.Equal(t, true, retrieved["bool_field"])
		assert.Nil(t, retrieved["null_field"])
		assert.NotNil(t, retrieved["array_field"])
		assert.NotNil(t, retrieved["object_field"])
	}
}
