package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// HTTPClient is a helper for making REST API calls
type HTTPClient struct {
	BaseURL string
	Client  *http.Client
	t       *testing.T
}

// NewHTTPClient creates a new HTTP client helper
func NewHTTPClient(t *testing.T, baseURL string) *HTTPClient {
	return &HTTPClient{
		BaseURL: baseURL,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
		t: t,
	}
}

// Get performs a GET request
func (c *HTTPClient) Get(path string) (*http.Response, error) {
	url := c.BaseURL + path
	c.t.Logf("GET %s", url)
	return c.Client.Get(url)
}

// Post performs a POST request
func (c *HTTPClient) Post(path string, body interface{}) (*http.Response, error) {
	url := c.BaseURL + path
	c.t.Logf("POST %s", url)

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	return c.Client.Do(req)
}

// Put performs a PUT request
func (c *HTTPClient) Put(path string, body interface{}) (*http.Response, error) {
	url := c.BaseURL + path
	c.t.Logf("PUT %s", url)

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest("PUT", url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	return c.Client.Do(req)
}

// Delete performs a DELETE request
func (c *HTTPClient) Delete(path string) (*http.Response, error) {
	url := c.BaseURL + path
	c.t.Logf("DELETE %s", url)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, err
	}

	return c.Client.Do(req)
}

// DecodeJSON decodes JSON response body
func (c *HTTPClient) DecodeJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// ClusterHealthResponse represents cluster health response
type ClusterHealthResponse struct {
	ClusterName            string  `json:"cluster_name"`
	Status                 string  `json:"status"`
	TimedOut               bool    `json:"timed_out"`
	NumberOfNodes          int     `json:"number_of_nodes"`
	NumberOfDataNodes      int     `json:"number_of_data_nodes"`
	ActivePrimaryShards    int     `json:"active_primary_shards"`
	ActiveShards           int     `json:"active_shards"`
	RelocatingShards       int     `json:"relocating_shards"`
	InitializingShards     int     `json:"initializing_shards"`
	UnassignedShards       int     `json:"unassigned_shards"`
	ActiveShardsPercentage float64 `json:"active_shards_percent_as_number"`
}

// SearchRequest represents a search request
type SearchRequest struct {
	Query interface{} `json:"query,omitempty"`
	Size  int         `json:"size,omitempty"`
	From  int         `json:"from,omitempty"`
}

// SearchResponse represents a search response
type SearchResponse struct {
	Took     int64       `json:"took"`
	TimedOut bool        `json:"timed_out"`
	Shards   ShardInfo   `json:"_shards"`
	Hits     HitsWrapper `json:"hits"`
}

// ShardInfo represents shard information
type ShardInfo struct {
	Total      int `json:"total"`
	Successful int `json:"successful"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
}

// HitsWrapper wraps the hits response
type HitsWrapper struct {
	Total    TotalHits              `json:"total"`
	MaxScore *float64               `json:"max_score"`
	Hits     []map[string]interface{} `json:"hits"`
}

// TotalHits represents total hit information
type TotalHits struct {
	Value    int    `json:"value"`
	Relation string `json:"relation"`
}

// IndexResponse represents an index creation response
type IndexResponse struct {
	Acknowledged       bool   `json:"acknowledged"`
	ShardsAcknowledged bool   `json:"shards_acknowledged"`
	Index              string `json:"index"`
}

// RetryWithBackoff retries a function with exponential backoff
func RetryWithBackoff(ctx context.Context, maxRetries int, initialDelay time.Duration, fn func() error) error {
	delay := initialDelay

	for i := 0; i < maxRetries; i++ {
		err := fn()
		if err == nil {
			return nil
		}

		if i == maxRetries-1 {
			return fmt.Errorf("max retries exceeded: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			delay *= 2
		}
	}

	return fmt.Errorf("retry failed after %d attempts", maxRetries)
}

// WaitForCondition waits for a condition to be true
func WaitForCondition(t *testing.T, timeout time.Duration, checkInterval time.Duration, condition func() bool) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if condition() {
			return nil
		}
		time.Sleep(checkInterval)
	}

	return fmt.Errorf("condition not met within %v", timeout)
}

// AssertEventually asserts that a condition becomes true within timeout
func AssertEventually(t *testing.T, timeout time.Duration, condition func() bool, msgFormat string, args ...interface{}) {
	t.Helper()

	err := WaitForCondition(t, timeout, 100*time.Millisecond, condition)
	if err != nil {
		t.Errorf(msgFormat, args...)
	}
}

// AssertHTTPStatus asserts that HTTP response has expected status
func AssertHTTPStatus(t *testing.T, resp *http.Response, expectedStatus int) {
	t.Helper()

	if resp.StatusCode != expectedStatus {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status %d, got %d. Body: %s", expectedStatus, resp.StatusCode, string(body))
	}
}

// AssertJSONField asserts that a JSON response contains a field with expected value
func AssertJSONField(t *testing.T, data map[string]interface{}, field string, expected interface{}) {
	t.Helper()

	actual, exists := data[field]
	if !exists {
		t.Errorf("Field %s not found in response", field)
		return
	}

	if actual != expected {
		t.Errorf("Field %s: expected %v, got %v", field, expected, actual)
	}
}

// GetCoordNodeURL returns the REST API URL for a coordination node
func GetCoordNodeURL(cluster *TestCluster, index int) string {
	node := cluster.GetCoordNode(index)
	if node == nil {
		return ""
	}
	return fmt.Sprintf("http://%s:%d", node.Config.BindAddr, node.Config.RESTPort)
}

// CreateTestIndex creates a test index through the coordination node
func CreateTestIndex(t *testing.T, client *HTTPClient, indexName string, shards, replicas int) error {
	t.Helper()

	body := map[string]interface{}{
		"settings": map[string]interface{}{
			"number_of_shards":   shards,
			"number_of_replicas": replicas,
		},
	}

	resp, err := client.Put("/"+indexName, body)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create index failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SearchIndex performs a search on an index
func SearchIndex(t *testing.T, client *HTTPClient, indexName string, query interface{}) (*SearchResponse, error) {
	t.Helper()

	searchReq := SearchRequest{
		Query: query,
		Size:  10,
		From:  0,
	}

	resp, err := client.Post("/"+indexName+"/_search", searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed with status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &searchResp, nil
}

// IndexDocument indexes a document
func IndexDocument(t *testing.T, client *HTTPClient, indexName, docID string, doc interface{}) error {
	t.Helper()

	path := fmt.Sprintf("/%s/_doc/%s", indexName, docID)
	resp, err := client.Put(path, doc)
	if err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("index document failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetClusterHealth gets cluster health
func GetClusterHealth(t *testing.T, client *HTTPClient) (*ClusterHealthResponse, error) {
	t.Helper()

	resp, err := client.Get("/_cluster/health")
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cluster health failed with status %d", resp.StatusCode)
	}

	var health ClusterHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &health, nil
}
