package planner

import (
	"fmt"
)

// PlanType represents the type of logical plan node
type PlanType string

const (
	PlanTypeScan      PlanType = "scan"
	PlanTypeFilter    PlanType = "filter"
	PlanTypeProject   PlanType = "project"
	PlanTypeAggregate PlanType = "aggregate"
	PlanTypeSort      PlanType = "sort"
	PlanTypeLimit     PlanType = "limit"
	PlanTypeTopN      PlanType = "topn"
	PlanTypeJoin      PlanType = "join"
)

// LogicalPlan represents a logical query plan node
// All logical plan nodes implement this interface
type LogicalPlan interface {
	// Type returns the plan node type
	Type() PlanType

	// Children returns child plan nodes
	Children() []LogicalPlan

	// SetChild replaces a child at the given index
	SetChild(index int, child LogicalPlan) error

	// Schema returns the output schema (field names and types)
	Schema() *Schema

	// Cardinality estimates the number of output rows
	Cardinality() int64

	// String returns a human-readable representation
	String() string
}

// Schema represents the output schema of a plan node
type Schema struct {
	Fields []*Field
}

// Field represents a field in the schema
type Field struct {
	Name string
	Type FieldType
}

// FieldType represents the data type of a field
type FieldType string

const (
	FieldTypeString  FieldType = "string"
	FieldTypeInteger FieldType = "integer"
	FieldTypeFloat   FieldType = "float"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeObject  FieldType = "object"
	FieldTypeArray   FieldType = "array"
)

// LogicalScan represents a scan operation on an index
type LogicalScan struct {
	IndexName        string
	Shards           []int32
	Filter           *Expression // Optional filter expression (pushdown)
	EstimatedRows    int64       // Estimated number of rows
}

func (s *LogicalScan) Type() PlanType               { return PlanTypeScan }
func (s *LogicalScan) Children() []LogicalPlan      { return nil }
func (s *LogicalScan) SetChild(int, LogicalPlan) error {
	return fmt.Errorf("scan node has no children")
}
func (s *LogicalScan) Schema() *Schema {
	// Schema will be determined from index metadata
	return &Schema{Fields: []*Field{}}
}
func (s *LogicalScan) Cardinality() int64 { return s.EstimatedRows }
func (s *LogicalScan) String() string {
	return fmt.Sprintf("Scan(index=%s, shards=%v, filter=%v)", s.IndexName, s.Shards, s.Filter)
}

// LogicalFilter represents a filter operation
type LogicalFilter struct {
	Condition     *Expression
	Child         LogicalPlan
	EstimatedRows int64 // Estimated number of rows after filtering
}

func (f *LogicalFilter) Type() PlanType          { return PlanTypeFilter }
func (f *LogicalFilter) Children() []LogicalPlan { return []LogicalPlan{f.Child} }
func (f *LogicalFilter) SetChild(index int, child LogicalPlan) error {
	if index != 0 {
		return fmt.Errorf("filter has only one child")
	}
	f.Child = child
	return nil
}
func (f *LogicalFilter) Schema() *Schema  { return f.Child.Schema() }
func (f *LogicalFilter) Cardinality() int64 { return f.EstimatedRows }
func (f *LogicalFilter) String() string {
	return fmt.Sprintf("Filter(condition=%v)", f.Condition)
}

// LogicalProject represents a projection operation (select specific fields)
type LogicalProject struct {
	Fields      []string // Field names to project
	Child       LogicalPlan
	OutputSchema *Schema
}

func (p *LogicalProject) Type() PlanType          { return PlanTypeProject }
func (p *LogicalProject) Children() []LogicalPlan { return []LogicalPlan{p.Child} }
func (p *LogicalProject) SetChild(index int, child LogicalPlan) error {
	if index != 0 {
		return fmt.Errorf("project has only one child")
	}
	p.Child = child
	return nil
}
func (p *LogicalProject) Schema() *Schema { return p.OutputSchema }
func (p *LogicalProject) Cardinality() int64 { return p.Child.Cardinality() }
func (p *LogicalProject) String() string {
	return fmt.Sprintf("Project(fields=%v)", p.Fields)
}

// AggregationType represents the type of aggregation
type AggregationType string

const (
	AggTypeCount          AggregationType = "count"
	AggTypeSum            AggregationType = "sum"
	AggTypeAvg            AggregationType = "avg"
	AggTypeMin            AggregationType = "min"
	AggTypeMax            AggregationType = "max"
	AggTypeTerms          AggregationType = "terms"
	AggTypeStats          AggregationType = "stats"
	AggTypeHistogram      AggregationType = "histogram"
	AggTypeDateHistogram  AggregationType = "date_histogram"
	AggTypePercentiles    AggregationType = "percentiles"
	AggTypeCardinality    AggregationType = "cardinality"
	AggTypeExtendedStats  AggregationType = "extended_stats"
)

// Aggregation represents an aggregation operation
type Aggregation struct {
	Name   string
	Type   AggregationType
	Field  string
	Params map[string]interface{} // Additional parameters (e.g., size for terms, interval for histogram)
}

