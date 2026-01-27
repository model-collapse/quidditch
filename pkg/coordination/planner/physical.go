package planner

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// PhysicalPlanType represents the type of physical plan node
type PhysicalPlanType string

const (
	PhysicalPlanTypeScan           PhysicalPlanType = "scan"
	PhysicalPlanTypeFilter         PhysicalPlanType = "filter"
	PhysicalPlanTypeProject        PhysicalPlanType = "project"
	PhysicalPlanTypeAggregate      PhysicalPlanType = "aggregate"
	PhysicalPlanTypeSort           PhysicalPlanType = "sort"
	PhysicalPlanTypeLimit          PhysicalPlanType = "limit"
	PhysicalPlanTypeTopN           PhysicalPlanType = "topn"
	PhysicalPlanTypeHashAggregate  PhysicalPlanType = "hash_aggregate"
	PhysicalPlanTypeIndexScan      PhysicalPlanType = "index_scan"
)

// PhysicalPlan represents a physical query plan node (executable)
type PhysicalPlan interface {
	// Type returns the physical plan node type
	Type() PhysicalPlanType

	// Children returns child plan nodes
	Children() []PhysicalPlan

	// Schema returns the output schema
	Schema() *Schema

	// Cost returns the estimated execution cost
	Cost() *Cost

	// Execute executes the physical plan and returns results
	Execute(ctx context.Context) (*ExecutionResult, error)

	// String returns a human-readable representation
	String() string
}

// ExecutionResult represents the result of executing a physical plan
type ExecutionResult struct {
	Rows         []map[string]interface{} // Result rows
	TotalHits    int64                    // Total number of matching documents
	MaxScore     float64                  // Maximum relevance score
	Aggregations map[string]*AggregationResult // Aggregation results
	TookMillis   int64                    // Execution time in milliseconds
}

// AggregationResult represents the result of an aggregation
type AggregationResult struct {
	Type    AggregationType
	Buckets []*Bucket // For terms, histogram, etc.
	Value   float64   // For single-value aggregations (sum, avg, etc.)
	Stats   *Stats    // For stats aggregations
}

// Bucket represents a bucket in a bucketing aggregation
type Bucket struct {
	Key      interface{} // Bucket key
	DocCount int64       // Number of documents in this bucket
	SubAggs  map[string]*AggregationResult // Sub-aggregations
}

// Stats represents statistics for a field
type Stats struct {
	Count int64
	Min   float64
	Max   float64
	Avg   float64
	Sum   float64
}

// PhysicalScan represents a physical scan operation
type PhysicalScan struct {
	IndexName   string
	Shards      []int32
	Filter      *Expression
	Fields      []string // Fields to retrieve (projection)
	OutputSchema *Schema
	EstimatedCost *Cost
}

func (s *PhysicalScan) Type() PhysicalPlanType      { return PhysicalPlanTypeScan }
func (s *PhysicalScan) Children() []PhysicalPlan    { return nil }
func (s *PhysicalScan) Schema() *Schema             { return s.OutputSchema }
func (s *PhysicalScan) Cost() *Cost                 { return s.EstimatedCost }
func (s *PhysicalScan) Execute(ctx context.Context) (*ExecutionResult, error) {
	// Get execution context
	execCtx, err := GetExecutionContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get execution context: %w", err)
	}

	if execCtx.Logger != nil {
		execCtx.Logger.Info("==> PhysicalScan.Execute ENTRY",
			zap.String("index", s.IndexName),
			zap.Int("num_shards", len(s.Shards)),
			zap.Bool("has_filter", s.Filter != nil))
	}

	// Convert filter expression to JSON query
	// If no filter, use match_all query
	var queryBytes []byte
	if s.Filter == nil {
		// Use match_all query when no filter is present
		queryBytes = []byte(`{"match_all":{}}`)
	} else {
		queryBytes, err = expressionToJSON(s.Filter)
		if err != nil {
			return nil, fmt.Errorf("failed to convert filter to JSON: %w", err)
		}
	}

	if execCtx.Logger != nil {
		execCtx.Logger.Info("Calling QueryExecutor.ExecuteSearch",
			zap.String("index", s.IndexName),
			zap.String("query", string(queryBytes)))
	}

	// Execute distributed search via QueryExecutor
	// Note: QueryExecutor handles pagination internally, but for scan we want all results
	executorResult, err := execCtx.QueryExecutor.ExecuteSearch(
		ctx,
		s.IndexName,
		queryBytes,
		nil, // filterExpression (separate from query)
		0,   // from
		10000, // size (large enough to get all results for this node)
	)
	if err != nil {
		if execCtx.Logger != nil {
			execCtx.Logger.Error("QueryExecutor.ExecuteSearch FAILED", zap.Error(err))
		}
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	if execCtx.Logger != nil {
		execCtx.Logger.Info("QueryExecutor.ExecuteSearch SUCCESS",
			zap.Int64("total_hits", executorResult.TotalHits),
			zap.Int("hits_returned", len(executorResult.Hits)))
	}

	// Convert executor result to execution result
	return convertExecutorResultToExecution(executorResult), nil
}
func (s *PhysicalScan) String() string {
	return fmt.Sprintf("PhysicalScan(index=%s, shards=%v, filter=%v)", s.IndexName, s.Shards, s.Filter)
}

