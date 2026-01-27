package planner

import (
	"fmt"
	"strings"

	"github.com/quidditch/quidditch/pkg/coordination/parser"
)

// Converter converts parser AST to logical plans
type Converter struct {
	defaultCardinality int64 // Default cardinality for scans when unknown
}

// NewConverter creates a new AST to logical plan converter
func NewConverter() *Converter {
	return &Converter{
		defaultCardinality: 100000, // Default: 100K documents
	}
}

// ConvertSearchRequest converts a parser.SearchRequest to a complete logical plan
func (c *Converter) ConvertSearchRequest(req *parser.SearchRequest, indexName string, shards []int32) (LogicalPlan, error) {
	// Convert query to filter expression (if present)
	var filterExpr *Expression
	var estimatedRows int64 = c.defaultCardinality
	if req.ParsedQuery != nil {
		var err error
		filterExpr, err = c.ConvertQuery(req.ParsedQuery)
		if err != nil {
			return nil, fmt.Errorf("failed to convert query: %w", err)
		}

		// Apply selectivity to estimate filtered rows
		selectivity := c.estimateSelectivity(req.ParsedQuery)
		estimatedRows = int64(float64(c.defaultCardinality) * selectivity)
	}

	// Create scan with filter pushed down
	scan := &LogicalScan{
		IndexName:     indexName,
		Shards:        shards,
		Filter:        filterExpr, // Push filter into scan!
		EstimatedRows: estimatedRows,
	}

	var plan LogicalPlan = scan

	// Add aggregations (if present)
	aggregations := req.Aggregations
	if aggregations == nil {
		aggregations = req.Aggs
	}
	if len(aggregations) > 0 {
		agg, err := c.convertAggregations(aggregations, plan)
		if err != nil {
			return nil, fmt.Errorf("failed to convert aggregations: %w", err)
		}
		plan = agg
	}

	// Add projection (if _source is specified)
	if req.Source != nil {
		project, err := c.convertSource(req.Source, plan)
		if err != nil {
			return nil, fmt.Errorf("failed to convert source: %w", err)
		}
		if project != nil {
			plan = project
		}
	}

	// Add sort (if present)
	if len(req.Sort) > 0 {
		sort, err := c.convertSort(req.Sort, plan)
		if err != nil {
			return nil, fmt.Errorf("failed to convert sort: %w", err)
		}
		plan = sort
	}

	// Add limit/pagination (from/size)
	if req.Size > 0 || req.From > 0 {
		size := req.Size
		if size == 0 {
			size = 10 // Default size
		}
		plan = &LogicalLimit{
			Offset: int64(req.From),
			Limit:  int64(size),
			Child:  plan,
		}
	}

	return plan, nil
}

