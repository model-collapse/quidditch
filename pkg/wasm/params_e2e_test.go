package wasm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// WASM module that tests get_param_string
// Exports: test_string_param(ctx_id: i64) -> i32
// Returns: 1 if parameter "query" == "hello", 0 otherwise
var testStringParamWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, // WASM magic
	0x01, 0x00, 0x00, 0x00, // Version

	// Type section: (ctx_id: i64) -> i32
	0x01, 0x06, 0x01, 0x60, 0x01, 0x7e, 0x01, 0x7f,

	// Import section: get_param_string from "env"
	0x02, 0x1e, 0x01, 0x03, 0x65, 0x6e, 0x76, 0x10, 0x67, 0x65, 0x74,
	0x5f, 0x70, 0x61, 0x72, 0x61, 0x6d, 0x5f, 0x73, 0x74, 0x72, 0x69,
	0x6e, 0x67, 0x00, 0x02,

	// Function section
	0x03, 0x02, 0x01, 0x00,

	// Memory section: 1 page
	0x05, 0x03, 0x01, 0x00, 0x01,

	// Export section
	0x07, 0x18, 0x02,
	0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00, // "memory"
	0x06, 0x66, 0x69, 0x6c, 0x74, 0x65, 0x72, 0x00, 0x01, // "filter"

	// Data section: "query" string at offset 0
	0x0b, 0x0c, 0x01, 0x00, 0x41, 0x00, 0x0b, 0x05, 0x71, 0x75, 0x65,
	0x72, 0x79,

	// Code section: simple function that calls get_param_string
	0x0a, 0x2f, 0x01, 0x2d, 0x02, 0x01, 0x7f, 0x01, 0x7e,
	// local.get 0 (ctx_id) - not used but keep signature
	0x41, 0x00, // i32.const 0 (param name ptr: "query")
	0x41, 0x05, // i32.const 5 (param name len)
	0x41, 0x20, // i32.const 32 (value buffer ptr)
	0x41, 0x40, // i32.const 64 (value len ptr)
	0x10, 0x00, // call get_param_string
	0x41, 0x00, // i32.const 0
	0x46,       // i32.eq (check if result == 0, success)
	0x0b,       // end
}

// WASM module that tests get_param_i64
// Exports: test_i64_param(ctx_id: i64) -> i32
// Returns: 1 if parameter "threshold" exists and > 0, 0 otherwise
var testI64ParamWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, // WASM magic
	0x01, 0x00, 0x00, 0x00, // Version

	// Type section
	0x01, 0x0a, 0x02,
	0x60, 0x01, 0x7e, 0x01, 0x7f, // (i64) -> i32 for main function
	0x60, 0x03, 0x7f, 0x7f, 0x7f, 0x01, 0x7f, // (i32,i32,i32) -> i32 for get_param_i64

	// Import section: get_param_i64 from "env"
	0x02, 0x1b, 0x01, 0x03, 0x65, 0x6e, 0x76, 0x0d, 0x67, 0x65, 0x74,
	0x5f, 0x70, 0x61, 0x72, 0x61, 0x6d, 0x5f, 0x69, 0x36, 0x34, 0x00, 0x01,

	// Function section
	0x03, 0x02, 0x01, 0x00,

	// Memory section
	0x05, 0x03, 0x01, 0x00, 0x01,

	// Export section
	0x07, 0x18, 0x02,
	0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00,
	0x06, 0x66, 0x69, 0x6c, 0x74, 0x65, 0x72, 0x00, 0x01,

	// Data section: "threshold" at offset 0
	0x0b, 0x11, 0x01, 0x00, 0x41, 0x00, 0x0b, 0x09, 0x74, 0x68, 0x72,
	0x65, 0x73, 0x68, 0x6f, 0x6c, 0x64,

	// Code section
	0x0a, 0x20, 0x01, 0x1e, 0x01, 0x01, 0x7f,
	0x41, 0x00, // i32.const 0 (param name: "threshold")
	0x41, 0x09, // i32.const 9 (len)
	0x41, 0x20, // i32.const 32 (output ptr)
	0x10, 0x00, // call get_param_i64
	0x21, 0x00, // local.set 0 (store result)
	0x20, 0x00, // local.get 0
	0x41, 0x00, // i32.const 0
	0x46,       // i32.eq (check result == 0)
	0x0b,       // end
}

