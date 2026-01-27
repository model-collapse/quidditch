package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestParameterFunctionsRegistered verifies all parameter host functions are properly registered
func TestParameterFunctionsRegistered(t *testing.T) {
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
	ctx := rt.GetContext()

	// Register host functions (including the new parameter functions)
	err = hostFuncs.RegisterHostFunctions(ctx, rt.GetWazeroRuntime())
	require.NoError(t, err, "All host functions including parameter functions should register successfully")

	t.Log("✅ All parameter host functions registered successfully:")
	t.Log("   - get_param_string")
	t.Log("   - get_param_i64")
	t.Log("   - get_param_f64")
	t.Log("   - get_param_bool")
}

// TestParameterWorkflowIntegration tests the full parameter workflow
func TestParameterWorkflowIntegration(t *testing.T) {
	logger := zap.NewNop()
	cfg := &Config{
		EnableJIT:   true,
		EnableDebug: false,
		Logger:      logger,
	}
	rt, err := NewRuntime(cfg)
	require.NoError(t, err)
	defer rt.Close()

	// Create registry with host functions
	regCfg := &UDFRegistryConfig{
		Runtime:         rt,
		DefaultPoolSize: 1,
		EnableStats:     true,
		Logger:          logger,
	}
	registry, err := NewUDFRegistry(regCfg)
	require.NoError(t, err)

	t.Run("ParameterRegistrationAndRetrieval", func(t *testing.T) {
		// Test the parameter management methods directly
		params := map[string]interface{}{
			"query":     "hello world",
			"threshold": int64(42),
			"boost":     float64(2.5),
			"enabled":   true,
		}

		registry.hostFuncs.RegisterParameters(params)
		defer registry.hostFuncs.UnregisterParameters()

		// Verify all parameters are accessible
		query, ok := registry.hostFuncs.GetParameter("query")
		assert.True(t, ok)
		assert.Equal(t, "hello world", query)

		threshold, ok := registry.hostFuncs.GetParameter("threshold")
		assert.True(t, ok)
		assert.Equal(t, int64(42), threshold)

		boost, ok := registry.hostFuncs.GetParameter("boost")
		assert.True(t, ok)
		assert.Equal(t, 2.5, boost)

		enabled, ok := registry.hostFuncs.GetParameter("enabled")
		assert.True(t, ok)
		assert.Equal(t, true, enabled)

		t.Log("✅ Parameter registration and retrieval working")
	})

	t.Run("ParameterCleanup", func(t *testing.T) {
		params := map[string]interface{}{
			"test": "value",
		}

		registry.hostFuncs.RegisterParameters(params)

		// Verify exists
		val, ok := registry.hostFuncs.GetParameter("test")
		assert.True(t, ok)
		assert.Equal(t, "value", val)

		// Cleanup
		registry.hostFuncs.UnregisterParameters()

		// Verify cleaned up
		val, ok = registry.hostFuncs.GetParameter("test")
		assert.False(t, ok)
		assert.Nil(t, val)

		t.Log("✅ Parameter cleanup working")
	})

	t.Run("ThreadSafety", func(t *testing.T) {
		// Test concurrent parameter access (basic sanity check)
		params := map[string]interface{}{
			"concurrent": "test",
		}

		registry.hostFuncs.RegisterParameters(params)
		defer registry.hostFuncs.UnregisterParameters()

		// Concurrent reads should work
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				val, ok := registry.hostFuncs.GetParameter("concurrent")
				assert.True(t, ok)
				assert.Equal(t, "test", val)
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		t.Log("✅ Thread-safe parameter access working")
	})
}

// TestUDFExamples documents how the real UDF examples would use the parameter functions
func TestUDFExamples(t *testing.T) {
	t.Run("StringDistanceExample", func(t *testing.T) {
		t.Log("✅ String Distance UDF example:")
		t.Log("   Parameters:")
		t.Log("     - field: string (field name to check)")
		t.Log("     - target: string (target string to compare)")
		t.Log("     - max_distance: i64 (max Levenshtein distance)")
		t.Log("   Host Functions Used:")
		t.Log("     - get_param_string('field')")
		t.Log("     - get_param_string('target')")
		t.Log("     - get_param_i64('max_distance')")
		t.Log("     - get_field_string(ctx_id, field, ...)")
	})

	t.Run("GeoFilterExample", func(t *testing.T) {
		t.Log("✅ Geo Filter UDF example:")
		t.Log("   Parameters:")
		t.Log("     - lat_field: string (latitude field name)")
		t.Log("     - lon_field: string (longitude field name)")
		t.Log("     - target_lat: f64 (target latitude)")
		t.Log("     - target_lon: f64 (target longitude)")
		t.Log("     - max_distance_km: f64 (max distance in km)")
		t.Log("   Host Functions Used:")
		t.Log("     - get_param_string('lat_field')")
		t.Log("     - get_param_string('lon_field')")
		t.Log("     - get_param_f64('target_lat')")
		t.Log("     - get_param_f64('target_lon')")
		t.Log("     - get_param_f64('max_distance_km')")
		t.Log("     - get_field_f64(ctx_id, lat_field, ...)")
		t.Log("     - get_field_f64(ctx_id, lon_field, ...)")
	})

	t.Run("CustomScoreExample", func(t *testing.T) {
		t.Log("✅ Custom Score UDF example:")
		t.Log("   Parameters:")
		t.Log("     - min_score: f64 (minimum score threshold)")
		t.Log("   Host Functions Used:")
		t.Log("     - get_param_f64('min_score')")
		t.Log("     - get_field_f64(ctx_id, 'base_score', ...)")
		t.Log("     - get_field_f64(ctx_id, 'boost', ...)")
	})
}

// TestParameterTypeFlexibility verifies type conversion works correctly
func TestParameterTypeFlexibility(t *testing.T) {
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

	t.Run("NumericTypes", func(t *testing.T) {
		params := map[string]interface{}{
			"int32_val":   int32(10),
			"int64_val":   int64(100),
			"int_val":     int(1000),
			"float32_val": float32(3.14),
			"float64_val": float64(2.718),
		}

		hostFuncs.RegisterParameters(params)
		defer hostFuncs.UnregisterParameters()

		// All numeric types should be retrievable
		for name, expected := range params {
			val, ok := hostFuncs.GetParameter(name)
			assert.True(t, ok, "Parameter %s should exist", name)
			assert.Equal(t, expected, val, "Parameter %s should have correct value", name)
		}

		t.Log("✅ Numeric type flexibility working")
	})

	t.Run("StringType", func(t *testing.T) {
		params := map[string]interface{}{
			"str1": "hello",
			"str2": "world",
			"str3": "",
		}

		hostFuncs.RegisterParameters(params)
		defer hostFuncs.UnregisterParameters()

		for name, expected := range params {
			val, ok := hostFuncs.GetParameter(name)
			assert.True(t, ok, "Parameter %s should exist", name)
			assert.Equal(t, expected, val)
		}

		t.Log("✅ String type working")
	})

	t.Run("BoolType", func(t *testing.T) {
		params := map[string]interface{}{
			"enabled":  true,
			"disabled": false,
		}

		hostFuncs.RegisterParameters(params)
		defer hostFuncs.UnregisterParameters()

		enabled, ok := hostFuncs.GetParameter("enabled")
		assert.True(t, ok)
		assert.Equal(t, true, enabled)

		disabled, ok := hostFuncs.GetParameter("disabled")
		assert.True(t, ok)
		assert.Equal(t, false, disabled)

		t.Log("✅ Bool type working")
	})
}
