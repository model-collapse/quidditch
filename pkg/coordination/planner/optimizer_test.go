package planner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterPushdownRule(t *testing.T) {
	// Create a filter over a scan
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

	// Apply filter pushdown rule
	rule := NewFilterPushdownRule()
	newPlan, applied := rule.Apply(filter)

	// Rule should apply
	assert.True(t, applied)
	require.NotNil(t, newPlan)

	// New plan should be a scan with filter
	newScan, ok := newPlan.(*LogicalScan)
	require.True(t, ok)
	assert.NotNil(t, newScan.Filter)
	assert.Equal(t, "category", newScan.Filter.Field)
}

func TestFilterPushdownDoesNotApplyToNonScan(t *testing.T) {
	// Create a filter over a project (not a scan)
	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	project := &LogicalProject{
		Fields: []string{"name", "price"},
		Child:  scan,
	}

	filter := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		Child:       project,
		EstimatedRows: 2000,
	}

	// Apply filter pushdown rule
	rule := NewFilterPushdownRule()
	_, applied := rule.Apply(filter)

	// Rule should not apply
	assert.False(t, applied)
}

func TestRedundantFilterElimination(t *testing.T) {
	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	// Create a filter with match_all (redundant)
	filter := &LogicalFilter{
		Condition: &Expression{
			Type: ExprTypeMatchAll,
		},
		Child:       scan,
		EstimatedRows: 10000,
	}

	// Apply redundant filter elimination rule
	rule := NewRedundantFilterEliminationRule()
	newPlan, applied := rule.Apply(filter)

	// Rule should apply and return the scan directly
	assert.True(t, applied)
	require.NotNil(t, newPlan)
	assert.Equal(t, scan, newPlan)
}

func TestProjectionMergingRule(t *testing.T) {
	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	// Create two consecutive projections
	project1 := &LogicalProject{
		Fields: []string{"name", "price", "category", "rating"},
		Child:  scan,
	}

	project2 := &LogicalProject{
		Fields: []string{"name", "price"}, // Subset of project1
		Child:  project1,
	}

	// Apply projection merging rule
	rule := NewProjectionMergingRule()
	newPlan, applied := rule.Apply(project2)

	// Rule should apply
	assert.True(t, applied)
	require.NotNil(t, newPlan)

	// New plan should be a single projection over scan
	merged, ok := newPlan.(*LogicalProject)
	require.True(t, ok)
	assert.Equal(t, scan, merged.Child)
	assert.Len(t, merged.Fields, 2)
	assert.Contains(t, merged.Fields, "name")
	assert.Contains(t, merged.Fields, "price")
}

func TestOptimizer(t *testing.T) {
	// Create a plan with optimization opportunities:
	// Filter (match_all) -> Filter (term) -> Scan

	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 10000,
	}

	filter1 := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		Child:       scan,
		EstimatedRows: 2000,
	}

	filter2 := &LogicalFilter{
		Condition: &Expression{
			Type: ExprTypeMatchAll,
		},
		Child:       filter1,
		EstimatedRows: 2000,
	}

	// Create optimizer with rules
	optimizer := NewOptimizer()
	optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)

	// Optimize the plan
	optimized, err := optimizer.Optimize(filter2)
	require.NoError(t, err)

	// After optimization:
	// 1. Redundant filter (match_all) should be eliminated
	// 2. Remaining filter should be pushed down to scan

	// Result should be a scan with filter
	optimizedScan, ok := optimized.(*LogicalScan)
	require.True(t, ok, "Expected optimized plan to be a scan")
	assert.NotNil(t, optimizedScan.Filter)
	assert.Equal(t, "category", optimizedScan.Filter.Field)
}

func TestOptimizerMaxPasses(t *testing.T) {
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

	optimizer := NewOptimizer()
	optimizer.MaxPasses = 1 // Only one pass
	optimizer.RuleSet = NewRuleSet(NewFilterPushdownRule())

	optimized, err := optimizer.Optimize(filter)
	require.NoError(t, err)
	assert.NotNil(t, optimized)
}

func TestRuleSetAddRule(t *testing.T) {
	ruleSet := NewRuleSet()
	assert.Len(t, ruleSet.Rules, 0)

	ruleSet.AddRule(NewFilterPushdownRule())
	assert.Len(t, ruleSet.Rules, 1)

	ruleSet.AddRule(NewRedundantFilterEliminationRule())
	assert.Len(t, ruleSet.Rules, 2)
}

