package parser

// SearchRequest represents a complete search request
type SearchRequest struct {
	Query       map[string]interface{}   `json:"query,omitempty"`
	Size        int                      `json:"size,omitempty"`
	From        int                      `json:"from,omitempty"`
	Sort        []map[string]interface{} `json:"sort,omitempty"`
	Source      interface{}              `json:"_source,omitempty"`
	Aggregations map[string]interface{}  `json:"aggregations,omitempty"`
	Aggs        map[string]interface{}   `json:"aggs,omitempty"` // Alias for aggregations
	Highlight   map[string]interface{}   `json:"highlight,omitempty"`
	Timeout     string                   `json:"timeout,omitempty"`

	// Parsed query (not from JSON)
	ParsedQuery Query `json:"-"`
}

// Query is the interface for all query types
type Query interface {
	QueryType() string
}

// ============================================================================
// Full-Text Queries
// ============================================================================

// MatchQuery represents a match query
type MatchQuery struct {
	Field    string
	Query    string
	Operator string  // "and" or "or"
	Boost    float64
	Analyzer string
}

func (q *MatchQuery) QueryType() string { return "match" }

// MatchPhraseQuery represents a match_phrase query
type MatchPhraseQuery struct {
	Field string
	Query string
	Slop  int // Maximum positions between matching terms
}

func (q *MatchPhraseQuery) QueryType() string { return "match_phrase" }

// MultiMatchQuery represents a multi_match query
type MultiMatchQuery struct {
	Query  string
	Fields []string
	Type   string // best_fields, most_fields, cross_fields, phrase, phrase_prefix
}

func (q *MultiMatchQuery) QueryType() string { return "multi_match" }

// QueryStringQuery represents a query_string query (Lucene syntax)
type QueryStringQuery struct {
	Query        string
	DefaultField string
	Fields       []string
}

func (q *QueryStringQuery) QueryType() string { return "query_string" }

// ============================================================================
// Term-Level Queries
// ============================================================================

// TermQuery represents a term query (exact match)
type TermQuery struct {
	Field string
	Value interface{}
	Boost float64
}

func (q *TermQuery) QueryType() string { return "term" }

// TermsQuery represents a terms query (multiple exact matches)
type TermsQuery struct {
	Field  string
	Values []interface{}
}

func (q *TermsQuery) QueryType() string { return "terms" }

// RangeQuery represents a range query
type RangeQuery struct {
	Field string
	Gt    interface{} // Greater than
	Gte   interface{} // Greater than or equal
	Lt    interface{} // Less than
	Lte   interface{} // Less than or equal
	Boost float64
}

func (q *RangeQuery) QueryType() string { return "range" }

// ExistsQuery represents an exists query (field has a value)
type ExistsQuery struct {
	Field string
}

func (q *ExistsQuery) QueryType() string { return "exists" }

// PrefixQuery represents a prefix query
type PrefixQuery struct {
	Field string
	Value string
}

func (q *PrefixQuery) QueryType() string { return "prefix" }

// WildcardQuery represents a wildcard query
type WildcardQuery struct {
	Field string
	Value string // Supports * and ?
}

func (q *WildcardQuery) QueryType() string { return "wildcard" }

// FuzzyQuery represents a fuzzy query
type FuzzyQuery struct {
	Field      string
	Value      string
	Fuzziness  string // "AUTO", "0", "1", "2"
}

func (q *FuzzyQuery) QueryType() string { return "fuzzy" }

// ============================================================================
// Compound Queries
// ============================================================================

// BoolQuery represents a bool query (boolean combinations)
type BoolQuery struct {
	Must                   []Query
	Should                 []Query
	MustNot                []Query
	Filter                 []Query
	MinimumShouldMatch     int
	MinimumShouldMatchStr  string // Can be "75%" or "3<90%"
}

func (q *BoolQuery) QueryType() string { return "bool" }

// MatchAllQuery represents a match_all query
type MatchAllQuery struct {
	Boost float64
}

func (q *MatchAllQuery) QueryType() string { return "match_all" }

// ============================================================================
// Expression Query (Custom Filter)
// ============================================================================

// ExpressionQuery represents an expression filter query
// Evaluated natively in C++ on data nodes with ~5ns per call
type ExpressionQuery struct {
	// Expression AST (from expressions package)
	Expression interface{}

	// Serialized expression bytes (for C++)
	SerializedExpression []byte
}

func (q *ExpressionQuery) QueryType() string { return "expr" }

// ============================================================================
// WASM UDF Query (User-Defined Functions)
// ============================================================================

// WasmUDFQuery represents a WASM User-Defined Function query
// Evaluated using WASM runtime with document context at ~3.8μs per call
type WasmUDFQuery struct {
	// UDF identification
	Name    string // UDF name
	Version string // UDF version (optional, uses latest if empty)

	// Parameters passed to the UDF
	// Keys are parameter names, values are parameter values
	Parameters map[string]interface{}
}

func (q *WasmUDFQuery) QueryType() string { return "wasm_udf" }

// ============================================================================
// Query AST Helper Methods
// ============================================================================

// IsBoolQuery checks if a query is a bool query
func IsBoolQuery(q Query) bool {
	_, ok := q.(*BoolQuery)
	return ok
}

