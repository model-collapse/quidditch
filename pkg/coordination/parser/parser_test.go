package parser

import (
	"testing"
)

func TestParseMatchQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name: "simple match query",
			query: `{
				"query": {
					"match": {
						"title": "search engine"
					}
				}
			}`,
			wantErr: false,
		},
		{
			name: "match query with options",
			query: `{
				"query": {
					"match": {
						"title": {
							"query": "search engine",
							"operator": "and",
							"boost": 2.0
						}
					}
				}
			}`,
			wantErr: false,
		},
	}

	parser := NewQueryParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := parser.ParseSearchRequest([]byte(tt.query))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSearchRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if req.ParsedQuery == nil {
					t.Error("ParsedQuery is nil")
					return
				}

				matchQuery, ok := req.ParsedQuery.(*MatchQuery)
				if !ok {
					t.Errorf("Expected MatchQuery, got %T", req.ParsedQuery)
					return
				}

				if matchQuery.Field != "title" {
					t.Errorf("Expected field 'title', got '%s'", matchQuery.Field)
				}
			}
		})
	}
}

func TestParseTermQuery(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantField string
		wantValue interface{}
		wantErr   bool
	}{
		{
			name: "term query with string",
			query: `{
				"query": {
					"term": {
						"status": "published"
					}
				}
			}`,
			wantField: "status",
			wantValue: "published",
			wantErr:   false,
		},
		{
			name: "term query with number",
			query: `{
				"query": {
					"term": {
						"age": 25
					}
				}
			}`,
			wantField: "age",
			wantValue: float64(25),
			wantErr:   false,
		},
	}

	parser := NewQueryParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := parser.ParseSearchRequest([]byte(tt.query))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSearchRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				termQuery, ok := req.ParsedQuery.(*TermQuery)
				if !ok {
					t.Errorf("Expected TermQuery, got %T", req.ParsedQuery)
					return
				}

				if termQuery.Field != tt.wantField {
					t.Errorf("Expected field '%s', got '%s'", tt.wantField, termQuery.Field)
				}

				if termQuery.Value != tt.wantValue {
					t.Errorf("Expected value '%v', got '%v'", tt.wantValue, termQuery.Value)
				}
			}
		})
	}
}

func TestParseRangeQuery(t *testing.T) {
	query := `{
		"query": {
			"range": {
				"age": {
					"gte": 20,
					"lte": 30
				}
			}
		}
	}`

	parser := NewQueryParser()
	req, err := parser.ParseSearchRequest([]byte(query))
	if err != nil {
		t.Fatalf("ParseSearchRequest() error = %v", err)
	}

	rangeQuery, ok := req.ParsedQuery.(*RangeQuery)
	if !ok {
		t.Fatalf("Expected RangeQuery, got %T", req.ParsedQuery)
	}

	if rangeQuery.Field != "age" {
		t.Errorf("Expected field 'age', got '%s'", rangeQuery.Field)
	}

	if rangeQuery.Gte != float64(20) {
		t.Errorf("Expected gte=20, got %v", rangeQuery.Gte)
	}

	if rangeQuery.Lte != float64(30) {
		t.Errorf("Expected lte=30, got %v", rangeQuery.Lte)
	}
}

func TestParseBoolQuery(t *testing.T) {
	query := `{
		"query": {
			"bool": {
				"must": [
					{
						"match": {
							"title": "search"
						}
					}
				],
				"filter": [
					{
						"term": {
							"status": "published"
						}
					},
					{
						"range": {
							"age": {
								"gte": 18
							}
						}
					}
				],
				"must_not": [
					{
						"term": {
							"category": "spam"
						}
					}
				],
				"should": [
					{
						"match": {
							"description": "engine"
						}
					}
				],
				"minimum_should_match": 1
			}
		}
	}`

	parser := NewQueryParser()
	req, err := parser.ParseSearchRequest([]byte(query))
	if err != nil {
		t.Fatalf("ParseSearchRequest() error = %v", err)
	}

	boolQuery, ok := req.ParsedQuery.(*BoolQuery)
	if !ok {
		t.Fatalf("Expected BoolQuery, got %T", req.ParsedQuery)
	}

	// Check must clauses
	if len(boolQuery.Must) != 1 {
		t.Errorf("Expected 1 must clause, got %d", len(boolQuery.Must))
	}

	// Check filter clauses
	if len(boolQuery.Filter) != 2 {
		t.Errorf("Expected 2 filter clauses, got %d", len(boolQuery.Filter))
	}

	// Check must_not clauses
	if len(boolQuery.MustNot) != 1 {
		t.Errorf("Expected 1 must_not clause, got %d", len(boolQuery.MustNot))
	}

	// Check should clauses
	if len(boolQuery.Should) != 1 {
		t.Errorf("Expected 1 should clause, got %d", len(boolQuery.Should))
	}

	// Check minimum_should_match
	if boolQuery.MinimumShouldMatch != 1 {
		t.Errorf("Expected minimum_should_match=1, got %d", boolQuery.MinimumShouldMatch)
	}
}