func TestGetDefaultRules(t *testing.T) {
	rules := GetDefaultRules()

	// Should have multiple rules
	assert.Greater(t, len(rules), 0)

	// Verify rule names
	ruleNames := make(map[string]bool)
	for _, rule := range rules {
		ruleNames[rule.Name()] = true
	}

	assert.True(t, ruleNames["FilterPushdown"])
	assert.True(t, ruleNames["RedundantFilterElimination"])
	assert.True(t, ruleNames["ProjectionMerging"])
}

func TestRulePriority(t *testing.T) {
	rule1 := NewFilterPushdownRule()
	rule2 := NewProjectionPushdownRule()
	rule3 := NewLimitPushdownRule()

	// Filter pushdown should have higher priority
	assert.Greater(t, rule1.Priority(), rule2.Priority())
	assert.Greater(t, rule2.Priority(), rule3.Priority())
}

func TestComplexOptimization(t *testing.T) {
	// Create a complex plan with multiple optimization opportunities:
	// Project -> Filter (match_all) -> Filter (term) -> Scan

	scan := &LogicalScan{
		IndexName:   "products",
		Shards:      []int32{0},
		EstimatedRows: 100000,
	}

	filter1 := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		Child:       scan,
		EstimatedRows: 20000,
	}

	filter2 := &LogicalFilter{
		Condition: &Expression{
			Type: ExprTypeMatchAll,
		},
		Child:       filter1,
		EstimatedRows: 20000,
	}

	project := &LogicalProject{
		Fields: []string{"name", "price"},
		Child:  filter2,
	}

	// Optimize
	optimizer := NewOptimizer()
	optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)

	optimized, err := optimizer.Optimize(project)
	require.NoError(t, err)

	// After optimization:
	// - Redundant filter should be eliminated
	// - Remaining filter should be pushed to scan
	// - Project should remain on top

	// Top should be project
	optimizedProject, ok := optimized.(*LogicalProject)
	require.True(t, ok)

	// Child of project should be scan (filter pushed down)
	optimizedScan, ok := optimizedProject.Child.(*LogicalScan)
	require.True(t, ok)
	assert.NotNil(t, optimizedScan.Filter)
}

func TestTopNOptimizationRule(t *testing.T) {
	// Create a plan: Limit -> Sort -> Scan
	scan := &LogicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedRows: 10000,
	}

	sort := &LogicalSort{
		SortFields: []*SortField{
			{Field: "price", Descending: true},
		},
		Child: scan,
	}

	limit := &LogicalLimit{
		Limit:  10,
		Offset: 0,
		Child:  sort,
	}

	// Apply TopN optimization rule
	rule := NewTopNOptimizationRule()
	newPlan, applied := rule.Apply(limit)

	// Rule should apply
	assert.True(t, applied)
	require.NotNil(t, newPlan)

	// New plan should be TopN
	topN, ok := newPlan.(*LogicalTopN)
	require.True(t, ok)
	assert.Equal(t, int64(10), topN.N)
	assert.Equal(t, int64(0), topN.Offset)
	assert.Len(t, topN.SortFields, 1)
	assert.Equal(t, "price", topN.SortFields[0].Field)

	// TopN should directly wrap the scan (sort eliminated)
	_, ok = topN.Child.(*LogicalScan)
	require.True(t, ok)
}

func TestTopNOptimizationRuleWithOffset(t *testing.T) {
	// Create a plan with pagination: Limit(offset=20) -> Sort -> Scan
	scan := &LogicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedRows: 10000,
	}

	sort := &LogicalSort{
		SortFields: []*SortField{
			{Field: "name", Descending: false},
			{Field: "price", Descending: true},
		},
		Child: scan,
	}

	limit := &LogicalLimit{
		Limit:  50,
		Offset: 20,
		Child:  sort,
	}

	// Apply TopN optimization rule
	rule := NewTopNOptimizationRule()
	newPlan, applied := rule.Apply(limit)

	// Rule should apply
	assert.True(t, applied)
	require.NotNil(t, newPlan)

	// New plan should be TopN with correct offset
	topN, ok := newPlan.(*LogicalTopN)
	require.True(t, ok)
	assert.Equal(t, int64(50), topN.N)
	assert.Equal(t, int64(20), topN.Offset)
	assert.Len(t, topN.SortFields, 2)
}

