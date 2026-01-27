package planner

import (
	"testing"

	"github.com/quidditch/quidditch/pkg/coordination/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertMatchAllQuery(t *testing.T) {
	converter := NewConverter()

	query := &parser.MatchAllQuery{}
	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	assert.Equal(t, ExprTypeMatchAll, expr.Type)
}

func TestConvertTermQuery(t *testing.T) {
	converter := NewConverter()

	query := &parser.TermQuery{
		Field: "status",
		Value: "active",
	}

	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	assert.Equal(t, ExprTypeTerm, expr.Type)
	assert.Equal(t, "status", expr.Field)
	assert.Equal(t, "active", expr.Value)
}

func TestConvertTermsQuery(t *testing.T) {
	converter := NewConverter()

	query := &parser.TermsQuery{
		Field:  "category",
		Values: []interface{}{"electronics", "books", "clothing"},
	}

	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	assert.Equal(t, ExprTypeBool, expr.Type)
	assert.Len(t, expr.Children, 3)

	// Each child should be a term query
	for i, child := range expr.Children {
		assert.Equal(t, ExprTypeTerm, child.Type)
		assert.Equal(t, "category", child.Field)
		assert.Equal(t, query.Values[i], child.Value)
	}
}

func TestConvertRangeQuery(t *testing.T) {
	converter := NewConverter()

	query := &parser.RangeQuery{
		Field: "price",
		Gte:   100,
		Lte:   500,
	}

	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	assert.Equal(t, ExprTypeRange, expr.Type)
	assert.Equal(t, "price", expr.Field)

	rangeMap, ok := expr.Value.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 100, rangeMap["gte"])
	assert.Equal(t, 500, rangeMap["lte"])
}

func TestConvertExistsQuery(t *testing.T) {
	converter := NewConverter()

	query := &parser.ExistsQuery{
		Field: "email",
	}

	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	assert.Equal(t, ExprTypeExists, expr.Type)
	assert.Equal(t, "email", expr.Field)
}

func TestConvertPrefixQuery(t *testing.T) {
	converter := NewConverter()

	query := &parser.PrefixQuery{
		Field: "name",
		Value: "john",
	}

	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	assert.Equal(t, ExprTypePrefix, expr.Type)
	assert.Equal(t, "name", expr.Field)
	assert.Equal(t, "john", expr.Value)
}

func TestConvertWildcardQuery(t *testing.T) {
	converter := NewConverter()

	query := &parser.WildcardQuery{
		Field: "name",
		Value: "jo*n",
	}

	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	assert.Equal(t, ExprTypeWildcard, expr.Type)
	assert.Equal(t, "name", expr.Field)
	assert.Equal(t, "jo*n", expr.Value)
}

func TestConvertMatchQuery(t *testing.T) {
	converter := NewConverter()

	query := &parser.MatchQuery{
		Field: "title",
		Query: "search engine",
	}

	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	assert.Equal(t, ExprTypeMatch, expr.Type)
	assert.Equal(t, "title", expr.Field)
	assert.Equal(t, "search engine", expr.Value)
}

func TestConvertBoolQueryMust(t *testing.T) {
	converter := NewConverter()

	query := &parser.BoolQuery{
		Must: []parser.Query{
			&parser.TermQuery{Field: "status", Value: "active"},
			&parser.RangeQuery{Field: "price", Gte: 100},
		},
	}

	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	assert.Equal(t, ExprTypeBool, expr.Type)
	assert.Len(t, expr.Children, 2)

	// First child is term query
	assert.Equal(t, ExprTypeTerm, expr.Children[0].Type)
	assert.Equal(t, "status", expr.Children[0].Field)

	// Second child is range query
	assert.Equal(t, ExprTypeRange, expr.Children[1].Type)
	assert.Equal(t, "price", expr.Children[1].Field)
}

func TestConvertBoolQueryShould(t *testing.T) {
	converter := NewConverter()

	query := &parser.BoolQuery{
		Should: []parser.Query{
			&parser.TermQuery{Field: "category", Value: "electronics"},
			&parser.TermQuery{Field: "category", Value: "books"},
		},
	}

	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	// Bool query with only should clauses returns the should bool expression
	assert.Equal(t, ExprTypeBool, expr.Type)
	assert.Len(t, expr.Children, 2) // Two term queries in OR

	// Both children should be term queries
	for _, child := range expr.Children {
		assert.Equal(t, ExprTypeTerm, child.Type)
		assert.Equal(t, "category", child.Field)
	}
}