// PhysicalFilter represents a physical filter operation
type PhysicalFilter struct {
	Condition   *Expression
	Child       PhysicalPlan
	OutputSchema *Schema
	EstimatedCost *Cost
}

func (f *PhysicalFilter) Type() PhysicalPlanType   { return PhysicalPlanTypeFilter }
func (f *PhysicalFilter) Children() []PhysicalPlan { return []PhysicalPlan{f.Child} }
func (f *PhysicalFilter) Schema() *Schema          { return f.OutputSchema }
func (f *PhysicalFilter) Cost() *Cost              { return f.EstimatedCost }
func (f *PhysicalFilter) Execute(ctx context.Context) (*ExecutionResult, error) {
	// Execute child and filter results
	childResult, err := f.Child.Execute(ctx)
	if err != nil {
		return nil, err
	}

	// Apply filter to child results (client-side filtering)
	// Note: This is typically a fallback; most filtering should be pushed to scan
	childResult.Rows = applyFilterToRows(childResult.Rows, f.Condition)
	// Note: TotalHits is preserved from child (represents global matches, not filtered subset)

	return childResult, nil
}
func (f *PhysicalFilter) String() string {
	return fmt.Sprintf("PhysicalFilter(condition=%v)", f.Condition)
}

// PhysicalProject represents a physical projection operation
type PhysicalProject struct {
	Fields       []string
	Child        PhysicalPlan
	OutputSchema *Schema
	EstimatedCost *Cost
}

func (p *PhysicalProject) Type() PhysicalPlanType   { return PhysicalPlanTypeProject }
func (p *PhysicalProject) Children() []PhysicalPlan { return []PhysicalPlan{p.Child} }
func (p *PhysicalProject) Schema() *Schema          { return p.OutputSchema }
func (p *PhysicalProject) Cost() *Cost              { return p.EstimatedCost }
func (p *PhysicalProject) Execute(ctx context.Context) (*ExecutionResult, error) {
	// Execute child and project results
	childResult, err := p.Child.Execute(ctx)
	if err != nil {
		return nil, err
	}

	// Apply projection to child results (field selection)
	childResult.Rows = applyProjectionToRows(childResult.Rows, p.Fields)

	return childResult, nil
}
func (p *PhysicalProject) String() string {
	return fmt.Sprintf("PhysicalProject(fields=%v)", p.Fields)
}

// PhysicalAggregate represents a physical aggregation operation
type PhysicalAggregate struct {
	GroupBy       []string
	Aggregations  []*Aggregation
	Child         PhysicalPlan
	OutputSchema  *Schema
	EstimatedCost *Cost
}

func (a *PhysicalAggregate) Type() PhysicalPlanType   { return PhysicalPlanTypeAggregate }
func (a *PhysicalAggregate) Children() []PhysicalPlan { return []PhysicalPlan{a.Child} }
func (a *PhysicalAggregate) Schema() *Schema          { return a.OutputSchema }
func (a *PhysicalAggregate) Cost() *Cost              { return a.EstimatedCost }
func (a *PhysicalAggregate) Execute(ctx context.Context) (*ExecutionResult, error) {
	// Execute child and aggregate results
	childResult, err := a.Child.Execute(ctx)
	if err != nil {
		return nil, err
	}

	// Aggregations are computed by the scan/distributed query executor
	// This node just passes them through (they're already in childResult.Aggregations)
	// For post-processing aggregations, we would compute them here from childResult.Rows

	return childResult, nil
}
func (a *PhysicalAggregate) String() string {
	return fmt.Sprintf("PhysicalAggregate(groupBy=%v, aggs=%d)", a.GroupBy, len(a.Aggregations))
}

// PhysicalHashAggregate represents a hash-based aggregation (more efficient for many groups)
type PhysicalHashAggregate struct {
	GroupBy       []string
	Aggregations  []*Aggregation
	Child         PhysicalPlan
	OutputSchema  *Schema
	EstimatedCost *Cost
}