func TestParseNestedBoolQuery(t *testing.T) {
	query := `{
		"query": {
			"bool": {
				"must": [
					{
						"bool": {
							"should": [
								{"term": {"status": "published"}},
								{"term": {"status": "draft"}}
							]
						}
					},
					{
						"match": {
							"title": "search"
						}
					}
				]
			}
		}
	}`

	parser := NewQueryParser()
	req, err := parser.ParseSearchRequest([]byte(query))
	if err != nil {
		t.Fatalf("ParseSearchRequest() error = %v", err)
	}

	boolQuery, ok := req.ParsedQuery.(*BoolQuery)
	if !ok {
		t.Fatalf("Expected BoolQuery, got %T", req.ParsedQuery)
	}

	if len(boolQuery.Must) != 2 {
		t.Errorf("Expected 2 must clauses, got %d", len(boolQuery.Must))
	}

	// Check nested bool query
	nestedBool, ok := boolQuery.Must[0].(*BoolQuery)
	if !ok {
		t.Errorf("Expected first must clause to be BoolQuery, got %T", boolQuery.Must[0])
	} else {
		if len(nestedBool.Should) != 2 {
			t.Errorf("Expected 2 should clauses in nested bool, got %d", len(nestedBool.Should))
		}
	}
}

func TestParseMultiMatchQuery(t *testing.T) {
	query := `{
		"query": {
			"multi_match": {
				"query": "search engine",
				"fields": ["title", "description", "content"],
				"type": "best_fields"
			}
		}
	}`

	parser := NewQueryParser()
	req, err := parser.ParseSearchRequest([]byte(query))
	if err != nil {
		t.Fatalf("ParseSearchRequest() error = %v", err)
	}

	multiMatch, ok := req.ParsedQuery.(*MultiMatchQuery)
	if !ok {
		t.Fatalf("Expected MultiMatchQuery, got %T", req.ParsedQuery)
	}

	if multiMatch.Query != "search engine" {
		t.Errorf("Expected query 'search engine', got '%s'", multiMatch.Query)
	}

	if len(multiMatch.Fields) != 3 {
		t.Errorf("Expected 3 fields, got %d", len(multiMatch.Fields))
	}

	if multiMatch.Type != "best_fields" {
		t.Errorf("Expected type 'best_fields', got '%s'", multiMatch.Type)
	}
}

func TestParseExistsQuery(t *testing.T) {
	query := `{
		"query": {
			"exists": {
				"field": "user"
			}
		}
	}`

	parser := NewQueryParser()
	req, err := parser.ParseSearchRequest([]byte(query))
	if err != nil {
		t.Fatalf("ParseSearchRequest() error = %v", err)
	}

	existsQuery, ok := req.ParsedQuery.(*ExistsQuery)
	if !ok {
		t.Fatalf("Expected ExistsQuery, got %T", req.ParsedQuery)
	}

	if existsQuery.Field != "user" {
		t.Errorf("Expected field 'user', got '%s'", existsQuery.Field)
	}
}

func TestParseMatchAllQuery(t *testing.T) {
	query := `{
		"query": {
			"match_all": {}
		}
	}`

	parser := NewQueryParser()
	req, err := parser.ParseSearchRequest([]byte(query))
	if err != nil {
		t.Fatalf("ParseSearchRequest() error = %v", err)
	}

	matchAll, ok := req.ParsedQuery.(*MatchAllQuery)
	if !ok {
		t.Fatalf("Expected MatchAllQuery, got %T", req.ParsedQuery)
	}

	if matchAll == nil {
		t.Error("MatchAllQuery is nil")
	}
}

func TestValidateQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   Query
		wantErr bool
	}{
		{
			name: "valid match query",
			query: &MatchQuery{
				Field: "title",
				Query: "search",
			},
			wantErr: false,
		},
		{
			name: "invalid match query - empty field",
			query: &MatchQuery{
				Field: "",
				Query: "search",
			},
			wantErr: true,
		},
		{
			name: "invalid match query - empty query",
			query: &MatchQuery{
				Field: "title",
				Query: "",
			},
			wantErr: true,
		},
		{
			name: "valid bool query",
			query: &BoolQuery{
				Must: []Query{
					&MatchQuery{Field: "title", Query: "search"},
				},
			},
			wantErr: false,
		},
		{
			name:    "invalid bool query - empty",
			query:   &BoolQuery{},
			wantErr: true,
		},
	}

	parser := NewQueryParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.Validate(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetQueryFields(t *testing.T) {
	query := &BoolQuery{
		Must: []Query{
			&MatchQuery{Field: "title", Query: "search"},
			&TermQuery{Field: "status", Value: "published"},
		},
		Filter: []Query{
			&RangeQuery{Field: "age", Gte: 18},
		},
	}

	fields := GetQueryFields(query)

	expectedFields := []string{"title", "status", "age"}
	if len(fields) != len(expectedFields) {
		t.Errorf("Expected %d fields, got %d", len(expectedFields), len(fields))
	}

	// Create a map for easy lookup
	fieldMap := make(map[string]bool)
	for _, f := range fields {
		fieldMap[f] = true
	}

	for _, expected := range expectedFields {
		if !fieldMap[expected] {
			t.Errorf("Expected field '%s' not found", expected)
		}
	}
}

func TestEstimateComplexity(t *testing.T) {
	tests := []struct {
		name     string
		query    Query
		minScore int
	}{
		{
			name:     "match_all",
			query:    &MatchAllQuery{},
			minScore: 1,
		},
		{
			name:     "term query",
			query:    &TermQuery{Field: "status", Value: "published"},
			minScore: 10,
		},
		{
			name:     "match query",
			query:    &MatchQuery{Field: "title", Query: "search"},
			minScore: 50,
		},
		{
			name: "bool query with multiple clauses",
			query: &BoolQuery{
				Must: []Query{
					&MatchQuery{Field: "title", Query: "search"},
					&TermQuery{Field: "status", Value: "published"},
				},
			},
			minScore: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			complexity := EstimateComplexity(tt.query)
			if complexity < tt.minScore {
				t.Errorf("Expected complexity >= %d, got %d", tt.minScore, complexity)
			}
		})
	}
}

