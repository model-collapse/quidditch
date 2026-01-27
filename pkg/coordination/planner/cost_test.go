package planner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDefaultCostModel(t *testing.T) {
	cm := NewDefaultCostModel()

	// Verify weights
	assert.Equal(t, 1.0, cm.CPUWeight)
	assert.Equal(t, 5.0, cm.IOWeight)
	assert.Equal(t, 10.0, cm.NetworkWeight)
	assert.Equal(t, 2.0, cm.MemoryWeight)

	// Verify performance parameters
	assert.Greater(t, cm.SeqReadCost, 0.0)
	assert.Greater(t, cm.RandomReadCost, cm.SeqReadCost)
	assert.Greater(t, cm.NetworkLatency, 0.0)
}

func TestCalculateTotalCost(t *testing.T) {
	cm := NewDefaultCostModel()

	cost := &Cost{
		CPUCost:     1.0,
		IOCost:      2.0,
		NetworkCost: 3.0,
		MemoryCost:  4.0,
	}

	total := cm.CalculateTotalCost(cost)

	// Total = 1.0*1 + 2.0*5 + 3.0*10 + 4.0*2 = 1 + 10 + 30 + 8 = 49
	assert.Equal(t, 49.0, total)
}

func TestEstimateScanCost(t *testing.T) {
	cm := NewDefaultCostModel()

	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0, 1, 2},
		Filter:      nil,
		EstimatedRows: 10000,
	}

	cost := cm.EstimateScanCost(scan)

	// Scan should have I/O cost (reading rows) and network cost (fetching from shards)
	assert.Greater(t, cost.IOCost, 0.0)
	assert.Greater(t, cost.NetworkCost, 0.0)
	assert.Greater(t, cost.CPUCost, 0.0)
	assert.Greater(t, cost.TotalCost, 0.0)

	// More shards = higher network cost
	scan.Shards = []int32{0, 1, 2, 3, 4}
	costWithMoreShards := cm.EstimateScanCost(scan)
	assert.Greater(t, costWithMoreShards.NetworkCost, cost.NetworkCost)
}

func TestEstimateScanCostWithFilter(t *testing.T) {
	cm := NewDefaultCostModel()

	// Compare scans with same cardinality, one with filter
	scanNoFilter := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		Filter:      nil,
		EstimatedRows: 10000,
	}

	scanWithFilter := &LogicalScan{
		IndexName: "products",
		Shards:    []int32{0},
		Filter: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		EstimatedRows: 10000, // Same cardinality to isolate filter cost
	}

	costNoFilter := cm.EstimateScanCost(scanNoFilter)
	costWithFilter := cm.EstimateScanCost(scanWithFilter)

	// With same cardinality, filter adds CPU cost for evaluation
	assert.Greater(t, costWithFilter.CPUCost, costNoFilter.CPUCost)
}

func TestEstimateFilterCost(t *testing.T) {
	cm := NewDefaultCostModel()

	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	childCost := cm.EstimateScanCost(scan)

	filter := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		Child:       scan,
		EstimatedRows: 2000,
	}

	filterCost := cm.EstimateFilterCost(filter, childCost)

	// Filter should add CPU cost
	assert.Greater(t, filterCost.CPUCost, childCost.CPUCost)

	// I/O and network costs should pass through
	assert.Equal(t, childCost.IOCost, filterCost.IOCost)
	assert.Equal(t, childCost.NetworkCost, filterCost.NetworkCost)
}

func TestEstimateFilterExpressionCost(t *testing.T) {
	cm := NewDefaultCostModel()
	cardinality := 10000.0

	// Simple term query
	termCost := cm.estimateFilterExpressionCost(&Expression{
		Type:  ExprTypeTerm,
		Field: "status",
		Value: "active",
	}, cardinality)
	assert.Greater(t, termCost, 0.0)

	// Range query (two comparisons)
	rangeCost := cm.estimateFilterExpressionCost(&Expression{
		Type:  ExprTypeRange,
		Field: "price",
		Value: map[string]interface{}{"gte": 100, "lte": 500},
	}, cardinality)
	assert.Greater(t, rangeCost, termCost)

	// Wildcard query (expensive)
	wildcardCost := cm.estimateFilterExpressionCost(&Expression{
		Type:  ExprTypeWildcard,
		Field: "name",
		Value: "prod*",
	}, cardinality)
	assert.Greater(t, wildcardCost, termCost)

	// Bool query with multiple children
	boolCost := cm.estimateFilterExpressionCost(&Expression{
		Type: ExprTypeBool,
		Children: []*Expression{
			{Type: ExprTypeTerm, Field: "category", Value: "electronics"},
			{Type: ExprTypeRange, Field: "price", Value: map[string]interface{}{"gte": 100}},
		},
	}, cardinality)
	assert.Greater(t, boolCost, termCost)

	// Match all (free)
	matchAllCost := cm.estimateFilterExpressionCost(&Expression{
		Type: ExprTypeMatchAll,
	}, cardinality)
	assert.Equal(t, 0.0, matchAllCost)
}