// LogicalAggregate represents an aggregation operation
type LogicalAggregate struct {
	GroupBy     []string      // Fields to group by
	Aggregations []*Aggregation // Aggregations to compute
	Child       LogicalPlan
	OutputSchema *Schema
}

func (a *LogicalAggregate) Type() PlanType          { return PlanTypeAggregate }
func (a *LogicalAggregate) Children() []LogicalPlan { return []LogicalPlan{a.Child} }
func (a *LogicalAggregate) SetChild(index int, child LogicalPlan) error {
	if index != 0 {
		return fmt.Errorf("aggregate has only one child")
	}
	a.Child = child
	return nil
}
func (a *LogicalAggregate) Schema() *Schema { return a.OutputSchema }
func (a *LogicalAggregate) Cardinality() int64 {
	// Cardinality after aggregation depends on group by cardinality
	// For now, estimate as 10% of child cardinality
	return a.Child.Cardinality() / 10
}
func (a *LogicalAggregate) String() string {
	return fmt.Sprintf("Aggregate(groupBy=%v, aggs=%d)", a.GroupBy, len(a.Aggregations))
}

// LogicalSort represents a sort operation
type LogicalSort struct {
	SortFields []*SortField
	Child      LogicalPlan
}

// SortField represents a field to sort by
type SortField struct {
	Field      string
	Descending bool
}

func (s *LogicalSort) Type() PlanType          { return PlanTypeSort }
func (s *LogicalSort) Children() []LogicalPlan { return []LogicalPlan{s.Child} }
func (s *LogicalSort) SetChild(index int, child LogicalPlan) error {
	if index != 0 {
		return fmt.Errorf("sort has only one child")
	}
	s.Child = child
	return nil
}
func (s *LogicalSort) Schema() *Schema  { return s.Child.Schema() }
func (s *LogicalSort) Cardinality() int64 { return s.Child.Cardinality() }
func (s *LogicalSort) String() string {
	return fmt.Sprintf("Sort(fields=%d)", len(s.SortFields))
}

// LogicalLimit represents a limit operation (pagination)
type LogicalLimit struct {
	Offset int64
	Limit  int64
	Child  LogicalPlan
}

func (l *LogicalLimit) Type() PlanType          { return PlanTypeLimit }
func (l *LogicalLimit) Children() []LogicalPlan { return []LogicalPlan{l.Child} }
func (l *LogicalLimit) SetChild(index int, child LogicalPlan) error {
	if index != 0 {
		return fmt.Errorf("limit has only one child")
	}
	l.Child = child
	return nil
}
func (l *LogicalLimit) Schema() *Schema { return l.Child.Schema() }
func (l *LogicalLimit) Cardinality() int64 {
	childCard := l.Child.Cardinality()
	if l.Offset >= childCard {
		return 0
	}
	remaining := childCard - l.Offset
	if remaining < l.Limit {
		return remaining
	}
	return l.Limit
}
func (l *LogicalLimit) String() string {
	return fmt.Sprintf("Limit(offset=%d, limit=%d)", l.Offset, l.Limit)
}

// LogicalTopN represents a TopN operation (optimized limit + sort)
// More efficient than separate Sort + Limit for small N
type LogicalTopN struct {
	N          int64         // Number of results to return
	Offset     int64         // Offset for pagination
	SortFields []*SortField  // Fields to sort by
	Child      LogicalPlan
}

func (t *LogicalTopN) Type() PlanType          { return PlanTypeTopN }
func (t *LogicalTopN) Children() []LogicalPlan { return []LogicalPlan{t.Child} }
func (t *LogicalTopN) SetChild(index int, child LogicalPlan) error {
	if index != 0 {
		return fmt.Errorf("topn has only one child")
	}
	t.Child = child
	return nil
}
func (t *LogicalTopN) Schema() *Schema { return t.Child.Schema() }
func (t *LogicalTopN) Cardinality() int64 {
	childCard := t.Child.Cardinality()
	if t.Offset >= childCard {
		return 0
	}
	remaining := childCard - t.Offset
	if remaining < t.N {
		return remaining
	}
	return t.N
}
func (t *LogicalTopN) String() string {
	return fmt.Sprintf("TopN(n=%d, offset=%d, fields=%v)", t.N, t.Offset, t.SortFields)
}

// Expression represents a filter/condition expression
// This will be expanded to support the full DSL expression tree
type Expression struct {
	Type     ExpressionType
	Field    string
	Value    interface{}
	Children []*Expression
}

// ExpressionType represents the type of expression
type ExpressionType string

const (
	ExprTypeTerm       ExpressionType = "term"
	ExprTypeMatch      ExpressionType = "match"
	ExprTypeRange      ExpressionType = "range"
	ExprTypeBool       ExpressionType = "bool"
	ExprTypeWildcard   ExpressionType = "wildcard"
	ExprTypePrefix     ExpressionType = "prefix"
	ExprTypeExists     ExpressionType = "exists"
	ExprTypeMatchAll   ExpressionType = "match_all"
)

func (e *Expression) String() string {
	if e == nil {
		return "nil"
	}
	return fmt.Sprintf("%s(%s=%v)", e.Type, e.Field, e.Value)
}
