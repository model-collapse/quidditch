package planner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhysicalScan(t *testing.T) {
	scan := &PhysicalScan{
		IndexName: "products",
		Shards:    []int32{0, 1, 2},
		Filter: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		Fields: []string{"name", "price"},
		OutputSchema: &Schema{
			Fields: []*Field{
				{Name: "name", Type: FieldTypeString},
				{Name: "price", Type: FieldTypeFloat},
			},
		},
		EstimatedCost: &Cost{TotalCost: 100.0},
	}

	assert.Equal(t, PhysicalPlanTypeScan, scan.Type())
	assert.Nil(t, scan.Children())
	assert.Equal(t, 2, len(scan.Schema().Fields))
	assert.Equal(t, 100.0, scan.Cost().TotalCost)
	assert.Contains(t, scan.String(), "products")
}

func TestPhysicalFilter(t *testing.T) {
	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedCost: &Cost{TotalCost: 50.0},
	}

	filter := &PhysicalFilter{
		Condition: &Expression{
			Type:  ExprTypeRange,
			Field: "price",
			Value: map[string]interface{}{"gte": 100},
		},
		Child:         scan,
		EstimatedCost: &Cost{TotalCost: 60.0},
	}

	assert.Equal(t, PhysicalPlanTypeFilter, filter.Type())
	assert.Len(t, filter.Children(), 1)
	assert.Equal(t, scan, filter.Children()[0])
	assert.Equal(t, 60.0, filter.Cost().TotalCost)
}

func TestPhysicalProject(t *testing.T) {
	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedCost: &Cost{TotalCost: 50.0},
	}

	project := &PhysicalProject{
		Fields: []string{"name", "price"},
		Child:  scan,
		OutputSchema: &Schema{
			Fields: []*Field{
				{Name: "name", Type: FieldTypeString},
				{Name: "price", Type: FieldTypeFloat},
			},
		},
		EstimatedCost: &Cost{TotalCost: 52.0},
	}

	assert.Equal(t, PhysicalPlanTypeProject, project.Type())
	assert.Len(t, project.Children(), 1)
	assert.Equal(t, scan, project.Children()[0])
	assert.Len(t, project.Fields, 2)
	assert.Equal(t, 52.0, project.Cost().TotalCost)
}

func TestPhysicalAggregate(t *testing.T) {
	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedCost: &Cost{TotalCost: 50.0},
	}

	agg := &PhysicalAggregate{
		GroupBy: []string{"category"},
		Aggregations: []*Aggregation{
			{Name: "avg_price", Type: AggTypeAvg, Field: "price"},
		},
		Child:         scan,
		EstimatedCost: &Cost{TotalCost: 100.0},
	}

	assert.Equal(t, PhysicalPlanTypeAggregate, agg.Type())
	assert.Len(t, agg.Children(), 1)
	assert.Len(t, agg.Aggregations, 1)
	assert.Equal(t, 100.0, agg.Cost().TotalCost)
}

func TestPhysicalHashAggregate(t *testing.T) {
	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedCost: &Cost{TotalCost: 50.0},
	}

	hashAgg := &PhysicalHashAggregate{
		GroupBy: []string{"category"},
		Aggregations: []*Aggregation{
			{Name: "count", Type: AggTypeCount, Field: "_id"},
		},
		Child:         scan,
		EstimatedCost: &Cost{TotalCost: 80.0},
	}

	assert.Equal(t, PhysicalPlanTypeHashAggregate, hashAgg.Type())
	assert.Len(t, hashAgg.Children(), 1)
	assert.Equal(t, 80.0, hashAgg.Cost().TotalCost)
}

func TestPhysicalSort(t *testing.T) {
	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedCost: &Cost{TotalCost: 50.0},
	}

	sort := &PhysicalSort{
		SortFields: []*SortField{
			{Field: "price", Descending: true},
			{Field: "name", Descending: false},
		},
		Child:         scan,
		EstimatedCost: &Cost{TotalCost: 150.0},
	}

	assert.Equal(t, PhysicalPlanTypeSort, sort.Type())
	assert.Len(t, sort.Children(), 1)
	assert.Len(t, sort.SortFields, 2)
	assert.Equal(t, 150.0, sort.Cost().TotalCost)
}

