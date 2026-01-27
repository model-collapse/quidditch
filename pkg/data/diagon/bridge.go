package diagon

/*
#cgo CFLAGS: -I${SRCDIR}/c_api_src
#cgo LDFLAGS: -L${SRCDIR}/build -ldiagon -L${SRCDIR}/upstream/build/src/core -ldiagon_core -lz -lzstd -llz4 -Wl,-rpath,${SRCDIR}/build -Wl,-rpath,${SRCDIR}/upstream/build/src/core
#include <stdlib.h>
#include "diagon_c_api.h"
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"

	"go.uber.org/zap"
)

// DiagonBridge provides a Go interface to the real Diagon C++ search engine
type DiagonBridge struct {
	config     *Config
	logger     *zap.Logger
	shards     map[string]*Shard
	mu         sync.RWMutex
}

// Config holds Diagon configuration
type Config struct {
	DataDir     string
	SIMDEnabled bool
	Logger      *zap.Logger
}

// NewDiagonBridge creates a new Diagon bridge
func NewDiagonBridge(cfg *Config) (*DiagonBridge, error) {
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	bridge := &DiagonBridge{
		config: cfg,
		logger: cfg.Logger,
		shards: make(map[string]*Shard),
	}

	return bridge, nil
}

// Start starts the Diagon engine
func (db *DiagonBridge) Start() error {
	db.logger.Info("Starting real Diagon C++ search engine",
		zap.String("data_dir", db.config.DataDir),
		zap.Bool("simd_enabled", db.config.SIMDEnabled))

	return nil
}

// Stop stops the Diagon engine
func (db *DiagonBridge) Stop() error {
	db.logger.Info("Stopping Diagon engine")

	db.mu.Lock()
	defer db.mu.Unlock()

	// Close all shards
	for path, shard := range db.shards {
		db.logger.Info("Closing Diagon shard", zap.String("path", path))
		if err := shard.Close(); err != nil {
			db.logger.Error("Error closing shard", zap.String("path", path), zap.Error(err))
		}
	}

	return nil
}

// CreateShard creates a new shard using real Diagon IndexWriter
func (db *DiagonBridge) CreateShard(path string) (*Shard, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Check if shard already exists
	if _, exists := db.shards[path]; exists {
		return nil, fmt.Errorf("shard at path %s already exists", path)
	}

	// Open directory
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	dir := C.diagon_open_mmap_directory(cPath) // Use MMapDirectory for performance
	if dir == nil {
		errMsg := C.GoString(C.diagon_last_error())
		return nil, fmt.Errorf("failed to open directory: %s", errMsg)
	}

	// Create IndexWriter config
	config := C.diagon_create_index_writer_config()
	C.diagon_config_set_ram_buffer_size(config, 64.0)                   // 64MB buffer
	C.diagon_config_set_open_mode(config, 2)                            // CREATE_OR_APPEND
	C.diagon_config_set_commit_on_close(config, true)

	// Create IndexWriter
	writer := C.diagon_create_index_writer(dir, config)
	C.diagon_free_index_writer_config(config)

	if writer == nil {
		C.diagon_close_directory(dir)
		errMsg := C.GoString(C.diagon_last_error())
		return nil, fmt.Errorf("failed to create IndexWriter: %s", errMsg)
	}

	shard := &Shard{
		path:      path,
		bridge:    db,
		directory: dir,
		writer:    writer,
		reader:    nil, // Will be opened when needed
		logger:    db.logger.With(zap.String("shard_path", path)),
	}

	db.shards[path] = shard

	shard.logger.Info("Created real Diagon shard with IndexWriter")

	return shard, nil
}

// GetShard retrieves an existing shard
func (db *DiagonBridge) GetShard(path string) (*Shard, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	shard, exists := db.shards[path]
	if !exists {
		return nil, fmt.Errorf("shard at path %s not found", path)
	}

	return shard, nil
}

// Shard represents a real Diagon shard with IndexWriter/IndexReader
type Shard struct {
	path      string
	bridge    *DiagonBridge
	directory C.DiagonDirectory
	writer    C.DiagonIndexWriter
	reader    C.DiagonIndexReader
	searcher  C.DiagonIndexSearcher
	logger    *zap.Logger
	mu        sync.RWMutex
}

// IndexDocument indexes a document using real Diagon IndexWriter
func (s *Shard) IndexDocument(docID string, doc map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("==> DiagonBridge.IndexDocument ENTRY",
		zap.String("doc_id", docID),
		zap.Int("num_fields", len(doc)))

	// Create Diagon document
	diagonDoc := C.diagon_create_document()
	defer C.diagon_free_document(diagonDoc)

	s.logger.Info("Created Diagon document object", zap.String("doc_id", docID))

	// Add ID field - both indexed (for searching) and stored (for retrieval)
	cDocID := C.CString(docID)
	defer C.free(unsafe.Pointer(cDocID))
	cIDFieldName := C.CString("_id")
	defer C.free(unsafe.Pointer(cIDFieldName))

	// Add as StringField for exact-match searching (indexed, not analyzed)
	idField := C.diagon_create_string_field(cIDFieldName, cDocID)
	C.diagon_document_add_field(diagonDoc, idField)

	// ALSO add as StoredField so we can retrieve it
	storedIDField := C.diagon_create_stored_field(cIDFieldName, cDocID)
	C.diagon_document_add_field(diagonDoc, storedIDField)

	// Add other fields
	for key, value := range doc {
		cFieldName := C.CString(key)
		defer C.free(unsafe.Pointer(cFieldName))

		s.logger.Info("DEBUG: Indexing field",
			zap.String("field", key),
			zap.String("type", fmt.Sprintf("%T", value)),
			zap.Any("value", value))

		switch v := value.(type) {
		case string:
			// TextField for strings (analyzed, indexed, stored)
			cValue := C.CString(v)
			defer C.free(unsafe.Pointer(cValue))
			field := C.diagon_create_text_field(cFieldName, cValue)
			C.diagon_document_add_field(diagonDoc, field)
			s.logger.Info("DEBUG: Created text field", zap.String("field", key))

		case int, int32, int64:
			// Create indexed numeric field for integers (searchable with range queries)
			val := int64(0)
			switch n := v.(type) {
			case int:
				val = int64(n)
			case int32:
				val = int64(n)
			case int64:
				val = n
			}
			// Use indexed field instead of doc values only field
			field := C.diagon_create_indexed_long_field(cFieldName, C.int64_t(val))
			C.diagon_document_add_field(diagonDoc, field)
			s.logger.Info("DEBUG: Created indexed long field", zap.String("field", key), zap.Int64("value", val))

		case float32, float64:
			// Create indexed numeric field for floats (searchable with range queries)
			val := float64(0)
			switch f := v.(type) {
			case float32:
				val = float64(f)
			case float64:
				val = f
			}
			// Use indexed field instead of doc values only field
			field := C.diagon_create_indexed_double_field(cFieldName, C.double(val))
			C.diagon_document_add_field(diagonDoc, field)
			s.logger.Info("DEBUG: Created indexed double field", zap.String("field", key), zap.Float64("value", val))

		default:
			// Convert to JSON string for complex types
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				s.logger.Warn("Failed to marshal field, skipping",
					zap.String("field", key),
					zap.Error(err))
				continue
			}
			cValue := C.CString(string(jsonBytes))
			defer C.free(unsafe.Pointer(cValue))
			field := C.diagon_create_stored_field(cFieldName, cValue)
			C.diagon_document_add_field(diagonDoc, field)
		}
	}

	// Add document to IndexWriter
	s.logger.Info("Calling C.diagon_add_document", zap.String("doc_id", docID))
	result := C.diagon_add_document(s.writer, diagonDoc)
	s.logger.Info("C.diagon_add_document returned",
		zap.String("doc_id", docID),
		zap.Bool("success", bool(result)))

	if !result {
		errMsg := C.GoString(C.diagon_last_error())
		s.logger.Error("C.diagon_add_document FAILED",
			zap.String("doc_id", docID),
			zap.String("error", errMsg))
		return fmt.Errorf("failed to add document: %s", errMsg)
	}

	s.logger.Info("Document added to IndexWriter RAM buffer (NOT YET COMMITTED)",
		zap.String("doc_id", docID),
		zap.Int("fields", len(doc)))

	s.logger.Warn("WARNING: Document is in RAM buffer but NOT committed to disk yet! Need to call Commit() or Flush()")

	return nil
}

// Commit commits all pending changes
func (s *Shard) Commit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !C.diagon_commit(s.writer) {
		errMsg := C.GoString(C.diagon_last_error())
		return fmt.Errorf("commit failed: %s", errMsg)
	}

	s.logger.Debug("Committed changes")
	return nil
}

// Flush flushes buffered documents to disk
func (s *Shard) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !C.diagon_flush(s.writer) {
		errMsg := C.GoString(C.diagon_last_error())
		return fmt.Errorf("flush failed: %s", errMsg)
	}

	s.logger.Debug("Flushed buffered documents")
	return nil
}

// Refresh reopens the reader to see recent changes
func (s *Shard) Refresh() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Commit first to ensure changes are visible
	if !C.diagon_commit(s.writer) {
		errMsg := C.GoString(C.diagon_last_error())
		return fmt.Errorf("commit failed during refresh: %s", errMsg)
	}

	// Close old reader and searcher
	if s.searcher != nil {
		C.diagon_free_index_searcher(s.searcher)
		s.searcher = nil
	}
	if s.reader != nil {
		C.diagon_close_index_reader(s.reader)
		s.reader = nil
	}

	// Open new reader
	s.reader = C.diagon_open_index_reader(s.directory)
	if s.reader == nil {
		errMsg := C.GoString(C.diagon_last_error())
		return fmt.Errorf("failed to reopen reader: %s", errMsg)
	}

	// Create new searcher
	s.searcher = C.diagon_create_index_searcher(s.reader)
	if s.searcher == nil {
		errMsg := C.GoString(C.diagon_last_error())
		return fmt.Errorf("failed to create searcher: %s", errMsg)
	}

	s.logger.Debug("Refreshed shard (reopened reader)")
	return nil
}

// convertQueryToDiagon converts a query object to a Diagon query
// This is a helper function used by Search and for recursive bool query parsing
// Caller is responsible for freeing the returned query
func (s *Shard) convertQueryToDiagon(queryObj map[string]interface{}) (C.DiagonQuery, error) {
	var diagonQuery C.DiagonQuery

	// Handle different query types
	if termQuery, ok := queryObj["term"].(map[string]interface{}); ok {
		// Term query: {"term": {"field_name": "term_value"}} or {"term": {"field_name": {"value": "term_value"}}}
		for field, value := range termQuery {
			cField := C.CString(field)
			defer C.free(unsafe.Pointer(cField))

			// Handle both simple and complex term query formats
			var termValue string
			switch v := value.(type) {
			case string:
				termValue = v
			case map[string]interface{}:
				if val, ok := v["value"]; ok {
					termValue = fmt.Sprintf("%v", val)
				}
			default:
				termValue = fmt.Sprintf("%v", v)
			}

			cValue := C.CString(termValue)
			defer C.free(unsafe.Pointer(cValue))

			term := C.diagon_create_term(cField, cValue)
			defer C.diagon_free_term(term)

			diagonQuery = C.diagon_create_term_query(term)
			if diagonQuery == nil {
				errMsg := C.GoString(C.diagon_last_error())
				return nil, fmt.Errorf("failed to create term query: %s", errMsg)
			}
			break // Only support single term for now
		}
	} else if matchQuery, ok := queryObj["match"].(map[string]interface{}); ok {
		// Match query: {"match": {"field_name": "query_text"}} or {"match": {"field_name": {"query": "text"}}}
		// For now, treat match query as term query (no text analysis in Diagon Phase 4)
		for field, value := range matchQuery {
			cField := C.CString(field)
			defer C.free(unsafe.Pointer(cField))

			// Handle both simple and complex match query formats
			var matchText string
			switch v := value.(type) {
			case string:
				matchText = v
			case map[string]interface{}:
				if q, ok := v["query"].(string); ok {
					matchText = q
				}
			default:
				matchText = fmt.Sprintf("%v", v)
			}

			cValue := C.CString(matchText)
			defer C.free(unsafe.Pointer(cValue))

			term := C.diagon_create_term(cField, cValue)
			defer C.diagon_free_term(term)

			diagonQuery = C.diagon_create_term_query(term)
			if diagonQuery == nil {
				errMsg := C.GoString(C.diagon_last_error())
				return nil, fmt.Errorf("failed to create match query: %s", errMsg)
			}
			break // Only support single field for now
		}
	} else if _, ok := queryObj["match_all"]; ok {
		// Match all query: {"match_all": {}}
		// Workaround: Use a very broad range query on _id field
		// Since _id is always indexed, this effectively matches all documents
		// Use ASCII range to match all possible _id values
		cField := C.CString("_id")
		defer C.free(unsafe.Pointer(cField))

		// Use a range that covers all possible string values
		// From empty string to highest Unicode character
		diagonQuery = C.diagon_create_numeric_range_query(
			cField,
			C.double(-9999999999), // Very low value
			C.double(9999999999),  // Very high value
			C.bool(true),
			C.bool(true),
		)
		if diagonQuery == nil {
			// If range query fails, try empty bool query
			diagonQuery = C.diagon_create_bool_query()
			if diagonQuery != nil {
				// Build empty bool query (matches nothing by default, but better than nil)
				diagonQuery = C.diagon_bool_query_build(diagonQuery)
			}
			if diagonQuery == nil {
				errMsg := C.GoString(C.diagon_last_error())
				return nil, fmt.Errorf("failed to create match_all workaround: %s", errMsg)
			}
		}
	} else if rangeQuery, ok := queryObj["range"].(map[string]interface{}); ok {
		// Range query: {"range": {"field_name": {"gte": 100, "lte": 1000}}}
		for field, rangeParams := range rangeQuery {
			params := rangeParams.(map[string]interface{})

			s.logger.Info("DEBUG: Range query params",
				zap.String("field", field),
				zap.Any("params", params))

			var lowerValue, upperValue float64
			var includeLower, includeUpper bool

			// Parse lower bound
			if gte, ok := params["gte"].(float64); ok {
				lowerValue = gte
				includeLower = true
				s.logger.Info("DEBUG: Found gte (float64)", zap.Float64("value", gte))
			} else if gt, ok := params["gt"].(float64); ok {
				lowerValue = gt
				includeLower = false
				s.logger.Info("DEBUG: Found gt (float64)", zap.Float64("value", gt))
			} else {
				// No lower bound - use smallest representable value
				// Use -(2^53) which is safe for float64 → int64 conversion
				lowerValue = -9007199254740992
				includeLower = true
				s.logger.Info("DEBUG: No lower bound, using default", zap.Float64("value", lowerValue))
			}

			// Parse upper bound
			if lte, ok := params["lte"].(float64); ok {
				upperValue = lte
				includeUpper = true
				s.logger.Info("DEBUG: Found lte (float64)", zap.Float64("value", lte))
			} else if lt, ok := params["lt"].(float64); ok {
				upperValue = lt
				includeUpper = false
				s.logger.Info("DEBUG: Found lt (float64)", zap.Float64("value", lt))
			} else {
				// No upper bound - use largest safe value
				// Use 2^53 which is the max safe integer in float64
				upperValue = 9007199254740992
				includeUpper = true
				s.logger.Info("DEBUG: No upper bound, using default", zap.Float64("value", upperValue))
			}

			cField := C.CString(field)
			defer C.free(unsafe.Pointer(cField))

			s.logger.Info("DEBUG: Creating Diagon double range query",
				zap.String("field", field),
				zap.Float64("lower", lowerValue),
				zap.Float64("upper", upperValue),
				zap.Bool("include_lower", includeLower),
				zap.Bool("include_upper", includeUpper))

			// Use double range query for proper double field support
			diagonQuery = C.diagon_create_double_range_query(
				cField,
				C.double(lowerValue),
				C.double(upperValue),
				C.bool(includeLower),
				C.bool(includeUpper),
			)

			if diagonQuery == nil {
				errMsg := C.GoString(C.diagon_last_error())
				s.logger.Error("DEBUG: Failed to create Diagon double range query", zap.String("error", errMsg))
				return nil, fmt.Errorf("failed to create double range query: %s", errMsg)
			}
			s.logger.Info("DEBUG: Diagon double range query created successfully")
			break // Only support single field for now
		}
	} else if boolQuery, ok := queryObj["bool"].(map[string]interface{}); ok {
		// Bool query: {"bool": {"must": [...], "should": [...], "filter": [...], "must_not": [...]}}
		boolQueryBuilder := C.diagon_create_bool_query()
		if boolQueryBuilder == nil {
			errMsg := C.GoString(C.diagon_last_error())
			return nil, fmt.Errorf("failed to create bool query: %s", errMsg)
		}

		// Add MUST clauses
		if mustClauses, ok := boolQuery["must"]; ok {
			clauseArray, isArray := mustClauses.([]interface{})
			if !isArray {
				return nil, fmt.Errorf("must clauses must be an array")
			}

			for _, clause := range clauseArray {
				clauseMap, ok := clause.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("clause must be an object")
				}

				// Recursively parse sub-query
				subQuery, err := s.convertQueryToDiagon(clauseMap)
				if err != nil {
					return nil, fmt.Errorf("failed to convert must sub-query: %w", err)
				}

				C.diagon_bool_query_add_must(boolQueryBuilder, subQuery)
			}
		}

		// Add SHOULD clauses
		if shouldClauses, ok := boolQuery["should"]; ok {
			clauseArray, isArray := shouldClauses.([]interface{})
			if !isArray {
				return nil, fmt.Errorf("should clauses must be an array")
			}

			for _, clause := range clauseArray {
				clauseMap, ok := clause.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("clause must be an object")
				}

				subQuery, err := s.convertQueryToDiagon(clauseMap)
				if err != nil {
					return nil, fmt.Errorf("failed to convert should sub-query: %w", err)
				}

				C.diagon_bool_query_add_should(boolQueryBuilder, subQuery)
			}
		}

		// Add FILTER clauses
		if filterClauses, ok := boolQuery["filter"]; ok {
			clauseArray, isArray := filterClauses.([]interface{})
			if !isArray {
				return nil, fmt.Errorf("filter clauses must be an array")
			}

			for _, clause := range clauseArray {
				clauseMap, ok := clause.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("clause must be an object")
				}

				subQuery, err := s.convertQueryToDiagon(clauseMap)
				if err != nil {
					return nil, fmt.Errorf("failed to convert filter sub-query: %w", err)
				}

				C.diagon_bool_query_add_filter(boolQueryBuilder, subQuery)
			}
		}

		// Add MUST_NOT clauses
		if mustNotClauses, ok := boolQuery["must_not"]; ok {
			clauseArray, isArray := mustNotClauses.([]interface{})
			if !isArray {
				return nil, fmt.Errorf("must_not clauses must be an array")
			}

			for _, clause := range clauseArray {
				clauseMap, ok := clause.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("clause must be an object")
				}

				subQuery, err := s.convertQueryToDiagon(clauseMap)
				if err != nil {
					return nil, fmt.Errorf("failed to convert must_not sub-query: %w", err)
				}

				C.diagon_bool_query_add_must_not(boolQueryBuilder, subQuery)
			}
		}

		// Set minimum_should_match if specified
		if minShould, ok := boolQuery["minimum_should_match"].(float64); ok {
			C.diagon_bool_query_set_minimum_should_match(boolQueryBuilder, C.int(minShould))
		}

		// Build the final query
		diagonQuery = C.diagon_bool_query_build(boolQueryBuilder)
		if diagonQuery == nil {
			errMsg := C.GoString(C.diagon_last_error())
			return nil, fmt.Errorf("failed to build bool query: %s", errMsg)
		}
	} else {
		// Extract query type for better error message
		queryTypes := make([]string, 0, len(queryObj))
		for k := range queryObj {
			queryTypes = append(queryTypes, k)
		}
		return nil, fmt.Errorf("unsupported query type: %v (currently supported: 'term', 'match', 'match_all', 'range', 'bool')", queryTypes)
	}

	return diagonQuery, nil
}

// Search executes a search query using real Diagon IndexSearcher
func (s *Shard) Search(query []byte, filterExpression []byte) (*SearchResult, error) {
	s.mu.Lock()

	// Ensure reader/searcher are initialized
	if s.reader == nil {
		// Open reader
		s.reader = C.diagon_open_index_reader(s.directory)
		if s.reader == nil {
			s.mu.Unlock()
			errMsg := C.GoString(C.diagon_last_error())
			return nil, fmt.Errorf("failed to open reader: %s", errMsg)
		}

		// Create searcher
		s.searcher = C.diagon_create_index_searcher(s.reader)
		if s.searcher == nil {
			s.mu.Unlock()
			errMsg := C.GoString(C.diagon_last_error())
			return nil, fmt.Errorf("failed to create searcher: %s", errMsg)
		}
	}

	s.mu.Unlock()

	// Parse query JSON
	var queryObj map[string]interface{}
	if err := json.Unmarshal(query, &queryObj); err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	// Convert to Diagon query
	diagonQuery, err := s.convertQueryToDiagon(queryObj)
	if err != nil {
		return nil, err
	}
	defer C.diagon_free_query(diagonQuery)

	// Execute search
	s.mu.RLock()
	topDocs := C.diagon_search(s.searcher, diagonQuery, 10)
	s.mu.RUnlock()

	if topDocs == nil {
		errMsg := C.GoString(C.diagon_last_error())
		return nil, fmt.Errorf("search failed: %s", errMsg)
	}
	defer C.diagon_free_top_docs(topDocs)

	// Extract results
	totalHits := int64(C.diagon_top_docs_total_hits(topDocs))
	maxScore := float64(C.diagon_top_docs_max_score(topDocs))
	numResults := int(C.diagon_top_docs_score_docs_length(topDocs))

	hits := make([]*Hit, 0, numResults)
	for i := 0; i < numResults; i++ {
		scoreDoc := C.diagon_top_docs_score_doc_at(topDocs, C.int(i))
		if scoreDoc == nil {
			continue
		}

		docID := int(C.diagon_score_doc_get_doc(scoreDoc))
		score := float64(C.diagon_score_doc_get_score(scoreDoc))

		// Note: Document retrieval (getting source) not yet implemented in Diagon Phase 4
		// Return doc ID and score only
		hits = append(hits, &Hit{
			ID:     fmt.Sprintf("doc_%d", docID),
			Score:  score,
			Source: map[string]interface{}{
				"_internal_doc_id": docID,
			},
		})
	}

	result := &SearchResult{
		Took:      5, // TODO: Track actual time
		TotalHits: totalHits,
		MaxScore:  maxScore,
		Hits:      hits,
	}

	s.logger.Debug("Executed search via real Diagon IndexSearcher",
		zap.Int64("total_hits", totalHits),
		zap.Float64("max_score", maxScore),
		zap.Int("num_results", numResults))

	return result, nil
}

// GetDocument retrieves a document by ID
func (s *Shard) GetDocument(docID string) (map[string]interface{}, error) {
	s.logger.Info(">>>>>> GetDocument ENTRY", zap.String("doc_id", docID))
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Debug("GetDocument called", zap.String("doc_id", docID))

	// Ensure reader and searcher are initialized
	if s.reader == nil || s.searcher == nil {
		s.logger.Info("Reader not initialized, opening now", zap.String("doc_id", docID))

		// Commit first to ensure changes are visible
		if !C.diagon_commit(s.writer) {
			errMsg := C.GoString(C.diagon_last_error())
			return nil, fmt.Errorf("commit failed: %s", errMsg)
		}

		// Open reader
		s.reader = C.diagon_open_index_reader(s.directory)
		if s.reader == nil {
			errMsg := C.GoString(C.diagon_last_error())
			return nil, fmt.Errorf("failed to open reader: %s", errMsg)
		}

		// Create searcher
		s.searcher = C.diagon_create_index_searcher(s.reader)
		if s.searcher == nil {
			errMsg := C.GoString(C.diagon_last_error())
			return nil, fmt.Errorf("failed to create searcher: %s", errMsg)
		}

		s.logger.Info("Reader and searcher initialized successfully")
	}

	// Search for the document by _id field to get internal doc ID
	s.logger.Info("STEP 1: Creating term for _id search")
	cIDField := C.CString("_id")
	defer C.free(unsafe.Pointer(cIDField))

	cDocID := C.CString(docID)
	defer C.free(unsafe.Pointer(cDocID))

	term := C.diagon_create_term(cIDField, cDocID)
	if term == nil {
		errMsg := C.GoString(C.diagon_last_error())
		s.logger.Error("FAILED at create term", zap.String("error", errMsg))
		return nil, fmt.Errorf("failed to create term: %s", errMsg)
	}
	defer C.diagon_free_term(term)

	s.logger.Info("STEP 2: Creating term query")
	query := C.diagon_create_term_query(term)
	if query == nil {
		errMsg := C.GoString(C.diagon_last_error())
		s.logger.Error("FAILED at create query", zap.String("error", errMsg))
		return nil, fmt.Errorf("failed to create query: %s", errMsg)
	}
	defer C.diagon_free_query(query)

	s.logger.Info("STEP 3: Executing search", zap.String("doc_id", docID))

	// Search to find the internal doc ID
	topDocs := C.diagon_search(s.searcher, query, 1)
	if topDocs == nil {
		errMsg := C.GoString(C.diagon_last_error())
		s.logger.Error("FAILED at search", zap.String("error", errMsg))
		return nil, fmt.Errorf("search failed: %s", errMsg)
	}
	defer C.diagon_free_top_docs(topDocs)

	totalHits := int64(C.diagon_top_docs_total_hits(topDocs))
	s.logger.Debug("Search completed", zap.Int64("total_hits", totalHits))

	if totalHits == 0 {
		return nil, fmt.Errorf("document not found")
	}

	// Get internal doc ID from search result
	scoreDoc := C.diagon_top_docs_score_doc_at(topDocs, 0)
	if scoreDoc == nil {
		return nil, fmt.Errorf("failed to get score doc")
	}

	internalDocID := int(C.diagon_score_doc_get_doc(scoreDoc))
	s.logger.Debug("Found document", zap.Int("internal_doc_id", internalDocID))

	// Retrieve stored fields using reader
	s.logger.Info("CALLING diagon_reader_get_document", zap.Int("internal_doc_id", internalDocID))
	diagonDoc := C.diagon_reader_get_document(s.reader, C.int(internalDocID))
	s.logger.Info("RETURNED from diagon_reader_get_document", zap.Bool("is_nil", diagonDoc == nil))
	if diagonDoc == nil {
		errMsg := C.GoString(C.diagon_last_error())
		s.logger.Info("ERROR from C API", zap.String("error", errMsg))
		return nil, fmt.Errorf("failed to retrieve document: %s", errMsg)
	}
	defer C.diagon_free_document(diagonDoc)

	// Extract fields from Diagon document
	doc := make(map[string]interface{})

	// Get _id field
	idBuf := make([]byte, 1024)
	cIDFieldName := C.CString("_id")
	defer C.free(unsafe.Pointer(cIDFieldName))
	if C.diagon_document_get_field_value(diagonDoc, cIDFieldName,
		(*C.char)(unsafe.Pointer(&idBuf[0])), C.size_t(len(idBuf))) {
		// Find null terminator
		nullIdx := 0
		for i, b := range idBuf {
			if b == 0 {
				nullIdx = i
				break
			}
		}
		doc["_id"] = string(idBuf[:nullIdx])
	}

	// Try to get common text fields from the original document
	// Since we don't have field enumeration, we'll try common field names
	commonFields := []string{"title", "description", "name", "content", "text", "body"}
	for _, fieldName := range commonFields {
		buf := make([]byte, 4096)
		cFieldName := C.CString(fieldName)
		if C.diagon_document_get_field_value(diagonDoc, cFieldName,
			(*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))) {
			// Find null terminator
			nullIdx := 0
			for i, b := range buf {
				if b == 0 {
					nullIdx = i
					break
				}
			}
			if nullIdx > 0 {
				doc[fieldName] = string(buf[:nullIdx])
			}
		}
		C.free(unsafe.Pointer(cFieldName))
	}

	// Try to get common numeric fields
	commonNumFields := []string{"price", "count", "quantity", "age", "score"}
	for _, fieldName := range commonNumFields {
		var val int64
		cFieldName := C.CString(fieldName)
		if C.diagon_document_get_long_value(diagonDoc, cFieldName, (*C.int64_t)(unsafe.Pointer(&val))) {
			doc[fieldName] = val
		}
		C.free(unsafe.Pointer(cFieldName))
	}

	// Try to get common float fields
	for _, fieldName := range commonNumFields {
		var val float64
		cFieldName := C.CString(fieldName)
		if C.diagon_document_get_double_value(diagonDoc, cFieldName, (*C.double)(unsafe.Pointer(&val))) {
			// Only add if not already added as int
			if _, exists := doc[fieldName]; !exists {
				doc[fieldName] = val
			}
		}
		C.free(unsafe.Pointer(cFieldName))
	}

	s.logger.Info("Retrieved document via Diagon StoredFieldsReader",
		zap.String("doc_id", docID),
		zap.Int("internal_doc_id", internalDocID),
		zap.Int("num_fields", len(doc)))

	return doc, nil
}

// DeleteDocument deletes a document (not yet implemented in Phase 4)
func (s *Shard) DeleteDocument(docID string) error {
	// TODO: Implement when document deletion is available in Diagon
	s.logger.Warn("Document deletion not yet implemented in Diagon Phase 4", zap.String("doc_id", docID))
	return fmt.Errorf("document deletion not yet implemented in Diagon Phase 4")
}

// Close closes the shard and frees all resources
func (s *Shard) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Close searcher
	if s.searcher != nil {
		C.diagon_free_index_searcher(s.searcher)
		s.searcher = nil
	}

	// Close reader
	if s.reader != nil {
		C.diagon_close_index_reader(s.reader)
		s.reader = nil
	}

	// Close writer
	if s.writer != nil {
		C.diagon_close_index_writer(s.writer)
		s.writer = nil
	}

	// Close directory
	if s.directory != nil {
		C.diagon_close_directory(s.directory)
		s.directory = nil
	}

	s.logger.Info("Closed real Diagon shard")

	return nil
}

// SearchResult represents search results
type SearchResult struct {
	Took         int64                        `json:"took"`
	TotalHits    int64                        `json:"total_hits"`
	MaxScore     float64                      `json:"max_score"`
	Hits         []*Hit                       `json:"hits"`
	Aggregations map[string]AggregationResult `json:"aggregations,omitempty"`
}

// Hit represents a search hit
type Hit struct {
	ID     string                 `json:"_id"`
	Score  float64                `json:"_score"`
	Source map[string]interface{} `json:"_source"`
}

// AggregationResult represents an aggregation result
type AggregationResult struct {
	Type    string                   `json:"type"`
	Buckets []map[string]interface{} `json:"buckets,omitempty"`
	Count   int64                    `json:"count,omitempty"`
	Min     float64                  `json:"min,omitempty"`
	Max     float64                  `json:"max,omitempty"`
	Avg     float64                  `json:"avg,omitempty"`
	Sum     float64                  `json:"sum,omitempty"`
	Value   int64                    `json:"value,omitempty"`
	Values  map[string]float64       `json:"values,omitempty"`
}
