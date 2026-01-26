package data

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/quidditch/quidditch/pkg/data/diagon"
	"github.com/quidditch/quidditch/pkg/wasm"
	"go.uber.org/zap"
)

// Mock UDF for testing
const mockUDFWasm = `
(module
  (import "env" "get_field_string" (func $get_field_string (param i64 i32 i32 i32 i32) (result i32)))
  (import "env" "has_field" (func $has_field (param i64 i32 i32) (result i32)))

  (memory (export "memory") 1)

  ;; Filter function: returns 1 if category == "electronics"
  (func (export "filter") (param $ctx_id i64) (result i32)
    (local $field_ptr i32)
    (local $field_len i32)
    (local $value_ptr i32)
    (local $value_len i32)
    (local $result i32)

    ;; Set field name "category" at memory offset 100
    (i32.store offset=100 (i32.const 0) (i32.const 0x74616763))  ;; "catg"
    (i32.store offset=104 (i32.const 0) (i32.const 0x79726f65))  ;; "eory"

    (local.set $field_ptr (i32.const 100))
    (local.set $field_len (i32.const 8))

    ;; Check if field exists
    (local.set $result
      (call $has_field
        (local.get $ctx_id)
        (local.get $field_ptr)
        (local.get $field_len)))

    (if (i32.eqz (local.get $result))
      (then (return (i32.const 0))))

    ;; Get field value
    (local.set $value_ptr (i32.const 200))
    (local.set $value_len (i32.const 100))

    (local.set $result
      (call $get_field_string
        (local.get $ctx_id)
        (local.get $field_ptr)
        (local.get $field_len)
        (local.get $value_ptr)
        (i32.add (local.get $value_ptr) (i32.const 96))))

    (if (i32.eqz (local.get $result))
      (then (return (i32.const 0))))

    ;; Compare with "electronics"
    ;; For simplicity, return 1 (pass all documents in test)
    (return (i32.const 1))
  )
)
`

func TestNewUDFFilter(t *testing.T) {
	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	filter := NewUDFFilter(registry, logger)

	if filter == nil {
		t.Fatal("NewUDFFilter returned nil")
	}
	if filter.registry != registry {
		t.Error("Registry not set correctly")
	}
	if filter.parser == nil {
		t.Error("Parser not initialized")
	}
}