func (a *PhysicalHashAggregate) Type() PhysicalPlanType   { return PhysicalPlanTypeHashAggregate }
func (a *PhysicalHashAggregate) Children() []PhysicalPlan { return []PhysicalPlan{a.Child} }
func (a *PhysicalHashAggregate) Schema() *Schema          { return a.OutputSchema }
func (a *PhysicalHashAggregate) Cost() *Cost              { return a.EstimatedCost }
func (a *PhysicalHashAggregate) Execute(ctx context.Context) (*ExecutionResult, error) {
	// Execute child and aggregate results using hash table
	childResult, err := a.Child.Execute(ctx)
	if err != nil {
		return nil, err
	}

	// Aggregations are computed by the scan/distributed query executor
	// This node just passes them through (they're already in childResult.Aggregations)
	// Hash aggregate is used for efficiency, but the aggregation merge is handled by QueryExecutor

	return childResult, nil
}
func (a *PhysicalHashAggregate) String() string {
	return fmt.Sprintf("PhysicalHashAggregate(groupBy=%v, aggs=%d)", a.GroupBy, len(a.Aggregations))
}

// PhysicalSort represents a physical sort operation
type PhysicalSort struct {
	SortFields    []*SortField
	Child         PhysicalPlan
	OutputSchema  *Schema
	EstimatedCost *Cost
}

func (s *PhysicalSort) Type() PhysicalPlanType   { return PhysicalPlanTypeSort }
func (s *PhysicalSort) Children() []PhysicalPlan { return []PhysicalPlan{s.Child} }
func (s *PhysicalSort) Schema() *Schema          { return s.OutputSchema }
func (s *PhysicalSort) Cost() *Cost              { return s.EstimatedCost }
func (s *PhysicalSort) Execute(ctx context.Context) (*ExecutionResult, error) {
	// Execute child and sort results
	childResult, err := s.Child.Execute(ctx)
	if err != nil {
		return nil, err
	}

	// Apply sorting to result rows
	childResult.Rows = sortRows(childResult.Rows, s.SortFields)

	return childResult, nil
}
func (s *PhysicalSort) String() string {
	return fmt.Sprintf("PhysicalSort(fields=%d)", len(s.SortFields))
}

// PhysicalLimit represents a physical limit operation
type PhysicalLimit struct {
	Offset        int64
	Limit         int64
	Child         PhysicalPlan
	OutputSchema  *Schema
	EstimatedCost *Cost
}

func (l *PhysicalLimit) Type() PhysicalPlanType   { return PhysicalPlanTypeLimit }
func (l *PhysicalLimit) Children() []PhysicalPlan { return []PhysicalPlan{l.Child} }
func (l *PhysicalLimit) Schema() *Schema          { return l.OutputSchema }
func (l *PhysicalLimit) Cost() *Cost              { return l.EstimatedCost }
func (l *PhysicalLimit) Execute(ctx context.Context) (*ExecutionResult, error) {
	// Execute child and apply limit
	childResult, err := l.Child.Execute(ctx)
	if err != nil {
		return nil, err
	}

	// Apply limit and offset to result rows
	childResult.Rows = applyLimitToRows(childResult.Rows, l.Offset, l.Limit)

	return childResult, nil
}
func (l *PhysicalLimit) String() string {
	return fmt.Sprintf("PhysicalLimit(offset=%d, limit=%d)", l.Offset, l.Limit)
}

// PhysicalTopN represents a physical TopN operation (optimized limit + sort)
// Uses a heap to efficiently maintain top N elements
type PhysicalTopN struct {
	N             int64
	Offset        int64
	SortFields    []*SortField
	Child         PhysicalPlan
	OutputSchema  *Schema
	EstimatedCost *Cost
}

func (t *PhysicalTopN) Type() PhysicalPlanType   { return PhysicalPlanTypeTopN }
func (t *PhysicalTopN) Children() []PhysicalPlan { return []PhysicalPlan{t.Child} }
func (t *PhysicalTopN) Schema() *Schema          { return t.OutputSchema }
func (t *PhysicalTopN) Cost() *Cost              { return t.EstimatedCost }
func (t *PhysicalTopN) Execute(ctx context.Context) (*ExecutionResult, error) {
	// Execute child
	childResult, err := t.Child.Execute(ctx)
	if err != nil {
		return nil, err
	}

	// Sort all rows first (in a real implementation, we'd use a heap for top-N)
	childResult.Rows = sortRows(childResult.Rows, t.SortFields)

	// Apply limit and offset
	childResult.Rows = applyLimitToRows(childResult.Rows, t.Offset, t.N)

	return childResult, nil
}
func (t *PhysicalTopN) String() string {
	return fmt.Sprintf("PhysicalTopN(n=%d, offset=%d, fields=%d)", t.N, t.Offset, len(t.SortFields))
}

// Planner converts a logical plan to a physical plan
type Planner struct {
	CostModel *CostModel
}

// NewPlanner creates a new planner
func NewPlanner(costModel *CostModel) *Planner {
	return &Planner{
		CostModel: costModel,
	}
}

