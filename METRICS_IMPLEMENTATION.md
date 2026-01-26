# Prometheus Metrics for Distributed Search

**Date:** 2026-01-26
**Status:** ✅ Complete
**Commit:** 6510ca6

## Overview

Comprehensive Prometheus metrics have been added to the Quidditch search engine to monitor distributed query performance, shard health, and aggregation efficiency. These metrics enable real-time monitoring, alerting, and performance analysis of the distributed search infrastructure.

## Metrics

### 1. Distributed Search Latency

**Metric:** `quidditch_distributed_search_latency_seconds`
**Type:** Histogram
**Labels:** `index`
**Description:** Tracks overall distributed search query latency from coordination node perspective.

**Buckets:** Exponential from 1ms to ~4s (0.001, 0.002, 0.004, ..., 4.096 seconds)

**Usage:**
```promql
# P95 latency per index
histogram_quantile(0.95, sum by (le, index) (
  rate(quidditch_distributed_search_latency_seconds_bucket[5m])
))

# Average latency per index
rate(quidditch_distributed_search_latency_seconds_sum[5m]) /
rate(quidditch_distributed_search_latency_seconds_count[5m])
```

### 2. Shard Query Latency

**Metric:** `quidditch_shard_query_latency_seconds`
**Type:** Histogram
**Labels:** `index`, `shard_id`, `node_id`
**Description:** Tracks per-shard query latency, enabling identification of slow shards or nodes.

**Buckets:** Exponential from 1ms to ~4s

**Usage:**
```promql
# Slowest shards (P95)
topk(10, histogram_quantile(0.95, sum by (le, index, shard_id, node_id) (
  rate(quidditch_shard_query_latency_seconds_bucket[5m])
)))

# Compare latency across nodes
avg by (node_id) (
  rate(quidditch_shard_query_latency_seconds_sum[5m]) /
  rate(quidditch_shard_query_latency_seconds_count[5m])
)
```

### 3. Shard Query Failures

**Metric:** `quidditch_shard_query_failures_total`
**Type:** Counter
**Labels:** `index`, `shard_id`, `node_id`, `error_type`
**Description:** Counts shard query failures by error type.

**Error Types:**
- `client_not_found`: DataNode client not registered
- `connection_failed`: Failed to connect to DataNode
- `search_failed`: Search operation failed on shard

**Usage:**
```promql
# Failure rate by error type
rate(quidditch_shard_query_failures_total[5m])

# Nodes with highest failure rate
topk(5, sum by (node_id) (
  rate(quidditch_shard_query_failures_total[5m])
))

# Alert on high failure rate
rate(quidditch_shard_query_failures_total[5m]) > 0.1
```

### 4. Aggregation Merge Time

**Metric:** `quidditch_aggregation_merge_seconds`
**Type:** Histogram
**Labels:** `aggregation_type`
**Description:** Tracks time spent merging aggregations across shards.

**Aggregation Types:** `terms`, `histogram`, `date_histogram`, `stats`, `extended_stats`, `percentiles`, `cardinality`

**Buckets:** Exponential from 0.1ms to ~400ms (0.0001, 0.0002, 0.0004, ..., 0.4096 seconds)

**Usage:**
```promql
# Most expensive aggregation types
topk(3, sum by (aggregation_type) (
  rate(quidditch_aggregation_merge_seconds_sum[5m])
))

# P99 merge time per aggregation type
histogram_quantile(0.99, sum by (le, aggregation_type) (
  rate(quidditch_aggregation_merge_seconds_bucket[5m])
))
```

### 5. Distributed Search Hits Total

**Metric:** `quidditch_distributed_search_hits_total`
**Type:** Histogram
**Labels:** `index`
**Description:** Tracks total number of hits returned by distributed search queries.

**Buckets:** Exponential from 1 to ~1M (1, 2, 4, 8, ..., 1048576)

**Usage:**
```promql
# Average hits per query
rate(quidditch_distributed_search_hits_total_sum[5m]) /
rate(quidditch_distributed_search_hits_total_count[5m])

# Queries returning > 10K hits
sum(rate(quidditch_distributed_search_hits_total_bucket{le="10000"}[5m]))
```

