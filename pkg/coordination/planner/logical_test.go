package planner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogicalScan(t *testing.T) {
	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0, 1, 2},
		Filter:      nil,
		EstimatedRows: 10000,
	}

	assert.Equal(t, PlanTypeScan, scan.Type())
	assert.Nil(t, scan.Children())
	assert.Equal(t, int64(10000), scan.Cardinality())
	assert.Contains(t, scan.String(), "products")
}

func TestLogicalFilter(t *testing.T) {
	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	filter := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		Child:       scan,
		EstimatedRows: 2000,
	}

	assert.Equal(t, PlanTypeFilter, filter.Type())
	assert.Len(t, filter.Children(), 1)
	assert.Equal(t, scan, filter.Children()[0])
	assert.Equal(t, int64(2000), filter.Cardinality())
}

func TestLogicalProject(t *testing.T) {
	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	project := &LogicalProject{
		Fields: []string{"name", "price"},
		Child:  scan,
		OutputSchema: &Schema{
			Fields: []*Field{
				{Name: "name", Type: FieldTypeString},
				{Name: "price", Type: FieldTypeFloat},
			},
		},
	}

	assert.Equal(t, PlanTypeProject, project.Type())
	assert.Len(t, project.Children(), 1)
	assert.Equal(t, scan, project.Children()[0])
	assert.Equal(t, int64(10000), project.Cardinality())
	assert.Len(t, project.Schema().Fields, 2)
}

func TestLogicalAggregate(t *testing.T) {
	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	agg := &LogicalAggregate{
		GroupBy: []string{"category"},
		Aggregations: []*Aggregation{
			{
				Name:  "avg_price",
				Type:  AggTypeAvg,
				Field: "price",
			},
			{
				Name:  "count",
				Type:  AggTypeCount,
				Field: "_id",
			},
		},
		Child: scan,
		OutputSchema: &Schema{
			Fields: []*Field{
				{Name: "category", Type: FieldTypeString},
				{Name: "avg_price", Type: FieldTypeFloat},
				{Name: "count", Type: FieldTypeInteger},
			},
		},
	}

	assert.Equal(t, PlanTypeAggregate, agg.Type())
	assert.Len(t, agg.Children(), 1)
	assert.Equal(t, scan, agg.Children()[0])
	assert.Len(t, agg.Aggregations, 2)
	// EstimatedRows should be ~10% of child (1000)
	assert.Equal(t, int64(1000), agg.Cardinality())
}

func TestLogicalSort(t *testing.T) {
	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	sort := &LogicalSort{
		SortFields: []*SortField{
			{Field: "price", Descending: true},
			{Field: "name", Descending: false},
		},
		Child: scan,
	}

	assert.Equal(t, PlanTypeSort, sort.Type())
	assert.Len(t, sort.Children(), 1)
	assert.Equal(t, scan, sort.Children()[0])
	assert.Len(t, sort.SortFields, 2)
	assert.Equal(t, int64(10000), sort.Cardinality())
}

func TestLogicalLimit(t *testing.T) {
	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	limit := &LogicalLimit{
		Offset: 100,
		Limit:  50,
		Child:  scan,
	}

	assert.Equal(t, PlanTypeLimit, limit.Type())
	assert.Len(t, limit.Children(), 1)
	assert.Equal(t, scan, limit.Children()[0])
	assert.Equal(t, int64(50), limit.Cardinality())
}

func TestLogicalLimitWithLargeOffset(t *testing.T) {
	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 100,
	}

	limit := &LogicalLimit{
		Offset: 150,
		Limit:  50,
		Child:  scan,
	}

	// Offset exceeds total rows, should return 0
	assert.Equal(t, int64(0), limit.Cardinality())
}

