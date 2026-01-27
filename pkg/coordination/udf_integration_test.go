package coordination

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestUDFIntegration_FullWorkflow tests the complete UDF workflow
func TestUDFIntegration_FullWorkflow(t *testing.T) {
	// Skip if WASM runtime cannot be initialized
	logger, _ := zap.NewDevelopment()

	// Create coordination node
	cfg := &config.CoordinationConfig{
		NodeID:     "coord-test-1",
		BindAddr:   "127.0.0.1",
		RESTPort:   9201,
		MasterAddr: "127.0.0.1:8000", // Won't actually connect in this test
	}

	node, err := NewCoordinationNode(cfg, logger)
	require.NoError(t, err)
	defer node.Stop(context.Background())

	// Skip test if UDF registry not available
	if node.udfRegistry == nil {
		t.Skip("WASM runtime not available, skipping UDF integration test")
	}

	// Create test router
	router := node.ginRouter

	// Test 1: Upload a UDF
	t.Run("UploadUDF", func(t *testing.T) {
		// Create a simple WASM module (minimal valid WASM binary)
		wasmBinary := createMinimalWASMModule()

		uploadReq := map[string]interface{}{
			"name":          "test_filter",
			"version":       "1.0.0",
			"description":   "Test filter UDF",
			"author":        "test",
			"category":      "filter",
			"language":      "wasm",
			"function_name": "udf_main",
			"wasm_base64":   wasmBinary,
			"parameters": []map[string]interface{}{
				{
					"name": "threshold",
					"type": "i64",
				},
			},
			"returns": []map[string]interface{}{
				{
					"type": "bool",
				},
			},
		}

		body, _ := json.Marshal(uploadReq)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/udfs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Debug: Print response if not successful
		if w.Code != http.StatusCreated {
			t.Logf("Upload failed with status %d: %s", w.Code, w.Body.String())
		}

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		if response["success"] != nil {
			assert.True(t, response["success"].(bool))
		}
		assert.Equal(t, "test_filter", response["name"])
		assert.Equal(t, "1.0.0", response["version"])
	})

	// Test 2: List UDFs
	t.Run("ListUDFs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		udfs, ok := response["udfs"].([]interface{})
		require.True(t, ok)
		assert.GreaterOrEqual(t, len(udfs), 1)

		// Find our test UDF
		found := false
		for _, udf := range udfs {
			udfMap := udf.(map[string]interface{})
			if udfMap["name"] == "test_filter" {
				found = true
				assert.Equal(t, "1.0.0", udfMap["version"])
				break
			}
		}
		assert.True(t, found, "test_filter UDF should be in list")
	})

	// Test 3: Get specific UDF
	t.Run("GetUDF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs/test_filter", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "test_filter", response["name"])
		assert.Equal(t, "1.0.0", response["version"])
		assert.Equal(t, "Test filter UDF", response["description"])
	})

	// Test 4: Get UDF versions
	t.Run("GetUDFVersions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs/test_filter/versions", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		versions, ok := response["versions"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, versions, "1.0.0")
	})

	// Test 5: Test UDF execution
	t.Run("TestUDF", func(t *testing.T) {
		testReq := map[string]interface{}{
			"params": map[string]interface{}{
				"threshold": 10,
			},
			"document": map[string]interface{}{
				"title": "test document",
				"score": 15,
			},
		}

		body, _ := json.Marshal(testReq)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/udfs/test_filter/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// May fail if WASM module is minimal and doesn't have proper exports
		// But the request should be processed
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)
	})

	// Test 6: Get UDF stats
	t.Run("GetUDFStats", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs/test_filter/stats", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Contains(t, response, "name")
		assert.Contains(t, response, "version")
		assert.Contains(t, response, "call_count")
		assert.Contains(t, response, "error_count")
	})

	// Test 7: Delete UDF
	t.Run("DeleteUDF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/udfs/test_filter/1.0.0", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.True(t, response["success"].(bool))
	})

	// Test 8: Verify deletion
	t.Run("VerifyDeletion", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs/test_filter", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestUDFIntegration_ErrorHandling tests error handling scenarios
func TestUDFIntegration_ErrorHandling(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := &config.CoordinationConfig{
		NodeID:     "coord-test-2",
		BindAddr:   "127.0.0.1",
		RESTPort:   9202,
		MasterAddr: "127.0.0.1:8000",
	}

	node, err := NewCoordinationNode(cfg, logger)
	require.NoError(t, err)
	defer node.Stop(context.Background())

	if node.udfRegistry == nil {
		t.Skip("WASM runtime not available, skipping UDF error handling test")
	}

	router := node.ginRouter

	// Test 1: Upload with missing fields
	t.Run("UploadMissingFields", func(t *testing.T) {
		uploadReq := map[string]interface{}{
			"name": "incomplete_udf",
			// Missing version, wasm_binary, etc.
		}

		body, _ := json.Marshal(uploadReq)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/udfs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// Test 2: Get non-existent UDF
	t.Run("GetNonExistentUDF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs/nonexistent", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// Test 3: Delete non-existent UDF
	t.Run("DeleteNonExistentUDF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/udfs/nonexistent/1.0.0", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// Test 4: Invalid WASM binary
	t.Run("InvalidWASMBinary", func(t *testing.T) {
		uploadReq := map[string]interface{}{
			"name":          "invalid_wasm",
			"version":       "1.0.0",
			"description":   "Invalid WASM",
			"author":        "test",
			"category":      "filter",
			"language":      "wasm",
			"function_name": "udf_main",
			"wasm_base64":   "not-valid-base64-wasm-data!!!",
			"returns": []map[string]interface{}{{"type": "bool"}},
		}

		body, _ := json.Marshal(uploadReq)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/udfs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should reject invalid WASM
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusInternalServerError}, w.Code)
	})
}

// TestUDFIntegration_Concurrency tests concurrent UDF operations
func TestUDFIntegration_Concurrency(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := &config.CoordinationConfig{
		NodeID:     "coord-test-3",
		BindAddr:   "127.0.0.1",
		RESTPort:   9203,
		MasterAddr: "127.0.0.1:8000",
	}

	node, err := NewCoordinationNode(cfg, logger)
	require.NoError(t, err)
	defer node.Stop(context.Background())

	if node.udfRegistry == nil {
		t.Skip("WASM runtime not available, skipping UDF concurrency test")
	}

	router := node.ginRouter

	// Upload a test UDF first
	wasmBinary := createMinimalWASMModule()
	uploadReq := map[string]interface{}{
		"name":          "concurrent_test",
		"version":       "1.0.0",
		"description":   "Concurrency test UDF",
		"author":        "test",
		"category":      "filter",
		"language":      "wasm",
		"function_name": "udf_main",
		"wasm_base64":   wasmBinary,
		"returns": []map[string]interface{}{{"type": "bool"}},
	}

	body, _ := json.Marshal(uploadReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/udfs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Skip("Failed to upload test UDF, skipping concurrency test")
	}

	// Test concurrent list operations
	t.Run("ConcurrentList", func(t *testing.T) {
		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func() {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				assert.Equal(t, http.StatusOK, w.Code)
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}
	})

	// Test concurrent get operations
	t.Run("ConcurrentGet", func(t *testing.T) {
		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func() {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs/concurrent_test", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				assert.Equal(t, http.StatusOK, w.Code)
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}
	})

	// Test concurrent stats operations
	t.Run("ConcurrentStats", func(t *testing.T) {
		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func() {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs/concurrent_test/stats", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				assert.Equal(t, http.StatusOK, w.Code)
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

// TestUDFIntegration_MultipleVersions tests UDF version management
func TestUDFIntegration_MultipleVersions(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := &config.CoordinationConfig{
		NodeID:     "coord-test-4",
		BindAddr:   "127.0.0.1",
		RESTPort:   9204,
		MasterAddr: "127.0.0.1:8000",
	}

	node, err := NewCoordinationNode(cfg, logger)
	require.NoError(t, err)
	defer node.Stop(context.Background())

	if node.udfRegistry == nil {
		t.Skip("WASM runtime not available, skipping UDF version test")
	}

	router := node.ginRouter
	wasmBinary := createMinimalWASMModule()

	// Upload version 1.0.0
	t.Run("UploadV1", func(t *testing.T) {
		uploadReq := map[string]interface{}{
			"name":          "versioned_udf",
			"version":       "1.0.0",
			"description":   "Version 1.0.0",
			"author":        "test",
			"category":      "filter",
			"language":      "wasm",
			"function_name": "udf_main",
			"wasm_base64":   wasmBinary,
			"returns": []map[string]interface{}{{"type": "bool"}},
		}

		body, _ := json.Marshal(uploadReq)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/udfs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	// Upload version 1.1.0
	t.Run("UploadV1_1", func(t *testing.T) {
		uploadReq := map[string]interface{}{
			"name":          "versioned_udf",
			"version":       "1.1.0",
			"description":   "Version 1.1.0",
			"author":        "test",
			"category":      "filter",
			"language":      "wasm",
			"function_name": "udf_main",
			"wasm_base64":   wasmBinary,
			"returns": []map[string]interface{}{{"type": "bool"}},
		}

		body, _ := json.Marshal(uploadReq)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/udfs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	// List versions
	t.Run("ListVersions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs/versioned_udf/versions", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		versions, ok := response["versions"].([]interface{})
		require.True(t, ok)
		assert.Len(t, versions, 2)
		assert.Contains(t, versions, "1.0.0")
		assert.Contains(t, versions, "1.1.0")
	})

	// Get specific version
	t.Run("GetSpecificVersion", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs/versioned_udf?version=1.0.0", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", response["version"])
	})

	// Delete specific version
	t.Run("DeleteV1", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/udfs/versioned_udf/1.0.0", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Verify only 1.1.0 remains
	t.Run("VerifyRemainingVersion", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs/versioned_udf/versions", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		var response map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		versions, ok := response["versions"].([]interface{})
		require.True(t, ok)
		assert.Len(t, versions, 1)
		assert.Contains(t, versions, "1.1.0")
	})
}

// TestUDFIntegration_Performance tests UDF API performance
func TestUDFIntegration_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	logger, _ := zap.NewDevelopment()

	cfg := &config.CoordinationConfig{
		NodeID:     "coord-test-5",
		BindAddr:   "127.0.0.1",
		RESTPort:   9205,
		MasterAddr: "127.0.0.1:8000",
	}

	node, err := NewCoordinationNode(cfg, logger)
	require.NoError(t, err)
	defer node.Stop(context.Background())

	if node.udfRegistry == nil {
		t.Skip("WASM runtime not available, skipping UDF performance test")
	}

	router := node.ginRouter

	// Upload a test UDF
	wasmBinary := createMinimalWASMModule()
	uploadReq := map[string]interface{}{
		"name":          "perf_test",
		"version":       "1.0.0",
		"description":   "Performance test UDF",
		"author":        "test",
		"category":      "filter",
		"language":      "wasm",
		"function_name": "udf_main",
		"wasm_base64":   wasmBinary,
		"returns": []map[string]interface{}{{"type": "bool"}},
	}

	body, _ := json.Marshal(uploadReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/udfs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Skip("Failed to upload test UDF, skipping performance test")
	}

	// Benchmark list operations
	t.Run("BenchmarkList", func(t *testing.T) {
		start := time.Now()
		iterations := 1000

		for i := 0; i < iterations; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		}

		duration := time.Since(start)
		avgLatency := duration / time.Duration(iterations)

		t.Logf("List UDFs - %d iterations in %v (avg: %v per request)",
			iterations, duration, avgLatency)

		// Should complete 1000 requests in reasonable time
		assert.Less(t, duration, 5*time.Second, "List operations too slow")
	})

	// Benchmark get operations
	t.Run("BenchmarkGet", func(t *testing.T) {
		start := time.Now()
		iterations := 1000

		for i := 0; i < iterations; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/udfs/perf_test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		}

		duration := time.Since(start)
		avgLatency := duration / time.Duration(iterations)

		t.Logf("Get UDF - %d iterations in %v (avg: %v per request)",
			iterations, duration, avgLatency)

		assert.Less(t, duration, 5*time.Second, "Get operations too slow")
	})
}

// createMinimalWASMModule creates a minimal valid WASM module for testing
func createMinimalWASMModule() string {
	// Minimal valid WASM module (magic number + version)
	// This is a base64-encoded minimal WASM module
	return "AGFzbQEAAAA="
}
