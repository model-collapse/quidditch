package data

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/quidditch/quidditch/pkg/data/diagon"
	"github.com/quidditch/quidditch/pkg/wasm"
	"go.uber.org/zap"
)

// ShardManager manages all shards on a data node
type ShardManager struct {
	cfg       *config.DataNodeConfig
	logger    *zap.Logger
	diagon    *diagon.DiagonBridge
	udfFilter *UDFFilter
	shards    map[string]*Shard // key: "index:shardID"
	mu        sync.RWMutex
}

// NewShardManager creates a new shard manager
func NewShardManager(cfg *config.DataNodeConfig, logger *zap.Logger, diagon *diagon.DiagonBridge, udfRegistry *wasm.UDFRegistry) *ShardManager {
	// Create UDF filter
	udfFilter := NewUDFFilter(udfRegistry, logger)

	return &ShardManager{
		cfg:       cfg,
		logger:    logger,
		diagon:    diagon,
		udfFilter: udfFilter,
		shards:    make(map[string]*Shard),
	}
}

// Start starts the shard manager
func (sm *ShardManager) Start(ctx context.Context) error {
	sm.logger.Info("Starting shard manager")

	// Load existing shards from disk
	if err := sm.loadShards(); err != nil {
		return fmt.Errorf("failed to load shards: %w", err)
	}

	return nil
}

// Stop stops the shard manager
func (sm *ShardManager) Stop(ctx context.Context) error {
	sm.logger.Info("Stopping shard manager")

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Close all shards
	for key, shard := range sm.shards {
		sm.logger.Info("Closing shard", zap.String("key", key))
		if err := shard.Close(); err != nil {
			sm.logger.Error("Error closing shard", zap.String("key", key), zap.Error(err))
		}
	}

	return nil
}

// CreateShard creates a new shard
func (sm *ShardManager) CreateShard(ctx context.Context, indexName string, shardID int32, isPrimary bool) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	key := shardKey(indexName, shardID)

	// Check if shard already exists
	if _, exists := sm.shards[key]; exists {
		return fmt.Errorf("shard %s already exists", key)
	}

	// Check max shards limit
	if len(sm.shards) >= sm.cfg.MaxShards {
		return fmt.Errorf("max shards limit reached (%d)", sm.cfg.MaxShards)
	}

	// Create shard directory
	shardPath := filepath.Join(sm.cfg.DataDir, indexName, fmt.Sprintf("shard_%d", shardID))
	if err := os.MkdirAll(shardPath, 0755); err != nil {
		return fmt.Errorf("failed to create shard directory: %w", err)
	}

	// Create shard using Diagon
	diagonShard, err := sm.diagon.CreateShard(shardPath)
	if err != nil {
		return fmt.Errorf("failed to create Diagon shard: %w", err)
	}

	// Create shard wrapper with default analyzer settings
	shard := &Shard{
		IndexName:        indexName,
		ShardID:          shardID,
		IsPrimary:        isPrimary,
		Path:             shardPath,
		State:            ShardStateInitializing,
		DiagonShard:      diagonShard,
		udfFilter:        sm.udfFilter,
		DocsCount:        0,
		SizeBytes:        0,
		logger:           sm.logger.With(zap.String("shard", key)),
		analyzerSettings: DefaultAnalyzerSettings(), // Use default analyzer settings
		analyzerCache:    NewAnalyzerCache(),        // Create analyzer cache
	}

	sm.shards[key] = shard

	// Mark as started
	shard.State = ShardStateStarted

	sm.logger.Info("Created shard",
		zap.String("index", indexName),
		zap.Int32("shard_id", shardID),
		zap.Bool("is_primary", isPrimary),
		zap.String("path", shardPath))

	return nil
}

// DeleteShard deletes a shard
func (sm *ShardManager) DeleteShard(ctx context.Context, indexName string, shardID int32) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	key := shardKey(indexName, shardID)

	shard, exists := sm.shards[key]
	if !exists {
		return fmt.Errorf("shard %s not found", key)
	}

	// Close the shard
	if err := shard.Close(); err != nil {
		return fmt.Errorf("failed to close shard: %w", err)
	}

	// Remove from map
	delete(sm.shards, key)

	sm.logger.Info("Deleted shard",
		zap.String("index", indexName),
		zap.Int32("shard_id", shardID))

	return nil
}