### 6. Distributed Search Shards Queried

**Metric:** `quidditch_distributed_search_shards_queried`
**Type:** Histogram
**Labels:** `index`
**Description:** Tracks number of shards queried per distributed search request.

**Buckets:** Linear from 1 to 20 (1, 2, 3, ..., 20)

**Usage:**
```promql
# Average shards queried per index
rate(quidditch_distributed_search_shards_queried_sum[5m]) /
rate(quidditch_distributed_search_shards_queried_count[5m])

# Query distribution (% queries hitting N shards)
histogram_quantile(0.5, sum by (le, index) (
  rate(quidditch_distributed_search_shards_queried_bucket[5m])
))
```

## Instrumentation Points

### ExecuteSearch Method (`pkg/coordination/executor/executor.go`)

**Line 170-241:** Per-shard query goroutine
- Records `shardQueryLatency` with defer statement (captures total goroutine time)
- Records `shardQueryFailures` on error paths (client_not_found, connection_failed, search_failed)

**Line 277-280:** After result aggregation
- Records `distributedSearchLatency` (overall query time)
- Records `distributedSearchShardsQueried` (number of successful shards)
- Records `distributedSearchHitsTotal` (total hits from all shards)

### mergeAggregations Method (`pkg/coordination/executor/aggregator.go`)

**Line 118-143:** Aggregation merge loop
- Records `aggregationMergeTime` per aggregation type
- Tracks merge duration for: terms, histogram, date_histogram, stats, extended_stats, percentiles, cardinality

## Grafana Dashboard

### Suggested Panels

#### 1. Query Performance
```yaml
- Panel: Distributed Search Latency (P50, P95, P99)
  Query: histogram_quantile(0.95, sum by (le) (
           rate(quidditch_distributed_search_latency_seconds_bucket[5m])
         ))

- Panel: Queries per Second (QPS)
  Query: sum(rate(quidditch_distributed_search_latency_seconds_count[5m]))

- Panel: Shard Latency Distribution
  Query: histogram_quantile(0.95, sum by (le, node_id) (
           rate(quidditch_shard_query_latency_seconds_bucket[5m])
         ))
```

#### 2. Failure Monitoring
```yaml
- Panel: Shard Failure Rate by Type
  Query: sum by (error_type) (
           rate(quidditch_shard_query_failures_total[5m])
         )

- Panel: Failed Shards per Node
  Query: sum by (node_id) (
           rate(quidditch_shard_query_failures_total[5m])
         )

- Panel: Success Rate
  Query: 1 - (
           sum(rate(quidditch_shard_query_failures_total[5m])) /
           sum(rate(quidditch_shard_query_latency_seconds_count[5m]))
         )
```

#### 3. Aggregation Performance
```yaml
- Panel: Aggregation Merge Time by Type
  Query: histogram_quantile(0.95, sum by (le, aggregation_type) (
           rate(quidditch_aggregation_merge_seconds_bucket[5m])
         ))

- Panel: Aggregation Overhead %
  Query: (sum(rate(quidditch_aggregation_merge_seconds_sum[5m])) /
          sum(rate(quidditch_distributed_search_latency_seconds_sum[5m]))) * 100
```

#### 4. Query Distribution
```yaml
- Panel: Average Shards per Query
  Query: rate(quidditch_distributed_search_shards_queried_sum[5m]) /
         rate(quidditch_distributed_search_shards_queried_count[5m])

- Panel: Average Hits per Query
  Query: rate(quidditch_distributed_search_hits_total_sum[5m]) /
         rate(quidditch_distributed_search_hits_total_count[5m])
```

## Alerting Rules

### Critical Alerts

#### High Query Latency
```yaml
- alert: HighDistributedSearchLatency
  expr: |
    histogram_quantile(0.95,
      sum by (le, index) (
        rate(quidditch_distributed_search_latency_seconds_bucket[5m])
      )
    ) > 1.0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "High distributed search latency on index {{ $labels.index }}"
    description: "P95 latency is {{ $value }}s (threshold: 1.0s)"
```

