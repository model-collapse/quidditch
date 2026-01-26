package udfs_test

import (
	"context"
	"os"
	"testing"

	"github.com/quidditch/quidditch/pkg/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestStringDistanceUDF demonstrates the Rust-based string distance UDF
func TestStringDistanceUDF(t *testing.T) {
	// This test would work if the WASM file is built
	// Run: cd string-distance && ./build.sh

	wasmPath := "string-distance/dist/string_distance.wasm"
	if _, err := os.Stat(wasmPath); os.IsNotExist(err) {
		t.Skip("string_distance.wasm not built. Run: cd string-distance && ./build.sh")
	}

	wasmBytes, err := os.ReadFile(wasmPath)
	require.NoError(t, err)

	// Create runtime
	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	require.NoError(t, err)
	defer runtime.Close()

	// Create registry
	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	require.NoError(t, err)

	// Register UDF
	err = registry.Register(&wasm.UDFMetadata{
		Name:         "string_distance",
		Version:      "1.0.0",
		FunctionName: "filter",
		Description:  "Fuzzy string matching using Levenshtein distance",
		WASMBytes:    wasmBytes,
		Parameters: []wasm.UDFParameter{
			{Name: "field", Type: wasm.ValueTypeString, Default: "name"},
			{Name: "target", Type: wasm.ValueTypeString, Required: true},
			{Name: "max_distance", Type: wasm.ValueTypeI64, Default: int64(2)},
		},
		Returns: []wasm.UDFReturnType{
			{Type: wasm.ValueTypeI32, Description: "1 if match, 0 otherwise"},
		},
	})
	require.NoError(t, err)

	// Test cases
	tests := []struct {
		name       string
		docName    string
		target     string
		maxDist    int64
		shouldPass bool
	}{
		{"Exact match", "iPhone", "iPhone", 2, true},
		{"1 char diff", "IPhone", "iPhone", 2, true},
		{"2 char diff", "iPhonne", "iPhone", 2, true},
		{"3 char diff", "iPhonnes", "iPhone", 2, false},
		{"Different word", "Android", "iPhone", 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create document context
			docCtx := wasm.NewDocumentContextFromMap("doc1", 1.0, map[string]interface{}{
				"name": tt.docName,
			})

			// Call UDF
			params := map[string]wasm.Value{
				"field":        wasm.NewStringValue("name"),
				"target":       wasm.NewStringValue(tt.target),
				"max_distance": wasm.NewI64Value(tt.maxDist),
			}

			results, err := registry.Call(
				context.Background(),
				"string_distance",
				"1.0.0",
				docCtx,
				params,
			)
			require.NoError(t, err)
			require.Len(t, results, 1)

			result, err := results[0].AsInt32()
			require.NoError(t, err)

			if tt.shouldPass {
				assert.Equal(t, int32(1), result, "Expected UDF to return 1 (match)")
			} else {
				assert.Equal(t, int32(0), result, "Expected UDF to return 0 (no match)")
			}
		})
	}
}

// TestGeoFilterUDF demonstrates the C-based geo filter UDF
func TestGeoFilterUDF(t *testing.T) {
	wasmPath := "geo-filter/dist/geo_filter.wasm"
	if _, err := os.Stat(wasmPath); os.IsNotExist(err) {
		t.Skip("geo_filter.wasm not built. Run: cd geo-filter && ./build.sh")
	}

	wasmBytes, err := os.ReadFile(wasmPath)
	require.NoError(t, err)

	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	require.NoError(t, err)
	defer runtime.Close()

	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	require.NoError(t, err)

	// Register UDF
	err = registry.Register(&wasm.UDFMetadata{
		Name:         "geo_filter",
		Version:      "1.0.0",
		FunctionName: "filter",
		Description:  "Geographic distance filter using Haversine formula",
		WASMBytes:    wasmBytes,
		Parameters: []wasm.UDFParameter{
			{Name: "lat_field", Type: wasm.ValueTypeString, Default: "latitude"},
			{Name: "lon_field", Type: wasm.ValueTypeString, Default: "longitude"},
			{Name: "target_lat", Type: wasm.ValueTypeF64, Required: true},
			{Name: "target_lon", Type: wasm.ValueTypeF64, Required: true},
			{Name: "max_distance_km", Type: wasm.ValueTypeF64, Default: 10.0},
		},
		Returns: []wasm.UDFReturnType{
			{Type: wasm.ValueTypeI32, Description: "1 if within distance, 0 otherwise"},
		},
	})
	require.NoError(t, err)

	// Test cases: San Francisco (37.7749, -122.4194)
	tests := []struct {
		name       string
		docLat     float64
		docLon     float64
		maxDistKm  float64
		shouldPass bool
	}{
		{"Same location", 37.7749, -122.4194, 10, true},
		{"Within 1km", 37.7650, -122.4100, 10, true},
		{"Within 10km", 37.7000, -122.4000, 10, true},
		{"Beyond 10km", 37.3000, -122.0000, 10, false},
		{"Far away", 40.7128, -74.0060, 10, false}, // NYC
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docCtx := wasm.NewDocumentContextFromMap("doc1", 1.0, map[string]interface{}{
				"latitude":  tt.docLat,
				"longitude": tt.docLon,
			})

			params := map[string]wasm.Value{
				"target_lat":      wasm.NewF64Value(37.7749),
				"target_lon":      wasm.NewF64Value(-122.4194),
				"max_distance_km": wasm.NewF64Value(tt.maxDistKm),
			}

			results, err := registry.Call(
				context.Background(),
				"geo_filter",
				"1.0.0",
				docCtx,
				params,
			)
			require.NoError(t, err)
			require.Len(t, results, 1)

			result, err := results[0].AsInt32()
			require.NoError(t, err)

			if tt.shouldPass {
				assert.Equal(t, int32(1), result, "Expected location to be within range")
			} else {
				assert.Equal(t, int32(0), result, "Expected location to be out of range")
			}
		})
	}
}