func TestConvertBoolQueryMustNot(t *testing.T) {
	converter := NewConverter()

	query := &parser.BoolQuery{
		MustNot: []parser.Query{
			&parser.TermQuery{Field: "status", Value: "deleted"},
		},
	}

	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	// Bool query with only must_not simplifies to NOT expression
	assert.Equal(t, ExprTypeBool, expr.Type)
	assert.Len(t, expr.Children, 1)

	// Should be the term query with must_not marker
	innerExpr := expr.Children[0]
	assert.Equal(t, ExprTypeTerm, innerExpr.Type)
	assert.Equal(t, "status", innerExpr.Field)

	// The wrapper has must_not marker
	assert.Equal(t, "must_not", expr.Value)
}

func TestConvertMultiMatchQuery(t *testing.T) {
	converter := NewConverter()

	query := &parser.MultiMatchQuery{
		Query:  "search term",
		Fields: []string{"title", "description", "content"},
	}

	expr, err := converter.ConvertQuery(query)

	require.NoError(t, err)
	assert.Equal(t, ExprTypeBool, expr.Type)
	assert.Len(t, expr.Children, 3)

	// Each child should be a match on different field
	for i, child := range expr.Children {
		assert.Equal(t, ExprTypeMatch, child.Type)
		assert.Equal(t, query.Fields[i], child.Field)
		assert.Equal(t, "search term", child.Value)
	}
}

func TestConvertSearchRequestSimple(t *testing.T) {
	converter := NewConverter()

	reqJSON := `{
		"query": {
			"term": {
				"status": "active"
			}
		},
		"size": 10
	}`

	p := parser.NewQueryParser()
	req, err := p.ParseSearchRequest([]byte(reqJSON))
	require.NoError(t, err)

	plan, err := converter.ConvertSearchRequest(req, "products", []int32{0, 1, 2})
	require.NoError(t, err)

	// Plan should be: Limit -> Filter -> Scan
	limit, ok := plan.(*LogicalLimit)
	require.True(t, ok)
	assert.Equal(t, int64(10), limit.Limit)

	filter, ok := limit.Child.(*LogicalFilter)
	require.True(t, ok)
	assert.Equal(t, ExprTypeTerm, filter.Condition.Type)

	scan, ok := filter.Child.(*LogicalScan)
	require.True(t, ok)
	assert.Equal(t, "products", scan.IndexName)
	assert.Len(t, scan.Shards, 3)
}

func TestConvertSearchRequestWithSort(t *testing.T) {
	converter := NewConverter()

	reqJSON := `{
		"query": {
			"match_all": {}
		},
		"sort": [
			{"price": "desc"},
			{"name": "asc"}
		],
		"size": 20
	}`

	p := parser.NewQueryParser()
	req, err := p.ParseSearchRequest([]byte(reqJSON))
	require.NoError(t, err)

	plan, err := converter.ConvertSearchRequest(req, "products", []int32{0})
	require.NoError(t, err)

	// Plan should be: Limit -> Sort -> Filter -> Scan
	limit, ok := plan.(*LogicalLimit)
	require.True(t, ok)
	assert.Equal(t, int64(20), limit.Limit)

	sort, ok := limit.Child.(*LogicalSort)
	require.True(t, ok)
	assert.Len(t, sort.SortFields, 2)
	assert.Equal(t, "price", sort.SortFields[0].Field)
	assert.True(t, sort.SortFields[0].Descending)
	assert.Equal(t, "name", sort.SortFields[1].Field)
	assert.False(t, sort.SortFields[1].Descending)
}

