package coordination

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/quidditch/quidditch/pkg/common/config"
	"go.uber.org/zap"
)

func init() {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)
}

func TestNewCoordinationNode(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := &config.CoordinationConfig{
		NodeID:     "coord-1",
		BindAddr:   "127.0.0.1",
		RESTPort:   9200,
		MasterAddr: "127.0.0.1:9300",
	}

	node, err := NewCoordinationNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create coordination node: %v", err)
	}

	if node == nil {
		t.Fatal("Coordination node is nil")
	}

	if node.cfg != cfg {
		t.Error("Config mismatch")
	}

	if node.logger != logger {
		t.Error("Logger mismatch")
	}

	if node.router == nil {
		t.Error("Router is nil")
	}

	if node.queryParser == nil {
		t.Error("Query parser is nil")
	}
}

func TestNewCoordinationNodeNilLogger(t *testing.T) {
	cfg := &config.CoordinationConfig{
		NodeID:   "coord-1",
		BindAddr: "127.0.0.1",
		RESTPort: 9200,
	}

	_, err := NewCoordinationNode(cfg, nil)
	if err == nil {
		t.Error("Expected error when logger is nil")
	}
}

func TestHandleRoot(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["name"] != "Quidditch" {
		t.Errorf("Expected name 'Quidditch', got %v", response["name"])
	}

	if response["tagline"] != "You Know, for Search (powered by Diagon)" {
		t.Error("Incorrect tagline")
	}
}

func TestHandleClusterHealth(t *testing.T) {
	node := createTestNode(t)

	tests := []struct {
		name string
		path string
	}{
		{"All indices", "/_cluster/health"},
		{"Specific index", "/_cluster/health/test-index"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()
			node.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response["cluster_name"] != "quidditch-cluster" {
				t.Error("Incorrect cluster name")
			}

			if response["status"] != "green" {
				t.Errorf("Expected status 'green', got %v", response["status"])
			}
		})
	}
}

