package planner

import (
	"math"
)

// Cost represents the estimated cost of a query operation
type Cost struct {
	// CPUCost represents CPU computation cost
	CPUCost float64

	// IOCost represents disk I/O cost
	IOCost float64

	// NetworkCost represents network transfer cost
	NetworkCost float64

	// MemoryCost represents memory usage cost
	MemoryCost float64

	// TotalCost is the weighted sum of all costs
	TotalCost float64
}

// CostModel estimates the cost of query operations
type CostModel struct {
	// Cost weights (for tuning)
	CPUWeight     float64
	IOWeight      float64
	NetworkWeight float64
	MemoryWeight  float64

	// Performance parameters
	SeqReadCost      float64 // Cost per row for sequential read
	RandomReadCost   float64 // Cost per row for random read
	NetworkLatency   float64 // Network latency cost per node
	HashTableCost    float64 // Cost per row for hash table operations
	ComparisonCost   float64 // Cost per comparison
	AggregationCost  float64 // Cost per aggregation operation
}

// NewDefaultCostModel creates a cost model with default parameters
func NewDefaultCostModel() *CostModel {
	return &CostModel{
		// Weights (tuned based on actual performance)
		CPUWeight:     1.0,
		IOWeight:      5.0,   // I/O is 5× more expensive than CPU
		NetworkWeight: 10.0,  // Network is 10× more expensive than CPU
		MemoryWeight:  2.0,   // Memory is 2× more expensive than CPU

		// Performance parameters (based on benchmarks)
		SeqReadCost:      0.001,  // 0.001 cost per row for sequential read
		RandomReadCost:   0.01,   // 0.01 cost per row for random read
		NetworkLatency:   1.0,    // 1.0 cost for network latency per node
		HashTableCost:    0.002,  // 0.002 cost per row for hash operations
		ComparisonCost:   0.0001, // 0.0001 cost per comparison
		AggregationCost:  0.005,  // 0.005 cost per aggregation
	}
}

// CalculateTotalCost computes the total weighted cost
func (cm *CostModel) CalculateTotalCost(cost *Cost) float64 {
	return cost.CPUCost*cm.CPUWeight +
		cost.IOCost*cm.IOWeight +
		cost.NetworkCost*cm.NetworkWeight +
		cost.MemoryCost*cm.MemoryWeight
}

// EstimateScanCost estimates the cost of a scan operation
func (cm *CostModel) EstimateScanCost(scan *LogicalScan) *Cost {
	cardinality := float64(scan.EstimatedRows)
	numShards := float64(len(scan.Shards))

	cost := &Cost{
		// I/O cost: sequential read of all rows
		IOCost: cardinality * cm.SeqReadCost,

		// Network cost: fetching data from multiple shards
		NetworkCost: numShards * cm.NetworkLatency,

		// CPU cost: minimal for scan
		CPUCost: cardinality * 0.0001,

		// Memory cost: buffer for scan results
		MemoryCost: cardinality * 0.0001,
	}

	// If there's a filter, add filter cost
	if scan.Filter != nil {
		filterCost := cm.estimateFilterExpressionCost(scan.Filter, cardinality)
		cost.CPUCost += filterCost
	}

	cost.TotalCost = cm.CalculateTotalCost(cost)
	return cost
}

// EstimateFilterCost estimates the cost of a filter operation
func (cm *CostModel) EstimateFilterCost(filter *LogicalFilter, childCost *Cost) *Cost {
	cardinality := float64(filter.Child.Cardinality())

	cost := &Cost{
		// CPU cost: evaluate filter condition for each row
		CPUCost: childCost.CPUCost + cm.estimateFilterExpressionCost(filter.Condition, cardinality),

		// I/O and network costs pass through from child
		IOCost:      childCost.IOCost,
		NetworkCost: childCost.NetworkCost,

		// Memory cost: same as child (no additional memory needed)
		MemoryCost: childCost.MemoryCost,
	}

	cost.TotalCost = cm.CalculateTotalCost(cost)
	return cost
}