#### High Shard Failure Rate
```yaml
- alert: HighShardFailureRate
  expr: |
    sum by (node_id) (
      rate(quidditch_shard_query_failures_total[5m])
    ) > 0.1
  for: 3m
  labels:
    severity: critical
  annotations:
    summary: "High shard failure rate on node {{ $labels.node_id }}"
    description: "Failure rate is {{ $value }} failures/sec"
```

#### Node Completely Down
```yaml
- alert: DataNodeDown
  expr: |
    sum by (node_id) (
      rate(quidditch_shard_query_failures_total{error_type="client_not_found"}[5m])
    ) > 0.5
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "Data node {{ $labels.node_id }} appears to be down"
    description: "All queries to this node are failing with client_not_found"
```

### Warning Alerts

#### Slow Aggregation Merging
```yaml
- alert: SlowAggregationMerge
  expr: |
    histogram_quantile(0.95,
      sum by (le, aggregation_type) (
        rate(quidditch_aggregation_merge_seconds_bucket[5m])
      )
    ) > 0.1
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "Slow aggregation merge for type {{ $labels.aggregation_type }}"
    description: "P95 merge time is {{ $value }}s (threshold: 0.1s)"
```

#### Unbalanced Shard Distribution
```yaml
- alert: UnbalancedShardQueries
  expr: |
    (max by (node_id) (
      rate(quidditch_shard_query_latency_seconds_count[5m])
    ) - min by (node_id) (
      rate(quidditch_shard_query_latency_seconds_count[5m])
    )) > 10
  for: 15m
  labels:
    severity: info
  annotations:
    summary: "Unbalanced query distribution across nodes"
    description: "Query rate difference between nodes: {{ $value }} QPS"
```

## Performance Benchmarks

### Expected Metrics Values (4-node cluster, 100K docs)

| Metric | Expected Value | Notes |
|--------|----------------|-------|
| Distributed Search Latency (P95) | <50ms | For match_all queries |
| Shard Query Latency (P95) | <20ms | For individual shards |
| Shard Failure Rate | <0.01% | Less than 1 failure per 10K queries |
| Aggregation Merge Time (terms) | <5ms | For 100 buckets |
| Aggregation Merge Time (stats) | <1ms | Simple numeric aggregation |
| Aggregation Overhead | <10% | Of total query time |
| Shards per Query | 6 | For index with 6 shards |
| Success Rate | >99.9% | Query success rate |

### Scalability Targets

| Nodes | Expected P95 Latency | Expected QPS |
|-------|---------------------|--------------|
| 1 | 30ms | 200 |
| 2 | 35ms | 350 |
| 4 | 45ms | 650 |
| 8 | 55ms | 1200 |

## Metric Cardinality

**Total Unique Time Series (estimated):**
- `distributedSearchLatency`: ~10 (10 indices)
- `shardQueryLatency`: ~180 (10 indices × 6 shards × 3 nodes)
- `shardQueryFailures`: ~360 (10 indices × 6 shards × 3 nodes × 2 error types)
- `aggregationMergeTime`: ~7 (7 aggregation types)
- `distributedSearchHitsTotal`: ~10 (10 indices)
- `distributedSearchShardsQueried`: ~10 (10 indices)

**Total: ~577 time series** (for typical 3-node, 10-index deployment)

## Integration with Existing Metrics

Quidditch metrics are prefixed with `quidditch_` to avoid conflicts. They complement existing Prometheus metrics:

- **Node Exporter:** System-level metrics (CPU, memory, disk)
- **gRPC Metrics:** RPC-level metrics (call duration, error rates)
- **Go Runtime:** goroutines, memory, GC stats

## Export Configuration

### Coordination Node (`pkg/coordination/coordination.go`)

Metrics are automatically registered with the default Prometheus registry and exposed on the `/metrics` endpoint:

```go
import "github.com/prometheus/client_golang/prometheus/promhttp"

http.Handle("/metrics", promhttp.Handler())
```

### Prometheus Scrape Config

```yaml
scrape_configs:
  - job_name: 'quidditch-coordination'
    static_configs:
      - targets: ['coord1:9200', 'coord2:9200']
    scrape_interval: 15s
    scrape_timeout: 10s
```

## Testing Metrics

