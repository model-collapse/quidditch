package planner

// Rule represents an optimization rule that transforms a logical plan
type Rule interface {
	// Name returns the rule name
	Name() string

	// Apply attempts to apply the rule to a plan node
	// Returns a new plan if the rule applies, nil otherwise
	Apply(plan LogicalPlan) (LogicalPlan, bool)

	// Priority returns the rule priority (higher = applied first)
	Priority() int
}

// RuleSet represents a collection of optimization rules
type RuleSet struct {
	Rules []Rule
}

// NewRuleSet creates a new rule set with the given rules
func NewRuleSet(rules ...Rule) *RuleSet {
	return &RuleSet{Rules: rules}
}

// AddRule adds a rule to the rule set
func (rs *RuleSet) AddRule(rule Rule) {
	rs.Rules = append(rs.Rules, rule)
}

// Optimizer applies optimization rules to a logical plan
type Optimizer struct {
	RuleSet    *RuleSet
	MaxPasses  int  // Maximum optimization passes
	CostBased  bool // Enable cost-based optimization
}

// NewOptimizer creates a new optimizer with default rules
func NewOptimizer() *Optimizer {
	return &Optimizer{
		RuleSet:   NewRuleSet(),
		MaxPasses: 10,
		CostBased: true,
	}
}

// Optimize applies optimization rules to a logical plan
func (o *Optimizer) Optimize(plan LogicalPlan) (LogicalPlan, error) {
	// Apply rules iteratively until no more changes or max passes reached
	current := plan
	for pass := 0; pass < o.MaxPasses; pass++ {
		changed := false

		// Recursively optimize the tree
		newPlan, didChange := o.optimizeNode(current)
		if didChange {
			current = newPlan
			changed = true
		}

		// If no rules applied, we're done
		if !changed {
			break
		}
	}

	// Apply cost-based optimization if enabled
	if o.CostBased {
		current = o.applyCostBasedOptimization(current)
	}

	return current, nil
}

// optimizeNode recursively optimizes a single node and its children
func (o *Optimizer) optimizeNode(plan LogicalPlan) (LogicalPlan, bool) {
	// First, try to apply rules to this node
	current := plan
	ruleChanged := false
	for _, rule := range o.RuleSet.Rules {
		newPlan, applied := rule.Apply(current)
		if applied {
			current = newPlan
			ruleChanged = true
			break // Apply one rule at a time, then restart with the new plan
		}
	}

	// If a rule was applied, recursively optimize the resulting plan
	if ruleChanged {
		// Recursively optimize the new plan
		newPlan, _ := o.optimizeNode(current)
		return newPlan, true
	}

	// Otherwise, recursively optimize children
	children := current.Children()
	childChanged := false
	for i, child := range children {
		newChild, changed := o.optimizeNode(child)
		if changed {
			current.SetChild(i, newChild)
			childChanged = true
		}
	}

	return current, childChanged
}

// applyCostBasedOptimization applies cost-based optimization
func (o *Optimizer) applyCostBasedOptimization(plan LogicalPlan) LogicalPlan {
	// TODO: Implement cost-based optimization
	// For now, just return the plan as-is
	return plan
}

// BaseRule provides common functionality for rules
type BaseRule struct {
	name     string
	priority int
}

func (r *BaseRule) Name() string     { return r.name }
func (r *BaseRule) Priority() int    { return r.priority }

// Common optimization rules

// FilterPushdownRule pushes filters down to scan nodes
type FilterPushdownRule struct {
	BaseRule
}

func NewFilterPushdownRule() *FilterPushdownRule {
	return &FilterPushdownRule{
		BaseRule: BaseRule{
			name:     "FilterPushdown",
			priority: 100,
		},
	}
}

func (r *FilterPushdownRule) Apply(plan LogicalPlan) (LogicalPlan, bool) {
	// If this is a filter over a scan, push the filter into the scan
	filter, ok := plan.(*LogicalFilter)
	if !ok {
		return nil, false
	}

	scan, ok := filter.Child.(*LogicalScan)
	if !ok {
		return nil, false
	}

	// Push filter into scan
	newScan := &LogicalScan{
		IndexName:     scan.IndexName,
		Shards:        scan.Shards,
		Filter:        r.combineFilters(scan.Filter, filter.Condition),
		EstimatedRows: filter.EstimatedRows,
	}

	return newScan, true
}

func (r *FilterPushdownRule) combineFilters(f1, f2 *Expression) *Expression {
	if f1 == nil {
		return f2
	}
	if f2 == nil {
		return f1
	}

	// Combine with AND
	return &Expression{
		Type:     ExprTypeBool,
		Children: []*Expression{f1, f2},
	}
}

// ProjectionPushdownRule pushes projections down to scan nodes
type ProjectionPushdownRule struct {
	BaseRule
}

func NewProjectionPushdownRule() *ProjectionPushdownRule {
	return &ProjectionPushdownRule{
		BaseRule: BaseRule{
			name:     "ProjectionPushdown",
			priority: 90,
		},
	}
}