func TestConvertSearchRequestWithAggregations(t *testing.T) {
	converter := NewConverter()

	reqJSON := `{
		"query": {
			"match_all": {}
		},
		"aggs": {
			"categories": {
				"terms": {
					"field": "category",
					"size": 10
				}
			},
			"avg_price": {
				"avg": {
					"field": "price"
				}
			}
		},
		"size": 0
	}`

	p := parser.NewQueryParser()
	req, err := p.ParseSearchRequest([]byte(reqJSON))
	require.NoError(t, err)

	plan, err := converter.ConvertSearchRequest(req, "products", []int32{0})
	require.NoError(t, err)

	// Plan should be: Aggregate -> Filter -> Scan (no limit because size=0)
	agg, ok := plan.(*LogicalAggregate)
	require.True(t, ok)
	assert.Len(t, agg.Aggregations, 2)

	// Find aggregations by name
	var termsAgg, avgAgg *Aggregation
	for _, a := range agg.Aggregations {
		if a.Name == "categories" {
			termsAgg = a
		} else if a.Name == "avg_price" {
			avgAgg = a
		}
	}

	require.NotNil(t, termsAgg)
	assert.Equal(t, AggTypeTerms, termsAgg.Type)
	assert.Equal(t, "category", termsAgg.Field)
	assert.Equal(t, 10, termsAgg.Params["size"])

	require.NotNil(t, avgAgg)
	assert.Equal(t, AggTypeAvg, avgAgg.Type)
	assert.Equal(t, "price", avgAgg.Field)
}

func TestConvertSearchRequestWithProjection(t *testing.T) {
	converter := NewConverter()

	reqJSON := `{
		"query": {
			"match_all": {}
		},
		"_source": ["name", "price"],
		"size": 10
	}`

	p := parser.NewQueryParser()
	req, err := p.ParseSearchRequest([]byte(reqJSON))
	require.NoError(t, err)

	plan, err := converter.ConvertSearchRequest(req, "products", []int32{0})
	require.NoError(t, err)

	// Plan should be: Limit -> Project -> Filter -> Scan
	limit, ok := plan.(*LogicalLimit)
	require.True(t, ok)

	project, ok := limit.Child.(*LogicalProject)
	require.True(t, ok)
	assert.Len(t, project.Fields, 2)
	assert.Contains(t, project.Fields, "name")
	assert.Contains(t, project.Fields, "price")
}

func TestConvertSearchRequestComplex(t *testing.T) {
	converter := NewConverter()

	reqJSON := `{
		"query": {
			"bool": {
				"must": [
					{"term": {"status": "active"}},
					{"range": {"price": {"gte": 100, "lte": 500}}}
				],
				"should": [
					{"term": {"category": "electronics"}},
					{"term": {"category": "books"}}
				],
				"must_not": [
					{"term": {"deleted": true}}
				]
			}
		},
		"_source": ["name", "price", "category"],
		"sort": [
			{"rating": "desc"}
		],
		"from": 20,
		"size": 10
	}`

	p := parser.NewQueryParser()
	req, err := p.ParseSearchRequest([]byte(reqJSON))
	require.NoError(t, err)

	plan, err := converter.ConvertSearchRequest(req, "products", []int32{0, 1, 2})
	require.NoError(t, err)

	// Plan should be: Limit -> Sort -> Project -> Filter -> Scan
	limit, ok := plan.(*LogicalLimit)
	require.True(t, ok)
	assert.Equal(t, int64(20), limit.Offset)
	assert.Equal(t, int64(10), limit.Limit)

	sort, ok := limit.Child.(*LogicalSort)
	require.True(t, ok)
	assert.Len(t, sort.SortFields, 1)

	project, ok := sort.Child.(*LogicalProject)
	require.True(t, ok)
	assert.Len(t, project.Fields, 3)

	filter, ok := project.Child.(*LogicalFilter)
	require.True(t, ok)
	assert.Equal(t, ExprTypeBool, filter.Condition.Type)

	scan, ok := filter.Child.(*LogicalScan)
	require.True(t, ok)
	assert.Equal(t, "products", scan.IndexName)
}

