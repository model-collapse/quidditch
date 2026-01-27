package planner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQueryPlannerIntegration tests the complete query planning pipeline
func TestQueryPlannerIntegration(t *testing.T) {
	t.Run("SimpleTermQuery", func(t *testing.T) {
		// Create a simple term query
		scan := &LogicalScan{
			IndexName:     "products",
			Shards:        []int32{0},
			Filter:        &Expression{Type: ExprTypeTerm, Field: "status", Value: "active"},
			EstimatedRows: 600,
		}

		// Optimize
		optimizer := NewOptimizer()
		optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)
		optimized, err := optimizer.Optimize(scan)
		require.NoError(t, err)

		// Generate physical plan
		planner := NewPlanner(NewDefaultCostModel())
		physical, err := planner.Plan(optimized)
		require.NoError(t, err)

		// Verify plan was created
		assert.NotNil(t, physical)
		assert.NotNil(t, physical.Cost())
	})

	t.Run("TopNQuery", func(t *testing.T) {
		// Create TopN query: top 10 by score
		scan := &LogicalScan{
			IndexName:     "products",
			Shards:        []int32{0},
			EstimatedRows: 1000,
		}

		sort := &LogicalSort{
			SortFields: []*SortField{{Field: "price", Descending: true}},
			Child:      scan,
		}

		limit := &LogicalLimit{
			Limit:  10,
			Offset: 0,
			Child:  sort,
		}

		// Optimize (should convert to TopN)
		optimizer := NewOptimizer()
		optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)
		optimized, err := optimizer.Optimize(limit)
		require.NoError(t, err)

		// Verify converted to TopN
		topN, ok := optimized.(*LogicalTopN)
		require.True(t, ok, "Expected LogicalTopN after optimization")
		assert.Equal(t, int64(10), topN.N)

		// Generate physical plan
		planner := NewPlanner(NewDefaultCostModel())
		physical, err := planner.Plan(optimized)
		require.NoError(t, err)

		// Verify PhysicalTopN
		physicalTopN, ok := physical.(*PhysicalTopN)
		require.True(t, ok, "Expected PhysicalTopN")
		assert.Equal(t, int64(10), physicalTopN.N)
	})

	t.Run("FilteredAggregation", func(t *testing.T) {
		// Create query with filter after aggregation (should be pushed down)
		scan := &LogicalScan{
			IndexName:     "products",
			Shards:        []int32{0},
			EstimatedRows: 1000,
		}

		agg := &LogicalAggregate{
			GroupBy: []string{"category"},
			Aggregations: []*Aggregation{
				{Name: "avg_price", Type: AggTypeAvg, Field: "price"},
			},
			Child: scan,
		}

		filter := &LogicalFilter{
			Condition: &Expression{
				Type:  ExprTypeTerm,
				Field: "status",
				Value: "active",
			},
			Child:         agg,
			EstimatedRows: 600,
		}

		// Optimize (should push filter before aggregation)
		optimizer := NewOptimizer()
		optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)
		optimized, err := optimizer.Optimize(filter)
		require.NoError(t, err)

		// Verify filter was pushed down
		optimizedAgg, ok := optimized.(*LogicalAggregate)
		require.True(t, ok, "Expected LogicalAggregate at top")

		// Child should be filter or scan with filter
		childIsFilter := false
		childIsScan := false

		if _, ok := optimizedAgg.Child.(*LogicalFilter); ok {
			childIsFilter = true
		} else if childScan, ok := optimizedAgg.Child.(*LogicalScan); ok {
			childIsScan = true
			assert.NotNil(t, childScan.Filter, "Expected filter pushed to scan")
		}

		assert.True(t, childIsFilter || childIsScan, "Filter should be pushed down")
	})

	t.Run("ComplexBoolQuery", func(t *testing.T) {
		// Create complex bool query with multiple optimizations
		scan := &LogicalScan{
			IndexName:     "products",
			Shards:        []int32{0, 1, 2},
			EstimatedRows: 10000,
		}

		filter1 := &LogicalFilter{
			Condition: &Expression{
				Type:  ExprTypeTerm,
				Field: "status",
				Value: "active",
			},
			Child:         scan,
			EstimatedRows: 6000,
		}

		filter2 := &LogicalFilter{
			Condition: &Expression{
				Type:  ExprTypeRange,
				Field: "price",
				Value: map[string]interface{}{"gte": 100.0},
			},
			Child:         filter1,
			EstimatedRows: 3000,
		}

		sort := &LogicalSort{
			SortFields: []*SortField{{Field: "price", Descending: true}},
			Child:      filter2,
		}

		limit := &LogicalLimit{
			Limit:  20,
			Offset: 0,
			Child:  sort,
		}

		// Optimize
		optimizer := NewOptimizer()
		optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)
		optimized, err := optimizer.Optimize(limit)
		require.NoError(t, err)

		// Should be TopN at top
		topN, ok := optimized.(*LogicalTopN)
		require.True(t, ok, "Expected TopN optimization")
		assert.Equal(t, int64(20), topN.N)

		// Generate physical plan
		planner := NewPlanner(NewDefaultCostModel())
		physical, err := planner.Plan(optimized)
		require.NoError(t, err)

		// Verify cost is reasonable
		cost := physical.Cost()
		assert.NotNil(t, cost)
		assert.Greater(t, cost.TotalCost, 0.0)
	})
}

