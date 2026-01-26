package bulk

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// OperationType represents the type of bulk operation
type OperationType string

const (
	OperationIndex  OperationType = "index"
	OperationCreate OperationType = "create"
	OperationUpdate OperationType = "update"
	OperationDelete OperationType = "delete"
)

// BulkOperation represents a single operation in a bulk request
type BulkOperation struct {
	Type      OperationType
	Index     string
	ID        string
	Document  map[string]interface{} // For index, create, update
	UpdateDoc map[string]interface{} // For update operations (the "doc" field)
}

// BulkRequest represents a parsed bulk request
type BulkRequest struct {
	Operations []*BulkOperation
}

// BulkItemResult represents the result of a single bulk operation
type BulkItemResult struct {
	Index   string                 `json:"_index"`
	ID      string                 `json:"_id"`
	Version int64                  `json:"_version,omitempty"`
	Result  string                 `json:"result,omitempty"`
	Status  int                    `json:"status"`
	Error   *BulkItemError         `json:"error,omitempty"`
	Shards  *BulkItemShards        `json:"_shards,omitempty"`
}

// BulkItemError represents an error for a bulk operation
type BulkItemError struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// BulkItemShards represents shard information for a bulk operation
type BulkItemShards struct {
	Total      int32 `json:"total"`
	Successful int32 `json:"successful"`
	Failed     int32 `json:"failed"`
}

// BulkResponse represents the response to a bulk request
type BulkResponse struct {
	Took   int64                         `json:"took"`
	Errors bool                          `json:"errors"`
	Items  []map[string]*BulkItemResult `json:"items"`
}

// ParseBulkRequest parses a bulk request in NDJSON format
// Format:
// { "index": { "_index": "test", "_id": "1" } }
// { "field": "value" }
// { "delete": { "_index": "test", "_id": "2" } }
func ParseBulkRequest(body []byte) (*BulkRequest, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty bulk request")
	}

	req := &BulkRequest{
		Operations: make([]*BulkOperation, 0),
	}

	scanner := bufio.NewScanner(bytes.NewReader(body))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		actionLine := scanner.Bytes()

		// Skip empty lines
		if len(bytes.TrimSpace(actionLine)) == 0 {
			continue
		}

		// Parse action line
		var actionMap map[string]interface{}
		if err := json.Unmarshal(actionLine, &actionMap); err != nil {
			return nil, fmt.Errorf("failed to parse action line %d: %w", lineNum, err)
		}

		// Determine operation type
		var opType OperationType
		var meta map[string]interface{}

		if indexMeta, ok := actionMap["index"]; ok {
			opType = OperationIndex
			meta = indexMeta.(map[string]interface{})
		} else if createMeta, ok := actionMap["create"]; ok {
			opType = OperationCreate
			meta = createMeta.(map[string]interface{})
		} else if updateMeta, ok := actionMap["update"]; ok {
			opType = OperationUpdate
			meta = updateMeta.(map[string]interface{})
		} else if deleteMeta, ok := actionMap["delete"]; ok {
			opType = OperationDelete
			meta = deleteMeta.(map[string]interface{})
		} else {
			return nil, fmt.Errorf("unknown bulk operation on line %d", lineNum)
		}

		// Extract index and ID
		index, _ := meta["_index"].(string)
		id, _ := meta["_id"].(string)

		if index == "" {
			return nil, fmt.Errorf("missing _index on line %d", lineNum)
		}

		op := &BulkOperation{
			Type:  opType,
			Index: index,
			ID:    id,
		}

		// For operations that require a document body, read the next line
		if opType == OperationIndex || opType == OperationCreate || opType == OperationUpdate {
			if !scanner.Scan() {
				return nil, fmt.Errorf("missing document body for %s operation on line %d", opType, lineNum)
			}

			lineNum++
			docLine := scanner.Bytes()

			if len(bytes.TrimSpace(docLine)) == 0 {
				return nil, fmt.Errorf("empty document body for %s operation on line %d", opType, lineNum-1)
			}

			var document map[string]interface{}
			if err := json.Unmarshal(docLine, &document); err != nil {
				return nil, fmt.Errorf("failed to parse document on line %d: %w", lineNum, err)
			}

			if opType == OperationUpdate {
				// For update operations, extract the "doc" field
				if doc, ok := document["doc"].(map[string]interface{}); ok {
					op.UpdateDoc = doc
				} else {
					op.UpdateDoc = document
				}
			} else {
				op.Document = document
			}
		}

		req.Operations = append(req.Operations, op)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading bulk request: %w", err)
	}

	if len(req.Operations) == 0 {
		return nil, fmt.Errorf("no operations in bulk request")
	}

	return req, nil
}

// NewBulkResponse creates a new bulk response
func NewBulkResponse() *BulkResponse {
	return &BulkResponse{
		Items: make([]map[string]*BulkItemResult, 0),
	}
}

// AddItem adds an item result to the bulk response
func (br *BulkResponse) AddItem(opType OperationType, result *BulkItemResult) {
	item := map[string]*BulkItemResult{
		string(opType): result,
	}
	br.Items = append(br.Items, item)

	// Update errors flag
	if result.Error != nil {
		br.Errors = true
	}
}

// ParseBulkRequestStream parses a bulk request from an io.Reader
func ParseBulkRequestStream(reader io.Reader) (*BulkRequest, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	return ParseBulkRequest(body)
}
