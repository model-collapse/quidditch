package parser

import (
	"testing"
)

func TestParseExpressionQuery(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name    string
		query   map[string]interface{}
		wantErr bool
	}{
		{
			name: "simple comparison",
			query: map[string]interface{}{
				"expr": map[string]interface{}{
					"op": ">",
					"left": map[string]interface{}{
						"field": "price",
					},
					"right": map[string]interface{}{
						"const": 100.0,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "arithmetic expression",
			query: map[string]interface{}{
				"expr": map[string]interface{}{
					"op": ">",
					"left": map[string]interface{}{
						"op": "*",
						"left": map[string]interface{}{
							"field": "price",
						},
						"right": map[string]interface{}{
							"const": 1.2,
						},
					},
					"right": map[string]interface{}{
						"const": 100.0,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "logical expression",
			query: map[string]interface{}{
				"expr": map[string]interface{}{
					"op": "&&",
					"left": map[string]interface{}{
						"op": ">",
						"left": map[string]interface{}{
							"field": "price",
						},
						"right": map[string]interface{}{
							"const": 100.0,
						},
					},
					"right": map[string]interface{}{
						"op": "<",
						"left": map[string]interface{}{
							"field": "price",
						},
						"right": map[string]interface{}{
							"const": 1000.0,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "function expression",
			query: map[string]interface{}{
				"expr": map[string]interface{}{
					"op": ">",
					"left": map[string]interface{}{
						"func": "abs",
						"args": []interface{}{
							map[string]interface{}{
								"field": "temperature",
							},
						},
					},
					"right": map[string]interface{}{
						"const": 10.0,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid expression - missing operator",
			query: map[string]interface{}{
				"expr": map[string]interface{}{
					"left": map[string]interface{}{
						"field": "price",
					},
					"right": map[string]interface{}{
						"const": 100.0,
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := parser.ParseQuery(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify it's an ExpressionQuery
				exprQuery, ok := query.(*ExpressionQuery)
				if !ok {
					t.Errorf("Expected *ExpressionQuery, got %T", query)
					return
				}

				// Verify expression is parsed
				if exprQuery.Expression == nil {
					t.Error("Expression is nil")
				}

				// Verify serialization was done
				if len(exprQuery.SerializedExpression) == 0 {
					t.Error("SerializedExpression is empty")
				}

				// Verify validation passes
				if err := parser.Validate(query); err != nil {
					t.Errorf("Validation failed: %v", err)
				}
			}
		})
	}
}

func TestParseExpressionQueryInBool(t *testing.T) {
	parser := NewQueryParser()

	// Test expression query inside bool query filter
	query := map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []interface{}{
				map[string]interface{}{
					"match": map[string]interface{}{
						"title": "laptop",
					},
				},
			},
			"filter": []interface{}{
				map[string]interface{}{
					"expr": map[string]interface{}{
						"op": ">",
						"left": map[string]interface{}{
							"field": "price",
						},
						"right": map[string]interface{}{
							"const": 100.0,
						},
					},
				},
			},
		},
	}

	parsedQuery, err := parser.ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery failed: %v", err)
	}

	boolQuery, ok := parsedQuery.(*BoolQuery)
	if !ok {
		t.Fatalf("Expected *BoolQuery, got %T", parsedQuery)
	}

	if len(boolQuery.Filter) != 1 {
		t.Fatalf("Expected 1 filter clause, got %d", len(boolQuery.Filter))
	}

	exprQuery, ok := boolQuery.Filter[0].(*ExpressionQuery)
	if !ok {
		t.Fatalf("Expected filter to be *ExpressionQuery, got %T", boolQuery.Filter[0])
	}

	if exprQuery.Expression == nil {
		t.Error("Expression is nil")
	}

	if len(exprQuery.SerializedExpression) == 0 {
		t.Error("SerializedExpression is empty")
	}
}

func TestParseExpressionQueryValidation(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name    string
		query   map[string]interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name: "type mismatch - string + int",
			query: map[string]interface{}{
				"expr": map[string]interface{}{
					"op": "+",
					"left": map[string]interface{}{
						"field": "name",
						"type":  "string",
					},
					"right": map[string]interface{}{
						"const": 10,
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid expression",
		},
		{
			name: "logical operator with non-bool",
			query: map[string]interface{}{
				"expr": map[string]interface{}{
					"op": "&&",
					"left": map[string]interface{}{
						"const": 10,
					},
					"right": map[string]interface{}{
						"const": 20,
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid expression",
		},
		{
			name: "function with wrong argument count",
			query: map[string]interface{}{
				"expr": map[string]interface{}{
					"func": "abs",
					"args": []interface{}{
						map[string]interface{}{
							"const": 10,
						},
						map[string]interface{}{
							"const": 20,
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid expression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.ParseQuery(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseQuery() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExpressionQueryType(t *testing.T) {
	exprQuery := &ExpressionQuery{}

	if exprQuery.QueryType() != "expr" {
		t.Errorf("Expected QueryType() to return 'expr', got '%s'", exprQuery.QueryType())
	}
}