// estimateFilterExpressionCost estimates the CPU cost of evaluating a filter expression
func (cm *CostModel) estimateFilterExpressionCost(expr *Expression, cardinality float64) float64 {
	if expr == nil {
		return 0
	}

	switch expr.Type {
	case ExprTypeTerm, ExprTypeMatch:
		// Simple comparison: one comparison per row
		return cardinality * cm.ComparisonCost

	case ExprTypeRange:
		// Two comparisons per row (min and max)
		return cardinality * cm.ComparisonCost * 2

	case ExprTypeBool:
		// Evaluate each child expression
		cost := 0.0
		for _, child := range expr.Children {
			cost += cm.estimateFilterExpressionCost(child, cardinality)
		}
		return cost

	case ExprTypeWildcard, ExprTypePrefix:
		// More expensive: pattern matching
		return cardinality * cm.ComparisonCost * 5

	case ExprTypeMatchAll:
		// Free: matches everything
		return 0

	default:
		// Default: one comparison per row
		return cardinality * cm.ComparisonCost
	}
}

// EstimateProjectCost estimates the cost of a projection operation
func (cm *CostModel) EstimateProjectCost(project *LogicalProject, childCost *Cost) *Cost {
	cardinality := float64(project.Child.Cardinality())
	numFields := float64(len(project.Fields))

	cost := &Cost{
		// CPU cost: copy selected fields for each row
		CPUCost: childCost.CPUCost + (cardinality * numFields * 0.0001),

		// I/O and network costs pass through from child
		IOCost:      childCost.IOCost,
		NetworkCost: childCost.NetworkCost,

		// Memory cost: reduced if projecting fewer fields
		MemoryCost: childCost.MemoryCost * 0.8, // Assume 20% reduction
	}

	cost.TotalCost = cm.CalculateTotalCost(cost)
	return cost
}

// EstimateAggregateCost estimates the cost of an aggregation operation
func (cm *CostModel) EstimateAggregateCost(agg *LogicalAggregate, childCost *Cost) *Cost {
	inputCardinality := float64(agg.Child.Cardinality())
	outputCardinality := float64(agg.Cardinality())
	numAggs := float64(len(agg.Aggregations))
	numGroupBy := float64(len(agg.GroupBy))

	cost := &Cost{
		// CPU cost: hash table operations + aggregation computations
		CPUCost: childCost.CPUCost +
			(inputCardinality * cm.HashTableCost * numGroupBy) + // Hash grouping
			(inputCardinality * cm.AggregationCost * numAggs), // Aggregation

		// I/O and network costs pass through from child
		IOCost:      childCost.IOCost,
		NetworkCost: childCost.NetworkCost,

		// Memory cost: hash table for groups
		MemoryCost: childCost.MemoryCost + (outputCardinality * 0.001),
	}

	cost.TotalCost = cm.CalculateTotalCost(cost)
	return cost
}

// EstimateSortCost estimates the cost of a sort operation
func (cm *CostModel) EstimateSortCost(sort *LogicalSort, childCost *Cost) *Cost {
	cardinality := float64(sort.Child.Cardinality())
	numSortFields := float64(len(sort.SortFields))

	// Sort cost: O(n log n) comparisons
	sortCost := cardinality * math.Log2(cardinality) * cm.ComparisonCost * numSortFields

	cost := &Cost{
		// CPU cost: sorting
		CPUCost: childCost.CPUCost + sortCost,

		// I/O and network costs pass through from child
		IOCost:      childCost.IOCost,
		NetworkCost: childCost.NetworkCost,

		// Memory cost: need to materialize all rows for sorting
		MemoryCost: childCost.MemoryCost + (cardinality * 0.001),
	}

	cost.TotalCost = cm.CalculateTotalCost(cost)
	return cost
}

// EstimateLimitCost estimates the cost of a limit operation
func (cm *CostModel) EstimateLimitCost(limit *LogicalLimit, childCost *Cost) *Cost {
	// Limit is essentially free - just stop processing early
	// However, we need to account for the fact that child still processes some rows

	limitCardinality := float64(limit.Cardinality())
	childCardinality := float64(limit.Child.Cardinality())

	// Discount factor based on how much we can reduce processing
	discountFactor := limitCardinality / childCardinality

	cost := &Cost{
		// Costs reduced proportionally to limit
		CPUCost:     childCost.CPUCost * discountFactor,
		IOCost:      childCost.IOCost * discountFactor,
		NetworkCost: childCost.NetworkCost * discountFactor,
		MemoryCost:  childCost.MemoryCost * discountFactor,
	}

	cost.TotalCost = cm.CalculateTotalCost(cost)
	return cost
}

// CompareCosts compares two costs and returns true if c1 is cheaper than c2
func (cm *CostModel) CompareCosts(c1, c2 *Cost) bool {
	return c1.TotalCost < c2.TotalCost
}

// FormatCost returns a human-readable string representation of the cost
func FormatCost(cost *Cost) string {
	return ""
}