func TestHandleClusterState(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("GET", "/_cluster/state", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["cluster_name"] != "quidditch-cluster" {
		t.Error("Incorrect cluster name")
	}
}

func TestHandleClusterStats(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("GET", "/_cluster/stats", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleClusterSettings(t *testing.T) {
	node := createTestNode(t)

	body := []byte(`{"persistent": {"cluster.routing.allocation.enable": "all"}}`)
	req, _ := http.NewRequest("PUT", "/_cluster/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["acknowledged"] != true {
		t.Error("Expected acknowledged to be true")
	}
}

func TestHandleCreateIndex(t *testing.T) {
	node := createTestNode(t)

	body := []byte(`{
		"settings": {
			"number_of_shards": 5,
			"number_of_replicas": 1
		}
	}`)

	req, _ := http.NewRequest("PUT", "/test-index", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["acknowledged"] != true {
		t.Error("Expected acknowledged to be true")
	}

	if response["index"] != "test-index" {
		t.Errorf("Expected index name 'test-index', got %v", response["index"])
	}
}

func TestHandleDeleteIndex(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("DELETE", "/test-index", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["acknowledged"] != true {
		t.Error("Expected acknowledged to be true")
	}
}

func TestHandleGetIndex(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("GET", "/test-index", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if _, exists := response["test-index"]; !exists {
		t.Error("Expected index metadata in response")
	}
}

func TestHandleIndexExists(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("HEAD", "/test-index", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// HEAD requests should return no body
	if w.Body.Len() > 0 {
		t.Error("HEAD request should return empty body")
	}
}

func TestHandleOpenIndex(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("POST", "/test-index/_open", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleCloseIndex(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("POST", "/test-index/_close", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleRefreshIndex(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("POST", "/test-index/_refresh", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleFlushIndex(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("POST", "/test-index/_flush", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleGetMapping(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("GET", "/test-index/_mapping", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if _, exists := response["test-index"]; !exists {
		t.Error("Expected index in response")
	}
}

func TestHandlePutMapping(t *testing.T) {
	node := createTestNode(t)

	body := []byte(`{
		"properties": {
			"title": {"type": "text"},
			"age": {"type": "integer"}
		}
	}`)

	req, _ := http.NewRequest("PUT", "/test-index/_mapping", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleGetSettings(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("GET", "/test-index/_settings", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandlePutSettings(t *testing.T) {
	node := createTestNode(t)

	body := []byte(`{"index": {"number_of_replicas": 2}}`)
	req, _ := http.NewRequest("PUT", "/test-index/_settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleIndexDocument(t *testing.T) {
	node := createTestNode(t)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"PUT with ID", "PUT", "/test-index/_doc/1"},
		{"POST without ID", "POST", "/test-index/_doc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"title": "Test Document", "content": "Hello World"}`)
			req, _ := http.NewRequest(tt.method, tt.path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			node.router.ServeHTTP(w, req)

			if w.Code != http.StatusCreated {
				t.Errorf("Expected status 201, got %d", w.Code)
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response["_index"] != "test-index" {
				t.Error("Incorrect index in response")
			}

			if response["result"] != "created" {
				t.Error("Expected result to be 'created'")
			}
		})
	}
}

func TestHandleGetDocument(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("GET", "/test-index/_doc/1", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["_index"] != "test-index" {
		t.Error("Incorrect index in response")
	}

	if response["_id"] != "1" {
		t.Error("Incorrect ID in response")
	}
}

func TestHandleDeleteDocument(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("DELETE", "/test-index/_doc/1", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["result"] != "deleted" {
		t.Error("Expected result to be 'deleted'")
	}
}

func TestHandleUpdateDocument(t *testing.T) {
	node := createTestNode(t)

	body := []byte(`{"doc": {"title": "Updated Title"}}`)
	req, _ := http.NewRequest("POST", "/test-index/_update/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["result"] != "updated" {
		t.Error("Expected result to be 'updated'")
	}
}

func TestHandleBulk(t *testing.T) {
	node := createTestNode(t)

	tests := []struct {
		name string
		path string
	}{
		{"Bulk without index", "/_bulk"},
		{"Bulk with index", "/test-index/_bulk"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"index":{"_index":"test","_id":"1"}}
{"field":"value1"}
`)
			req, _ := http.NewRequest("POST", tt.path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/x-ndjson")
			w := httptest.NewRecorder()
			node.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}
		})
	}
}

func TestHandleSearch(t *testing.T) {
	node := createTestNode(t)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"GET with index", "GET", "/test-index/_search", ""},
		{"POST with index", "POST", "/test-index/_search", `{"query":{"match_all":{}}}`},
		{"GET all indices", "GET", "/_search", ""},
		{"POST all indices", "POST", "/_search", `{"query":{"match_all":{}}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req, _ = http.NewRequest(tt.method, tt.path, bytes.NewReader([]byte(tt.body)))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req, _ = http.NewRequest(tt.method, tt.path, nil)
			}

			w := httptest.NewRecorder()
			node.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if _, exists := response["hits"]; !exists {
				t.Error("Response should contain 'hits' field")
			}

			if _, exists := response["took"]; !exists {
				t.Error("Response should contain 'took' field")
			}
		})
	}
}

func TestHandleSearchWithComplexQuery(t *testing.T) {
	node := createTestNode(t)

	// Test with match query
	body := []byte(`{
		"query": {
			"match": {
				"title": "test query"
			}
		},
		"size": 10,
		"from": 0
	}`)

	req, _ := http.NewRequest("POST", "/test-index/_search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["timed_out"] != false {
		t.Error("Expected timed_out to be false")
	}
}

func TestHandleSearchWithBoolQuery(t *testing.T) {
	node := createTestNode(t)

	body := []byte(`{
		"query": {
			"bool": {
				"must": [
					{"match": {"title": "test"}}
				],
				"filter": [
					{"term": {"status": "published"}}
				]
			}
		}
	}`)

	req, _ := http.NewRequest("POST", "/test-index/_search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleSearchWithInvalidQuery(t *testing.T) {
	node := createTestNode(t)

	// Invalid JSON
	body := []byte(`{invalid json`)

	req, _ := http.NewRequest("POST", "/test-index/_search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if _, exists := response["error"]; !exists {
		t.Error("Expected error field in response")
	}
}

func TestHandleMultiSearch(t *testing.T) {
	node := createTestNode(t)

	tests := []struct {
		name string
		path string
	}{
		{"Multi-search without index", "/_msearch"},
		{"Multi-search with index", "/test-index/_msearch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{}
{"query":{"match_all":{}}}
`)
			req, _ := http.NewRequest("POST", tt.path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/x-ndjson")
			w := httptest.NewRecorder()
			node.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}
		})
	}
}

func TestHandleCount(t *testing.T) {
	node := createTestNode(t)

	tests := []struct {
		name   string
		method string
	}{
		{"GET count", "GET"},
		{"POST count", "POST"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, "/test-index/_count", nil)
			w := httptest.NewRecorder()
			node.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if _, exists := response["count"]; !exists {
				t.Error("Response should contain 'count' field")
			}
		})
	}
}

func TestHandleNodes(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("GET", "/_nodes", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["cluster_name"] != "quidditch-cluster" {
		t.Error("Incorrect cluster name")
	}
}

func TestHandleNodesStats(t *testing.T) {
	node := createTestNode(t)

	req, _ := http.NewRequest("GET", "/_nodes/stats", nil)
	w := httptest.NewRecorder()
	node.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestCoordinationNodeStopWithoutStart(t *testing.T) {
	node := createTestNode(t)

	ctx := context.Background()

	// Stopping without starting should not cause errors
	err := node.Stop(ctx)
	if err != nil {
		t.Errorf("Stop without start failed: %v", err)
	}
}

func TestCoordinationNodeStartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger, _ := zap.NewDevelopment()

	// Use a random high port to avoid conflicts
	cfg := &config.CoordinationConfig{
		NodeID:     "test-coord",
		BindAddr:   "127.0.0.1",
		RESTPort:   19200, // High port to avoid conflicts
		MasterAddr: "127.0.0.1:19300",
	}

	node, err := NewCoordinationNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create coordination node: %v", err)
	}

	ctx := context.Background()

	// Start the node
	if err := node.Start(ctx); err != nil {
		// Master connection will fail, but server should start
		if err.Error() != "failed to connect to master: failed to dial master: context deadline exceeded" {
			// Expected to fail connecting to master since it's not running
			_ = err
		}
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the node
	if err := node.Stop(ctx); err != nil {
		t.Errorf("Failed to stop coordination node: %v", err)
	}
}

func TestRouteSetup(t *testing.T) {
	node := createTestNode(t)

	// Test that routes are properly set up
	routes := node.router.Routes()

	expectedRoutes := []string{
		"/",
		"/_cluster/health",
		"/_cluster/state",
		"/_cluster/stats",
		"/:index/_search",
		"/_search",
		"/:index/_doc/:id",
		"/_bulk",
		"/_nodes",
	}

	routeMap := make(map[string]bool)
	for _, route := range routes {
		routeMap[route.Path] = true
	}

	for _, expected := range expectedRoutes {
		if !routeMap[expected] {
			t.Errorf("Expected route %s not found", expected)
		}
	}
}

// Helper function to create a test node
func createTestNode(t *testing.T) *CoordinationNode {
	logger, _ := zap.NewDevelopment()

	cfg := &config.CoordinationConfig{
		NodeID:     "test-coord",
		BindAddr:   "127.0.0.1",
		RESTPort:   9200,
		MasterAddr: "127.0.0.1:9300",
	}

	node, err := NewCoordinationNode(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create test node: %v", err)
	}

	return node
}

// Benchmark tests

func BenchmarkHandleSearch(b *testing.B) {
	node := createTestNodeBench(b)

	body := []byte(`{"query":{"match_all":{}}}`)
	req, _ := http.NewRequest("POST", "/test-index/_search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		node.router.ServeHTTP(w, req)
	}
}

func BenchmarkHandleClusterHealth(b *testing.B) {
	node := createTestNodeBench(b)

	req, _ := http.NewRequest("GET", "/_cluster/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		node.router.ServeHTTP(w, req)
	}
}

func createTestNodeBench(b *testing.B) *CoordinationNode {
	logger, _ := zap.NewDevelopment()

	cfg := &config.CoordinationConfig{
		NodeID:     "bench-coord",
		BindAddr:   "127.0.0.1",
		RESTPort:   9200,
		MasterAddr: "127.0.0.1:9300",
	}

	node, err := NewCoordinationNode(cfg, logger)
	if err != nil {
		b.Fatalf("Failed to create bench node: %v", err)
	}

	return node
}