func TestPhysicalLimit(t *testing.T) {
	scan := &PhysicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedCost: &Cost{TotalCost: 50.0},
	}

	limit := &PhysicalLimit{
		Offset:        100,
		Limit:         10,
		Child:         scan,
		EstimatedCost: &Cost{TotalCost: 5.0},
	}

	assert.Equal(t, PhysicalPlanTypeLimit, limit.Type())
	assert.Len(t, limit.Children(), 1)
	assert.Equal(t, int64(100), limit.Offset)
	assert.Equal(t, int64(10), limit.Limit)
	assert.Equal(t, 5.0, limit.Cost().TotalCost)
}

func TestPlannerScan(t *testing.T) {
	cm := NewDefaultCostModel()
	planner := NewPlanner(cm)

	logical := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0, 1, 2},
		EstimatedRows: 10000,
	}

	physical, err := planner.Plan(logical)
	require.NoError(t, err)

	scan, ok := physical.(*PhysicalScan)
	require.True(t, ok)
	assert.Equal(t, "products", scan.IndexName)
	assert.Len(t, scan.Shards, 3)
	assert.NotNil(t, scan.EstimatedCost)
}

func TestPlannerFilter(t *testing.T) {
	cm := NewDefaultCostModel()
	planner := NewPlanner(cm)

	logical := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		Child: &LogicalScan{
			IndexName:   "products",
			Shards:      []int32{0},
			EstimatedRows: 10000,
		},
		EstimatedRows: 2000,
	}

	physical, err := planner.Plan(logical)
	require.NoError(t, err)

	filter, ok := physical.(*PhysicalFilter)
	require.True(t, ok)
	assert.NotNil(t, filter.Condition)
	assert.NotNil(t, filter.Child)
	assert.NotNil(t, filter.EstimatedCost)
}

func TestPlannerProject(t *testing.T) {
	cm := NewDefaultCostModel()
	planner := NewPlanner(cm)

	logical := &LogicalProject{
		Fields: []string{"name", "price"},
		Child: &LogicalScan{
			IndexName:   "products",
			Shards:      []int32{0},
			EstimatedRows: 10000,
		},
	}

	physical, err := planner.Plan(logical)
	require.NoError(t, err)

	project, ok := physical.(*PhysicalProject)
	require.True(t, ok)
	assert.Len(t, project.Fields, 2)
	assert.NotNil(t, project.Child)
	assert.NotNil(t, project.EstimatedCost)
}

func TestPlannerAggregateSmallDataset(t *testing.T) {
	cm := NewDefaultCostModel()
	planner := NewPlanner(cm)

	// Small dataset (< 1000 rows) should use regular aggregate
	logical := &LogicalAggregate{
		GroupBy: []string{"category"},
		Aggregations: []*Aggregation{
			{Name: "count", Type: AggTypeCount, Field: "_id"},
		},
		Child: &LogicalScan{
			IndexName:   "products",
			Shards:      []int32{0},
			EstimatedRows: 500, // Small dataset
		},
	}

	physical, err := planner.Plan(logical)
	require.NoError(t, err)

	// Should be regular aggregate, not hash aggregate
	_, ok := physical.(*PhysicalAggregate)
	assert.True(t, ok)
}

func TestPlannerAggregateLargeDataset(t *testing.T) {
	cm := NewDefaultCostModel()
	planner := NewPlanner(cm)

	// Large dataset (> 1000 rows) should use hash aggregate
	logical := &LogicalAggregate{
		GroupBy: []string{"category"},
		Aggregations: []*Aggregation{
			{Name: "count", Type: AggTypeCount, Field: "_id"},
		},
		Child: &LogicalScan{
			IndexName:   "products",
			Shards:      []int32{0, 1, 2},
			EstimatedRows: 100000, // Large dataset
		},
	}

	physical, err := planner.Plan(logical)
	require.NoError(t, err)

	// Should be hash aggregate for large dataset
	hashAgg, ok := physical.(*PhysicalHashAggregate)
	require.True(t, ok)
	assert.NotNil(t, hashAgg.EstimatedCost)
}

func TestPlannerSort(t *testing.T) {
	cm := NewDefaultCostModel()
	planner := NewPlanner(cm)

	logical := &LogicalSort{
		SortFields: []*SortField{
			{Field: "price", Descending: true},
		},
		Child: &LogicalScan{
			IndexName:   "products",
			Shards:      []int32{0},
			EstimatedRows: 10000,
		},
	}

	physical, err := planner.Plan(logical)
	require.NoError(t, err)

	sort, ok := physical.(*PhysicalSort)
	require.True(t, ok)
	assert.Len(t, sort.SortFields, 1)
	assert.NotNil(t, sort.EstimatedCost)
}