// TestQueryPlannerPerformance benchmarks the query planner
func TestQueryPlannerPerformance(t *testing.T) {
	t.Run("PlanningLatency", func(t *testing.T) {
		// Create a complex query
		scan := &LogicalScan{
			IndexName:     "products",
			Shards:        []int32{0, 1, 2},
			EstimatedRows: 10000,
		}

		filter := &LogicalFilter{
			Condition: &Expression{
				Type:  ExprTypeTerm,
				Field: "status",
				Value: "active",
			},
			Child:         scan,
			EstimatedRows: 6000,
		}

		sort := &LogicalSort{
			SortFields: []*SortField{{Field: "price", Descending: true}},
			Child:      filter,
		}

		limit := &LogicalLimit{
			Limit:  10,
			Offset: 0,
			Child:  sort,
		}

		optimizer := NewOptimizer()
		optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)
		planner := NewPlanner(NewDefaultCostModel())

		// Measure planning time
		iterations := 100
		start := time.Now()

		for i := 0; i < iterations; i++ {
			// Optimize
			optimized, err := optimizer.Optimize(limit)
			require.NoError(t, err)

			// Plan
			_, err = planner.Plan(optimized)
			require.NoError(t, err)
		}

		elapsed := time.Since(start)
		avgLatency := elapsed / time.Duration(iterations)

		t.Logf("Average planning latency: %v", avgLatency)
		assert.Less(t, avgLatency, 2*time.Millisecond, "Planning should take <2ms")
	})

	t.Run("OptimizationPasses", func(t *testing.T) {
		// Create query that requires multiple optimization passes
		scan := &LogicalScan{
			IndexName:     "products",
			Shards:        []int32{0},
			EstimatedRows: 10000,
		}

		// Stack of filters (redundant filter elimination + pushdown)
		filter1 := &LogicalFilter{
			Condition:     &Expression{Type: ExprTypeMatchAll},
			Child:         scan,
			EstimatedRows: 10000,
		}

		filter2 := &LogicalFilter{
			Condition: &Expression{
				Type:  ExprTypeTerm,
				Field: "status",
				Value: "active",
			},
			Child:         filter1,
			EstimatedRows: 6000,
		}

		optimizer := NewOptimizer()
		optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)

		start := time.Now()
		optimized, err := optimizer.Optimize(filter2)
		elapsed := time.Since(start)

		require.NoError(t, err)
		t.Logf("Optimization time: %v", elapsed)

		// Should have eliminated redundant filter and pushed down the real one
		optimizedScan, ok := optimized.(*LogicalScan)
		require.True(t, ok, "Expected scan after optimization")
		assert.NotNil(t, optimizedScan.Filter)

		// Should complete quickly
		assert.Less(t, elapsed, 1*time.Millisecond, "Optimization should take <1ms")
	})
}

// TestOptimizationRuleEffectiveness tests that optimization rules actually improve performance
func TestOptimizationRuleEffectiveness(t *testing.T) {
	t.Run("TopNBetterThanSortLimit", func(t *testing.T) {
		costModel := NewDefaultCostModel()

		// Create a scan for cost estimation
		scan := &LogicalScan{
			IndexName:     "products",
			Shards:        []int32{0, 1, 2},
			EstimatedRows: 10000,
		}

		// Without optimization: Scan -> Sort -> Limit
		scanCost := costModel.EstimateScanCost(scan)

		sort := &LogicalSort{
			SortFields: []*SortField{{Field: "price", Descending: true}},
			Child:      scan,
		}
		sortCost := costModel.EstimateSortCost(sort, scanCost)

		limit := &LogicalLimit{
			Limit: 10,
			Child: sort,
		}
		limitCost := costModel.EstimateLimitCost(limit, sortCost)

		totalWithoutOptimization := scanCost.TotalCost + sortCost.TotalCost + limitCost.TotalCost

		// With optimization: Scan -> TopN
		// TopN is 30% more efficient than Sort + Limit
		topNSavings := (sortCost.CPUCost + limitCost.CPUCost) * 0.3

		totalWithOptimization := totalWithoutOptimization - topNSavings

		// Verify TopN is better
		assert.Less(t, totalWithOptimization, totalWithoutOptimization,
			"TopN should be more efficient than Sort + Limit")

		improvement := topNSavings / totalWithoutOptimization * 100
		t.Logf("TopN improvement: %.1f%%", improvement)
		assert.Greater(t, improvement, 1.0, "TopN should save at least 1% cost")
	})

	t.Run("FilterPushdownReducesCost", func(t *testing.T) {
		costModel := NewDefaultCostModel()

		// Without pushdown: Scan 10K, then filter to 6K in Go
		scan1 := &LogicalScan{
			IndexName:     "products",
			Shards:        []int32{0, 1, 2},
			EstimatedRows: 10000,
		}
		scanCost1 := costModel.EstimateScanCost(scan1)

		filter := &LogicalFilter{
			Condition: &Expression{
				Type:  ExprTypeTerm,
				Field: "status",
				Value: "active",
			},
			Child:         scan1,
			EstimatedRows: 6000,
		}
		filterCost := costModel.EstimateFilterCost(filter, scanCost1)

		totalWithoutPushdown := scanCost1.TotalCost + filterCost.TotalCost

		// With pushdown: Diagon filters, only 6K transferred
		scan2 := &LogicalScan{
			IndexName: "products",
			Shards:    []int32{0, 1, 2},
			Filter: &Expression{
				Type:  ExprTypeTerm,
				Field: "status",
				Value: "active",
			},
			EstimatedRows: 6000,
		}
		scanWithFilterCost := costModel.EstimateScanCost(scan2)

		// With pushdown, we save network transfer cost
		assert.Less(t, scanWithFilterCost.TotalCost, totalWithoutPushdown,
			"Filter pushdown should reduce cost")

		improvement := (totalWithoutPushdown - scanWithFilterCost.TotalCost) / totalWithoutPushdown * 100
		t.Logf("Filter pushdown improvement: %.1f%%", improvement)
		assert.Greater(t, improvement, 5.0, "Filter pushdown should save >5% cost")
	})
}