// GetShard returns a shard
func (sm *ShardManager) GetShard(indexName string, shardID int32) (*Shard, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	key := shardKey(indexName, shardID)

	shard, exists := sm.shards[key]
	if !exists {
		return nil, fmt.Errorf("shard %s not found", key)
	}

	if shard.State != ShardStateStarted {
		return nil, fmt.Errorf("shard %s is not ready (state: %s)", key, shard.State)
	}

	return shard, nil
}

// Count returns the number of active shards
func (sm *ShardManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.shards)
}

// List returns all shards
func (sm *ShardManager) List() []*Shard {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	shards := make([]*Shard, 0, len(sm.shards))
	for _, shard := range sm.shards {
		shards = append(shards, shard)
	}

	return shards
}

// loadShards loads existing shards from disk
func (sm *ShardManager) loadShards() error {
	sm.logger.Info("Loading shards from disk", zap.String("data_dir", sm.cfg.DataDir))

	// Check if data directory exists
	if _, err := os.Stat(sm.cfg.DataDir); os.IsNotExist(err) {
		sm.logger.Info("Data directory does not exist yet, no shards to load")
		return nil
	}

	// Scan data directory for index directories
	indexEntries, err := os.ReadDir(sm.cfg.DataDir)
	if err != nil {
		return fmt.Errorf("failed to read data directory: %w", err)
	}

	shardsLoaded := 0
	for _, indexEntry := range indexEntries {
		if !indexEntry.IsDir() {
			continue
		}

		indexName := indexEntry.Name()
		indexPath := filepath.Join(sm.cfg.DataDir, indexName)

		// Scan for shard directories (format: shard_0, shard_1, etc.)
		shardEntries, err := os.ReadDir(indexPath)
		if err != nil {
			sm.logger.Warn("Failed to read index directory",
				zap.String("index", indexName),
				zap.Error(err))
			continue
		}

		for _, shardEntry := range shardEntries {
			if !shardEntry.IsDir() {
				continue
			}

			shardDirName := shardEntry.Name()
			if !strings.HasPrefix(shardDirName, "shard_") {
				continue
			}

			// Extract shard ID from directory name (e.g., "shard_0" -> 0)
			shardIDStr := strings.TrimPrefix(shardDirName, "shard_")
			shardID, err := strconv.ParseInt(shardIDStr, 10, 32)
			if err != nil {
				sm.logger.Warn("Invalid shard directory name",
					zap.String("name", shardDirName),
					zap.Error(err))
				continue
			}

			// Load the shard (CreateShard uses CREATE_OR_APPEND mode, so it will open existing)
			shardPath := filepath.Join(indexPath, shardDirName)
			key := shardKey(indexName, int32(shardID))

			// Check if shard is already loaded (shouldn't happen, but be safe)
			sm.mu.RLock()
			_, exists := sm.shards[key]
			sm.mu.RUnlock()
			if exists {
				sm.logger.Debug("Shard already loaded, skipping",
					zap.String("index", indexName),
					zap.Int64("shard_id", shardID))
				continue
			}

			// Create/open the Diagon shard
			diagonShard, err := sm.diagon.CreateShard(shardPath)
			if err != nil {
				sm.logger.Error("Failed to load shard from disk",
					zap.String("index", indexName),
					zap.Int64("shard_id", shardID),
					zap.String("path", shardPath),
					zap.Error(err))
				continue
			}

			// Create shard wrapper
			shard := &Shard{
				IndexName:        indexName,
				ShardID:          int32(shardID),
				IsPrimary:        false, // Will be set by master during registration
				Path:             shardPath,
				State:            ShardStateStarted,
				DiagonShard:      diagonShard,
				udfFilter:        sm.udfFilter,
				DocsCount:        0, // TODO: Could load actual count from Diagon
				SizeBytes:        0, // TODO: Could calculate from disk
				logger:           sm.logger.With(zap.String("shard", key)),
				analyzerSettings: DefaultAnalyzerSettings(), // Use default analyzer settings
				analyzerCache:    NewAnalyzerCache(),        // Create analyzer cache
			}

			sm.mu.Lock()
			sm.shards[key] = shard
			sm.mu.Unlock()

			shardsLoaded++
			sm.logger.Info("Loaded shard from disk",
				zap.String("index", indexName),
				zap.Int32("shard_id", int32(shardID)),
				zap.String("path", shardPath))
		}
	}

	sm.logger.Info("Shard loading complete",
		zap.Int("shards_loaded", shardsLoaded))

	return nil
}