func TestConvertAllAggregationTypes(t *testing.T) {
	converter := NewConverter()

	aggTypes := []struct {
		name     string
		aggType  string
		field    string
		expected AggregationType
	}{
		{"terms_agg", "terms", "category", AggTypeTerms},
		{"stats_agg", "stats", "price", AggTypeStats},
		{"extended_stats_agg", "extended_stats", "price", AggTypeExtendedStats},
		{"sum_agg", "sum", "quantity", AggTypeSum},
		{"avg_agg", "avg", "price", AggTypeAvg},
		{"min_agg", "min", "price", AggTypeMin},
		{"max_agg", "max", "price", AggTypeMax},
		{"count_agg", "count", "_id", AggTypeCount},
		{"cardinality_agg", "cardinality", "user_id", AggTypeCardinality},
		{"percentiles_agg", "percentiles", "response_time", AggTypePercentiles},
		{"histogram_agg", "histogram", "price", AggTypeHistogram},
		{"date_histogram_agg", "date_histogram", "timestamp", AggTypeDateHistogram},
	}

	for _, tt := range aggTypes {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]interface{}{
				"field": tt.field,
			}

			// Add type-specific parameters
			if tt.aggType == "terms" {
				body["size"] = 10
			} else if tt.aggType == "histogram" {
				body["interval"] = 100
			} else if tt.aggType == "date_histogram" {
				body["interval"] = "1d"
			}

			agg, err := converter.convertAggregation(tt.name, tt.aggType, body)
			require.NoError(t, err)

			assert.Equal(t, tt.name, agg.Name)
			assert.Equal(t, tt.expected, agg.Type)
			assert.Equal(t, tt.field, agg.Field)
		})
	}
}

func TestEstimateSelectivity(t *testing.T) {
	converter := NewConverter()

	tests := []struct {
		name       string
		query      parser.Query
		selectivity float64
	}{
		{
			"match_all",
			&parser.MatchAllQuery{},
			1.0,
		},
		{
			"term",
			&parser.TermQuery{Field: "status", Value: "active"},
			0.1,
		},
		{
			"range",
			&parser.RangeQuery{Field: "price", Gte: 100, Lte: 500},
			0.3,
		},
		{
			"exists",
			&parser.ExistsQuery{Field: "email"},
			0.8,
		},
		{
			"bool_must",
			&parser.BoolQuery{
				Must: []parser.Query{
					&parser.TermQuery{Field: "status", Value: "active"},
					&parser.RangeQuery{Field: "price", Gte: 100},
				},
			},
			0.1 * 0.3, // AND: multiply selectivities
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selectivity := converter.estimateSelectivity(tt.query)
			assert.InDelta(t, tt.selectivity, selectivity, 0.001)
		})
	}
}

func TestConvertSourceFalse(t *testing.T) {
	converter := NewConverter()

	scan := &LogicalScan{
		IndexName:     "products",
		EstimatedRows: 1000,
	}

	// _source: false means no fields
	project, err := converter.convertSource(false, scan)
	require.NoError(t, err)
	assert.Nil(t, project)
}

func TestConvertSourceTrue(t *testing.T) {
	converter := NewConverter()

	scan := &LogicalScan{
		IndexName:     "products",
		EstimatedRows: 1000,
	}

	// _source: true means all fields (no projection needed)
	project, err := converter.convertSource(true, scan)
	require.NoError(t, err)
	assert.Nil(t, project)
}

func TestConvertSourceArray(t *testing.T) {
	converter := NewConverter()

	scan := &LogicalScan{
		IndexName:     "products",
		EstimatedRows: 1000,
	}

	// _source: ["name", "price"]
	fields := []interface{}{"name", "price"}
	project, err := converter.convertSource(fields, scan)
	require.NoError(t, err)
	require.NotNil(t, project)
	assert.Len(t, project.Fields, 2)
	assert.Contains(t, project.Fields, "name")
	assert.Contains(t, project.Fields, "price")
}

func TestConvertSortComplex(t *testing.T) {
	converter := NewConverter()

	scan := &LogicalScan{
		IndexName:     "products",
		EstimatedRows: 1000,
	}

	sortSpec := []map[string]interface{}{
		{
			"price": map[string]interface{}{
				"order": "desc",
			},
		},
		{
			"name": "asc",
		},
		{
			"_score": "desc",
		},
	}

	sort, err := converter.convertSort(sortSpec, scan)
	require.NoError(t, err)
	require.NotNil(t, sort)
	assert.Len(t, sort.SortFields, 3)

	assert.Equal(t, "price", sort.SortFields[0].Field)
	assert.True(t, sort.SortFields[0].Descending)

	assert.Equal(t, "name", sort.SortFields[1].Field)
	assert.False(t, sort.SortFields[1].Descending)

	assert.Equal(t, "_score", sort.SortFields[2].Field)
	assert.True(t, sort.SortFields[2].Descending)
}