func TestPlannerLimit(t *testing.T) {
	cm := NewDefaultCostModel()
	planner := NewPlanner(cm)

	logical := &LogicalLimit{
		Offset: 0,
		Limit:  10,
		Child: &LogicalScan{
			IndexName:   "products",
			Shards:      []int32{0},
			EstimatedRows: 10000,
		},
	}

	physical, err := planner.Plan(logical)
	require.NoError(t, err)

	limit, ok := physical.(*PhysicalLimit)
	require.True(t, ok)
	assert.Equal(t, int64(10), limit.Limit)
	assert.NotNil(t, limit.EstimatedCost)
}

func TestPlannerComplexPlan(t *testing.T) {
	cm := NewDefaultCostModel()
	planner := NewPlanner(cm)

	// Build complex logical plan: Limit -> Sort -> Project -> Filter -> Scan
	logical := &LogicalLimit{
		Offset: 0,
		Limit:  10,
		Child: &LogicalSort{
			SortFields: []*SortField{
				{Field: "rating", Descending: true},
			},
			Child: &LogicalProject{
				Fields: []string{"name", "price", "rating"},
				Child: &LogicalFilter{
					Condition: &Expression{
						Type:  ExprTypeTerm,
						Field: "category",
						Value: "electronics",
					},
					Child: &LogicalScan{
						IndexName:   "products",
						Shards:      []int32{0, 1, 2},
						EstimatedRows: 100000,
					},
					EstimatedRows: 20000,
				},
			},
		},
	}

	physical, err := planner.Plan(logical)
	require.NoError(t, err)

	// Verify physical plan structure
	limit, ok := physical.(*PhysicalLimit)
	require.True(t, ok)

	sort, ok := limit.Child.(*PhysicalSort)
	require.True(t, ok)

	project, ok := sort.Child.(*PhysicalProject)
	require.True(t, ok)

	filter, ok := project.Child.(*PhysicalFilter)
	require.True(t, ok)

	scan, ok := filter.Child.(*PhysicalScan)
	require.True(t, ok)
	assert.Equal(t, "products", scan.IndexName)

	// Verify costs are propagated
	assert.NotNil(t, limit.EstimatedCost)
	assert.NotNil(t, sort.EstimatedCost)
	assert.NotNil(t, project.EstimatedCost)
	assert.NotNil(t, filter.EstimatedCost)
	assert.NotNil(t, scan.EstimatedCost)
}

func TestExecutionResult(t *testing.T) {
	result := &ExecutionResult{
		Rows: []map[string]interface{}{
			{"id": "1", "name": "Product 1", "price": 100.0},
			{"id": "2", "name": "Product 2", "price": 200.0},
		},
		TotalHits:  2,
		MaxScore:   1.5,
		Aggregations: map[string]*AggregationResult{
			"avg_price": {
				Type:  AggTypeAvg,
				Value: 150.0,
			},
		},
		TookMillis: 50,
	}

	assert.Len(t, result.Rows, 2)
	assert.Equal(t, int64(2), result.TotalHits)
	assert.Equal(t, 1.5, result.MaxScore)
	assert.Contains(t, result.Aggregations, "avg_price")
	assert.Equal(t, 150.0, result.Aggregations["avg_price"].Value)
	assert.Equal(t, int64(50), result.TookMillis)
}

func TestAggregationResultBuckets(t *testing.T) {
	result := &AggregationResult{
		Type: AggTypeTerms,
		Buckets: []*Bucket{
			{Key: "electronics", DocCount: 500},
			{Key: "books", DocCount: 300},
			{Key: "clothing", DocCount: 200},
		},
	}

	assert.Len(t, result.Buckets, 3)
	assert.Equal(t, "electronics", result.Buckets[0].Key)
	assert.Equal(t, int64(500), result.Buckets[0].DocCount)
}

func TestStatsAggregation(t *testing.T) {
	stats := &Stats{
		Count: 1000,
		Min:   10.0,
		Max:   500.0,
		Avg:   100.0,
		Sum:   100000.0,
	}

	assert.Equal(t, int64(1000), stats.Count)
	assert.Equal(t, 10.0, stats.Min)
	assert.Equal(t, 500.0, stats.Max)
	assert.Equal(t, 100.0, stats.Avg)
	assert.Equal(t, 100000.0, stats.Sum)
}
