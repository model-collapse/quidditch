package wasm

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// DocumentContext provides document field access for WASM functions
// This is the Go-side representation of a document that WASM can query
type DocumentContext struct {
	// Document data as map for easy field access
	data map[string]interface{}

	// Document metadata
	documentID string
	score      float64

	// Memory management
	mu sync.RWMutex

	// For debugging
	fieldAccesses int
}

// NewDocumentContext creates a context from JSON document data
func NewDocumentContext(documentID string, score float64, jsonData []byte) (*DocumentContext, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal document: %w", err)
	}

	return &DocumentContext{
		data:       data,
		documentID: documentID,
		score:      score,
	}, nil
}

// NewDocumentContextFromMap creates a context from a map
func NewDocumentContextFromMap(documentID string, score float64, data map[string]interface{}) *DocumentContext {
	return &DocumentContext{
		data:       data,
		documentID: documentID,
		score:      score,
	}
}

// GetFieldString retrieves a string field value
func (dc *DocumentContext) GetFieldString(fieldPath string) (string, bool) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	dc.fieldAccesses++

	value, exists := dc.getNestedField(fieldPath)
	if !exists {
		return "", false
	}

	// Try to convert to string
	switch v := value.(type) {
	case string:
		return v, true
	case fmt.Stringer:
		return v.String(), true
	default:
		// Try JSON marshaling as fallback
		if bytes, err := json.Marshal(v); err == nil {
			return string(bytes), true
		}
		return "", false
	}
}

// GetFieldInt64 retrieves an int64 field value
func (dc *DocumentContext) GetFieldInt64(fieldPath string) (int64, bool) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	dc.fieldAccesses++

	value, exists := dc.getNestedField(fieldPath)
	if !exists {
		return 0, false
	}

	// Try to convert to int64
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	default:
		return 0, false
	}
}

// GetFieldFloat64 retrieves a float64 field value
func (dc *DocumentContext) GetFieldFloat64(fieldPath string) (float64, bool) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	dc.fieldAccesses++

	value, exists := dc.getNestedField(fieldPath)
	if !exists {
		return 0, false
	}

	// Try to convert to float64
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

// GetFieldBool retrieves a bool field value
func (dc *DocumentContext) GetFieldBool(fieldPath string) (bool, bool) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	dc.fieldAccesses++

	value, exists := dc.getNestedField(fieldPath)
	if !exists {
		return false, false
	}

	// Try to convert to bool
	switch v := value.(type) {
	case bool:
		return v, true
	default:
		return false, false
	}
}

// HasField checks if a field exists
func (dc *DocumentContext) HasField(fieldPath string) bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	_, exists := dc.getNestedField(fieldPath)
	return exists
}

// GetDocumentID returns the document ID
func (dc *DocumentContext) GetDocumentID() string {
	return dc.documentID
}

// GetScore returns the document score
func (dc *DocumentContext) GetScore() float64 {
	return dc.score
}

// GetFieldAccessCount returns the number of field accesses (for debugging)
func (dc *DocumentContext) GetFieldAccessCount() int {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.fieldAccesses
}

// getNestedField retrieves a value from a nested field path
// Supports dot notation: "metadata.category", "tags[0]", etc.
func (dc *DocumentContext) getNestedField(fieldPath string) (interface{}, bool) {
	// Split path by dots
	components := strings.Split(fieldPath, ".")

	var current interface{} = dc.data

	for _, component := range components {
		// Handle array indexing (e.g., "tags[0]")
		if strings.Contains(component, "[") && strings.Contains(component, "]") {
			// Parse array access
			fieldName, index, ok := parseArrayAccess(component)
			if !ok {
				return nil, false
			}

			// Get the array
			currentMap, ok := current.(map[string]interface{})
			if !ok {
				return nil, false
			}

			array, exists := currentMap[fieldName]
			if !exists {
				return nil, false
			}

			// Access array element
			arraySlice, ok := array.([]interface{})
			if !ok {
				return nil, false
			}

			if index < 0 || index >= len(arraySlice) {
				return nil, false
			}

			current = arraySlice[index]
		} else {
			// Simple field access
			currentMap, ok := current.(map[string]interface{})
			if !ok {
				return nil, false
			}

			value, exists := currentMap[component]
			if !exists {
				return nil, false
			}

			current = value
		}
	}

	return current, true
}

// parseArrayAccess parses "field[index]" into field name and index
func parseArrayAccess(component string) (string, int, bool) {
	openBracket := strings.Index(component, "[")
	closeBracket := strings.Index(component, "]")

	if openBracket == -1 || closeBracket == -1 || closeBracket < openBracket {
		return "", 0, false
	}

	fieldName := component[:openBracket]
	indexStr := component[openBracket+1 : closeBracket]

	var index int
	if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
		return "", 0, false
	}

	return fieldName, index, true
}

// GetRawData returns the raw document data (for testing)
func (dc *DocumentContext) GetRawData() map[string]interface{} {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.data
}

// ContextPool manages a pool of reusable document contexts
type ContextPool struct {
	pool chan *DocumentContext
	size int
	mu   sync.RWMutex
}

// NewContextPool creates a pool of document contexts
func NewContextPool(poolSize int) *ContextPool {
	if poolSize <= 0 {
		poolSize = 100
	}

	return &ContextPool{
		pool: make(chan *DocumentContext, poolSize),
		size: poolSize,
	}
}

// Get retrieves a context from the pool or creates a new one
func (cp *ContextPool) Get(documentID string, score float64, jsonData []byte) (*DocumentContext, error) {
	select {
	case ctx := <-cp.pool:
		// Reuse existing context - reset it with new data
		var data map[string]interface{}
		if err := json.Unmarshal(jsonData, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal document: %w", err)
		}
		ctx.data = data
		ctx.documentID = documentID
		ctx.score = score
		ctx.fieldAccesses = 0
		return ctx, nil
	default:
		// Pool empty, create new context
		return NewDocumentContext(documentID, score, jsonData)
	}
}

// Put returns a context to the pool
func (cp *ContextPool) Put(ctx *DocumentContext) {
	if ctx == nil {
		return
	}

	select {
	case cp.pool <- ctx:
		// Context returned to pool
	default:
		// Pool full, let it be garbage collected
	}
}

// Close closes the context pool
func (cp *ContextPool) Close() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	close(cp.pool)

	// Drain pool
	for range cp.pool {
		// Just drain, contexts will be GC'd
	}
}