// ConvertQuery converts a parser.Query to an Expression
func (c *Converter) ConvertQuery(q parser.Query) (*Expression, error) {
	switch query := q.(type) {
	case *parser.MatchAllQuery:
		return &Expression{
			Type: ExprTypeMatchAll,
		}, nil

	case *parser.TermQuery:
		return &Expression{
			Type:  ExprTypeTerm,
			Field: query.Field,
			Value: query.Value,
		}, nil

	case *parser.TermsQuery:
		// Terms query is OR of multiple term queries
		children := make([]*Expression, len(query.Values))
		for i, value := range query.Values {
			children[i] = &Expression{
				Type:  ExprTypeTerm,
				Field: query.Field,
				Value: value,
			}
		}
		return &Expression{
			Type:     ExprTypeBool,
			Children: children,
		}, nil

	case *parser.RangeQuery:
		// Only include non-nil range parameters
		rangeParams := make(map[string]interface{})
		if query.Gt != nil {
			rangeParams["gt"] = query.Gt
		}
		if query.Gte != nil {
			rangeParams["gte"] = query.Gte
		}
		if query.Lt != nil {
			rangeParams["lt"] = query.Lt
		}
		if query.Lte != nil {
			rangeParams["lte"] = query.Lte
		}
		return &Expression{
			Type:  ExprTypeRange,
			Field: query.Field,
			Value: rangeParams,
		}, nil

	case *parser.ExistsQuery:
		return &Expression{
			Type:  ExprTypeExists,
			Field: query.Field,
		}, nil

	case *parser.PrefixQuery:
		return &Expression{
			Type:  ExprTypePrefix,
			Field: query.Field,
			Value: query.Value,
		}, nil

	case *parser.WildcardQuery:
		return &Expression{
			Type:  ExprTypeWildcard,
			Field: query.Field,
			Value: query.Value,
		}, nil

	case *parser.MatchQuery:
		return &Expression{
			Type:  ExprTypeMatch,
			Field: query.Field,
			Value: query.Query,
		}, nil

	case *parser.MatchPhraseQuery:
		return &Expression{
			Type:  ExprTypeMatch, // Match phrase treated as match for now
			Field: query.Field,
			Value: query.Query,
		}, nil

	case *parser.BoolQuery:
		return c.convertBoolQuery(query)

	case *parser.MultiMatchQuery:
		// Multi-match is a bool should over multiple fields
		children := make([]*Expression, len(query.Fields))
		for i, field := range query.Fields {
			children[i] = &Expression{
				Type:  ExprTypeMatch,
				Field: field,
				Value: query.Query,
			}
		}
		return &Expression{
			Type:     ExprTypeBool,
			Children: children,
		}, nil

	case *parser.FuzzyQuery:
		// Treat fuzzy as wildcard for logical planning
		return &Expression{
			Type:  ExprTypeWildcard,
			Field: query.Field,
			Value: query.Value,
		}, nil

	case *parser.QueryStringQuery:
		// Query string is complex - for now treat as match on default field
		field := query.DefaultField
		if field == "" && len(query.Fields) > 0 {
			field = query.Fields[0]
		}
		if field == "" {
			field = "_all"
		}
		return &Expression{
			Type:  ExprTypeMatch,
			Field: field,
			Value: query.Query,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported query type: %T", query)
	}
}

// convertBoolQuery converts a bool query to an expression
func (c *Converter) convertBoolQuery(q *parser.BoolQuery) (*Expression, error) {
	// Bool query combines multiple clauses with AND (must/filter) and OR (should)
	var allClauses []*Expression

	// Convert must clauses (AND)
	for _, mustQuery := range q.Must {
		expr, err := c.ConvertQuery(mustQuery)
		if err != nil {
			return nil, err
		}
		allClauses = append(allClauses, expr)
	}

	// Convert filter clauses (AND, non-scoring)
	for _, filterQuery := range q.Filter {
		expr, err := c.ConvertQuery(filterQuery)
		if err != nil {
			return nil, err
		}
		allClauses = append(allClauses, expr)
	}

	// Convert should clauses (OR)
	if len(q.Should) > 0 {
		shouldChildren := make([]*Expression, len(q.Should))
		for i, shouldQuery := range q.Should {
			expr, err := c.ConvertQuery(shouldQuery)
			if err != nil {
				return nil, err
			}
			shouldChildren[i] = expr
		}
		shouldExpr := &Expression{
			Type:     ExprTypeBool,
			Children: shouldChildren,
		}
		allClauses = append(allClauses, shouldExpr)
	}

	// Convert must_not clauses (NOT)
	for _, mustNotQuery := range q.MustNot {
		expr, err := c.ConvertQuery(mustNotQuery)
		if err != nil {
			return nil, err
		}
		// Wrap in NOT expression (represented as bool with special marker)
		notExpr := &Expression{
			Type:     ExprTypeBool,
			Children: []*Expression{expr},
			Value:    "must_not", // Marker for NOT
		}
		allClauses = append(allClauses, notExpr)
	}

	// If only one clause, return it directly
	if len(allClauses) == 1 {
		return allClauses[0], nil
	}

	// Otherwise, return bool expression with all clauses
	return &Expression{
		Type:     ExprTypeBool,
		Children: allClauses,
	}, nil
}

// convertAggregations converts aggregations to a LogicalAggregate node
func (c *Converter) convertAggregations(aggs map[string]interface{}, child LogicalPlan) (*LogicalAggregate, error) {
	aggregations := make([]*Aggregation, 0, len(aggs))

	for name, aggDef := range aggs {
		aggMap, ok := aggDef.(map[string]interface{})
		if !ok {
			continue
		}

		for aggType, aggBody := range aggMap {
			agg, err := c.convertAggregation(name, aggType, aggBody)
			if err != nil {
				return nil, err
			}
			aggregations = append(aggregations, agg)
		}
	}

	if len(aggregations) == 0 {
		return nil, fmt.Errorf("no valid aggregations found")
	}

	return &LogicalAggregate{
		GroupBy:      []string{}, // TODO: Extract group by from terms agg
		Aggregations: aggregations,
		Child:        child,
		OutputSchema: &Schema{Fields: []*Field{}}, // TODO: Build schema
	}, nil
}

// convertAggregation converts a single aggregation
func (c *Converter) convertAggregation(name, aggType string, body interface{}) (*Aggregation, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("aggregation body must be an object")
	}

	agg := &Aggregation{
		Name:   name,
		Params: make(map[string]interface{}),
	}

	// Extract field
	if field, ok := bodyMap["field"].(string); ok {
		agg.Field = field
	}

	// Convert aggregation type
	switch aggType {
	case "terms":
		agg.Type = AggTypeTerms
		if size, ok := bodyMap["size"].(float64); ok {
			agg.Params["size"] = int(size)
		} else {
			agg.Params["size"] = 10 // Default
		}

	case "stats":
		agg.Type = AggTypeStats

	case "extended_stats":
		agg.Type = AggTypeExtendedStats

	case "sum":
		agg.Type = AggTypeSum

	case "avg":
		agg.Type = AggTypeAvg

	case "min":
		agg.Type = AggTypeMin

	case "max":
		agg.Type = AggTypeMax

	case "count":
		agg.Type = AggTypeCount

	case "cardinality":
		agg.Type = AggTypeCardinality

	case "percentiles":
		agg.Type = AggTypePercentiles
		if percents, ok := bodyMap["percents"].([]interface{}); ok {
			agg.Params["percents"] = percents
		}

	case "histogram":
		agg.Type = AggTypeHistogram
		if interval, ok := bodyMap["interval"].(float64); ok {
			agg.Params["interval"] = interval
		}

	case "date_histogram":
		agg.Type = AggTypeDateHistogram
		if interval, ok := bodyMap["interval"].(string); ok {
			agg.Params["interval"] = interval
		}
		if calendarInterval, ok := bodyMap["calendar_interval"].(string); ok {
			agg.Params["calendar_interval"] = calendarInterval
		}
		if fixedInterval, ok := bodyMap["fixed_interval"].(string); ok {
			agg.Params["fixed_interval"] = fixedInterval
		}

	default:
		return nil, fmt.Errorf("unsupported aggregation type: %s", aggType)
	}

	return agg, nil
}

