package coordination

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/quidditch/quidditch/pkg/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestUDFHandlers_UploadUDF(t *testing.T) {
	// Create test runtime and registry
	logger := zap.NewNop()
	rtCfg := &wasm.Config{
		EnableJIT:   false,
		EnableDebug: false,
		Logger:      logger,
	}
	rt, err := wasm.NewRuntime(rtCfg)
	require.NoError(t, err)
	defer rt.Close()

	regCfg := &wasm.UDFRegistryConfig{
		Runtime:         rt,
		DefaultPoolSize: 1,
		EnableStats:     true,
		Logger:          logger,
	}
	registry, err := wasm.NewUDFRegistry(regCfg)
	require.NoError(t, err)

	// Create handlers
	handlers := NewUDFHandlers(registry, logger)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	handlers.RegisterRoutes(api)

	t.Run("ValidUpload", func(t *testing.T) {
		// Simple WASM module that exports a filter function
		wasmBytes := []byte{
			0x00, 0x61, 0x73, 0x6d, // WASM magic
			0x01, 0x00, 0x00, 0x00, // Version
		}

		req := UDFUploadRequest{
			Name:         "test_udf",
			Version:      "1.0.0",
			Description:  "Test UDF",
			Category:     "filter",
			Author:       "test",
			Language:     "wasm",
			FunctionName: "filter",
			WASMBase64:   string(wasmBytes),
			Parameters: []wasm.UDFParameter{
				{Name: "threshold", Type: wasm.ValueTypeI64, Required: true},
			},
			Returns: []wasm.UDFReturnType{
				{Type: wasm.ValueTypeI32},
			},
			Tags: []string{"test"},
		}

		body, _ := json.Marshal(req)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v1/udfs", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "test_udf", response["name"])
		assert.Equal(t, "1.0.0", response["version"])
		assert.Contains(t, response, "registered_at")
	})

	t.Run("InvalidRequest", func(t *testing.T) {
		// Missing required fields
		req := map[string]interface{}{
			"name": "test_udf",
			// Missing version, function_name, etc.
		}

		body, _ := json.Marshal(req)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v1/udfs", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestUDFHandlers_ListUDFs(t *testing.T) {
	// Create test setup
	logger := zap.NewNop()
	rtCfg := &wasm.Config{
		EnableJIT:   false,
		EnableDebug: false,
		Logger:      logger,
	}
	rt, err := wasm.NewRuntime(rtCfg)
	require.NoError(t, err)
	defer rt.Close()

	regCfg := &wasm.UDFRegistryConfig{
		Runtime:         rt,
		DefaultPoolSize: 1,
		EnableStats:     true,
		Logger:          logger,
	}
	registry, err := wasm.NewUDFRegistry(regCfg)
	require.NoError(t, err)

	// Register test UDFs
	wasmBytes := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

	udf1 := &wasm.UDFMetadata{
		Name:         "udf1",
		Version:      "1.0.0",
		Category:     "filter",
		FunctionName: "filter",
		WASMBytes:    wasmBytes,
		Parameters:   []wasm.UDFParameter{},
		Returns:      []wasm.UDFReturnType{{Type: wasm.ValueTypeI32}},
	}
	err = registry.Register(udf1)
	require.NoError(t, err)

	udf2 := &wasm.UDFMetadata{
		Name:         "udf2",
		Version:      "1.0.0",
		Category:     "scorer",
		FunctionName: "score",
		WASMBytes:    wasmBytes,
		Parameters:   []wasm.UDFParameter{},
		Returns:      []wasm.UDFReturnType{{Type: wasm.ValueTypeF64}},
	}
	err = registry.Register(udf2)
	require.NoError(t, err)

	// Create handlers
	handlers := NewUDFHandlers(registry, logger)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	handlers.RegisterRoutes(api)

	t.Run("ListAll", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/udfs", nil)

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, float64(2), response["total"])
		udfs := response["udfs"].([]interface{})
		assert.Len(t, udfs, 2)
	})

	t.Run("FilterByCategory", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/udfs?category=filter", nil)

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, float64(1), response["total"])
	})
}

