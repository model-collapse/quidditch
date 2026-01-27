package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/quidditch/quidditch/pkg/coordination/executor"
	"go.uber.org/zap"
)

// QueryExecutorInterface defines the interface for query execution
type QueryExecutorInterface interface {
	ExecuteSearch(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error)
}

// ExecutionContext provides the execution environment for physical plans
type ExecutionContext struct {
	QueryExecutor QueryExecutorInterface
	Logger        *zap.Logger
}

// contextKey is the type for context keys to avoid collisions
type contextKey string

const executionContextKey contextKey = "execution_context"

// WithExecutionContext adds an execution context to the Go context
func WithExecutionContext(ctx context.Context, execCtx *ExecutionContext) context.Context {
	return context.WithValue(ctx, executionContextKey, execCtx)
}

// GetExecutionContext retrieves the execution context from the Go context
func GetExecutionContext(ctx context.Context) (*ExecutionContext, error) {
	execCtx, ok := ctx.Value(executionContextKey).(*ExecutionContext)
	if !ok || execCtx == nil {
		return nil, fmt.Errorf("execution context not found in context")
	}
	return execCtx, nil
}

// convertExecutorResultToExecution converts executor.SearchResult to ExecutionResult
func convertExecutorResultToExecution(result *executor.SearchResult) *ExecutionResult {
	execResult := &ExecutionResult{
		Rows:         make([]map[string]interface{}, len(result.Hits)),
		TotalHits:    result.TotalHits,
		MaxScore:     result.MaxScore,
		Aggregations: make(map[string]*AggregationResult),
		TookMillis:   result.TookMillis,
	}

	// Convert hits to rows
	for i, hit := range result.Hits {
		row := hit.Source
		if row == nil {
			row = make(map[string]interface{})
		}
		row["_id"] = hit.ID
		row["_score"] = hit.Score
		execResult.Rows[i] = row
	}

	// Convert aggregations
	for name, agg := range result.Aggregations {
		execResult.Aggregations[name] = convertExecutorAggregation(agg)
	}

	return execResult
}

// convertExecutorAggregation converts executor.AggregationResult to planner.AggregationResult
func convertExecutorAggregation(agg *executor.AggregationResult) *AggregationResult {
	result := &AggregationResult{
		Type:    AggregationType(agg.Type),
		Buckets: make([]*Bucket, len(agg.Buckets)),
	}

	// Convert buckets
	for i, bucket := range agg.Buckets {
		key := bucket.Key
		if key == "" {
			key = fmt.Sprintf("%v", bucket.NumericKey)
		}
		result.Buckets[i] = &Bucket{
			Key:      key,
			DocCount: bucket.DocCount,
			SubAggs:  make(map[string]*AggregationResult),
		}
	}

	// For stats aggregations
	if agg.Type == "stats" || agg.Type == "extended_stats" {
		result.Stats = &Stats{
			Count: agg.Count,
			Min:   agg.Min,
			Max:   agg.Max,
			Avg:   agg.Avg,
			Sum:   agg.Sum,
		}
	}

	// For single-value aggregations (sum, avg, min, max, cardinality)
	if agg.Type == "sum" || agg.Type == "avg" || agg.Type == "min" || agg.Type == "max" {
		result.Value = agg.Sum // For sum
		if agg.Type == "avg" {
			result.Value = agg.Avg
		} else if agg.Type == "min" {
			result.Value = agg.Min
		} else if agg.Type == "max" {
			result.Value = agg.Max
		}
	}

	if agg.Type == "cardinality" {
		result.Value = float64(agg.Value)
	}

	return result
}

// expressionToJSON converts an Expression to JSON query bytes
func expressionToJSON(expr *Expression) ([]byte, error) {
	if expr == nil {
		return nil, nil
	}

	query := expressionToMap(expr)
	return json.Marshal(query)
}

