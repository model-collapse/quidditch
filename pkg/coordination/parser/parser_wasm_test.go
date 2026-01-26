package parser

import (
	"encoding/json"
	"testing"
)

func TestParseWasmUDFQuery(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantName string
		wantVer  string
		wantErr  bool
	}{
		{
			name: "basic wasm_udf query",
			json: `{
				"wasm_udf": {
					"name": "string_distance",
					"version": "1.0.0",
					"parameters": {
						"field": "product_name",
						"target": "iPhone 15",
						"max_distance": 3
					}
				}
			}`,
			wantName: "string_distance",
			wantVer:  "1.0.0",
			wantErr:  false,
		},
		{
			name: "wasm_udf without version",
			json: `{
				"wasm_udf": {
					"name": "custom_score",
					"parameters": {
						"price_weight": 0.3,
						"rating_weight": 0.5
					}
				}
			}`,
			wantName: "custom_score",
			wantVer:  "",
			wantErr:  false,
		},
		{
			name: "wasm_udf with params alias",
			json: `{
				"wasm_udf": {
					"name": "json_path",
					"params": {
						"field": "metadata",
						"path": "$.category"
					}
				}
			}`,
			wantName: "json_path",
			wantVer:  "",
			wantErr:  false,
		},
		{
			name: "wasm_udf without name - should fail",
			json: `{
				"wasm_udf": {
					"parameters": {
						"field": "test"
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "wasm_udf with empty parameters",
			json: `{
				"wasm_udf": {
					"name": "simple_udf"
				}
			}`,
			wantName: "simple_udf",
			wantVer:  "",
			wantErr:  false,
		},
	}

	parser := NewQueryParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var queryMap map[string]interface{}
			if err := json.Unmarshal([]byte(tt.json), &queryMap); err != nil {
				t.Fatalf("Failed to parse JSON: %v", err)
			}

			query, err := parser.ParseQuery(queryMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			wasmQuery, ok := query.(*WasmUDFQuery)
			if !ok {
				t.Fatalf("Expected WasmUDFQuery, got %T", query)
			}

			if wasmQuery.Name != tt.wantName {
				t.Errorf("Expected name %s, got %s", tt.wantName, wasmQuery.Name)
			}

			if wasmQuery.Version != tt.wantVer {
				t.Errorf("Expected version %s, got %s", tt.wantVer, wasmQuery.Version)
			}

			if wasmQuery.Parameters == nil {
				t.Error("Expected parameters map, got nil")
			}
		})
	}
}

func TestWasmUDFQueryType(t *testing.T) {
	query := &WasmUDFQuery{
		Name:       "test",
		Version:    "1.0.0",
		Parameters: map[string]interface{}{},
	}

	if query.QueryType() != "wasm_udf" {
		t.Errorf("Expected QueryType 'wasm_udf', got '%s'", query.QueryType())
	}
}

func TestIsTermLevelQueryWithWasmUDF(t *testing.T) {
	query := &WasmUDFQuery{
		Name:       "test",
		Parameters: map[string]interface{}{},
	}

	if !IsTermLevelQuery(query) {
		t.Error("Expected WasmUDFQuery to be term-level query")
	}
}

func TestCanUseFilterWithWasmUDF(t *testing.T) {
	query := &WasmUDFQuery{
		Name:       "test",
		Parameters: map[string]interface{}{},
	}

	if !CanUseFilter(query) {
		t.Error("Expected WasmUDFQuery to be usable as filter")
	}
}

func TestEstimateComplexityWithWasmUDF(t *testing.T) {
	query := &WasmUDFQuery{
		Name:       "test",
		Parameters: map[string]interface{}{},
	}

	complexity := EstimateComplexity(query)
	if complexity != 40 {
		t.Errorf("Expected complexity 40, got %d", complexity)
	}
}

func TestWasmUDFQueryValidation(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name    string
		query   *WasmUDFQuery
		wantErr bool
	}{
		{
			name: "valid query",
			query: &WasmUDFQuery{
				Name:       "test_udf",
				Version:    "1.0.0",
				Parameters: map[string]interface{}{"field": "test"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			query: &WasmUDFQuery{
				Parameters: map[string]interface{}{"field": "test"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.Validate(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWasmUDFInBoolQuery(t *testing.T) {
	jsonStr := `{
		"bool": {
			"filter": [
				{
					"term": {
						"category": "electronics"
					}
				},
				{
					"wasm_udf": {
						"name": "string_distance",
						"version": "1.0.0",
						"parameters": {
							"field": "product_name",
							"target": "iPhone",
							"max_distance": 2
						}
					}
				}
			]
		}
	}`

	var queryMap map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &queryMap); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	parser := NewQueryParser()
	query, err := parser.ParseQuery(queryMap)
	if err != nil {
		t.Fatalf("ParseQuery() failed: %v", err)
	}

	boolQuery, ok := query.(*BoolQuery)
	if !ok {
		t.Fatalf("Expected BoolQuery, got %T", query)
	}

	if len(boolQuery.Filter) != 2 {
		t.Fatalf("Expected 2 filters, got %d", len(boolQuery.Filter))
	}

	// Check second filter is WasmUDFQuery
	wasmQuery, ok := boolQuery.Filter[1].(*WasmUDFQuery)
	if !ok {
		t.Fatalf("Expected second filter to be WasmUDFQuery, got %T", boolQuery.Filter[1])
	}

	if wasmQuery.Name != "string_distance" {
		t.Errorf("Expected name 'string_distance', got '%s'", wasmQuery.Name)
	}

	if wasmQuery.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", wasmQuery.Version)
	}

	// Verify it can be used as filter
	if !CanUseFilter(boolQuery) {
		t.Error("Expected bool query with WasmUDF to be usable as filter")
	}

	t.Log("✅ WasmUDF query in bool filter working correctly")
}