func TestCanUseFilter(t *testing.T) {
	tests := []struct {
		name      string
		query     Query
		canFilter bool
	}{
		{
			name:      "term query can be filter",
			query:     &TermQuery{Field: "status", Value: "published"},
			canFilter: true,
		},
		{
			name:      "range query can be filter",
			query:     &RangeQuery{Field: "age", Gte: 18},
			canFilter: true,
		},
		{
			name:      "match query cannot be filter",
			query:     &MatchQuery{Field: "title", Query: "search"},
			canFilter: false,
		},
		{
			name: "bool with only filterable queries",
			query: &BoolQuery{
				Must: []Query{
					&TermQuery{Field: "status", Value: "published"},
					&RangeQuery{Field: "age", Gte: 18},
				},
			},
			canFilter: true,
		},
		{
			name: "bool with non-filterable query",
			query: &BoolQuery{
				Must: []Query{
					&MatchQuery{Field: "title", Query: "search"},
					&TermQuery{Field: "status", Value: "published"},
				},
			},
			canFilter: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanUseFilter(tt.query)
			if result != tt.canFilter {
				t.Errorf("CanUseFilter() = %v, want %v", result, tt.canFilter)
			}
		})
	}
}

func TestParseSearchRequestWithOptions(t *testing.T) {
	query := `{
		"query": {
			"match": {
				"title": "search"
			}
		},
		"size": 20,
		"from": 10,
		"sort": [
			{"date": "desc"}
		],
		"_source": ["title", "date"],
		"timeout": "5s"
	}`

	parser := NewQueryParser()
	req, err := parser.ParseSearchRequest([]byte(query))
	if err != nil {
		t.Fatalf("ParseSearchRequest() error = %v", err)
	}

	if req.Size != 20 {
		t.Errorf("Expected size=20, got %d", req.Size)
	}

	if req.From != 10 {
		t.Errorf("Expected from=10, got %d", req.From)
	}

	if req.Timeout != "5s" {
		t.Errorf("Expected timeout='5s', got '%s'", req.Timeout)
	}

	if len(req.Sort) != 1 {
		t.Errorf("Expected 1 sort clause, got %d", len(req.Sort))
	}
}

// Benchmark tests
func BenchmarkParseSimpleMatch(b *testing.B) {
	query := `{"query": {"match": {"title": "search"}}}`
	parser := NewQueryParser()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.ParseSearchRequest([]byte(query))
	}
}

func BenchmarkParseComplexBool(b *testing.B) {
	query := `{
		"query": {
			"bool": {
				"must": [
					{"match": {"title": "search"}},
					{"match": {"description": "engine"}}
				],
				"filter": [
					{"term": {"status": "published"}},
					{"range": {"age": {"gte": 18}}}
				]
			}
		}
	}`
	parser := NewQueryParser()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.ParseSearchRequest([]byte(query))
	}
}

// Example usage
func ExampleQueryParser_ParseSearchRequest() {
	parser := NewQueryParser()

	queryJSON := `{
		"query": {
			"bool": {
				"must": [
					{"match": {"title": "search engine"}}
				],
				"filter": [
					{"term": {"status": "published"}}
				]
			}
		},
		"size": 10
	}`

	req, err := parser.ParseSearchRequest([]byte(queryJSON))
	if err != nil {
		panic(err)
	}

	// Validate the query
	if err := parser.Validate(req.ParsedQuery); err != nil {
		panic(err)
	}

	// Get query fields
	fields := GetQueryFields(req.ParsedQuery)
	_ = fields // ["title", "status"]

	// Check complexity
	complexity := EstimateComplexity(req.ParsedQuery)
	_ = complexity

	// Output: Query parsed successfully
	println("Query parsed successfully")
}