func TestTopNOptimizationRuleDoesNotApplyWithoutSort(t *testing.T) {
	// Create a plan: Limit -> Scan (no sort)
	scan := &LogicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedRows: 10000,
	}

	limit := &LogicalLimit{
		Limit:  10,
		Offset: 0,
		Child:  scan,
	}

	// Apply TopN optimization rule
	rule := NewTopNOptimizationRule()
	newPlan, applied := rule.Apply(limit)

	// Rule should NOT apply (no sort to optimize)
	assert.False(t, applied)
	assert.Nil(t, newPlan)
}

func TestTopNOptimizationInFullPipeline(t *testing.T) {
	// Create a plan: Limit -> Sort -> Filter -> Scan
	scan := &LogicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedRows: 10000,
	}

	filter := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		Child:         scan,
		EstimatedRows: 2000,
	}

	sort := &LogicalSort{
		SortFields: []*SortField{
			{Field: "price", Descending: true},
		},
		Child: filter,
	}

	limit := &LogicalLimit{
		Limit:  10,
		Offset: 0,
		Child:  sort,
	}

	// Optimize with full rule set
	optimizer := NewOptimizer()
	optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)

	optimized, err := optimizer.Optimize(limit)
	require.NoError(t, err)

	// Should be TopN at top
	topN, ok := optimized.(*LogicalTopN)
	require.True(t, ok)
	assert.Equal(t, int64(10), topN.N)

	// Child should be scan with filter pushed down
	optimizedScan, ok := topN.Child.(*LogicalScan)
	require.True(t, ok)
	assert.NotNil(t, optimizedScan.Filter)
}

func TestPredicatePushdownForAggregationsRule(t *testing.T) {
	// Create a plan: Filter -> Aggregate -> Scan
	scan := &LogicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
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
		EstimatedRows: 5000,
	}

	// Apply predicate pushdown for aggregations rule
	rule := NewPredicatePushdownForAggregationsRule()
	newPlan, applied := rule.Apply(filter)

	// Rule should apply
	assert.True(t, applied)
	require.NotNil(t, newPlan)

	// New plan should be Aggregate at top
	newAgg, ok := newPlan.(*LogicalAggregate)
	require.True(t, ok)

	// Aggregate's child should now be Filter
	newFilter, ok := newAgg.Child.(*LogicalFilter)
	require.True(t, ok)
	assert.Equal(t, "status", newFilter.Condition.Field)

	// Filter's child should be the original scan
	_, ok = newFilter.Child.(*LogicalScan)
	require.True(t, ok)
}

func TestPredicatePushdownForAggregationsDoesNotApplyWithoutAggregate(t *testing.T) {
	// Create a plan: Filter -> Scan (no aggregate)
	scan := &LogicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedRows: 10000,
	}

	filter := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "status",
			Value: "active",
		},
		Child:         scan,
		EstimatedRows: 5000,
	}

	// Apply predicate pushdown for aggregations rule
	rule := NewPredicatePushdownForAggregationsRule()
	newPlan, applied := rule.Apply(filter)

	// Rule should NOT apply (no aggregate to push through)
	assert.False(t, applied)
	assert.Nil(t, newPlan)
}

func TestPredicatePushdownForAggregationsInFullPipeline(t *testing.T) {
	// Create a complex plan: Filter -> Aggregate -> Filter -> Scan
	scan := &LogicalScan{
		IndexName:     "products",
		Shards:        []int32{0},
		EstimatedRows: 10000,
	}

	// Filter 1: Before aggregation
	filter1 := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "status",
			Value: "active",
		},
		Child:         scan,
		EstimatedRows: 8000,
	}

	// Aggregation
	agg := &LogicalAggregate{
		GroupBy: []string{"category"},
		Aggregations: []*Aggregation{
			{
				Name:  "count",
				Type:  AggTypeCount,
				Field: "*",
			},
		},
		Child: filter1,
	}

	// Filter 2: After aggregation (should be pushed down)
	filter2 := &LogicalFilter{
		Condition: &Expression{
			Type:  ExprTypeTerm,
			Field: "category",
			Value: "electronics",
		},
		Child:         agg,
		EstimatedRows: 2000,
	}

	// Optimize with full rule set
	optimizer := NewOptimizer()
	optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)

	optimized, err := optimizer.Optimize(filter2)
	require.NoError(t, err)

	// Should be Aggregate at top
	optimizedAgg, ok := optimized.(*LogicalAggregate)
	require.True(t, ok)

	// Child should have filters pushed to scan
	optimizedScan, ok := optimizedAgg.Child.(*LogicalScan)
	require.True(t, ok)
	assert.NotNil(t, optimizedScan.Filter)
}
