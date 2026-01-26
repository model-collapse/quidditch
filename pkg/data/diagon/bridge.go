package diagon

/*
#cgo LDFLAGS: -L${SRCDIR}/build -ldiagon_expression -lstdc++

#include <stdlib.h>
#include "cgo_wrapper.h"
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"

	"go.uber.org/zap"
)

// DiagonBridge provides a Go interface to the Diagon C++ search engine
type DiagonBridge struct {
	config     *Config
	logger     *zap.Logger
	shards     map[string]*Shard
	mu         sync.RWMutex
	cgoEnabled bool // Flag to track if CGO is available
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
		config:     cfg,
		logger:     cfg.Logger,
		shards:     make(map[string]*Shard),
		cgoEnabled: true, // ENABLED: C++ indexing now available
	}

	return bridge, nil
}

// Start starts the Diagon engine
func (db *DiagonBridge) Start() error {
	db.logger.Info("Starting Diagon engine",
		zap.String("data_dir", db.config.DataDir),
		zap.Bool("simd_enabled", db.config.SIMDEnabled),
		zap.Bool("cgo_enabled", db.cgoEnabled))

	if db.cgoEnabled {
		db.logger.Info("Diagon C++ engine ready")
	} else {
		db.logger.Warn("Running in stub mode - Diagon C++ core not yet implemented")
	}

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

// CreateShard creates a new shard
func (db *DiagonBridge) CreateShard(path string) (*Shard, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Check if shard already exists
	if _, exists := db.shards[path]; exists {
		return nil, fmt.Errorf("shard at path %s already exists", path)
	}

	shard := &Shard{
		path:       path,
		bridge:     db,
		shardPtr:   nil,
		documents:  make(map[string]map[string]interface{}), // In-memory stub
		logger:     db.logger.With(zap.String("shard_path", path)),
		cgoEnabled: db.cgoEnabled,
	}

	if db.cgoEnabled {
		// Create C++ shard
		cPath := C.CString(path)
		defer C.free(unsafe.Pointer(cPath))

		shard.shardPtr = C.diagon_create_shard(cPath)
		if shard.shardPtr == nil {
			return nil, fmt.Errorf("failed to create Diagon shard")
		}

		shard.logger.Info("Created Diagon C++ shard")
	} else {
		// Stub mode - in-memory storage
		shard.logger.Info("Created stub shard (in-memory)")
	}

	db.shards[path] = shard

	return shard, nil
}

// Shard represents a Diagon shard
type Shard struct {
	path       string
	bridge     *DiagonBridge
	shardPtr   *C.diagon_shard_t // C pointer to shard
	documents  map[string]map[string]interface{} // In-memory stub storage
	logger     *zap.Logger
	mu         sync.RWMutex
	cgoEnabled bool
}

// IndexDocument indexes a document
func (s *Shard) IndexDocument(docID string, doc map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cgoEnabled && s.shardPtr != nil {
		// Marshal document to JSON
		docJSON, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("failed to marshal document: %w", err)
		}

		// Call C++ API
		cDocID := C.CString(docID)
		defer C.free(unsafe.Pointer(cDocID))

		cDocJSON := C.CString(string(docJSON))
		defer C.free(unsafe.Pointer(cDocJSON))

		result := C.diagon_index_document(s.shardPtr, cDocID, cDocJSON)
		if result != 0 {
			return fmt.Errorf("diagon_index_document failed with code %d", result)
		}

		s.logger.Debug("Indexed document via Diagon C++",
			zap.String("doc_id", docID),
			zap.Int("doc_size", len(docJSON)))

		// Also store in memory for fallback operations
		s.documents[docID] = doc
	} else {
		// Stub mode - store in memory
		s.documents[docID] = doc
		s.logger.Debug("Indexed document (stub)", zap.String("doc_id", docID))
	}

	return nil
}

// Search executes a search query with optional filter expression
func (s *Shard) Search(query []byte, filterExpression []byte) (*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.cgoEnabled && s.shardPtr != nil {
		// Prepare C strings
		cQuery := C.CString(string(query))
		defer C.free(unsafe.Pointer(cQuery))

		// Prepare filter expression pointer
		var filterPtr *C.uint8_t
		var filterLen C.size_t

		if len(filterExpression) > 0 {
			filterPtr = (*C.uint8_t)(unsafe.Pointer(&filterExpression[0]))
			filterLen = C.size_t(len(filterExpression))
		}

		// Call C++ API
		resultJSON := C.diagon_search_with_filter(
			s.shardPtr,
			cQuery,
			filterPtr,
			filterLen,
			C.int(0),   // from
			C.int(10),  // size
		)

		if resultJSON == nil {
			return nil, fmt.Errorf("search failed: C++ returned null")
		}
		defer C.free(unsafe.Pointer(resultJSON))

		// Parse JSON result
		var result SearchResult
		if err := json.Unmarshal([]byte(C.GoString(resultJSON)), &result); err != nil {
			return nil, fmt.Errorf("failed to parse search results: %w", err)
		}

		s.logger.Debug("Executed search via Diagon C++",
			zap.Int("query_len", len(query)),
			zap.Int("filter_expr_len", len(filterExpression)),
			zap.Int64("total_hits", result.TotalHits))

		return &result, nil
	}

	// Stub mode - return empty results
	s.logger.Debug("Executed search (stub)",
		zap.Int("query_len", len(query)),
		zap.Int("filter_expr_len", len(filterExpression)))

	result := &SearchResult{
		Took:      5,
		TotalHits: 0,
		MaxScore:  0.0,
		Hits:      make([]*Hit, 0),
	}

	// In stub mode, do a simple scan of in-memory documents
	for docID, doc := range s.documents {
		result.Hits = append(result.Hits, &Hit{
			ID:     docID,
			Score:  1.0,
			Source: doc,
		})
		result.TotalHits++
		if len(result.Hits) >= 10 {
			break
		}
	}

	return result, nil
}

// GetDocument retrieves a document by ID
func (s *Shard) GetDocument(docID string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.cgoEnabled && s.shardPtr != nil {
		// Call C++ API
		cDocID := C.CString(docID)
		defer C.free(unsafe.Pointer(cDocID))

		cDocJSON := C.diagon_get_document(s.shardPtr, cDocID)
		if cDocJSON == nil {
			// Fall back to memory if C++ returns null
			doc, exists := s.documents[docID]
			if !exists {
				return nil, fmt.Errorf("document not found")
			}
			return doc, nil
		}
		defer C.free(unsafe.Pointer(cDocJSON))

		// Parse JSON result
		var doc map[string]interface{}
		if err := json.Unmarshal([]byte(C.GoString(cDocJSON)), &doc); err != nil {
			return nil, fmt.Errorf("failed to parse document: %w", err)
		}

		s.logger.Debug("Retrieved document via Diagon C++", zap.String("doc_id", docID))
		return doc, nil
	}

	// Stub mode or fallback - get from memory
	doc, exists := s.documents[docID]
	if !exists {
		return nil, fmt.Errorf("document not found")
	}

	return doc, nil
}

// DeleteDocument deletes a document
func (s *Shard) DeleteDocument(docID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cgoEnabled && s.shardPtr != nil {
		// Call C++ API
		cDocID := C.CString(docID)
		defer C.free(unsafe.Pointer(cDocID))

		result := C.diagon_delete_document(s.shardPtr, cDocID)
		if result != 0 {
			s.logger.Warn("diagon_delete_document returned non-zero",
				zap.String("doc_id", docID),
				zap.Int("code", int(result)))
		}

		s.logger.Debug("Deleted document via Diagon C++", zap.String("doc_id", docID))
	}

	// Also delete from memory (for fallback operations)
	delete(s.documents, docID)
	s.logger.Debug("Deleted document", zap.String("doc_id", docID))

	return nil
}

// Refresh makes recent changes visible
func (s *Shard) Refresh() error {
	if s.cgoEnabled && s.shardPtr != nil {
		// Call C++ API
		result := C.diagon_refresh(s.shardPtr)
		if result != 0 {
			return fmt.Errorf("diagon_refresh failed with code %d", result)
		}
		s.logger.Debug("Refreshed shard via Diagon C++")
	} else {
		s.logger.Debug("Refreshed shard (stub)")
	}

	return nil
}

// Flush persists changes to disk
func (s *Shard) Flush() error {
	if s.cgoEnabled && s.shardPtr != nil {
		// Call C++ API
		result := C.diagon_flush(s.shardPtr)
		if result != 0 {
			return fmt.Errorf("diagon_flush failed with code %d", result)
		}
		s.logger.Debug("Flushed shard via Diagon C++")
	} else {
		s.logger.Debug("Flushed shard (stub)")
	}

	return nil
}

// Close closes the shard
func (s *Shard) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cgoEnabled && s.shardPtr != nil {
		// Destroy C++ shard
		C.diagon_destroy_shard(s.shardPtr)
		s.shardPtr = nil
		s.logger.Info("Closed Diagon C++ shard")
	} else {
		// Stub mode - clear memory
		s.documents = nil
		s.logger.Info("Closed stub shard")
	}

	return nil
}

// SearchResult represents search results
type SearchResult struct {
	Took         int64                         `json:"took"`
	TotalHits    int64                         `json:"total_hits"`
	MaxScore     float64                       `json:"max_score"`
	Hits         []*Hit                        `json:"hits"`
	Aggregations map[string]AggregationResult  `json:"aggregations,omitempty"`
}

// Hit represents a search hit
type Hit struct {
	ID     string                 `json:"_id"`
	Score  float64                `json:"_score"`
	Source map[string]interface{} `json:"_source"`
}

// AggregationResult represents an aggregation result
type AggregationResult struct {
	Type    string                       `json:"type"`    // "terms" or "stats"
	Buckets []map[string]interface{}     `json:"buckets,omitempty"`
	Count   int64                        `json:"count,omitempty"`
	Min     float64                      `json:"min,omitempty"`
	Max     float64                      `json:"max,omitempty"`
	Avg     float64                      `json:"avg,omitempty"`
	Sum     float64                      `json:"sum,omitempty"`
}

// ShardManager manages shard distribution
type ShardManager struct {
	managerPtr     *C.diagon_shard_manager_t
	nodeID         string
	totalShards    int
	logger         *zap.Logger
}

// NewShardManager creates a new shard manager
func NewShardManager(nodeID string, totalShards int) (*ShardManager, error) {
	cNodeID := C.CString(nodeID)
	defer C.free(unsafe.Pointer(cNodeID))

	managerPtr := C.diagon_create_shard_manager(cNodeID, C.int(totalShards))
	if managerPtr == nil {
		return nil, fmt.Errorf("failed to create shard manager")
	}

	return &ShardManager{
		managerPtr:  managerPtr,
		nodeID:      nodeID,
		totalShards: totalShards,
		logger:      zap.NewNop(),
	}, nil
}

// Close closes the shard manager
func (sm *ShardManager) Close() error {
	if sm.managerPtr != nil {
		C.diagon_destroy_shard_manager(sm.managerPtr)
		sm.managerPtr = nil
	}
	return nil
}

// RegisterShard registers a shard with the manager
func (sm *ShardManager) RegisterShard(shardIndex int, shard *Shard, isPrimary bool) error {
	if sm.managerPtr == nil || shard.shardPtr == nil {
		return fmt.Errorf("invalid shard manager or shard")
	}

	cIsPrimary := C.int(0)
	if isPrimary {
		cIsPrimary = C.int(1)
	}

	result := C.diagon_register_shard(sm.managerPtr, C.int(shardIndex), shard.shardPtr, cIsPrimary)
	if result != 0 {
		return fmt.Errorf("failed to register shard %d", shardIndex)
	}

	return nil
}

// GetShardForDocument returns the shard index for a document ID
func (sm *ShardManager) GetShardForDocument(docID string) int {
	if sm.managerPtr == nil {
		return -1
	}

	cDocID := C.CString(docID)
	defer C.free(unsafe.Pointer(cDocID))

	return int(C.diagon_get_shard_for_document(sm.managerPtr, cDocID))
}

// DistributedCoordinator coordinates distributed search
type DistributedCoordinator struct {
	coordinatorPtr *C.diagon_distributed_coordinator_t
	shardManager   *ShardManager
	logger         *zap.Logger
}

// NewDistributedCoordinator creates a new distributed coordinator
func NewDistributedCoordinator(shardManager *ShardManager) (*DistributedCoordinator, error) {
	if shardManager == nil || shardManager.managerPtr == nil {
		return nil, fmt.Errorf("invalid shard manager")
	}

	coordinatorPtr := C.diagon_create_coordinator(shardManager.managerPtr)
	if coordinatorPtr == nil {
		return nil, fmt.Errorf("failed to create distributed coordinator")
	}

	return &DistributedCoordinator{
		coordinatorPtr: coordinatorPtr,
		shardManager:   shardManager,
		logger:         zap.NewNop(),
	}, nil
}

// Close closes the coordinator
func (dc *DistributedCoordinator) Close() error {
	if dc.coordinatorPtr != nil {
		C.diagon_destroy_coordinator(dc.coordinatorPtr)
		dc.coordinatorPtr = nil
	}
	return nil
}

// Search executes a distributed search with default options
func (dc *DistributedCoordinator) Search(query []byte, filterExpression []byte) (*SearchResult, error) {
	return dc.SearchWithOptions(query, filterExpression, 0, 10)
}

// SearchWithOptions executes a distributed search with custom pagination
func (dc *DistributedCoordinator) SearchWithOptions(query []byte, filterExpression []byte, from int, size int) (*SearchResult, error) {
	if dc.coordinatorPtr == nil {
		return nil, fmt.Errorf("coordinator not initialized")
	}

	cQuery := C.CString(string(query))
	defer C.free(unsafe.Pointer(cQuery))

	var filterPtr *C.uint8_t
	var filterLen C.size_t

	if len(filterExpression) > 0 {
		filterPtr = (*C.uint8_t)(unsafe.Pointer(&filterExpression[0]))
		filterLen = C.size_t(len(filterExpression))
	}

	resultJSON := C.diagon_distributed_search(
		dc.coordinatorPtr,
		cQuery,
		filterPtr,
		filterLen,
		C.int(from),
		C.int(size),
	)

	if resultJSON == nil {
		return nil, fmt.Errorf("distributed search failed")
	}
	defer C.free(unsafe.Pointer(resultJSON))

	var result SearchResult
	if err := json.Unmarshal([]byte(C.GoString(resultJSON)), &result); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	return &result, nil
}