// expressionToMap converts an Expression to a map for JSON serialization
func expressionToMap(expr *Expression) map[string]interface{} {
	switch expr.Type {
	case ExprTypeMatchAll:
		return map[string]interface{}{
			"match_all": map[string]interface{}{},
		}

	case ExprTypeTerm:
		return map[string]interface{}{
			"term": map[string]interface{}{
				expr.Field: expr.Value,
			},
		}

	case ExprTypeMatch:
		return map[string]interface{}{
			"match": map[string]interface{}{
				expr.Field: expr.Value,
			},
		}

	case ExprTypeRange:
		return map[string]interface{}{
			"range": map[string]interface{}{
				expr.Field: expr.Value,
			},
		}

	case ExprTypeExists:
		return map[string]interface{}{
			"exists": map[string]interface{}{
				"field": expr.Field,
			},
		}

	case ExprTypePrefix:
		return map[string]interface{}{
			"prefix": map[string]interface{}{
				expr.Field: expr.Value,
			},
		}

	case ExprTypeWildcard:
		return map[string]interface{}{
			"wildcard": map[string]interface{}{
				expr.Field: expr.Value,
			},
		}

	case ExprTypeBool:
		boolQuery := make(map[string]interface{})

		// Check if this is a must_not wrapper
		if expr.Value == "must_not" && len(expr.Children) == 1 {
			return map[string]interface{}{
				"bool": map[string]interface{}{
					"must_not": []interface{}{expressionToMap(expr.Children[0])},
				},
			}
		}

		// Handle bool with multiple children (could be AND or OR)
		// For now, treat as should (OR)
		if len(expr.Children) > 0 {
			should := make([]interface{}, len(expr.Children))
			for i, child := range expr.Children {
				should[i] = expressionToMap(child)
			}
			boolQuery["should"] = should
		}

		return map[string]interface{}{
			"bool": boolQuery,
		}

	default:
		// Fallback to match_all
		return map[string]interface{}{
			"match_all": map[string]interface{}{},
		}
	}
}

// applyFilterToRows applies a filter expression to rows (client-side filtering)
func applyFilterToRows(rows []map[string]interface{}, condition *Expression) []map[string]interface{} {
	if condition == nil {
		return rows
	}

	filtered := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		if evaluateExpression(condition, row) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

// evaluateExpression evaluates an expression against a document
func evaluateExpression(expr *Expression, doc map[string]interface{}) bool {
	switch expr.Type {
	case ExprTypeMatchAll:
		return true

	case ExprTypeTerm:
		value, exists := doc[expr.Field]
		if !exists {
			return false
		}
		return value == expr.Value

	case ExprTypeExists:
		_, exists := doc[expr.Field]
		return exists

	case ExprTypeBool:
		// For must_not
		if expr.Value == "must_not" && len(expr.Children) > 0 {
			return !evaluateExpression(expr.Children[0], doc)
		}

		// For OR (should)
		for _, child := range expr.Children {
			if evaluateExpression(child, doc) {
				return true
			}
		}
		return false

	default:
		// For other types, default to true (server-side filtering already applied)
		return true
	}
}

// applyProjectionToRows applies projection to rows (field selection)
func applyProjectionToRows(rows []map[string]interface{}, fields []string) []map[string]interface{} {
	if len(fields) == 0 {
		return rows
	}

	projected := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		projectedRow := make(map[string]interface{})

		// Always include _id and _score
		if id, exists := row["_id"]; exists {
			projectedRow["_id"] = id
		}
		if score, exists := row["_score"]; exists {
			projectedRow["_score"] = score
		}

		// Include requested fields
		for _, field := range fields {
			if value, exists := row[field]; exists {
				projectedRow[field] = value
			}
		}
		projected[i] = projectedRow
	}
	return projected
}

// sortRows sorts rows by specified sort fields
func sortRows(rows []map[string]interface{}, sortFields []*SortField) []map[string]interface{} {
	if len(sortFields) == 0 {
		return rows
	}

	sorted := make([]map[string]interface{}, len(rows))
	copy(sorted, rows)

	sort.SliceStable(sorted, func(i, j int) bool {
		for _, sf := range sortFields {
			vi := getFieldValue(sorted[i], sf.Field)
			vj := getFieldValue(sorted[j], sf.Field)

			cmp := compareValues(vi, vj)
			if cmp != 0 {
				if sf.Descending {
					return cmp > 0
				}
				return cmp < 0
			}
		}
		return false
	})

	return sorted
}

// getFieldValue gets a field value from a document, handling special fields
func getFieldValue(doc map[string]interface{}, field string) interface{} {
	if value, exists := doc[field]; exists {
		return value
	}
	return nil
}

// compareValues compares two values for sorting
func compareValues(a, b interface{}) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Handle numeric types
	af, aok := toFloat64(a)
	bf, bok := toFloat64(b)
	if aok && bok {
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}

	// Handle strings
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		if as < bs {
			return -1
		}
		if as > bs {
			return 1
		}
		return 0
	}

	return 0
}

// toFloat64 attempts to convert a value to float64
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	default:
		return 0, false
	}
}

// applyLimitToRows applies offset and limit to rows
func applyLimitToRows(rows []map[string]interface{}, offset, limit int64) []map[string]interface{} {
	if offset >= int64(len(rows)) {
		return []map[string]interface{}{}
	}

	start := int(offset)
	end := start + int(limit)
	if end > len(rows) {
		end = len(rows)
	}

	return rows[start:end]
}
