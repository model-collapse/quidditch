package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/quidditch/quidditch/pkg/coordination/parser"
	"github.com/quidditch/quidditch/pkg/data/diagon"
	"github.com/quidditch/quidditch/pkg/wasm"
	"go.uber.org/zap"
)

// UDFFilter handles WASM UDF query filtering
type UDFFilter struct {
	registry *wasm.UDFRegistry
	parser   *parser.QueryParser
	logger   *zap.Logger
}

// NewUDFFilter creates a new UDF filter
func NewUDFFilter(registry *wasm.UDFRegistry, logger *zap.Logger) *UDFFilter {
	return &UDFFilter{
		registry: registry,
		parser:   parser.NewQueryParser(),
		logger:   logger,
	}
}

// FilterResults filters search results using WASM UDF queries
func (uf *UDFFilter) FilterResults(
	ctx context.Context,
	queryJSON []byte,
	results *diagon.SearchResult,
) (*diagon.SearchResult, error) {
	// Parse query JSON to detect UDF queries
	udfQuery, err := uf.extractWasmUDFQuery(queryJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	// If no UDF query found, return results unchanged
	if udfQuery == nil {
		return results, nil
	}

	uf.logger.Debug("Filtering results with WASM UDF",
		zap.String("udf_name", udfQuery.Name),
		zap.String("udf_version", udfQuery.Version),
		zap.Int("total_hits", len(results.Hits)))

	// Filter hits using UDF
	filteredHits, err := uf.filterHits(ctx, udfQuery, results.Hits)
	if err != nil {
		return nil, fmt.Errorf("failed to filter hits: %w", err)
	}

	uf.logger.Debug("UDF filtering complete",
		zap.String("udf_name", udfQuery.Name),
		zap.Int("before", len(results.Hits)),
		zap.Int("after", len(filteredHits)))

	// Return filtered results
	return &diagon.SearchResult{
		Took:      results.Took,
		TotalHits: int64(len(filteredHits)),
		MaxScore:  results.MaxScore,
		Hits:      filteredHits,
	}, nil
}

// extractWasmUDFQuery extracts WasmUDFQuery from query JSON
// Returns nil if no UDF query is found
func (uf *UDFFilter) extractWasmUDFQuery(queryJSON []byte) (*parser.WasmUDFQuery, error) {
	// Parse query JSON
	var queryMap map[string]interface{}
	if err := json.Unmarshal(queryJSON, &queryMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal query: %w", err)
	}

	// Parse query using parser
	query, err := uf.parser.ParseQuery(queryMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	// Check if it's a WasmUDFQuery
	if wasmQuery, ok := query.(*parser.WasmUDFQuery); ok {
		return wasmQuery, nil
	}

	// Check if it's a BoolQuery with WasmUDFQuery in filter
	if boolQuery, ok := query.(*parser.BoolQuery); ok {
		// Check filter clauses
		for _, filterQuery := range boolQuery.Filter {
			if wasmQuery, ok := filterQuery.(*parser.WasmUDFQuery); ok {
				return wasmQuery, nil
			}
		}

		// Check must clauses
		for _, mustQuery := range boolQuery.Must {
			if wasmQuery, ok := mustQuery.(*parser.WasmUDFQuery); ok {
				return wasmQuery, nil
			}
		}
	}

	// No UDF query found
	return nil, nil
}

// filterHits filters search hits using UDF
func (uf *UDFFilter) filterHits(
	ctx context.Context,
	udfQuery *parser.WasmUDFQuery,
	hits []*diagon.Hit,
) ([]*diagon.Hit, error) {
	filteredHits := make([]*diagon.Hit, 0, len(hits))

	// Convert parameters to wasm.Value map
	params, err := uf.convertParameters(udfQuery.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert parameters: %w", err)
	}

	// Process each hit
	for _, hit := range hits {
		// Create document context from map
		docCtx := wasm.NewDocumentContextFromMap(hit.ID, hit.Score, hit.Source)

		// Call UDF
		results, err := uf.registry.Call(
			ctx,
			udfQuery.Name,
			udfQuery.Version,
			docCtx,
			params,
		)

		if err != nil {
			// Log error but continue processing other documents
			uf.logger.Warn("UDF execution failed for document",
				zap.String("doc_id", hit.ID),
				zap.String("udf_name", udfQuery.Name),
				zap.Error(err))
			continue
		}

		// Check if UDF returned true (include document)
		if len(results) > 0 {
			var include bool

			// Handle both bool and i32 return types
			// i32: 0 = false, non-zero = true
			switch results[0].Type {
			case wasm.ValueTypeBool:
				var err error
				include, err = results[0].AsBool()
				if err != nil {
					uf.logger.Warn("Failed to convert bool result",
						zap.String("doc_id", hit.ID),
						zap.String("udf_name", udfQuery.Name),
						zap.Error(err))
					continue
				}
			case wasm.ValueTypeI32:
				i32Val, err := results[0].AsInt32()
				if err != nil {
					uf.logger.Warn("Failed to convert i32 result",
						zap.String("doc_id", hit.ID),
						zap.String("udf_name", udfQuery.Name),
						zap.Error(err))
					continue
				}
				include = (i32Val != 0)
			default:
				uf.logger.Warn("UDF returned unsupported type",
					zap.String("doc_id", hit.ID),
					zap.String("udf_name", udfQuery.Name),
					zap.Any("return_type", results[0].Type))
				continue
			}

			if include {
				filteredHits = append(filteredHits, hit)
			}
		}
	}

	return filteredHits, nil
}

// convertParameters converts query parameters to WASM values
func (uf *UDFFilter) convertParameters(params map[string]interface{}) (map[string]wasm.Value, error) {
	values := make(map[string]wasm.Value, len(params))

	// Convert each parameter
	for key, val := range params {
		wasmVal, err := uf.convertValue(val)
		if err != nil {
			return nil, fmt.Errorf("failed to convert parameter %s: %w", key, err)
		}
		values[key] = wasmVal
	}

	return values, nil
}

// convertValue converts a Go value to a WASM value
func (uf *UDFFilter) convertValue(val interface{}) (wasm.Value, error) {
	switch v := val.(type) {
	case bool:
		return wasm.NewBoolValue(v), nil
	case int:
		return wasm.NewI64Value(int64(v)), nil
	case int32:
		return wasm.NewI64Value(int64(v)), nil
	case int64:
		return wasm.NewI64Value(v), nil
	case float32:
		return wasm.NewF64Value(float64(v)), nil
	case float64:
		return wasm.NewF64Value(v), nil
	case string:
		return wasm.NewStringValue(v), nil
	default:
		return wasm.Value{}, fmt.Errorf("unsupported parameter type: %T", val)
	}
}

// HasWasmUDFQuery checks if query contains a WASM UDF query
func (uf *UDFFilter) HasWasmUDFQuery(queryJSON []byte) bool {
	query, err := uf.extractWasmUDFQuery(queryJSON)
	if err != nil {
		uf.logger.Debug("Failed to parse query", zap.Error(err))
		return false
	}
	return query != nil
}