### Manual Testing

```bash
# 1. Start Quidditch cluster with 3 nodes
./scripts/start_cluster.sh --nodes 3

# 2. Create index and index documents
curl -X PUT http://localhost:9200/test -d '{"settings":{"index":{"number_of_shards":6}}}'
./scripts/bulk_index.sh test 10000

# 3. Run queries to generate metrics
for i in {1..100}; do
  curl -X POST http://localhost:9200/test/_search -d '{"query":{"match_all":{}}}'
done

# 4. Check metrics endpoint
curl http://localhost:9200/metrics | grep quidditch_

# 5. Verify metrics are present
curl -s http://localhost:9200/metrics | grep -E "quidditch_(distributed_search_latency|shard_query_latency|shard_query_failures|aggregation_merge)"
```

### Expected Output

```
# HELP quidditch_distributed_search_latency_seconds Distributed search query latency in seconds
# TYPE quidditch_distributed_search_latency_seconds histogram
quidditch_distributed_search_latency_seconds_bucket{index="test",le="0.001"} 0
quidditch_distributed_search_latency_seconds_bucket{index="test",le="0.002"} 5
quidditch_distributed_search_latency_seconds_bucket{index="test",le="0.004"} 25
...
quidditch_distributed_search_latency_seconds_sum{index="test"} 2.45
quidditch_distributed_search_latency_seconds_count{index="test"} 100

# HELP quidditch_shard_query_latency_seconds Per-shard query latency in seconds
# TYPE quidditch_shard_query_latency_seconds histogram
quidditch_shard_query_latency_seconds_bucket{index="test",node_id="data1",shard_id="0",le="0.001"} 2
...

# HELP quidditch_shard_query_failures_total Total number of shard query failures
# TYPE quidditch_shard_query_failures_total counter
quidditch_shard_query_failures_total{error_type="client_not_found",index="test",node_id="data1",shard_id="0"} 0
```

## Files Modified

### Metrics Implementation
- `pkg/coordination/executor/executor.go` (+115 lines)
  - Added Prometheus imports
  - Defined 6 metrics variables
  - Instrumented ExecuteSearch method
  - Added per-shard latency and failure tracking

- `pkg/coordination/executor/aggregator.go` (+5 lines)
  - Added time import
  - Instrumented mergeAggregations method
  - Added per-aggregation-type timing

## Benefits

### 1. Observability
- **Real-time visibility** into distributed query performance
- **Per-shard granularity** for identifying slow or failing shards
- **Aggregation breakdown** showing which types are expensive

### 2. Debugging
- **Error type classification** (client_not_found, connection_failed, search_failed)
- **Per-node metrics** for isolating problematic DataNodes
- **Latency distribution** for understanding tail latencies

### 3. Capacity Planning
- **Query throughput tracking** (QPS per index)
- **Resource utilization** (shards queried, hits returned)
- **Scalability validation** (compare 2-node vs 4-node performance)

### 4. Alerting
- **Proactive monitoring** of query latency degradation
- **Automated failure detection** for node outages
- **SLO tracking** for search performance guarantees

## Future Enhancements

### Short-term
- [ ] Add cache hit/miss metrics (when caching is implemented)
- [ ] Add query routing metrics (broadcast vs targeted)
- [ ] Add result size metrics (bytes returned)

### Medium-term
- [ ] Add histogram of hits per query (query complexity)
- [ ] Add shard rebalancing metrics (when implemented)
- [ ] Add replica query metrics (when replication is added)

### Long-term
- [ ] Add distributed tracing integration (Jaeger/Zipkin)
- [ ] Add query AST depth metrics (query complexity)
- [ ] Add index-level metrics (doc count, size, age)

## Conclusion

The Prometheus metrics implementation provides comprehensive observability into the Quidditch distributed search engine. With 6 core metrics tracking latency, failures, aggregation performance, and query distribution, operators can:

- Monitor system health in real-time
- Identify performance bottlenecks quickly
- Set up proactive alerting for failures
- Validate scalability characteristics
- Plan capacity based on actual usage patterns

**Metrics are production-ready** and follow Prometheus best practices for naming, labeling, and cardinality management.