func TestUDFHandlers_GetUDF(t *testing.T) {
	// Create test setup
	logger := zap.NewNop()
	rtCfg := &wasm.Config{
		EnableJIT:   false,
		EnableDebug: false,
		Logger:      logger,
	}
	rt, err := wasm.NewRuntime(rtCfg)
	require.NoError(t, err)
	defer rt.Close()

	regCfg := &wasm.UDFRegistryConfig{
		Runtime:         rt,
		DefaultPoolSize: 1,
		EnableStats:     true,
		Logger:          logger,
	}
	registry, err := wasm.NewUDFRegistry(regCfg)
	require.NoError(t, err)

	// Register test UDF
	wasmBytes := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	metadata := &wasm.UDFMetadata{
		Name:         "test_udf",
		Version:      "1.0.0",
		Description:  "Test UDF",
		FunctionName: "filter",
		WASMBytes:    wasmBytes,
		Parameters:   []wasm.UDFParameter{},
		Returns:      []wasm.UDFReturnType{{Type: wasm.ValueTypeI32}},
	}
	err = registry.Register(metadata)
	require.NoError(t, err)

	// Create handlers
	handlers := NewUDFHandlers(registry, logger)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	handlers.RegisterRoutes(api)

	t.Run("GetExisting", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/udfs/test_udf?version=1.0.0", nil)

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "test_udf", response["name"])
		assert.Equal(t, "1.0.0", response["version"])
		assert.Equal(t, "Test UDF", response["description"])
	})

	t.Run("GetLatest", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/udfs/test_udf", nil)

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "test_udf", response["name"])
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/udfs/nonexistent", nil)

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestUDFHandlers_DeleteUDF(t *testing.T) {
	// Create test setup
	logger := zap.NewNop()
	rtCfg := &wasm.Config{
		EnableJIT:   false,
		EnableDebug: false,
		Logger:      logger,
	}
	rt, err := wasm.NewRuntime(rtCfg)
	require.NoError(t, err)
	defer rt.Close()

	regCfg := &wasm.UDFRegistryConfig{
		Runtime:         rt,
		DefaultPoolSize: 1,
		EnableStats:     true,
		Logger:          logger,
	}
	registry, err := wasm.NewUDFRegistry(regCfg)
	require.NoError(t, err)

	// Register test UDF
	wasmBytes := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	metadata := &wasm.UDFMetadata{
		Name:         "test_udf",
		Version:      "1.0.0",
		FunctionName: "filter",
		WASMBytes:    wasmBytes,
		Parameters:   []wasm.UDFParameter{},
		Returns:      []wasm.UDFReturnType{{Type: wasm.ValueTypeI32}},
	}
	err = registry.Register(metadata)
	require.NoError(t, err)

	// Create handlers
	handlers := NewUDFHandlers(registry, logger)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	handlers.RegisterRoutes(api)

	t.Run("DeleteExisting", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/v1/udfs/test_udf/1.0.0", nil)

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Contains(t, response["message"], "deleted successfully")
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/v1/udfs/nonexistent/1.0.0", nil)

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestUDFHandlers_GetStats(t *testing.T) {
	// Create test setup
	logger := zap.NewNop()
	rtCfg := &wasm.Config{
		EnableJIT:   false,
		EnableDebug: false,
		Logger:      logger,
	}
	rt, err := wasm.NewRuntime(rtCfg)
	require.NoError(t, err)
	defer rt.Close()

	regCfg := &wasm.UDFRegistryConfig{
		Runtime:         rt,
		DefaultPoolSize: 1,
		EnableStats:     true,
		Logger:          logger,
	}
	registry, err := wasm.NewUDFRegistry(regCfg)
	require.NoError(t, err)

	// Register test UDF
	wasmBytes := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	metadata := &wasm.UDFMetadata{
		Name:         "test_udf",
		Version:      "1.0.0",
		FunctionName: "filter",
		WASMBytes:    wasmBytes,
		Parameters:   []wasm.UDFParameter{},
		Returns:      []wasm.UDFReturnType{{Type: wasm.ValueTypeI32}},
	}
	err = registry.Register(metadata)
	require.NoError(t, err)

	// Create handlers
	handlers := NewUDFHandlers(registry, logger)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	handlers.RegisterRoutes(api)

	t.Run("GetStatsForExisting", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/udfs/test_udf/stats?version=1.0.0", nil)

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "test_udf", response["name"])
		assert.Equal(t, "1.0.0", response["version"])
		assert.Contains(t, response, "call_count")
		assert.Contains(t, response, "error_count")
		assert.Contains(t, response, "avg_duration_ms")
	})

	t.Run("GetStatsForNonExistent", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/udfs/nonexistent/stats", nil)

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