// convertSource converts _source field specification to projection
func (c *Converter) convertSource(source interface{}, child LogicalPlan) (*LogicalProject, error) {
	var fields []string

	switch s := source.(type) {
	case bool:
		// false = no fields, true = all fields
		if !s {
			return nil, nil // No projection
		}
		return nil, nil // All fields (no projection needed)

	case string:
		// Single field
		fields = []string{s}

	case []interface{}:
		// Array of fields
		fields = make([]string, 0, len(s))
		for _, field := range s {
			if fieldStr, ok := field.(string); ok {
				fields = append(fields, fieldStr)
			}
		}

	case []string:
		// Array of fields (already strings)
		fields = s

	default:
		return nil, fmt.Errorf("unsupported _source type: %T", source)
	}

	if len(fields) == 0 {
		return nil, nil
	}

	return &LogicalProject{
		Fields:       fields,
		Child:        child,
		OutputSchema: &Schema{Fields: []*Field{}}, // TODO: Build schema
	}, nil
}

// convertSort converts sort specification to LogicalSort
func (c *Converter) convertSort(sort []map[string]interface{}, child LogicalPlan) (*LogicalSort, error) {
	sortFields := make([]*SortField, 0, len(sort))

	for _, sortSpec := range sort {
		for field, order := range sortSpec {
			sf := &SortField{
				Field:      field,
				Descending: false,
			}

			// Parse order
			switch orderValue := order.(type) {
			case string:
				if strings.ToLower(orderValue) == "desc" {
					sf.Descending = true
				}

			case map[string]interface{}:
				if orderStr, ok := orderValue["order"].(string); ok {
					if strings.ToLower(orderStr) == "desc" {
						sf.Descending = true
					}
				}
			}

			sortFields = append(sortFields, sf)
		}
	}

	if len(sortFields) == 0 {
		return nil, fmt.Errorf("no sort fields found")
	}

	return &LogicalSort{
		SortFields: sortFields,
		Child:      child,
	}, nil
}

// estimateSelectivity estimates the selectivity of a query (fraction of rows that match)
func (c *Converter) estimateSelectivity(q parser.Query) float64 {
	switch query := q.(type) {
	case *parser.MatchAllQuery:
		return 1.0

	case *parser.TermQuery:
		return 0.1 // Assume term matches 10% of documents

	case *parser.TermsQuery:
		// Multiple terms, higher selectivity
		return float64(len(query.Values)) * 0.1

	case *parser.RangeQuery:
		return 0.3 // Assume range matches 30% of documents

	case *parser.ExistsQuery:
		return 0.8 // Assume field exists in 80% of documents

	case *parser.PrefixQuery, *parser.WildcardQuery:
		return 0.2 // Prefix/wildcard more selective

	case *parser.MatchQuery, *parser.MatchPhraseQuery:
		return 0.15 // Text queries moderately selective

	case *parser.BoolQuery:
		// Combine selectivities
		selectivity := 1.0

		// Must clauses multiply (AND)
		for _, mustQuery := range query.Must {
			selectivity *= c.estimateSelectivity(mustQuery)
		}
		for _, filterQuery := range query.Filter {
			selectivity *= c.estimateSelectivity(filterQuery)
		}

		// Should clauses add (OR) but capped at 1.0
		if len(query.Should) > 0 {
			shouldSelectivity := 0.0
			for _, shouldQuery := range query.Should {
				shouldSelectivity += c.estimateSelectivity(shouldQuery)
			}
			selectivity *= min(1.0, shouldSelectivity)
		}

		// Must not clauses invert
		for _, mustNotQuery := range query.MustNot {
			selectivity *= (1.0 - c.estimateSelectivity(mustNotQuery))
		}

		return selectivity

	default:
		return 0.5 // Default: 50% selectivity
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