// Plan converts a logical plan to a physical plan
func (p *Planner) Plan(logical LogicalPlan) (PhysicalPlan, error) {
	switch node := logical.(type) {
	case *LogicalScan:
		return p.planScan(node)
	case *LogicalFilter:
		return p.planFilter(node)
	case *LogicalProject:
		return p.planProject(node)
	case *LogicalAggregate:
		return p.planAggregate(node)
	case *LogicalSort:
		return p.planSort(node)
	case *LogicalLimit:
		return p.planLimit(node)
	case *LogicalTopN:
		return p.planTopN(node)
	default:
		return nil, fmt.Errorf("unsupported logical plan type: %T", node)
	}
}

func (p *Planner) planScan(logical *LogicalScan) (PhysicalPlan, error) {
	cost := p.CostModel.EstimateScanCost(logical)
	return &PhysicalScan{
		IndexName:     logical.IndexName,
		Shards:        logical.Shards,
		Filter:        logical.Filter,
		Fields:        []string{}, // TODO: Get from projection
		OutputSchema:  logical.Schema(),
		EstimatedCost: cost,
	}, nil
}

func (p *Planner) planFilter(logical *LogicalFilter) (PhysicalPlan, error) {
	child, err := p.Plan(logical.Child)
	if err != nil {
		return nil, err
	}

	cost := p.CostModel.EstimateFilterCost(logical, child.Cost())
	return &PhysicalFilter{
		Condition:     logical.Condition,
		Child:         child,
		OutputSchema:  logical.Schema(),
		EstimatedCost: cost,
	}, nil
}

func (p *Planner) planProject(logical *LogicalProject) (PhysicalPlan, error) {
	child, err := p.Plan(logical.Child)
	if err != nil {
		return nil, err
	}

	cost := p.CostModel.EstimateProjectCost(logical, child.Cost())
	return &PhysicalProject{
		Fields:        logical.Fields,
		Child:         child,
		OutputSchema:  logical.OutputSchema,
		EstimatedCost: cost,
	}, nil
}

func (p *Planner) planAggregate(logical *LogicalAggregate) (PhysicalPlan, error) {
	child, err := p.Plan(logical.Child)
	if err != nil {
		return nil, err
	}

	// Choose between hash aggregate and regular aggregate based on cardinality
	cost := p.CostModel.EstimateAggregateCost(logical, child.Cost())

	if logical.Child.Cardinality() > 1000 {
		// Use hash aggregate for large datasets
		return &PhysicalHashAggregate{
			GroupBy:       logical.GroupBy,
			Aggregations:  logical.Aggregations,
			Child:         child,
			OutputSchema:  logical.OutputSchema,
			EstimatedCost: cost,
		}, nil
	}

	return &PhysicalAggregate{
		GroupBy:       logical.GroupBy,
		Aggregations:  logical.Aggregations,
		Child:         child,
		OutputSchema:  logical.OutputSchema,
		EstimatedCost: cost,
	}, nil
}

func (p *Planner) planSort(logical *LogicalSort) (PhysicalPlan, error) {
	child, err := p.Plan(logical.Child)
	if err != nil {
		return nil, err
	}

	cost := p.CostModel.EstimateSortCost(logical, child.Cost())
	return &PhysicalSort{
		SortFields:    logical.SortFields,
		Child:         child,
		OutputSchema:  logical.Schema(),
		EstimatedCost: cost,
	}, nil
}

func (p *Planner) planLimit(logical *LogicalLimit) (PhysicalPlan, error) {
	child, err := p.Plan(logical.Child)
	if err != nil {
		return nil, err
	}

	cost := p.CostModel.EstimateLimitCost(logical, child.Cost())
	return &PhysicalLimit{
		Offset:        logical.Offset,
		Limit:         logical.Limit,
		Child:         child,
		OutputSchema:  logical.Schema(),
		EstimatedCost: cost,
	}, nil
}

func (p *Planner) planTopN(logical *LogicalTopN) (PhysicalPlan, error) {
	child, err := p.Plan(logical.Child)
	if err != nil {
		return nil, err
	}

	// TopN is more efficient than separate Sort + Limit for small N
	// Cost is roughly: child cost + N * log(N) for heap maintenance
	cost := p.CostModel.EstimateSortCost(&LogicalSort{
		SortFields: logical.SortFields,
		Child:      logical.Child,
	}, child.Cost())

	// Reduce cost slightly since TopN is more efficient than full sort
	cost.CPUCost *= 0.7

	return &PhysicalTopN{
		N:             logical.N,
		Offset:        logical.Offset,
		SortFields:    logical.SortFields,
		Child:         child,
		OutputSchema:  logical.Schema(),
		EstimatedCost: cost,
	}, nil
}
