package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Namespace for all Quidditch metrics
const (
	Namespace = "quidditch"
)

// MetricsCollector aggregates all metrics for a Quidditch component
type MetricsCollector struct {
	// HTTP metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestSize     *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec

	// Query metrics
	QueryTotal          *prometheus.CounterVec
	QueryDuration       *prometheus.HistogramVec
	QueryComplexity     *prometheus.HistogramVec
	QueryCacheHits      prometheus.Counter
	QueryCacheMisses    prometheus.Counter
	QueryShardCount     *prometheus.HistogramVec

	// Bulk operation metrics
	BulkOperationsTotal *prometheus.CounterVec
	BulkOperationsDuration prometheus.Histogram
	BulkOperationsPerRequest *prometheus.HistogramVec

	// Document operation metrics
	DocumentsIndexed    *prometheus.CounterVec
	DocumentsDeleted    *prometheus.CounterVec
	DocumentsRetrieved  *prometheus.CounterVec

	// Cluster metrics
	ClusterNodes        *prometheus.GaugeVec
	ClusterShards       *prometheus.GaugeVec
	ClusterDocuments    prometheus.Gauge
	ClusterIndices      prometheus.Gauge

	// Shard metrics
	ShardOperations     *prometheus.CounterVec
	ShardSize           *prometheus.GaugeVec
	ShardDocuments      *prometheus.GaugeVec

	// gRPC metrics
	GRPCRequestsTotal   *prometheus.CounterVec
	GRPCRequestDuration *prometheus.HistogramVec

	// Raft metrics (for master nodes)
	RaftLeader          prometheus.Gauge
	RaftTerm            prometheus.Gauge
	RaftCommitIndex     prometheus.Gauge
	RaftAppliedIndex    prometheus.Gauge
}

// NewMetricsCollector creates a new metrics collector for a component
func NewMetricsCollector(component string) *MetricsCollector {
	return &MetricsCollector{
		// HTTP metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),
		HTTPRequestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "http_request_size_bytes",
				Help:      "HTTP request size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "http_response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),

		// Query metrics
		QueryTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "query_total",
				Help:      "Total number of queries executed",
			},
			[]string{"index", "query_type", "status"},
		),
		QueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "query_duration_seconds",
				Help:      "Query execution duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"index", "query_type"},
		),
		QueryComplexity: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "query_complexity",
				Help:      "Query complexity score (0-100)",
				Buckets:   []float64{1, 5, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
			},
			[]string{"index"},
		),
		QueryCacheHits: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "query_cache_hits_total",
				Help:      "Total number of query cache hits",
			},
		),
		QueryCacheMisses: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "query_cache_misses_total",
				Help:      "Total number of query cache misses",
			},
		),
		QueryShardCount: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "query_shard_count",
				Help:      "Number of shards queried per request",
				Buckets:   []float64{1, 2, 3, 5, 10, 20, 50, 100},
			},
			[]string{"index"},
		),

		// Bulk operation metrics
		BulkOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "bulk_operations_total",
				Help:      "Total number of bulk operations",
			},
			[]string{"operation", "status"},
		),
		BulkOperationsDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "bulk_operations_duration_seconds",
				Help:      "Bulk operation duration in seconds",
				Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60},
			},
		),
		BulkOperationsPerRequest: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "bulk_operations_per_request",
				Help:      "Number of operations per bulk request",
				Buckets:   []float64{1, 10, 50, 100, 500, 1000, 5000, 10000},
			},
			[]string{"has_errors"},
		),

		// Document operation metrics
		DocumentsIndexed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "documents_indexed_total",
				Help:      "Total number of documents indexed",
			},
			[]string{"index", "status"},
		),
		DocumentsDeleted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "documents_deleted_total",
				Help:      "Total number of documents deleted",
			},
			[]string{"index", "status"},
		),
		DocumentsRetrieved: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "documents_retrieved_total",
				Help:      "Total number of documents retrieved",
			},
			[]string{"index", "status"},
		),

		// Cluster metrics
		ClusterNodes: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "cluster_nodes",
				Help:      "Number of nodes in the cluster by type",
			},
			[]string{"node_type", "status"},
		),
		ClusterShards: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "cluster_shards",
				Help:      "Number of shards in the cluster",
			},
			[]string{"index", "state"},
		),
		ClusterDocuments: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "cluster_documents_total",
				Help:      "Total number of documents in the cluster",
			},
		),
		ClusterIndices: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "cluster_indices_total",
				Help:      "Total number of indices in the cluster",
			},
		),

		// Shard metrics
		ShardOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "shard_operations_total",
				Help:      "Total number of shard operations",
			},
			[]string{"operation", "status"},
		),
		ShardSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "shard_size_bytes",
				Help:      "Shard size in bytes",
			},
			[]string{"index", "shard_id"},
		),
		ShardDocuments: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "shard_documents",
				Help:      "Number of documents in shard",
			},
			[]string{"index", "shard_id"},
		),

		// gRPC metrics
		GRPCRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "grpc_requests_total",
				Help:      "Total number of gRPC requests",
			},
			[]string{"method", "status"},
		),
		GRPCRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "grpc_request_duration_seconds",
				Help:      "gRPC request duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method"},
		),

		// Raft metrics
		RaftLeader: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "raft_leader",
				Help:      "Whether this node is the Raft leader (1=leader, 0=follower)",
			},
		),
		RaftTerm: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "raft_term",
				Help:      "Current Raft term",
			},
		),
		RaftCommitIndex: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "raft_commit_index",
				Help:      "Current Raft commit index",
			},
		),
		RaftAppliedIndex: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Subsystem: component,
				Name:      "raft_applied_index",
				Help:      "Current Raft applied index",
			},
		),
	}
}

