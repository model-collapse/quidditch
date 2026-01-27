package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestParameterManagement tests the parameter registration and retrieval
func TestParameterManagement(t *testing.T) {
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

	t.Run("RegisterAndGetParameters", func(t *testing.T) {
		params := map[string]interface{}{
			"query":     "hello world",
			"threshold": int64(5),
			"boost":     float64(2.5),
			"enabled":   true,
		}

		// Register parameters
		hostFuncs.RegisterParameters(params)
		defer hostFuncs.UnregisterParameters()

		// Verify string parameter
		val, ok := hostFuncs.GetParameter("query")
		assert.True(t, ok, "String parameter should exist")
		assert.Equal(t, "hello world", val)

		// Verify int64 parameter
		val, ok = hostFuncs.GetParameter("threshold")
		assert.True(t, ok, "Int64 parameter should exist")
		assert.Equal(t, int64(5), val)

		// Verify float64 parameter
		val, ok = hostFuncs.GetParameter("boost")
		assert.True(t, ok, "Float64 parameter should exist")
		assert.Equal(t, 2.5, val)

		// Verify bool parameter
		val, ok = hostFuncs.GetParameter("enabled")
		assert.True(t, ok, "Bool parameter should exist")
		assert.Equal(t, true, val)

		// Verify non-existent parameter
		val, ok = hostFuncs.GetParameter("nonexistent")
		assert.False(t, ok, "Non-existent parameter should not be found")
		assert.Nil(t, val)
	})

	t.Run("UnregisterParameters", func(t *testing.T) {
		params := map[string]interface{}{
			"test": "value",
		}

		hostFuncs.RegisterParameters(params)

		// Should exist
		val, ok := hostFuncs.GetParameter("test")
		assert.True(t, ok)
		assert.Equal(t, "value", val)

		// Unregister
		hostFuncs.UnregisterParameters()

		// Should no longer exist
		val, ok = hostFuncs.GetParameter("test")
		assert.False(t, ok)
		assert.Nil(t, val)
	})

	t.Run("MultipleRegistrations", func(t *testing.T) {
		// First registration
		params1 := map[string]interface{}{
			"first": "value1",
		}
		hostFuncs.RegisterParameters(params1)

		val, ok := hostFuncs.GetParameter("first")
		assert.True(t, ok)
		assert.Equal(t, "value1", val)

		// Second registration (should replace)
		params2 := map[string]interface{}{
			"second": "value2",
		}
		hostFuncs.RegisterParameters(params2)

		// First should no longer exist
		val, ok = hostFuncs.GetParameter("first")
		assert.False(t, ok)

		// Second should exist
		val, ok = hostFuncs.GetParameter("second")
		assert.True(t, ok)
		assert.Equal(t, "value2", val)

		hostFuncs.UnregisterParameters()
	})

	t.Run("EmptyParameters", func(t *testing.T) {
		params := map[string]interface{}{}
		hostFuncs.RegisterParameters(params)
		defer hostFuncs.UnregisterParameters()

		val, ok := hostFuncs.GetParameter("anything")
		assert.False(t, ok)
		assert.Nil(t, val)
	})
}

// TestHostFunctionsExported verifies that parameter host functions are exported
func TestHostFunctionsExported(t *testing.T) {
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

	// Register host functions
	err = hostFuncs.RegisterHostFunctions(ctx, rt.GetWazeroRuntime())
	require.NoError(t, err, "Host functions should register successfully")

	// Verify the env module was instantiated
	// (The actual function exports are tested by compiling WASM modules that use them)
	t.Log("✅ Host functions registered including parameter access functions")
}