func TestSetChild(t *testing.T) {
	scan1 := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	scan2 := &LogicalScan{
		IndexName:   "users",
		Shards:      []int32{0},
		EstimatedRows: 5000,
	}

	filter := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "status",
			Value: "active",
		},
		Child:       scan1,
		EstimatedRows: 2000,
	}

	// Replace child
	err := filter.SetChild(0, scan2)
	require.NoError(t, err)
	assert.Equal(t, scan2, filter.Child)

	// Invalid index
	err = filter.SetChild(1, scan2)
	assert.Error(t, err)
}

func TestComplexPlanTree(t *testing.T) {
	// Build a complex plan tree:
	// Limit -> Sort -> Project -> Filter -> Scan

	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0, 1, 2},
		EstimatedRows: 100000,
	}

	filter := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		Child:       scan,
		EstimatedRows: 20000,
	}

	project := &LogicalProject{
		Fields: []string{"name", "price", "rating"},
		Child:  filter,
		OutputSchema: &Schema{
			Fields: []*Field{
				{Name: "name", Type: FieldTypeString},
				{Name: "price", Type: FieldTypeFloat},
				{Name: "rating", Type: FieldTypeFloat},
			},
		},
	}

	sort := &LogicalSort{
		SortFields: []*SortField{
			{Field: "rating", Descending: true},
		},
		Child: project,
	}

	limit := &LogicalLimit{
		Offset: 0,
		Limit:  10,
		Child:  sort,
	}

	// Verify tree structure
	assert.Equal(t, PlanTypeLimit, limit.Type())
	assert.Equal(t, PlanTypeSort, limit.Children()[0].Type())
	assert.Equal(t, PlanTypeProject, sort.Children()[0].Type())
	assert.Equal(t, PlanTypeFilter, project.Children()[0].Type())
	assert.Equal(t, PlanTypeScan, filter.Children()[0].Type())

	// Verify cardinality propagation
	assert.Equal(t, int64(10), limit.Cardinality())
	assert.Equal(t, int64(20000), sort.Cardinality())
	assert.Equal(t, int64(20000), project.Cardinality())
	assert.Equal(t, int64(20000), filter.Cardinality())
	assert.Equal(t, int64(100000), scan.Cardinality())
}

func TestExpression(t *testing.T) {
	// Test simple term expression
	termExpr := &Expression{
		Type:  ExprTypeTerm,
		Field: "status",
		Value: "active",
	}
	assert.Contains(t, termExpr.String(), "term")
	assert.Contains(t, termExpr.String(), "status")

	// Test bool expression with children
	boolExpr := &Expression{
		Type: ExprTypeBool,
		Children: []*Expression{
			{
				Type:  ExprTypeTerm,
				Field: "category",
				Value: "electronics",
			},
			{
				Type:  ExprTypeRange,
				Field: "price",
				Value: map[string]interface{}{"gte": 100, "lte": 500},
			},
		},
	}
	assert.Len(t, boolExpr.Children, 2)

	// Test nil expression
	var nilExpr *Expression
	assert.Equal(t, "nil", nilExpr.String())
}

func TestSchema(t *testing.T) {
	schema := &Schema{
		Fields: []*Field{
			{Name: "id", Type: FieldTypeString},
			{Name: "name", Type: FieldTypeString},
			{Name: "price", Type: FieldTypeFloat},
			{Name: "quantity", Type: FieldTypeInteger},
			{Name: "available", Type: FieldTypeBoolean},
			{Name: "metadata", Type: FieldTypeObject},
			{Name: "tags", Type: FieldTypeArray},
		},
	}

	assert.Len(t, schema.Fields, 7)
	assert.Equal(t, "id", schema.Fields[0].Name)
	assert.Equal(t, FieldTypeString, schema.Fields[0].Type)
	assert.Equal(t, FieldTypeFloat, schema.Fields[2].Type)
	assert.Equal(t, FieldTypeInteger, schema.Fields[3].Type)
	assert.Equal(t, FieldTypeBoolean, schema.Fields[4].Type)
	assert.Equal(t, FieldTypeObject, schema.Fields[5].Type)
	assert.Equal(t, FieldTypeArray, schema.Fields[6].Type)
}