// shardKey generates a unique key for a shard
func shardKey(indexName string, shardID int32) string {
	return fmt.Sprintf("%s:%d", indexName, shardID)
}

// Shard represents a single shard on a data node
type Shard struct {
	IndexName        string
	ShardID          int32
	IsPrimary        bool
	Path             string
	State            ShardState
	DiagonShard      *diagon.Shard
	udfFilter        *UDFFilter
	DocsCount        int64
	SizeBytes        int64
	logger           *zap.Logger
	mu               sync.RWMutex
	analyzerSettings *AnalyzerSettings // Analyzer configuration for this shard
	analyzerCache    *AnalyzerCache    // Cached analyzer instances
}

// ShardState represents the state of a shard
type ShardState string

const (
	ShardStateInitializing ShardState = "initializing"
	ShardStateStarted      ShardState = "started"
	ShardStateRelocating   ShardState = "relocating"
	ShardStateClosed       ShardState = "closed"
)

// SetAnalyzerSettings updates the analyzer settings for this shard
func (s *Shard) SetAnalyzerSettings(settings *AnalyzerSettings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.analyzerSettings = settings
}

// GetAnalyzerSettings returns the current analyzer settings
func (s *Shard) GetAnalyzerSettings() *AnalyzerSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.analyzerSettings
}

// AnalyzeText analyzes text using the configured analyzer for a field
func (s *Shard) AnalyzeText(fieldName, text string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.analyzerSettings == nil || s.analyzerCache == nil {
		return nil, fmt.Errorf("analyzer settings not initialized")
	}

	return AnalyzeField(s.analyzerCache, s.analyzerSettings, fieldName, text)
}

// IndexDocument indexes a document in the shard
func (s *Shard) IndexDocument(ctx context.Context, docID string, doc map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("==> Shard.IndexDocument ENTRY",
		zap.String("index", s.IndexName),
		zap.Int32("shard_id", s.ShardID),
		zap.String("doc_id", docID))

	if s.State != ShardStateStarted {
		return fmt.Errorf("shard is not ready")
	}

	// Index document using Diagon
	s.logger.Info("Calling DiagonShard.IndexDocument", zap.String("doc_id", docID))
	if err := s.DiagonShard.IndexDocument(docID, doc); err != nil {
		s.logger.Error("DiagonShard.IndexDocument FAILED", zap.Error(err))
		return fmt.Errorf("failed to index document: %w", err)
	}

	s.logger.Info("DiagonShard.IndexDocument SUCCESS", zap.String("doc_id", docID))

	// CRITICAL FIX: Commit the document to disk so it's searchable
	s.logger.Info("Calling DiagonShard.Commit to flush to disk", zap.String("doc_id", docID))
	if err := s.DiagonShard.Commit(); err != nil {
		s.logger.Error("DiagonShard.Commit FAILED", zap.Error(err))
		return fmt.Errorf("failed to commit document: %w", err)
	}

	s.logger.Info("DiagonShard.Commit SUCCESS - document now on disk", zap.String("doc_id", docID))

	// CRITICAL FIX: Refresh the reader so searches can see the new document
	s.logger.Info("Calling DiagonShard.Refresh to reopen reader", zap.String("doc_id", docID))
	if err := s.DiagonShard.Refresh(); err != nil {
		s.logger.Error("DiagonShard.Refresh FAILED", zap.Error(err))
		return fmt.Errorf("failed to refresh reader: %w", err)
	}

	s.logger.Info("DiagonShard.Refresh SUCCESS - document now searchable", zap.String("doc_id", docID))

	s.DocsCount++

	s.logger.Info("Indexed document successfully",
		zap.String("doc_id", docID),
		zap.Int64("docs_count", s.DocsCount))

	return nil
}