func TestConvertWithOptimization(t *testing.T) {
	converter := NewConverter()

	reqJSON := `{
		"query": {
			"term": {
				"category": "electronics"
			}
		},
		"size": 10
	}`

	p := parser.NewQueryParser()
	req, err := p.ParseSearchRequest([]byte(reqJSON))
	require.NoError(t, err)

	plan, err := converter.ConvertSearchRequest(req, "products", []int32{0, 1, 2})
	require.NoError(t, err)

	// Optimize the plan
	optimizer := NewOptimizer()
	optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)
	optimized, err := optimizer.Optimize(plan)
	require.NoError(t, err)

	// After optimization, filter should be pushed to scan
	limit, ok := optimized.(*LogicalLimit)
	require.True(t, ok)

	scan, ok := limit.Child.(*LogicalScan)
	require.True(t, ok)
	assert.NotNil(t, scan.Filter)
	assert.Equal(t, "category", scan.Filter.Field)
}

func TestConvertToPhysicalPlan(t *testing.T) {
	converter := NewConverter()

	reqJSON := `{
		"query": {
			"term": {
				"status": "active"
			}
		},
		"size": 10
	}`

	p := parser.NewQueryParser()
	req, err := p.ParseSearchRequest([]byte(reqJSON))
	require.NoError(t, err)

	// Convert to logical plan
	logicalPlan, err := converter.ConvertSearchRequest(req, "products", []int32{0, 1, 2})
	require.NoError(t, err)

	// Optimize
	optimizer := NewOptimizer()
	optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)
	optimized, err := optimizer.Optimize(logicalPlan)
	require.NoError(t, err)

	// Convert to physical plan
	costModel := NewDefaultCostModel()
	planner := NewPlanner(costModel)
	physicalPlan, err := planner.Plan(optimized)
	require.NoError(t, err)

	// Verify physical plan structure
	limit, ok := physicalPlan.(*PhysicalLimit)
	require.True(t, ok)
	assert.Equal(t, int64(10), limit.Limit)

	scan, ok := limit.Child.(*PhysicalScan)
	require.True(t, ok)
	assert.Equal(t, "products", scan.IndexName)
	assert.NotNil(t, scan.Filter)
	assert.NotNil(t, scan.EstimatedCost)
}

func TestFullPipelineEndToEnd(t *testing.T) {
	// This test demonstrates the complete pipeline:
	// JSON → Parser → Converter → Logical Plan → Optimizer → Physical Plan

	reqJSON := `{
		"query": {
			"bool": {
				"must": [
					{"term": {"status": "active"}},
					{"range": {"price": {"gte": 100, "lte": 500}}}
				]
			}
		},
		"_source": ["name", "price"],
		"sort": [{"rating": "desc"}],
		"from": 0,
		"size": 10
	}`

	// Step 1: Parse JSON
	p := parser.NewQueryParser()
	req, err := p.ParseSearchRequest([]byte(reqJSON))
	require.NoError(t, err)

	// Step 2: Convert to logical plan
	converter := NewConverter()
	logicalPlan, err := converter.ConvertSearchRequest(req, "products", []int32{0, 1, 2})
	require.NoError(t, err)

	// Step 3: Optimize
	optimizer := NewOptimizer()
	optimizer.RuleSet = NewRuleSet(GetDefaultRules()...)
	optimizedPlan, err := optimizer.Optimize(logicalPlan)
	require.NoError(t, err)

	// Step 4: Convert to physical plan
	costModel := NewDefaultCostModel()
	planner := NewPlanner(costModel)
	physicalPlan, err := planner.Plan(optimizedPlan)
	require.NoError(t, err)

	// Verify final physical plan
	assert.NotNil(t, physicalPlan)
	assert.NotNil(t, physicalPlan.Cost())

	t.Logf("Final physical plan: %s", physicalPlan.String())
	t.Logf("Estimated cost: %.2f", physicalPlan.Cost().TotalCost)
}
