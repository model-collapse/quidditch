package planner

import (
	"context"
	"testing"

	"github.com/quidditch/quidditch/pkg/coordination/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Mock QueryExecutor for testing
type mockQueryExecutor struct {
	searchFunc func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error)
}

func (m *mockQueryExecutor) ExecuteSearch(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, indexName, query, filterExpr, from, size)
	}
	return &executor.SearchResult{
		TotalHits: 0,
		MaxScore:  0,
		Hits:      []*executor.SearchHit{},
	}, nil
}

func (m *mockQueryExecutor) RegisterDataNode(client interface{}) error {
	return nil
}

func (m *mockQueryExecutor) HasDataNodeClient(nodeID string) bool {
	return false
}

func TestExecutionContext(t *testing.T) {
	logger := zap.NewNop()
	mockExec := &mockQueryExecutor{}

	execCtx := &ExecutionContext{
		QueryExecutor: mockExec,
		Logger:        logger,
	}

	ctx := context.Background()
	ctx = WithExecutionContext(ctx, execCtx)

	retrieved, err := GetExecutionContext(ctx)
	require.NoError(t, err)
	assert.Equal(t, execCtx, retrieved)
}

func TestGetExecutionContextMissing(t *testing.T) {
	ctx := context.Background()

	_, err := GetExecutionContext(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execution context not found")
}

func TestConvertExecutorResultToExecution(t *testing.T) {
	executorResult := &executor.SearchResult{
		TotalHits:  100,
		MaxScore:   2.5,
		TookMillis: 42,
		Hits: []*executor.SearchHit{
			{
				ID:    "doc1",
				Score: 2.5,
				Source: map[string]interface{}{
					"title": "Test Document",
					"count": 10,
				},
			},
			{
				ID:    "doc2",
				Score: 1.8,
				Source: map[string]interface{}{
					"title": "Another Document",
					"count": 20,
				},
			},
		},
		Aggregations: map[string]*executor.AggregationResult{
			"terms_agg": {
				Type: "terms",
				Buckets: []*executor.AggregationBucket{
					{Key: "cat1", DocCount: 50},
					{Key: "cat2", DocCount: 30},
				},
			},
			"stats_agg": {
				Type:  "stats",
				Count: 100,
				Min:   1.0,
				Max:   100.0,
				Avg:   50.0,
				Sum:   5000.0,
			},
		},
	}

	execResult := convertExecutorResultToExecution(executorResult)

	assert.Equal(t, int64(100), execResult.TotalHits)
	assert.Equal(t, 2.5, execResult.MaxScore)
	assert.Equal(t, int64(42), execResult.TookMillis)
	assert.Len(t, execResult.Rows, 2)

	// Check first row
	assert.Equal(t, "doc1", execResult.Rows[0]["_id"])
	assert.Equal(t, 2.5, execResult.Rows[0]["_score"])
	assert.Equal(t, "Test Document", execResult.Rows[0]["title"])
	assert.Equal(t, 10, execResult.Rows[0]["count"])

	// Check aggregations
	assert.Len(t, execResult.Aggregations, 2)

	termsAgg := execResult.Aggregations["terms_agg"]
	require.NotNil(t, termsAgg)
	assert.Equal(t, AggregationType("terms"), termsAgg.Type)
	assert.Len(t, termsAgg.Buckets, 2)
	assert.Equal(t, "cat1", termsAgg.Buckets[0].Key)
	assert.Equal(t, int64(50), termsAgg.Buckets[0].DocCount)

	statsAgg := execResult.Aggregations["stats_agg"]
	require.NotNil(t, statsAgg)
	assert.Equal(t, AggregationType("stats"), statsAgg.Type)
	assert.Equal(t, int64(100), statsAgg.Stats.Count)
	assert.Equal(t, 1.0, statsAgg.Stats.Min)
	assert.Equal(t, 100.0, statsAgg.Stats.Max)
	assert.Equal(t, 50.0, statsAgg.Stats.Avg)
	assert.Equal(t, 5000.0, statsAgg.Stats.Sum)
}

func TestExpressionToJSON(t *testing.T) {
	tests := []struct {
		name     string
		expr     *Expression
		expected string
	}{
		{
			name: "match_all",
			expr: &Expression{
				Type: ExprTypeMatchAll,
			},
			expected: `{"match_all":{}}`,
		},
		{
			name: "term",
			expr: &Expression{
				Type:  ExprTypeTerm,
				Field: "status",
				Value: "active",
			},
			expected: `{"term":{"status":"active"}}`,
		},
		{
			name: "match",
			expr: &Expression{
				Type:  ExprTypeMatch,
				Field: "title",
				Value: "search engine",
			},
			expected: `{"match":{"title":"search engine"}}`,
		},
		{
			name: "exists",
			expr: &Expression{
				Type:  ExprTypeExists,
				Field: "email",
			},
			expected: `{"exists":{"field":"email"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := expressionToJSON(tt.expr)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(jsonBytes))
		})
	}
}

func TestApplyFilterToRows(t *testing.T) {
	rows := []map[string]interface{}{
		{"id": "1", "status": "active", "count": 10},
		{"id": "2", "status": "inactive", "count": 20},
		{"id": "3", "status": "active", "count": 30},
		{"id": "4", "count": 40}, // missing status
	}

	t.Run("term_filter", func(t *testing.T) {
		filter := &Expression{
			Type:  ExprTypeTerm,
			Field: "status",
			Value: "active",
		}

		filtered := applyFilterToRows(rows, filter)
		assert.Len(t, filtered, 2)
		assert.Equal(t, "1", filtered[0]["id"])
		assert.Equal(t, "3", filtered[1]["id"])
	})

	t.Run("exists_filter", func(t *testing.T) {
		filter := &Expression{
			Type:  ExprTypeExists,
			Field: "status",
		}

		filtered := applyFilterToRows(rows, filter)
		assert.Len(t, filtered, 3)
	})

	t.Run("match_all", func(t *testing.T) {
		filter := &Expression{
			Type: ExprTypeMatchAll,
		}

		filtered := applyFilterToRows(rows, filter)
		assert.Len(t, filtered, 4)
	})

	t.Run("nil_filter", func(t *testing.T) {
		filtered := applyFilterToRows(rows, nil)
		assert.Len(t, filtered, 4)
	})
}

func TestApplyProjectionToRows(t *testing.T) {
	rows := []map[string]interface{}{
		{"_id": "1", "_score": 2.5, "title": "Doc1", "count": 10, "status": "active"},
		{"_id": "2", "_score": 1.8, "title": "Doc2", "count": 20, "status": "inactive"},
	}

	t.Run("project_specific_fields", func(t *testing.T) {
		projected := applyProjectionToRows(rows, []string{"title", "count"})
		assert.Len(t, projected, 2)

		// Check first row
		assert.Equal(t, "1", projected[0]["_id"])    // _id always included
		assert.Equal(t, 2.5, projected[0]["_score"]) // _score always included
		assert.Equal(t, "Doc1", projected[0]["title"])
		assert.Equal(t, 10, projected[0]["count"])
		assert.NotContains(t, projected[0], "status") // Not projected

		// Check second row
		assert.Equal(t, "2", projected[1]["_id"])
		assert.Equal(t, 1.8, projected[1]["_score"])
		assert.Equal(t, "Doc2", projected[1]["title"])
		assert.Equal(t, 20, projected[1]["count"])
		assert.NotContains(t, projected[1], "status")
	})

	t.Run("empty_fields", func(t *testing.T) {
		projected := applyProjectionToRows(rows, []string{})
		assert.Equal(t, rows, projected) // No projection applied
	})
}

func TestSortRows(t *testing.T) {
	rows := []map[string]interface{}{
		{"_id": "1", "name": "Charlie", "score": 85, "age": 25},
		{"_id": "2", "name": "Alice", "score": 95, "age": 30},
		{"_id": "3", "name": "Bob", "score": 90, "age": 20},
	}

	t.Run("sort_by_score_desc", func(t *testing.T) {
		sortFields := []*SortField{
			{Field: "score", Descending: true},
		}

		sorted := sortRows(rows, sortFields)
		assert.Equal(t, "2", sorted[0]["_id"]) // Alice: 95
		assert.Equal(t, "3", sorted[1]["_id"]) // Bob: 90
		assert.Equal(t, "1", sorted[2]["_id"]) // Charlie: 85
	})

	t.Run("sort_by_name_asc", func(t *testing.T) {
		sortFields := []*SortField{
			{Field: "name", Descending: false},
		}

		sorted := sortRows(rows, sortFields)
		assert.Equal(t, "2", sorted[0]["_id"]) // Alice
		assert.Equal(t, "3", sorted[1]["_id"]) // Bob
		assert.Equal(t, "1", sorted[2]["_id"]) // Charlie
	})

	t.Run("multi_field_sort", func(t *testing.T) {
		// Sort by age desc, then by name asc
		sortFields := []*SortField{
			{Field: "age", Descending: true},
			{Field: "name", Descending: false},
		}

		sorted := sortRows(rows, sortFields)
		assert.Equal(t, "2", sorted[0]["_id"]) // Age 30
		assert.Equal(t, "1", sorted[1]["_id"]) // Age 25
		assert.Equal(t, "3", sorted[2]["_id"]) // Age 20
	})

	t.Run("empty_sort_fields", func(t *testing.T) {
		sorted := sortRows(rows, []*SortField{})
		assert.Equal(t, rows, sorted) // No sorting applied
	})
}

func TestApplyLimitToRows(t *testing.T) {
	rows := []map[string]interface{}{
		{"_id": "1"},
		{"_id": "2"},
		{"_id": "3"},
		{"_id": "4"},
		{"_id": "5"},
	}

	t.Run("normal_limit", func(t *testing.T) {
		limited := applyLimitToRows(rows, 1, 2)
		assert.Len(t, limited, 2)
		assert.Equal(t, "2", limited[0]["_id"])
		assert.Equal(t, "3", limited[1]["_id"])
	})

	t.Run("zero_offset", func(t *testing.T) {
		limited := applyLimitToRows(rows, 0, 3)
		assert.Len(t, limited, 3)
		assert.Equal(t, "1", limited[0]["_id"])
		assert.Equal(t, "2", limited[1]["_id"])
		assert.Equal(t, "3", limited[2]["_id"])
	})

	t.Run("offset_beyond_rows", func(t *testing.T) {
		limited := applyLimitToRows(rows, 10, 5)
		assert.Len(t, limited, 0)
	})

	t.Run("limit_beyond_rows", func(t *testing.T) {
		limited := applyLimitToRows(rows, 2, 100)
		assert.Len(t, limited, 3) // Rows 3, 4, 5
		assert.Equal(t, "3", limited[0]["_id"])
		assert.Equal(t, "4", limited[1]["_id"])
		assert.Equal(t, "5", limited[2]["_id"])
	})
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		name     string
		a        interface{}
		b        interface{}
		expected int
	}{
		{"both_nil", nil, nil, 0},
		{"a_nil", nil, 10, -1},
		{"b_nil", 10, nil, 1},
		{"int_less", 5, 10, -1},
		{"int_greater", 10, 5, 1},
		{"int_equal", 10, 10, 0},
		{"float_less", 5.5, 10.5, -1},
		{"float_greater", 10.5, 5.5, 1},
		{"string_less", "apple", "banana", -1},
		{"string_greater", "banana", "apple", 1},
		{"string_equal", "apple", "apple", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareValues(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected float64
		ok       bool
	}{
		{"float64", float64(10.5), 10.5, true},
		{"float32", float32(10.5), 10.5, true},
		{"int", 10, 10.0, true},
		{"int32", int32(10), 10.0, true},
		{"int64", int64(10), 10.0, true},
		{"uint", uint(10), 10.0, true},
		{"uint32", uint32(10), 10.0, true},
		{"uint64", uint64(10), 10.0, true},
		{"string", "not a number", 0, false},
		{"bool", true, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := toFloat64(tt.value)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