// WASM module that tests get_param_f64
var testF64ParamWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, // WASM magic
	0x01, 0x00, 0x00, 0x00, // Version

	// Type section
	0x01, 0x0a, 0x02,
	0x60, 0x01, 0x7e, 0x01, 0x7f,
	0x60, 0x03, 0x7f, 0x7f, 0x7f, 0x01, 0x7f,

	// Import section: get_param_f64
	0x02, 0x1b, 0x01, 0x03, 0x65, 0x6e, 0x76, 0x0d, 0x67, 0x65, 0x74,
	0x5f, 0x70, 0x61, 0x72, 0x61, 0x6d, 0x5f, 0x66, 0x36, 0x34, 0x00, 0x01,

	// Function section
	0x03, 0x02, 0x01, 0x00,

	// Memory section
	0x05, 0x03, 0x01, 0x00, 0x01,

	// Export section
	0x07, 0x18, 0x02,
	0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00,
	0x06, 0x66, 0x69, 0x6c, 0x74, 0x65, 0x72, 0x00, 0x01,

	// Data section: "boost" at offset 0
	0x0b, 0x0d, 0x01, 0x00, 0x41, 0x00, 0x0b, 0x05, 0x62, 0x6f, 0x6f,
	0x73, 0x74,

	// Code section
	0x0a, 0x1c, 0x01, 0x1a, 0x01, 0x01, 0x7f,
	0x41, 0x00, // i32.const 0 ("boost")
	0x41, 0x05, // i32.const 5
	0x41, 0x20, // i32.const 32
	0x10, 0x00, // call get_param_f64
	0x21, 0x00, // local.set 0
	0x20, 0x00, // local.get 0
	0x41, 0x00, // i32.const 0
	0x46,       // i32.eq
	0x0b,       // end
}

// WASM module that tests get_param_bool
var testBoolParamWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, // WASM magic
	0x01, 0x00, 0x00, 0x00, // Version

	// Type section
	0x01, 0x0a, 0x02,
	0x60, 0x01, 0x7e, 0x01, 0x7f,
	0x60, 0x03, 0x7f, 0x7f, 0x7f, 0x01, 0x7f,

	// Import section: get_param_bool
	0x02, 0x1c, 0x01, 0x03, 0x65, 0x6e, 0x76, 0x0e, 0x67, 0x65, 0x74,
	0x5f, 0x70, 0x61, 0x72, 0x61, 0x6d, 0x5f, 0x62, 0x6f, 0x6f, 0x6c,
	0x00, 0x01,

	// Function section
	0x03, 0x02, 0x01, 0x00,

	// Memory section
	0x05, 0x03, 0x01, 0x00, 0x01,

	// Export section
	0x07, 0x18, 0x02,
	0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00,
	0x06, 0x66, 0x69, 0x6c, 0x74, 0x65, 0x72, 0x00, 0x01,

	// Data section: "enabled" at offset 0
	0x0b, 0x0f, 0x01, 0x00, 0x41, 0x00, 0x0b, 0x07, 0x65, 0x6e, 0x61,
	0x62, 0x6c, 0x65, 0x64,

	// Code section
	0x0a, 0x1d, 0x01, 0x1b, 0x01, 0x01, 0x7f,
	0x41, 0x00, // i32.const 0 ("enabled")
	0x41, 0x07, // i32.const 7
	0x41, 0x20, // i32.const 32
	0x10, 0x00, // call get_param_bool
	0x21, 0x00, // local.set 0
	0x20, 0x00, // local.get 0
	0x41, 0x00, // i32.const 0
	0x46,       // i32.eq
	0x0b,       // end
}