func TestHasWasmUDFQuery(t *testing.T) {
	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	filter := NewUDFFilter(registry, logger)

	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name: "standalone wasm_udf query",
			query: `{
				"wasm_udf": {
					"name": "test_udf",
					"version": "1.0.0"
				}
			}`,
			expected: true,
		},
		{
			name: "bool query with wasm_udf in filter",
			query: `{
				"bool": {
					"filter": [
						{
							"term": {
								"status": "active"
							}
						},
						{
							"wasm_udf": {
								"name": "test_udf"
							}
						}
					]
				}
			}`,
			expected: true,
		},
		{
			name: "term query (no UDF)",
			query: `{
				"term": {
					"status": "active"
				}
			}`,
			expected: false,
		},
		{
			name: "bool query without UDF",
			query: `{
				"bool": {
					"must": [
						{
							"term": {
								"status": "active"
							}
						}
					]
				}
			}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.HasWasmUDFQuery([]byte(tt.query))
			if result != tt.expected {
				t.Errorf("HasWasmUDFQuery() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractWasmUDFQuery(t *testing.T) {
	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	filter := NewUDFFilter(registry, logger)

	tests := []struct {
		name        string
		query       string
		expectUDF   bool
		expectedName string
	}{
		{
			name: "standalone wasm_udf",
			query: `{
				"wasm_udf": {
					"name": "my_udf",
					"version": "2.0.0"
				}
			}`,
			expectUDF:    true,
			expectedName: "my_udf",
		},
		{
			name: "bool query with UDF",
			query: `{
				"bool": {
					"filter": [
						{
							"wasm_udf": {
								"name": "filter_udf"
							}
						}
					]
				}
			}`,
			expectUDF:    true,
			expectedName: "filter_udf",
		},
		{
			name: "no UDF query",
			query: `{
				"term": {
					"field": "value"
				}
			}`,
			expectUDF: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			udfQuery, err := filter.extractWasmUDFQuery([]byte(tt.query))
			if err != nil {
				t.Fatalf("extractWasmUDFQuery() error: %v", err)
			}

			if tt.expectUDF {
				if udfQuery == nil {
					t.Fatal("Expected UDF query, got nil")
				}
				if udfQuery.Name != tt.expectedName {
					t.Errorf("Expected UDF name %s, got %s", tt.expectedName, udfQuery.Name)
				}
			} else {
				if udfQuery != nil {
					t.Errorf("Expected no UDF query, got %+v", udfQuery)
				}
			}
		})
	}
}

func TestConvertValue(t *testing.T) {
	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	filter := NewUDFFilter(registry, logger)

	tests := []struct {
		name      string
		input     interface{}
		expectErr bool
		checkType func(wasm.Value) bool
	}{
		{
			name:      "boolean true",
			input:     true,
			expectErr: false,
			checkType: func(v wasm.Value) bool { return v.Type == wasm.ValueTypeBool },
		},
		{
			name:      "int",
			input:     42,
			expectErr: false,
			checkType: func(v wasm.Value) bool { return v.Type == wasm.ValueTypeI64 },
		},
		{
			name:      "int64",
			input:     int64(123),
			expectErr: false,
			checkType: func(v wasm.Value) bool { return v.Type == wasm.ValueTypeI64 },
		},
		{
			name:      "float64",
			input:     3.14,
			expectErr: false,
			checkType: func(v wasm.Value) bool { return v.Type == wasm.ValueTypeF64 },
		},
		{
			name:      "string",
			input:     "test",
			expectErr: false,
			checkType: func(v wasm.Value) bool { return v.Type == wasm.ValueTypeString },
		},
		{
			name:      "unsupported type",
			input:     []int{1, 2, 3},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := filter.convertValue(tt.input)

			if tt.expectErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !tt.checkType(val) {
				t.Errorf("Value type mismatch: got %v", val.Type)
			}
		})
	}
}

func TestFilterResultsNoUDF(t *testing.T) {
	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	filter := NewUDFFilter(registry, logger)

	// Query without UDF
	query := []byte(`{"term": {"status": "active"}}`)

	// Test results
	results := &diagon.SearchResult{
		Took:      10,
		TotalHits: 2,
		MaxScore:  1.0,
		Hits: []*diagon.Hit{
			{ID: "doc1", Score: 1.0, Source: map[string]interface{}{"status": "active"}},
			{ID: "doc2", Score: 0.9, Source: map[string]interface{}{"status": "active"}},
		},
	}

	// Filter should return results unchanged
	filtered, err := filter.FilterResults(context.Background(), query, results)
	if err != nil {
		t.Fatalf("FilterResults() error: %v", err)
	}

	if filtered.TotalHits != results.TotalHits {
		t.Errorf("TotalHits changed: got %d, want %d", filtered.TotalHits, results.TotalHits)
	}

	if len(filtered.Hits) != len(results.Hits) {
		t.Errorf("Hits count changed: got %d, want %d", len(filtered.Hits), len(results.Hits))
	}
}

func TestConvertParameters(t *testing.T) {
	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	filter := NewUDFFilter(registry, logger)

	params := map[string]interface{}{
		"field":   "product_name",
		"target":  "iPhone",
		"max_distance": 3,
		"enabled": true,
	}

	values, err := filter.convertParameters(params)
	if err != nil {
		t.Fatalf("convertParameters() error: %v", err)
	}

	if len(values) != len(params) {
		t.Errorf("Expected %d values, got %d", len(params), len(values))
	}

	// Verify at least one of each type
	hasString := false
	hasInt := false
	hasBool := false

	for _, v := range values {
		switch v.Type {
		case wasm.ValueTypeString:
			hasString = true
		case wasm.ValueTypeI64:
			hasInt = true
		case wasm.ValueTypeBool:
			hasBool = true
		}
	}

	if !hasString {
		t.Error("No string parameter found")
	}
	if !hasInt {
		t.Error("No int parameter found")
	}
	if !hasBool {
		t.Error("No bool parameter found")
	}
}

func TestInvalidQueryJSON(t *testing.T) {
	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	filter := NewUDFFilter(registry, logger)

	// Invalid JSON
	invalidQuery := []byte(`{invalid json`)

	results := &diagon.SearchResult{
		TotalHits: 1,
		Hits: []*diagon.Hit{
			{ID: "doc1", Score: 1.0, Source: map[string]interface{}{}},
		},
	}

	_, err = filter.FilterResults(context.Background(), invalidQuery, results)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func BenchmarkHasWasmUDFQuery(b *testing.B) {
	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	if err != nil {
		b.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		b.Fatalf("Failed to create registry: %v", err)
	}

	filter := NewUDFFilter(registry, logger)

	query := []byte(`{
		"bool": {
			"filter": [
				{"term": {"category": "electronics"}},
				{
					"wasm_udf": {
						"name": "filter_udf",
						"version": "1.0.0"
					}
				}
			]
		}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.HasWasmUDFQuery(query)
	}
}

func BenchmarkExtractWasmUDFQuery(b *testing.B) {
	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	if err != nil {
		b.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		b.Fatalf("Failed to create registry: %v", err)
	}

	filter := NewUDFFilter(registry, logger)

	query := []byte(`{
		"wasm_udf": {
			"name": "test_udf",
			"version": "1.0.0",
			"parameters": {
				"field": "name",
				"value": "test"
			}
		}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = filter.extractWasmUDFQuery(query)
	}
}

// Helper function to create test query JSON
func createTestQuery(queryType string, content map[string]interface{}) []byte {
	query := map[string]interface{}{
		queryType: content,
	}
	data, _ := json.Marshal(query)
	return data
}
