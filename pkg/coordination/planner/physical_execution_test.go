package planner

import (
	"context"
	"testing"

	"github.com/quidditch/quidditch/pkg/coordination/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPhysicalScanExecute(t *testing.T) {
	logger := zap.NewNop()

	mockExec := &mockQueryExecutor{
		searchFunc: func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
			return &executor.SearchResult{
				TotalHits:  100,
				MaxScore:   2.5,
				TookMillis: 42,
				Hits: []*executor.SearchHit{
					{
						ID:     "doc1",
						Score:  2.5,
						Source: map[string]interface{}{"title": "Test Doc", "count": 10},
					},
					{
						ID:     "doc2",
						Score:  1.8,
						Source: map[string]interface{}{"title": "Another Doc", "count": 20},
					},
				},
			}, nil
		},
	}

	execCtx := &ExecutionContext{
		QueryExecutor: mockExec,
		Logger:        logger,
	}

	ctx := WithExecutionContext(context.Background(), execCtx)

	scan := &PhysicalScan{
		IndexName: "products",
		Shards:    []int32{0, 1, 2},
		Filter: &Expression{
			Type:  ExprTypeTerm,
			Field: "status",
			Value: "active",
		},
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	result, err := scan.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(100), result.TotalHits)
	assert.Equal(t, 2.5, result.MaxScore)
	assert.Len(t, result.Rows, 2)
	assert.Equal(t, "doc1", result.Rows[0]["_id"])
	assert.Equal(t, "Test Doc", result.Rows[0]["title"])
}

func TestPhysicalFilterExecute(t *testing.T) {
	logger := zap.NewNop()

	mockExec := &mockQueryExecutor{
		searchFunc: func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
			return &executor.SearchResult{
				TotalHits: 4,
				Hits: []*executor.SearchHit{
					{ID: "1", Source: map[string]interface{}{"status": "active"}},
					{ID: "2", Source: map[string]interface{}{"status": "inactive"}},
					{ID: "3", Source: map[string]interface{}{"status": "active"}},
					{ID: "4", Source: map[string]interface{}{"status": "pending"}},
				},
			}, nil
		},
	}

	execCtx := &ExecutionContext{
		QueryExecutor: mockExec,
		Logger:        logger,
	}

	ctx := WithExecutionContext(context.Background(), execCtx)

	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	filter := &PhysicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "status",
			Value: "active",
		},
		Child:         scan,
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	result, err := filter.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(4), result.TotalHits) // TotalHits preserved from child (total docs)
	assert.Len(t, result.Rows, 2)               // But only 2 rows pass the filter
	assert.Equal(t, "1", result.Rows[0]["_id"])
	assert.Equal(t, "3", result.Rows[1]["_id"])
}

func TestPhysicalProjectExecute(t *testing.T) {
	logger := zap.NewNop()

	mockExec := &mockQueryExecutor{
		searchFunc: func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
			return &executor.SearchResult{
				TotalHits: 2,
				Hits: []*executor.SearchHit{
					{
						ID:    "1",
						Score: 2.5,
						Source: map[string]interface{}{
							"title":  "Doc1",
							"count":  10,
							"status": "active",
						},
					},
					{
						ID:    "2",
						Score: 1.8,
						Source: map[string]interface{}{
							"title":  "Doc2",
							"count":  20,
							"status": "inactive",
						},
					},
				},
			}, nil
		},
	}

	execCtx := &ExecutionContext{
		QueryExecutor: mockExec,
		Logger:        logger,
	}

	ctx := WithExecutionContext(context.Background(), execCtx)

	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	project := &PhysicalProject{
		Fields:        []string{"title", "count"},
		Child:         scan,
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	result, err := project.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, result.Rows, 2)

	// Check first row has only projected fields (plus _id and _score)
	assert.Contains(t, result.Rows[0], "_id")
	assert.Contains(t, result.Rows[0], "_score")
	assert.Contains(t, result.Rows[0], "title")
	assert.Contains(t, result.Rows[0], "count")
	assert.NotContains(t, result.Rows[0], "status") // Not projected
}

func TestPhysicalSortExecute(t *testing.T) {
	logger := zap.NewNop()

	mockExec := &mockQueryExecutor{
		searchFunc: func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
			return &executor.SearchResult{
				TotalHits: 3,
				Hits: []*executor.SearchHit{
					{ID: "1", Score: 2.5, Source: map[string]interface{}{"name": "Charlie", "age": 25}},
					{ID: "2", Score: 1.8, Source: map[string]interface{}{"name": "Alice", "age": 30}},
					{ID: "3", Score: 2.0, Source: map[string]interface{}{"name": "Bob", "age": 20}},
				},
			}, nil
		},
	}

	execCtx := &ExecutionContext{
		QueryExecutor: mockExec,
		Logger:        logger,
	}

	ctx := WithExecutionContext(context.Background(), execCtx)

	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	sort := &PhysicalSort{
		SortFields: []*SortField{
			{Field: "name", Descending: false},
		},
		Child:         scan,
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	result, err := sort.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, result.Rows, 3)

	// Check sorting by name ascending
	assert.Equal(t, "2", result.Rows[0]["_id"]) // Alice
	assert.Equal(t, "3", result.Rows[1]["_id"]) // Bob
	assert.Equal(t, "1", result.Rows[2]["_id"]) // Charlie
}