// TestCustomScoreUDF demonstrates the WAT-based custom score UDF
func TestCustomScoreUDF(t *testing.T) {
	wasmPath := "custom-score/dist/custom_score.wasm"
	if _, err := os.Stat(wasmPath); os.IsNotExist(err) {
		t.Skip("custom_score.wasm not built. Run: cd custom-score && ./build.sh")
	}

	wasmBytes, err := os.ReadFile(wasmPath)
	require.NoError(t, err)

	logger := zap.NewNop()
	runtime, err := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	require.NoError(t, err)
	defer runtime.Close()

	registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	require.NoError(t, err)

	// Register UDF
	err = registry.Register(&wasm.UDFMetadata{
		Name:         "custom_score",
		Version:      "1.0.0",
		FunctionName: "filter",
		Description:  "Custom scoring with boost",
		WASMBytes:    wasmBytes,
		Parameters: []wasm.UDFParameter{
			{Name: "min_score", Type: wasm.ValueTypeF64, Default: 0.5},
		},
		Returns: []wasm.UDFReturnType{
			{Type: wasm.ValueTypeI32, Description: "1 if score >= min, 0 otherwise"},
		},
	})
	require.NoError(t, err)

	tests := []struct {
		name       string
		baseScore  float64
		boost      *float64 // nil = field not present
		minScore   float64
		shouldPass bool
	}{
		{"High score, no boost", 0.8, nil, 0.7, true},
		{"High score with boost", 0.6, ptr(1.5), 0.7, true}, // 0.6 * 1.5 = 0.9
		{"Low score", 0.3, nil, 0.7, false},
		{"Low score with boost", 0.4, ptr(1.5), 0.7, false}, // 0.4 * 1.5 = 0.6
		{"Exact threshold", 0.7, nil, 0.7, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := map[string]interface{}{
				"base_score": tt.baseScore,
			}
			if tt.boost != nil {
				doc["boost"] = *tt.boost
			}

			docCtx := wasm.NewDocumentContextFromMap("doc1", 1.0, doc)

			params := map[string]wasm.Value{
				"min_score": wasm.NewF64Value(tt.minScore),
			}

			results, err := registry.Call(
				context.Background(),
				"custom_score",
				"1.0.0",
				docCtx,
				params,
			)
			require.NoError(t, err)
			require.Len(t, results, 1)

			result, err := results[0].AsInt32()
			require.NoError(t, err)

			if tt.shouldPass {
				assert.Equal(t, int32(1), result, "Expected score to pass threshold")
			} else {
				assert.Equal(t, int32(0), result, "Expected score below threshold")
			}
		})
	}
}

// BenchmarkUDFExecution measures UDF execution performance
func BenchmarkUDFExecution(b *testing.B) {
	// Load a simple WASM module for benchmarking
	// Using the minimal test module from integration tests
	simpleWasm := []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x06, 0x01, 0x60, 0x01, 0x7e, 0x01, 0x7f,
		0x03, 0x02, 0x01, 0x00, 0x05, 0x03, 0x01, 0x00,
		0x01, 0x07, 0x13, 0x02, 0x06, 0x6d, 0x65, 0x6d,
		0x6f, 0x72, 0x79, 0x02, 0x00, 0x06, 0x66, 0x69,
		0x6c, 0x74, 0x65, 0x72, 0x00, 0x00, 0x0a, 0x06,
		0x01, 0x04, 0x00, 0x41, 0x01, 0x0b,
	}

	logger := zap.NewNop()
	runtime, _ := wasm.NewRuntime(&wasm.Config{
		EnableJIT: true,
		Logger:    logger,
	})
	defer runtime.Close()

	registry, _ := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})

	registry.Register(&wasm.UDFMetadata{
		Name:         "benchmark_udf",
		Version:      "1.0.0",
		FunctionName: "filter",
		WASMBytes:    simpleWasm,
		Parameters:   []wasm.UDFParameter{},
		Returns:      []wasm.UDFReturnType{{Type: wasm.ValueTypeI32}},
	})

	docCtx := wasm.NewDocumentContextFromMap("doc1", 1.0, map[string]interface{}{
		"name": "test",
	})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.Call(ctx, "benchmark_udf", "1.0.0", docCtx, nil)
	}
}

// Helper function
func ptr(f float64) *float64 {
	return &f
}