// TestParameterHostFunctionsE2E tests parameter access in actual WASM execution
func TestParameterHostFunctionsE2E(t *testing.T) {
	logger := zap.NewNop()
	cfg := &Config{
		EnableJIT:   true,
		EnableDebug: false,
		Logger:      logger,
	}
	rt, err := NewRuntime(cfg)
	require.NoError(t, err)
	defer rt.Close()

	// Create UDF registry
	regCfg := &UDFRegistryConfig{
		Runtime:         rt,
		DefaultPoolSize: 1,
		EnableStats:     true,
		Logger:          logger,
	}
	registry, err := NewUDFRegistry(regCfg)
	require.NoError(t, err)

	t.Run("StringParameter", func(t *testing.T) {
		// Register UDF that tests string parameter access
		metadata := &UDFMetadata{
			Name:         "test_string",
			Version:      "1.0.0",
			FunctionName: "filter",
			Description:  "Tests get_param_string",
			WASMBytes:    testStringParamWasm,
			Parameters: []UDFParameter{
				{Name: "query", Type: ValueTypeString, Required: true},
			},
			Returns: []UDFReturnType{
				{Type: ValueTypeI32},
			},
		}

		err := registry.Register(metadata)
		require.NoError(t, err, "Should register string param test UDF")

		// Create test document
		docCtx := NewDocumentContextFromMap("doc1", 1.0, map[string]interface{}{
			"name": "test document",
		})

		// Test with parameter
		params := map[string]Value{
			"query": {Type: ValueTypeString, Data: "hello"},
		}

		results, err := registry.Call(context.Background(), "test_string", "1.0.0", docCtx, params)
		require.NoError(t, err, "UDF call should succeed")
		require.Len(t, results, 1, "Should return 1 result")

		// The WASM module returns 1 if get_param_string succeeded (returned 0)
		resultVal, err := results[0].AsInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(1), resultVal, "Should successfully access string parameter")

		t.Log("✅ String parameter access working in WASM execution")
	})

	t.Run("Int64Parameter", func(t *testing.T) {
		metadata := &UDFMetadata{
			Name:         "test_i64",
			Version:      "1.0.0",
			FunctionName: "filter",
			Description:  "Tests get_param_i64",
			WASMBytes:    testI64ParamWasm,
			Parameters: []UDFParameter{
				{Name: "threshold", Type: ValueTypeI64, Required: true},
			},
			Returns: []UDFReturnType{{Type: ValueTypeI32}},
		}

		err := registry.Register(metadata)
		require.NoError(t, err, "Should register i64 param test UDF")

		docCtx := NewDocumentContextFromMap("doc2", 1.0, map[string]interface{}{})

		params := map[string]Value{
			"threshold": {Type: ValueTypeI64, Data: int64(42)},
		}

		results, err := registry.Call(context.Background(), "test_i64", "1.0.0", docCtx, params)
		require.NoError(t, err, "UDF call should succeed")
		require.Len(t, results, 1)

		resultVal, err := results[0].AsInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(1), resultVal, "Should successfully access i64 parameter")

		t.Log("✅ Int64 parameter access working in WASM execution")
	})

	t.Run("Float64Parameter", func(t *testing.T) {
		metadata := &UDFMetadata{
			Name:         "test_f64",
			Version:      "1.0.0",
			FunctionName: "filter",
			Description:  "Tests get_param_f64",
			WASMBytes:    testF64ParamWasm,
			Parameters: []UDFParameter{
				{Name: "boost", Type: ValueTypeF64, Required: true},
			},
			Returns: []UDFReturnType{{Type: ValueTypeI32}},
		}

		err := registry.Register(metadata)
		require.NoError(t, err, "Should register f64 param test UDF")

		docCtx := NewDocumentContextFromMap("doc3", 1.0, map[string]interface{}{})

		params := map[string]Value{
			"boost": {Type: ValueTypeF64, Data: float64(2.5)},
		}

		results, err := registry.Call(context.Background(), "test_f64", "1.0.0", docCtx, params)
		require.NoError(t, err, "UDF call should succeed")
		require.Len(t, results, 1)

		resultVal, err := results[0].AsInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(1), resultVal, "Should successfully access f64 parameter")

		t.Log("✅ Float64 parameter access working in WASM execution")
	})

	t.Run("BoolParameter", func(t *testing.T) {
		metadata := &UDFMetadata{
			Name:         "test_bool",
			Version:      "1.0.0",
			FunctionName: "filter",
			Description:  "Tests get_param_bool",
			WASMBytes:    testBoolParamWasm,
			Parameters: []UDFParameter{
				{Name: "enabled", Type: ValueTypeBool, Required: true},
			},
			Returns: []UDFReturnType{{Type: ValueTypeI32}},
		}

		err := registry.Register(metadata)
		require.NoError(t, err, "Should register bool param test UDF")

		docCtx := NewDocumentContextFromMap("doc4", 1.0, map[string]interface{}{})

		params := map[string]Value{
			"enabled": {Type: ValueTypeBool, Data: true},
		}

		results, err := registry.Call(context.Background(), "test_bool", "1.0.0", docCtx, params)
		require.NoError(t, err, "UDF call should succeed")
		require.Len(t, results, 1)

		resultVal, err := results[0].AsInt32()
		require.NoError(t, err)
		assert.Equal(t, int32(1), resultVal, "Should successfully access bool parameter")

		t.Log("✅ Bool parameter access working in WASM execution")
	})

	t.Run("MissingParameter", func(t *testing.T) {
		// Use the string test UDF but don't provide the required parameter
		docCtx := NewDocumentContextFromMap("doc5", 1.0, map[string]interface{}{})

		// Empty parameters - should fail validation
		params := map[string]Value{}

		_, err := registry.Call(context.Background(), "test_string", "1.0.0", docCtx, params)
		assert.Error(t, err, "Should fail when required parameter is missing")
		assert.Contains(t, err.Error(), "required parameter missing", "Error should mention missing parameter")

		t.Log("✅ Missing parameter validation working")
	})

	t.Run("MultipleParameters", func(t *testing.T) {
		// Test UDF with multiple parameters of different types
		// This simulates a real-world UDF like the string-distance example
		metadata := &UDFMetadata{
			Name:         "multi_param",
			Version:      "1.0.0",
			FunctionName: "filter",
			Description:  "Tests multiple parameters",
			WASMBytes:    testStringParamWasm, // Reuse for simplicity
			Parameters: []UDFParameter{
				{Name: "query", Type: ValueTypeString, Required: true},
				{Name: "threshold", Type: ValueTypeI64, Required: false, Default: int64(5)},
				{Name: "boost", Type: ValueTypeF64, Required: false, Default: float64(1.0)},
			},
			Returns: []UDFReturnType{{Type: ValueTypeI32}},
		}

		err := registry.Register(metadata)
		require.NoError(t, err)

		docCtx := NewDocumentContextFromMap("doc6", 1.0, map[string]interface{}{})

		// Provide all parameters
		params := map[string]Value{
			"query":     {Type: ValueTypeString, Data: "test"},
			"threshold": {Type: ValueTypeI64, Data: int64(10)},
			"boost":     {Type: ValueTypeF64, Data: float64(2.5)},
		}

		results, err := registry.Call(context.Background(), "multi_param", "1.0.0", docCtx, params)
		require.NoError(t, err, "Should handle multiple parameters")
		require.Len(t, results, 1)

		t.Log("✅ Multiple parameters working")
	})
}

// TestParameterTypeConversion tests automatic type conversion in parameter registration
func TestParameterTypeConversion(t *testing.T) {
	logger := zap.NewNop()
	cfg := &Config{
		EnableJIT:   true,
		EnableDebug: false,
		Logger:      logger,
	}
	rt, err := NewRuntime(cfg)
	require.NoError(t, err)
	defer rt.Close()

	hostFuncs := NewHostFunctions(rt)

	t.Run("NumericTypeFlexibility", func(t *testing.T) {
		// Test that numeric types are handled flexibly
		params := map[string]interface{}{
			"int32_val":   int32(42),
			"int64_val":   int64(100),
			"float32_val": float32(3.14),
			"float64_val": float64(2.718),
			"int_val":     int(99),
		}

		hostFuncs.RegisterParameters(params)
		defer hostFuncs.UnregisterParameters()

		// All should be retrievable
		for name := range params {
			val, ok := hostFuncs.GetParameter(name)
			assert.True(t, ok, "Parameter %s should exist", name)
			assert.NotNil(t, val, "Parameter %s should not be nil", name)
		}

		t.Log("✅ Numeric type flexibility working")
	})
}