func TestPhysicalLimitExecute(t *testing.T) {
	logger := zap.NewNop()

	mockExec := &mockQueryExecutor{
		searchFunc: func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
			return &executor.SearchResult{
				TotalHits: 5,
				Hits: []*executor.SearchHit{
					{ID: "1", Score: 5.0, Source: map[string]interface{}{}},
					{ID: "2", Score: 4.0, Source: map[string]interface{}{}},
					{ID: "3", Score: 3.0, Source: map[string]interface{}{}},
					{ID: "4", Score: 2.0, Source: map[string]interface{}{}},
					{ID: "5", Score: 1.0, Source: map[string]interface{}{}},
				},
			}, nil
		},
	}

	execCtx := &ExecutionContext{
		QueryExecutor: mockExec,
		Logger:        logger,
	}

	ctx := WithExecutionContext(context.Background(), execCtx)

	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	limit := &PhysicalLimit{
		Offset:        1,
		Limit:         2,
		Child:         scan,
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	result, err := limit.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, result.Rows, 2) // Offset 1, limit 2
	assert.Equal(t, "2", result.Rows[0]["_id"])
	assert.Equal(t, "3", result.Rows[1]["_id"])
}

func TestPhysicalAggregateExecute(t *testing.T) {
	logger := zap.NewNop()

	mockExec := &mockQueryExecutor{
		searchFunc: func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
			return &executor.SearchResult{
				TotalHits: 100,
				Hits:      []*executor.SearchHit{},
				Aggregations: map[string]*executor.AggregationResult{
					"categories": {
						Type: "terms",
						Buckets: []*executor.AggregationBucket{
							{Key: "electronics", DocCount: 50},
							{Key: "books", DocCount: 30},
						},
					},
					"avg_price": {
						Type: "avg",
						Avg:  45.5,
					},
				},
			}, nil
		},
	}

	execCtx := &ExecutionContext{
		QueryExecutor: mockExec,
		Logger:        logger,
	}

	ctx := WithExecutionContext(context.Background(), execCtx)

	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	aggregate := &PhysicalAggregate{
		GroupBy: []string{},
		Aggregations: []*Aggregation{
			{Name: "categories", Type: AggTypeTerms, Field: "category"},
			{Name: "avg_price", Type: AggTypeAvg, Field: "price"},
		},
		Child:         scan,
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	result, err := aggregate.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, result.Aggregations, 2)

	// Check terms aggregation
	termsAgg := result.Aggregations["categories"]
	require.NotNil(t, termsAgg)
	assert.Equal(t, AggregationType("terms"), termsAgg.Type)
	assert.Len(t, termsAgg.Buckets, 2)

	// Check avg aggregation
	avgAgg := result.Aggregations["avg_price"]
	require.NotNil(t, avgAgg)
	assert.Equal(t, AggregationType("avg"), avgAgg.Type)
	assert.Equal(t, 45.5, avgAgg.Value)
}

func TestComplexPhysicalPlanExecution(t *testing.T) {
	// Test a complex physical plan: Limit -> Sort -> Project -> Filter -> Scan
	logger := zap.NewNop()

	mockExec := &mockQueryExecutor{
		searchFunc: func(ctx context.Context, indexName string, query []byte, filterExpr []byte, from, size int) (*executor.SearchResult, error) {
			return &executor.SearchResult{
				TotalHits: 10,
				Hits: []*executor.SearchHit{
					{ID: "1", Score: 5.0, Source: map[string]interface{}{"name": "Product A", "price": 100, "status": "active"}},
					{ID: "2", Score: 4.0, Source: map[string]interface{}{"name": "Product B", "price": 200, "status": "active"}},
					{ID: "3", Score: 3.0, Source: map[string]interface{}{"name": "Product C", "price": 150, "status": "inactive"}},
					{ID: "4", Score: 2.0, Source: map[string]interface{}{"name": "Product D", "price": 50, "status": "active"}},
					{ID: "5", Score: 1.0, Source: map[string]interface{}{"name": "Product E", "price": 300, "status": "active"}},
				},
			}, nil
		},
	}

	execCtx := &ExecutionContext{
		QueryExecutor: mockExec,
		Logger:        logger,
	}

	ctx := WithExecutionContext(context.Background(), execCtx)

	// Build plan: Limit -> Sort -> Project -> Filter -> Scan
	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	filter := &PhysicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "status",
			Value: "active",
		},
		Child:         scan,
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	project := &PhysicalProject{
		Fields:        []string{"name", "price"},
		Child:         filter,
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	sort := &PhysicalSort{
		SortFields: []*SortField{
			{Field: "price", Descending: true}, // Sort by price descending
		},
		Child:         project,
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	limit := &PhysicalLimit{
		Offset:        0,
		Limit:         2,
		Child:         sort,
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	result, err := limit.Execute(ctx)
	require.NoError(t, err)

	// Should return top 2 active products by price (descending)
	assert.Len(t, result.Rows, 2)
	assert.Equal(t, "5", result.Rows[0]["_id"]) // Product E: $300
	assert.Equal(t, "2", result.Rows[1]["_id"]) // Product B: $200

	// Check projection (only name and price, plus _id and _score)
	assert.Contains(t, result.Rows[0], "_id")
	assert.Contains(t, result.Rows[0], "_score")
	assert.Contains(t, result.Rows[0], "name")
	assert.Contains(t, result.Rows[0], "price")
	assert.NotContains(t, result.Rows[0], "status")
}

func TestPhysicalExecutionWithoutContext(t *testing.T) {
	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		OutputSchema:  &Schema{},
		EstimatedCost: &Cost{},
	}

	ctx := context.Background() // No execution context

	_, err := scan.Execute(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execution context not found")
}