// Search executes a search query on the shard
func (s *Shard) Search(ctx context.Context, query []byte) (*diagon.SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.State != ShardStateStarted {
		return nil, fmt.Errorf("shard is not ready")
	}

	// Execute search using Diagon (pass empty filterExpression)
	result, err := s.DiagonShard.Search(query, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	s.logger.Debug("Executed search",
		zap.Int64("total_hits", result.TotalHits),
		zap.Int("num_hits", len(result.Hits)))

	// Apply WASM UDF filtering if query contains UDF
	if s.udfFilter != nil && s.udfFilter.HasWasmUDFQuery(query) {
		s.logger.Debug("Applying WASM UDF filter")

		filteredResult, err := s.udfFilter.FilterResults(ctx, query, result)
		if err != nil {
			// Log error but return original results
			s.logger.Error("Failed to apply UDF filter",
				zap.Error(err),
				zap.String("index", s.IndexName),
				zap.Int32("shard_id", s.ShardID))
			return result, nil
		}

		return filteredResult, nil
	}

	return result, nil
}

// GetDocument retrieves a document by ID
func (s *Shard) GetDocument(ctx context.Context, docID string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.State != ShardStateStarted {
		return nil, fmt.Errorf("shard is not ready")
	}

	// Get document using Diagon
	doc, err := s.DiagonShard.GetDocument(docID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	return doc, nil
}

// DeleteDocument deletes a document by ID
func (s *Shard) DeleteDocument(ctx context.Context, docID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State != ShardStateStarted {
		return fmt.Errorf("shard is not ready")
	}

	// Delete document using Diagon
	if err := s.DiagonShard.DeleteDocument(docID); err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	s.DocsCount--

	s.logger.Debug("Deleted document", zap.String("doc_id", docID))

	return nil
}

// Refresh refreshes the shard (makes recent changes visible)
func (s *Shard) Refresh() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State != ShardStateStarted {
		return fmt.Errorf("shard is not ready")
	}

	// Refresh using Diagon
	if err := s.DiagonShard.Refresh(); err != nil {
		return fmt.Errorf("failed to refresh shard: %w", err)
	}

	s.logger.Debug("Refreshed shard")

	return nil
}

// Flush flushes the shard (persists translog to disk)
func (s *Shard) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State != ShardStateStarted {
		return fmt.Errorf("shard is not ready")
	}

	// Flush using Diagon
	if err := s.DiagonShard.Flush(); err != nil {
		return fmt.Errorf("failed to flush shard: %w", err)
	}

	s.logger.Debug("Flushed shard")

	return nil
}

// Close closes the shard
func (s *Shard) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == ShardStateClosed {
		return nil
	}

	// Close Diagon shard
	if err := s.DiagonShard.Close(); err != nil {
		return fmt.Errorf("failed to close Diagon shard: %w", err)
	}

	// Close analyzer cache
	if s.analyzerCache != nil {
		s.analyzerCache.Close()
	}

	s.State = ShardStateClosed

	s.logger.Info("Closed shard")

	return nil
}

// Stats returns shard statistics
func (s *Shard) Stats() *ShardStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &ShardStats{
		IndexName: s.IndexName,
		ShardID:   s.ShardID,
		IsPrimary: s.IsPrimary,
		State:     s.State,
		DocsCount: s.DocsCount,
		SizeBytes: s.SizeBytes,
	}
}

// ShardStats represents shard statistics
type ShardStats struct {
	IndexName string
	ShardID   int32
	IsPrimary bool
	State     ShardState
	DocsCount int64
	SizeBytes int64
}