// IsTermLevelQuery checks if a query is a term-level query
func IsTermLevelQuery(q Query) bool {
	switch q.(type) {
	case *TermQuery, *TermsQuery, *RangeQuery, *ExistsQuery, *PrefixQuery, *WildcardQuery, *ExpressionQuery, *WasmUDFQuery:
		return true
	default:
		return false
	}
}

// IsFullTextQuery checks if a query is a full-text query
func IsFullTextQuery(q Query) bool {
	switch q.(type) {
	case *MatchQuery, *MatchPhraseQuery, *MultiMatchQuery, *QueryStringQuery:
		return true
	default:
		return false
	}
}

// GetQueryFields returns all fields referenced in a query
func GetQueryFields(q Query) []string {
	fields := make([]string, 0)

	switch query := q.(type) {
	case *MatchQuery:
		fields = append(fields, query.Field)
	case *MatchPhraseQuery:
		fields = append(fields, query.Field)
	case *MultiMatchQuery:
		fields = append(fields, query.Fields...)
	case *TermQuery:
		fields = append(fields, query.Field)
	case *TermsQuery:
		fields = append(fields, query.Field)
	case *RangeQuery:
		fields = append(fields, query.Field)
	case *ExistsQuery:
		fields = append(fields, query.Field)
	case *PrefixQuery:
		fields = append(fields, query.Field)
	case *WildcardQuery:
		fields = append(fields, query.Field)
	case *FuzzyQuery:
		fields = append(fields, query.Field)
	case *BoolQuery:
		for _, subQuery := range query.Must {
			fields = append(fields, GetQueryFields(subQuery)...)
		}
		for _, subQuery := range query.Should {
			fields = append(fields, GetQueryFields(subQuery)...)
		}
		for _, subQuery := range query.MustNot {
			fields = append(fields, GetQueryFields(subQuery)...)
		}
		for _, subQuery := range query.Filter {
			fields = append(fields, GetQueryFields(subQuery)...)
		}
	case *ExpressionQuery:
		// Expression queries can reference multiple fields
		// We'll need to extract field references from the expression AST
		// For now, we'll leave this empty as expression field extraction
		// is a separate concern handled by the expression package
	}

	return fields
}

// EstimateComplexity estimates the complexity of a query
func EstimateComplexity(q Query) int {
	switch query := q.(type) {
	case *MatchAllQuery:
		return 1
	case *TermQuery, *ExistsQuery:
		return 10
	case *TermsQuery:
		return 10 * len(query.Values)
	case *RangeQuery:
		return 20
	case *MatchQuery, *MatchPhraseQuery:
		return 50
	case *MultiMatchQuery:
		return 50 * len(query.Fields)
	case *PrefixQuery, *WildcardQuery:
		return 100
	case *FuzzyQuery:
		return 200
	case *BoolQuery:
		complexity := 0
		for _, subQuery := range query.Must {
			complexity += EstimateComplexity(subQuery)
		}
		for _, subQuery := range query.Should {
			complexity += EstimateComplexity(subQuery)
		}
		for _, subQuery := range query.MustNot {
			complexity += EstimateComplexity(subQuery)
		}
		for _, subQuery := range query.Filter {
			complexity += EstimateComplexity(subQuery)
		}
		return complexity
	case *ExpressionQuery:
		// Expression queries are evaluated natively in C++ at ~5ns per call
		// Complexity similar to term queries
		return 10
	case *WasmUDFQuery:
		// WASM UDF queries are evaluated at ~3.8μs per call
		// Complexity higher than expression but still filter-like
		return 40
	default:
		return 100
	}
}

// ============================================================================
// Query Rewriting/Optimization
// ============================================================================

// SimplifyBoolQuery simplifies a bool query by flattening nested bool queries
func SimplifyBoolQuery(q *BoolQuery) *BoolQuery {
	simplified := &BoolQuery{
		Must:                  make([]Query, 0),
		Should:                make([]Query, 0),
		MustNot:               make([]Query, 0),
		Filter:                make([]Query, 0),
		MinimumShouldMatch:    q.MinimumShouldMatch,
		MinimumShouldMatchStr: q.MinimumShouldMatchStr,
	}

	// Flatten nested bool queries in must clauses
	for _, subQuery := range q.Must {
		if boolQuery, ok := subQuery.(*BoolQuery); ok && len(boolQuery.Should) == 0 {
			// Can flatten if no should clauses
			simplified.Must = append(simplified.Must, boolQuery.Must...)
			simplified.MustNot = append(simplified.MustNot, boolQuery.MustNot...)
			simplified.Filter = append(simplified.Filter, boolQuery.Filter...)
		} else {
			simplified.Must = append(simplified.Must, subQuery)
		}
	}

	// Keep other clauses as-is for now
	simplified.Should = q.Should
	simplified.MustNot = q.MustNot
	simplified.Filter = q.Filter

	return simplified
}

// CanUseFilter checks if a query can be executed as a filter (non-scoring)
func CanUseFilter(q Query) bool {
	switch query := q.(type) {
	case *TermQuery, *TermsQuery, *RangeQuery, *ExistsQuery, *PrefixQuery, *ExpressionQuery, *WasmUDFQuery:
		return true
	case *BoolQuery:
		// Bool query can be filter if all sub-queries can be filters
		for _, subQuery := range query.Must {
			if !CanUseFilter(subQuery) {
				return false
			}
		}
		for _, subQuery := range query.Should {
			if !CanUseFilter(subQuery) {
				return false
			}
		}
		for _, subQuery := range query.MustNot {
			if !CanUseFilter(subQuery) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