func TestEstimateProjectCost(t *testing.T) {
	cm := NewDefaultCostModel()

	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	childCost := cm.EstimateScanCost(scan)

	project := &LogicalProject{
		Fields: []string{"name", "price"},
		Child:  scan,
	}

	projectCost := cm.EstimateProjectCost(project, childCost)

	// Project should add minimal CPU cost
	assert.Greater(t, projectCost.CPUCost, childCost.CPUCost)

	// Memory cost should be reduced (projecting fewer fields)
	assert.Less(t, projectCost.MemoryCost, childCost.MemoryCost)

	// I/O and network costs should pass through
	assert.Equal(t, childCost.IOCost, projectCost.IOCost)
	assert.Equal(t, childCost.NetworkCost, projectCost.NetworkCost)
}

func TestEstimateAggregateCost(t *testing.T) {
	cm := NewDefaultCostModel()

	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 100000,
	}

	childCost := cm.EstimateScanCost(scan)

	agg := &LogicalAggregate{
		GroupBy: []string{"category"},
		Aggregations: []*Aggregation{
			{Name: "avg_price", Type: AggTypeAvg, Field: "price"},
			{Name: "count", Type: AggTypeCount, Field: "_id"},
		},
		Child: scan,
	}

	aggCost := cm.EstimateAggregateCost(agg, childCost)

	// Aggregation should add significant CPU cost (hash table + aggregation)
	assert.Greater(t, aggCost.CPUCost, childCost.CPUCost)

	// Memory cost should increase (hash table for groups)
	assert.Greater(t, aggCost.MemoryCost, childCost.MemoryCost)

	// I/O and network costs should pass through
	assert.Equal(t, childCost.IOCost, aggCost.IOCost)
	assert.Equal(t, childCost.NetworkCost, aggCost.NetworkCost)
}

func TestEstimateSortCost(t *testing.T) {
	cm := NewDefaultCostModel()

	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	childCost := cm.EstimateScanCost(scan)

	sort := &LogicalSort{
		SortFields: []*SortField{
			{Field: "price", Descending: true},
		},
		Child: scan,
	}

	sortCost := cm.EstimateSortCost(sort, childCost)

	// Sort should add significant CPU cost (O(n log n))
	assert.Greater(t, sortCost.CPUCost, childCost.CPUCost)

	// Memory cost should increase (need to materialize all rows)
	assert.Greater(t, sortCost.MemoryCost, childCost.MemoryCost)

	// Multiple sort fields should increase cost
	sort.SortFields = append(sort.SortFields, &SortField{Field: "name", Descending: false})
	sortCostMultiField := cm.EstimateSortCost(sort, childCost)
	assert.Greater(t, sortCostMultiField.CPUCost, sortCost.CPUCost)
}

func TestEstimateLimitCost(t *testing.T) {
	cm := NewDefaultCostModel()

	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	childCost := cm.EstimateScanCost(scan)

	limit := &LogicalLimit{
		Offset: 0,
		Limit:  100,
		Child:  scan,
	}

	limitCost := cm.EstimateLimitCost(limit, childCost)

	// Limit should reduce all costs proportionally
	assert.Less(t, limitCost.CPUCost, childCost.CPUCost)
	assert.Less(t, limitCost.IOCost, childCost.IOCost)
	assert.Less(t, limitCost.NetworkCost, childCost.NetworkCost)
	assert.Less(t, limitCost.MemoryCost, childCost.MemoryCost)

	// Verify discount factor: limit 100 out of 10000 = 1% cost
	expectedDiscount := 100.0 / 10000.0
	assert.InDelta(t, childCost.CPUCost*expectedDiscount, limitCost.CPUCost, 0.001)
}

func TestCompareCosts(t *testing.T) {
	cm := NewDefaultCostModel()

	cost1 := &Cost{
		CPUCost:     1.0,
		IOCost:      1.0,
		NetworkCost: 1.0,
		MemoryCost:  1.0,
	}
	cost1.TotalCost = cm.CalculateTotalCost(cost1)

	cost2 := &Cost{
		CPUCost:     2.0,
		IOCost:      2.0,
		NetworkCost: 2.0,
		MemoryCost:  2.0,
	}
	cost2.TotalCost = cm.CalculateTotalCost(cost2)

	// cost1 should be cheaper than cost2
	assert.True(t, cm.CompareCosts(cost1, cost2))
	assert.False(t, cm.CompareCosts(cost2, cost1))
}

func TestCostModelRealistic(t *testing.T) {
	cm := NewDefaultCostModel()

	// Realistic scenario: 100K rows, 3 shards, filter to 10K rows
	scan := &LogicalScan{
		IndexName: "products",
		Shards:    []int32{0, 1, 2},
		Filter: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		EstimatedRows: 10000,
	}

	scanCost := cm.EstimateScanCost(scan)

	// Network cost should be significant (3 shards)
	assert.Greater(t, scanCost.NetworkCost, 0.0)

	// I/O cost should be based on cardinality
	assert.Greater(t, scanCost.IOCost, 0.0)

	// Total cost should be reasonable
	assert.Greater(t, scanCost.TotalCost, 0.0)
	assert.Less(t, scanCost.TotalCost, 1000000.0) // Not astronomically high

	t.Logf("Scan cost: CPU=%.2f, IO=%.2f, Network=%.2f, Memory=%.2f, Total=%.2f",
		scanCost.CPUCost, scanCost.IOCost, scanCost.NetworkCost, scanCost.MemoryCost, scanCost.TotalCost)
}