// RecordHTTPRequest records HTTP request metrics
func (m *MetricsCollector) RecordHTTPRequest(method, path string, status int, duration time.Duration, requestSize, responseSize int64) {
	m.HTTPRequestsTotal.WithLabelValues(method, path, statusClass(status)).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
	m.HTTPRequestSize.WithLabelValues(method, path).Observe(float64(requestSize))
	m.HTTPResponseSize.WithLabelValues(method, path).Observe(float64(responseSize))
}

// RecordQuery records query execution metrics
func (m *MetricsCollector) RecordQuery(index, queryType, status string, duration time.Duration, complexity int, shardCount int) {
	m.QueryTotal.WithLabelValues(index, queryType, status).Inc()
	m.QueryDuration.WithLabelValues(index, queryType).Observe(duration.Seconds())
	m.QueryComplexity.WithLabelValues(index).Observe(float64(complexity))
	m.QueryShardCount.WithLabelValues(index).Observe(float64(shardCount))
}

// RecordCacheHit records a cache hit
func (m *MetricsCollector) RecordCacheHit() {
	m.QueryCacheHits.Inc()
}

// RecordCacheMiss records a cache miss
func (m *MetricsCollector) RecordCacheMiss() {
	m.QueryCacheMisses.Inc()
}

// RecordBulkOperation records bulk operation metrics
func (m *MetricsCollector) RecordBulkOperation(operation, status string, duration time.Duration, operationCount int, hasErrors bool) {
	m.BulkOperationsTotal.WithLabelValues(operation, status).Inc()
	m.BulkOperationsDuration.Observe(duration.Seconds())

	hasErrorsStr := "false"
	if hasErrors {
		hasErrorsStr = "true"
	}
	m.BulkOperationsPerRequest.WithLabelValues(hasErrorsStr).Observe(float64(operationCount))
}

// statusClass converts HTTP status code to status class (2xx, 3xx, 4xx, 5xx)
func statusClass(status int) string {
	class := status / 100
	return fmt.Sprintf("%dxx", class)
}