func (r *ProjectionPushdownRule) Apply(plan LogicalPlan) (LogicalPlan, bool) {
	// TODO: Fix infinite recursion bug before enabling
	// Problem: Returning Project -> Scan matches the rule pattern again,
	// causing infinite loop in optimizer.
	//
	// Proper solution: Either:
	// 1. Return just the Scan with projected fields (eliminate Project), or
	// 2. Add "ProjectedFields" to LogicalScan and mark projection as pushed
	// 3. Add a flag to prevent re-application
	//
	// For now, disabled to allow other optimizations to work.
	return nil, false
}

// LimitPushdownRule pushes limits down to scan nodes
type LimitPushdownRule struct {
	BaseRule
}

func NewLimitPushdownRule() *LimitPushdownRule {
	return &LimitPushdownRule{
		BaseRule: BaseRule{
			name:     "LimitPushdown",
			priority: 80,
		},
	}
}

func (r *LimitPushdownRule) Apply(plan LogicalPlan) (LogicalPlan, bool) {
	// TODO: Implement limit pushdown
	return nil, false
}

// RedundantFilterEliminationRule removes redundant filters
type RedundantFilterEliminationRule struct {
	BaseRule
}

func NewRedundantFilterEliminationRule() *RedundantFilterEliminationRule {
	return &RedundantFilterEliminationRule{
		BaseRule: BaseRule{
			name:     "RedundantFilterElimination",
			priority: 70,
		},
	}
}

func (r *RedundantFilterEliminationRule) Apply(plan LogicalPlan) (LogicalPlan, bool) {
	// If this is a filter with a trivial condition (e.g., match_all), remove it
	filter, ok := plan.(*LogicalFilter)
	if !ok {
		return nil, false
	}

	if filter.Condition != nil && filter.Condition.Type == ExprTypeMatchAll {
		return filter.Child, true
	}

	return nil, false
}

// ProjectionMergingRule merges consecutive projections
type ProjectionMergingRule struct {
	BaseRule
}

func NewProjectionMergingRule() *ProjectionMergingRule {
	return &ProjectionMergingRule{
		BaseRule: BaseRule{
			name:     "ProjectionMerging",
			priority: 60,
		},
	}
}

func (r *ProjectionMergingRule) Apply(plan LogicalPlan) (LogicalPlan, bool) {
	// If this is a projection over another projection, merge them
	proj1, ok := plan.(*LogicalProject)
	if !ok {
		return nil, false
	}

	proj2, ok := proj1.Child.(*LogicalProject)
	if !ok {
		return nil, false
	}

	// Merge: only keep fields from proj1 that exist in proj2
	mergedFields := []string{}
	for _, field := range proj1.Fields {
		for _, childField := range proj2.Fields {
			if field == childField {
				mergedFields = append(mergedFields, field)
				break
			}
		}
	}

	return &LogicalProject{
		Fields:       mergedFields,
		Child:        proj2.Child,
		OutputSchema: proj1.OutputSchema,
	}, true
}

// TopNOptimizationRule combines limit + sort into a single TopN operator
type TopNOptimizationRule struct {
	BaseRule
}

func NewTopNOptimizationRule() *TopNOptimizationRule {
	return &TopNOptimizationRule{
		BaseRule: BaseRule{
			name:     "TopNOptimization",
			priority: 85, // Higher than limit pushdown
		},
	}
}

func (r *TopNOptimizationRule) Apply(plan LogicalPlan) (LogicalPlan, bool) {
	// Pattern 1: Limit -> Sort => TopN
	limit, ok := plan.(*LogicalLimit)
	if !ok {
		return nil, false
	}

	sort, ok := limit.Child.(*LogicalSort)
	if !ok {
		return nil, false
	}

	// Combine into TopN
	topN := &LogicalTopN{
		N:          limit.Limit,
		Offset:     limit.Offset,
		SortFields: sort.SortFields,
		Child:      sort.Child,
	}

	return topN, true
}

// PredicatePushdownForAggregationsRule pushes filters into aggregation nodes
type PredicatePushdownForAggregationsRule struct {
	BaseRule
}

func NewPredicatePushdownForAggregationsRule() *PredicatePushdownForAggregationsRule {
	return &PredicatePushdownForAggregationsRule{
		BaseRule: BaseRule{
			name:     "PredicatePushdownForAggregations",
			priority: 75,
		},
	}
}

func (r *PredicatePushdownForAggregationsRule) Apply(plan LogicalPlan) (LogicalPlan, bool) {
	// Pattern: Filter -> Aggregate => Aggregate with filter pushed to child
	filter, ok := plan.(*LogicalFilter)
	if !ok {
		return nil, false
	}

	agg, ok := filter.Child.(*LogicalAggregate)
	if !ok {
		return nil, false
	}

	// Push filter below aggregation to reduce rows before aggregating
	newFilter := &LogicalFilter{
		Condition: filter.Condition,
		Child:     agg.Child,
		EstimatedRows: filter.EstimatedRows,
	}

	newAgg := &LogicalAggregate{
		GroupBy:      agg.GroupBy,
		Aggregations: agg.Aggregations,
		Child:        newFilter,
	}

	return newAgg, true
}

// GetDefaultRules returns the default set of optimization rules
func GetDefaultRules() []Rule {
	return []Rule{
		NewFilterPushdownRule(),
		NewTopNOptimizationRule(),
		NewProjectionPushdownRule(),
		NewLimitPushdownRule(),
		NewPredicatePushdownForAggregationsRule(),
		NewRedundantFilterEliminationRule(),
		NewProjectionMergingRule(),
	}
}
