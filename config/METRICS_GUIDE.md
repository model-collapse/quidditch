# Quidditch Metrics Guide

This guide explains the metrics exposed by Quidditch and how to monitor your cluster.

## Metrics Endpoints

All Quidditch nodes expose metrics on the `/metrics` endpoint in Prometheus format:

- **Master nodes**: `http://<master-host>:9000/metrics`
- **Coordination nodes**: `http://<coordination-host>:8080/metrics`
- **Data nodes**: `http://<data-host>:9090/metrics`

## Available Metrics

### HTTP Metrics (All Nodes)

#### `quidditch_<component>_http_requests_total`
Total number of HTTP requests by method, path, and status code.

Labels:
- `method`: HTTP method (GET, POST, PUT, DELETE)
- `path`: Request path
- `status`: HTTP status class (2xx, 3xx, 4xx, 5xx)

#### `quidditch_<component>_http_request_duration_seconds`
HTTP request duration histogram.

Labels:
- `method`: HTTP method
- `path`: Request path

Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s

#### `quidditch_<component>_http_request_size_bytes`
HTTP request size histogram.

Labels:
- `method`: HTTP method
- `path`: Request path

#### `quidditch_<component>_http_response_size_bytes`
HTTP response size histogram.

Labels:
- `method`: HTTP method
- `path`: Request path

### Query Metrics (Coordination Nodes)

#### `quidditch_coordination_query_total`
Total number of queries executed.

Labels:
- `index`: Index name
- `query_type`: Type of query (match, term, bool, etc.)
- `status`: Query status (success, error)

#### `quidditch_coordination_query_duration_seconds`
Query execution duration histogram.

Labels:
- `index`: Index name
- `query_type`: Type of query

Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s

#### `quidditch_coordination_query_complexity`
Query complexity score (0-100).

Labels:
- `index`: Index name

Buckets: 1, 5, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100

#### `quidditch_coordination_query_cache_hits_total`
Total number of query cache hits.

#### `quidditch_coordination_query_cache_misses_total`
Total number of query cache misses.

#### `quidditch_coordination_query_shard_count`
Number of shards queried per request.

Labels:
- `index`: Index name

Buckets: 1, 2, 3, 5, 10, 20, 50, 100

### Bulk Operation Metrics (Coordination Nodes)

#### `quidditch_coordination_bulk_operations_total`
Total number of bulk operations.

Labels:
- `operation`: Operation type (index, create, update, delete)
- `status`: Operation status (success, error)

#### `quidditch_coordination_bulk_operations_duration_seconds`
Bulk operation duration histogram.

Buckets: 10ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s, 30s, 60s

#### `quidditch_coordination_bulk_operations_per_request`
Number of operations per bulk request.

Labels:
- `has_errors`: Whether the bulk request had any errors (true, false)

Buckets: 1, 10, 50, 100, 500, 1000, 5000, 10000

### Document Metrics (Coordination & Data Nodes)

#### `quidditch_<component>_documents_indexed_total`
Total number of documents indexed.

Labels:
- `index`: Index name
- `status`: Operation status (success, error)

#### `quidditch_<component>_documents_deleted_total`
Total number of documents deleted.

Labels:
- `index`: Index name
- `status`: Operation status (success, error)

#### `quidditch_<component>_documents_retrieved_total`
Total number of documents retrieved.

Labels:
- `index`: Index name
- `status`: Operation status (success, error)

### Cluster Metrics (Master & Coordination Nodes)

#### `quidditch_<component>_cluster_nodes`
Number of nodes in the cluster by type and status.

Labels:
- `node_type`: Type of node (master, coordination, data)
- `status`: Node status (active, inactive, failed)

#### `quidditch_<component>_cluster_shards`
Number of shards in the cluster.

Labels:
- `index`: Index name
- `state`: Shard state (started, initializing, relocating, unassigned)

#### `quidditch_<component>_cluster_documents_total`
Total number of documents in the cluster.

#### `quidditch_<component>_cluster_indices_total`
Total number of indices in the cluster.

### Shard Metrics (Data Nodes)

#### `quidditch_data_shard_operations_total`
Total number of shard operations.

Labels:
- `operation`: Operation type (create, delete, index, search, etc.)
- `status`: Operation status (success, error)

#### `quidditch_data_shard_size_bytes`
Shard size in bytes.

Labels:
- `index`: Index name
- `shard_id`: Shard ID

#### `quidditch_data_shard_documents`
Number of documents in shard.

Labels:
- `index`: Index name
- `shard_id`: Shard ID

### gRPC Metrics (All Nodes)

#### `quidditch_<component>_grpc_requests_total`
Total number of gRPC requests.

Labels:
- `method`: gRPC method name
- `status`: Request status (OK, error codes)

#### `quidditch_<component>_grpc_request_duration_seconds`
gRPC request duration histogram.

Labels:
- `method`: gRPC method name

Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s

### Raft Metrics (Master Nodes)

#### `quidditch_master_raft_leader`
Whether this node is the Raft leader (1=leader, 0=follower).

#### `quidditch_master_raft_term`
Current Raft term.

#### `quidditch_master_raft_commit_index`
Current Raft commit index.

#### `quidditch_master_raft_applied_index`
Current Raft applied index.

## Example Queries

### Request Rate
```promql
rate(quidditch_coordination_http_requests_total[5m])
```

### P95 Query Latency
```promql
histogram_quantile(0.95, rate(quidditch_coordination_query_duration_seconds_bucket[5m]))
```

### Query Cache Hit Rate
```promql
rate(quidditch_coordination_query_cache_hits_total[5m]) /
(rate(quidditch_coordination_query_cache_hits_total[5m]) + rate(quidditch_coordination_query_cache_misses_total[5m]))
```

### Bulk Operations Throughput
```promql
rate(quidditch_coordination_bulk_operations_total[5m])
```

### Error Rate
```promql
rate(quidditch_coordination_http_requests_total{status="5xx"}[5m]) /
rate(quidditch_coordination_http_requests_total[5m])
```

### Cluster Document Count
```promql
quidditch_master_cluster_documents_total
```

### Shard Distribution
```promql
sum by (index, state) (quidditch_master_cluster_shards)
```

## Grafana Dashboards

Example Grafana dashboards are available in the `config/grafana/` directory:

- `cluster-overview.json`: Overall cluster health and status
- `coordination-node.json`: Coordination node metrics
- `query-performance.json`: Query performance and analysis
- `data-node.json`: Data node and shard metrics

## Alerting Rules

Example alert rules are available in `config/prometheus_rules.yml`:

- High error rate
- Slow queries
- Cache hit rate too low
- Cluster unhealthy
- Node down
- Shard allocation issues

## Setting Up Monitoring

1. **Configure Prometheus**: Use the example configuration in `config/prometheus.yml`

2. **Start Prometheus**:
   ```bash
   prometheus --config.file=config/prometheus.yml
   ```

3. **Configure Grafana** (optional):
   - Add Prometheus as a data source
   - Import the example dashboards from `config/grafana/`

4. **Configure Alertmanager** (optional):
   - Set up alert routing
   - Configure notification channels

## Health Checks

All nodes expose a health check endpoint:

```
GET /_health
```

Returns:
```json
{
  "status": "green",
  "checks": {
    "master_connection": "ok",
    "query_executor": "ok",
    "query_planner": "ok"
  }
}
```

Status codes:
- `200 OK`: All checks passed
- `503 Service Unavailable`: One or more checks failed
