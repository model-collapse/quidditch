package diagon

import (
	"encoding/json"
	"testing"

	"go.uber.org/zap"
)

func TestRangeQueryConversion(t *testing.T) {
	// Create a test shard (mock) with noop logger
	logger := zap.NewNop()
	shard := &Shard{
		logger: logger,
	}

	tests := []struct {
		name        string
		queryJSON   string
		shouldError bool
		description string
	}{
		{
			name: "range_both_bounds",
			queryJSON: `{
				"range": {
					"price": {
						"gte": 100,
						"lte": 1000
					}
				}
			}`,
			shouldError: false,
			description: "Range query with both inclusive bounds",
		},
		{
			name: "range_lower_only",
			queryJSON: `{
				"range": {
					"price": {
						"gte": 100
					}
				}
			}`,
			shouldError: false,
			description: "Range query with lower bound only",
		},
		{
			name: "range_upper_only",
			queryJSON: `{
				"range": {
					"price": {
						"lte": 1000
					}
				}
			}`,
			shouldError: false,
			description: "Range query with upper bound only",
		},
		{
			name: "range_exclusive",
			queryJSON: `{
				"range": {
					"price": {
						"gt": 100,
						"lt": 1000
					}
				}
			}`,
			shouldError: false,
			description: "Range query with exclusive bounds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var queryObj map[string]interface{}
			if err := json.Unmarshal([]byte(tt.queryJSON), &queryObj); err != nil {
				t.Fatalf("Failed to parse test query JSON: %v", err)
			}

			// Note: We can't actually execute this without a real Diagon index
			// But we can test that the conversion doesn't crash
			_, err := shard.convertQueryToDiagon(queryObj)

			if tt.shouldError && err == nil {
				t.Errorf("%s: expected error but got none", tt.description)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
			}
		})
	}
}

func TestBoolQueryConversion(t *testing.T) {
	logger := zap.NewNop()
	shard := &Shard{
		logger: logger,
	}

	tests := []struct {
		name        string
		queryJSON   string
		shouldError bool
		description string
	}{
		{
			name: "bool_must_only",
			queryJSON: `{
				"bool": {
					"must": [
						{"term": {"category": "electronics"}}
					]
				}
			}`,
			shouldError: false,
			description: "Bool query with MUST clause",
		},
		{
			name: "bool_must_filter",
			queryJSON: `{
				"bool": {
					"must": [
						{"term": {"category": "electronics"}}
					],
					"filter": [
						{"range": {"price": {"lte": 1000}}}
					]
				}
			}`,
			shouldError: false,
			description: "Bool query with MUST and FILTER",
		},
		{
			name: "bool_complex",
			queryJSON: `{
				"bool": {
					"must": [
						{"term": {"category": "electronics"}}
					],
					"should": [
						{"term": {"brand": "Apple"}},
						{"term": {"brand": "Samsung"}}
					],
					"filter": [
						{"range": {"price": {"gte": 100, "lte": 1500}}}
					],
					"must_not": [
						{"term": {"refurbished": true}}
					],
					"minimum_should_match": 1
				}
			}`,
			shouldError: false,
			description: "Complex bool query with all clause types",
		},
		{
			name: "bool_nested",
			queryJSON: `{
				"bool": {
					"must": [
						{
							"bool": {
								"should": [
									{"term": {"brand": "Apple"}},
									{"term": {"brand": "Samsung"}}
								]
							}
						}
					],
					"filter": [
						{"range": {"price": {"lte": 2000}}}
					]
				}
			}`,
			shouldError: false,
			description: "Nested bool query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var queryObj map[string]interface{}
			if err := json.Unmarshal([]byte(tt.queryJSON), &queryObj); err != nil {
				t.Fatalf("Failed to parse test query JSON: %v", err)
			}

			_, err := shard.convertQueryToDiagon(queryObj)

			if tt.shouldError && err == nil {
				t.Errorf("%s: expected error but got none", tt.description)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
			}
		})
	}
}

func TestQueryTypeSupport(t *testing.T) {
	logger := zap.NewNop()
	shard := &Shard{
		logger: logger,
	}

	supportedQueries := []string{
		`{"term": {"field": "value"}}`,
		`{"match": {"field": "text"}}`,
		// Note: match_all is parsed but returns stub error (not yet implemented in Diagon)
		// `{"match_all": {}}`,
		`{"range": {"price": {"gte": 100}}}`,
		`{"bool": {"must": [{"term": {"field": "value"}}]}}`,
	}

	for _, queryJSON := range supportedQueries {
		var queryObj map[string]interface{}
		if err := json.Unmarshal([]byte(queryJSON), &queryObj); err != nil {
			t.Fatalf("Failed to parse query: %v", err)
		}

		_, err := shard.convertQueryToDiagon(queryObj)
		if err != nil {
			t.Errorf("Query should be supported but got error: %s\nQuery: %s", err, queryJSON)
		}
	}

	unsupportedQueries := []string{
		`{"wildcard": {"field": "val*"}}`,
		`{"fuzzy": {"field": "value"}}`,
		`{"prefix": {"field": "pre"}}`,
	}

	for _, queryJSON := range unsupportedQueries {
		var queryObj map[string]interface{}
		if err := json.Unmarshal([]byte(queryJSON), &queryObj); err != nil {
			t.Fatalf("Failed to parse query: %v", err)
		}

		_, err := shard.convertQueryToDiagon(queryObj)
		if err == nil {
			t.Errorf("Query should be unsupported but got no error. Query: %s", queryJSON)
		}
	}
}
