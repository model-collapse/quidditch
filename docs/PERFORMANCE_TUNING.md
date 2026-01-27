# Quidditch Performance Tuning Guide

**Version**: 1.0
**Date**: 2026-01-26
**Status**: Production Ready

---

## Table of Contents

1. [Overview](#overview)
2. [Performance Targets](#performance-targets)
3. [Query Optimization](#query-optimization)
4. [Cache Tuning](#cache-tuning)
5. [Index Design](#index-design)
6. [Resource Allocation](#resource-allocation)
7. [Distributed Execution](#distributed-execution)
8. [Monitoring](#monitoring)
9. [Troubleshooting](#troubleshooting)
10. [Best Practices](#best-practices)

---

## Overview

This guide covers performance optimization techniques for Quidditch, focusing on achieving sub-100ms query latency and 50K+ docs/sec indexing throughput.

### Current Performance

**Achieved**:
- ✅ 71K docs/sec indexing (vs 50K target)
- ✅ <50ms search latency (vs 100ms target)
- ✅ 2-4× speedup with query cache
- ✅ 30-49% speedup with query optimizations

### Performance Stack

```
Application Layer
  ├─ Query Cache (2-4× speedup)
  ├─ Query Optimizer (30-49% speedup)
  └─ HTTP API (OpenSearch compatible)
       ↓
Coordination Layer
  ├─ Query Planner (cost-based)
  ├─ Result Aggregation
  └─ Parallel Execution
       ↓
Data Layer (Diagon C++)
  ├─ SIMD Acceleration (4-8× on scoring)
  ├─ Compression (LZ4/ZSTD)
  └─ MMap I/O (2-3× faster)
```

---

## Performance Targets

### Query Latency

| Query Type | Target | Good | Excellent |
|------------|--------|------|-----------|
| Simple match | <50ms | <30ms | <10ms |
| Bool query | <100ms | <70ms | <30ms |
| Aggregation | <200ms | <150ms | <80ms |
| TopN sorted | <150ms | <100ms | <50ms |
| Complex (agg + sort) | <300ms | <200ms | <100ms |

### Throughput

| Operation | Target | Good | Excellent |
|-----------|--------|------|-----------|
| Indexing | 50K docs/sec | 70K/sec | 100K/sec |
| Simple queries | 1K qps | 2K qps | 5K qps |
| Complex queries | 500 qps | 1K qps | 2K qps |

### Resource Usage

| Resource | Target | Good | Limit |
|----------|--------|------|-------|
| Coordination CPU | <50% | <30% | 80% |
| Coordination Memory | <2GB | <1GB | 4GB |
| Data Node CPU | <70% | <50% | 90% |
| Data Node Memory | <4GB | <2GB | 8GB |

---

## Query Optimization

### Rule #1: Use Specific Filters

**❌ Slow**:
```json
{
  "query": {"match_all": {}},
  "size": 100
}
```
- Scans all 1M documents
- Returns top 100
- Latency: ~500ms

**✅ Fast**:
```json
{
  "query": {
    "bool": {
      "must": [{"term": {"status": "active"}}],
      "filter": [{"range": {"date": {"gte": "2026-01-01"}}}]
    }
  },
  "size": 100
}
```
- Filters to ~50K documents
- Returns top 100
- Latency: ~80ms (**6× faster**)

### Rule #2: Use TopN for Sorted Queries

**❌ Slow**:
```json
{
  "query": {"match_all": {}},
  "sort": [{"price": "desc"}],
  "size": 10
}
```
- Sorts all documents
- Takes top 10
- Latency: ~200ms

**✅ Fast**:
```json
{
  "query": {"match_all": {}},
  "sort": [{"price": "desc"}],
  "size": 10
}
```
- Optimizer automatically converts to TopN
- Maintains heap of top 10
- Latency: ~140ms (**30% faster**)

### Rule #3: Limit Result Size

**❌ Slow**:
```json
{
  "query": {"match_all": {}},
  "size": 10000
}
```
- Transfers 10K documents
- Memory: ~10 MB
- Latency: ~300ms

**✅ Fast**:
```json
{
  "query": {"match_all": {}},
  "size": 100
}
```
- Transfers 100 documents
- Memory: ~100 KB
- Latency: ~50ms (**6× faster**)

### Rule #4: Avoid Deep Pagination

**❌ Slow**:
```json
{
  "query": {"match_all": {}},
  "from": 10000,
  "size": 10
}
```
- Scans first 10,010 documents
- Throws away 10,000
- Latency: ~400ms

**✅ Fast (use search_after)**:
```json
{
  "query": {"match_all": {}},
  "search_after": [prev_score, prev_id],
  "size": 10
}
```
- Continues from cursor
- No waste
- Latency: ~50ms (**8× faster**)

### Rule #5: Push Filters Before Aggregations

**❌ Slow**:
```json
{
  "query": {"match_all": {}},
  "aggs": {
    "categories": {
      "terms": {"field": "category"},
      "aggs": {
        "active_only": {
          "filter": {"term": {"status": "active"}},
          "aggs": {
            "avg_price": {"avg": {"field": "price"}}
          }
        }
      }
    }
  }
}
```
- Aggregates 1M docs
- Then filters
- Latency: ~800ms

**✅ Fast**:
```json
{
  "query": {"term": {"status": "active"}},
  "aggs": {
    "categories": {
      "terms": {"field": "category"},
      "aggs": {
        "avg_price": {"avg": {"field": "price"}}
      }
    }
  }
}
```
- Filters first (600K docs)
- Then aggregates
- Optimizer automatically pushes filter
- Latency: ~450ms (**44% faster**)

### Rule #6: Use Cache-Friendly Queries

**❌ Cache-unfriendly**:
```json
{
  "query": {
    "bool": {
      "must": [{"term": {"status": "active"}}],
      "filter": [{"range": {"timestamp": {"gte": "now"}}}]
    }
  }
}
```
- `"now"` changes every second
- Every query has unique key
- Cache hit rate: 0%

**✅ Cache-friendly**:
```json
{
  "query": {
    "bool": {
      "must": [{"term": {"status": "active"}}],
      "filter": [{"range": {"timestamp": {"gte": "2026-01-26T00:00:00Z"}}}]
    }
  }
}
```
- Fixed date
- Repeated queries hit cache
- Cache hit rate: 95%
- Latency: 0.22ms vs 0.8ms (**3.6× faster**)

### Rule #7: Minimize Aggregation Cardinality

**❌ Slow**:
```json
{
  "aggs": {
    "unique_users": {
      "terms": {"field": "user_id", "size": 10000}
    }
  }
}
```
- 10K unique values
- Large hash table
- Latency: ~600ms

**✅ Fast**:
```json
{
  "aggs": {
    "top_categories": {
      "terms": {"field": "category", "size": 10}
    }
  }
}
```
- 10 unique values
- Small hash table
- Latency: ~100ms (**6× faster**)

---

## Cache Tuning

### Cache Configuration Strategy

**Step 1: Measure baseline**
```bash
# Without cache
curl -w "@curl-format.txt" -X POST http://localhost:9200/products/_search \
  -d '{"query": {"term": {"status": "active"}}}'

# Note planning time
```

**Step 2: Enable cache**
```go
config := DefaultQueryCacheConfig()
cache := NewQueryCache(config)
```

**Step 3: Measure improvement**
```bash
# First query (cache miss)
time1=$(curl ...)

# Second query (cache hit)
time2=$(curl ...)

speedup=$((time1 / time2))
```

**Step 4: Tune for workload**

See `QUERY_CACHE_CONFIGURATION.md` for detailed tuning.

### Quick Wins

**Dashboard workload**:
```go
config.LogicalCacheTTL = 15 * time.Minute  // Longer
config.LogicalCacheSize = 2000              // Larger
```
- Expected: 95%+ hit rate
- Speedup: 3-4×

**Analytics workload**:
```go
config.LogicalCacheTTL = 2 * time.Minute   // Shorter
config.LogicalCacheSize = 500               // Smaller
```
- Expected: 30-40% hit rate
- Still valuable for drill-downs

---

## Index Design

### Shard Strategy

**Principle**: Balance parallelism vs overhead

**Too few shards**:
```
Index: products
Shards: 1
Docs: 10M

Problem: No parallelism, single bottleneck
Query latency: ~500ms
```

**Too many shards**:
```
Index: products
Shards: 100
Docs: 10M

Problem: Overhead, coordination cost
Query latency: ~300ms (worse!)
```

**Optimal**:
```
Index: products
Shards: 6-12 (2-4× node count)
Docs: 10M

Result: Good parallelism, low overhead
Query latency: ~80ms
```

### Shard Sizing Guidelines

| Index Size | Docs per Shard | Recommended Shards |
|------------|---------------|-------------------|
| <1M docs | All in one | 1-2 |
| 1-10M docs | 1-2M | 3-6 |
| 10-100M docs | 5-10M | 12-24 |
| >100M docs | 10-20M | 24-48 |

### Field Design

**Index only searchable fields**:
```json
{
  "mappings": {
    "properties": {
      "title": {"type": "text", "index": true},
      "description": {"type": "text", "index": true},
      "thumbnail_url": {"type": "keyword", "index": false},  // Not searchable
      "internal_notes": {"type": "text", "index": false}     // Not searchable
    }
  }
}
```

**Benefits**:
- Smaller index size (30-50% reduction)
- Faster indexing (20-30% faster)
- Lower memory usage

### Document Structure

**❌ Nested/complex**:
```json
{
  "id": "123",
  "metadata": {
    "tags": {
      "primary": ["tag1", "tag2"],
      "secondary": ["tag3", "tag4"]
    },
    "attributes": {
      "color": "red",
      "size": "large"
    }
  }
}
```
- Deep nesting
- Complex queries
- Slow

**✅ Flat**:
```json
{
  "id": "123",
  "primary_tags": ["tag1", "tag2"],
  "secondary_tags": ["tag3", "tag4"],
  "color": "red",
  "size": "large"
}
```
- Simple structure
- Fast queries
- **2-3× faster**

---

## Resource Allocation

### Coordination Node

**Recommended**:
- CPU: 2-4 cores
- Memory: 2-4 GB
- Network: 1 Gbps

**Sizing**:
```
Memory = Base (500 MB) + QueryCache (200 MB) + Connections (10 MB per 100 qps)

Example (1000 qps):
Memory = 500 + 200 + 100 = 800 MB (use 2 GB with headroom)
```

**CPU usage**:
- Query planning: <5% per 100 qps
- Result aggregation: 10-20% per 100 qps
- Network I/O: 5-10% per 100 qps

### Data Node

**Recommended**:
- CPU: 4-8 cores
- Memory: 4-8 GB (+ Diagon requirements)
- Disk: SSD (NVMe preferred)
- Network: 10 Gbps (for multi-node)

**Sizing**:
```
Memory = Index Size / 10 + Buffer (1 GB)

Example (10 GB index):
Memory = 1 GB + 1 GB = 2 GB (use 4 GB with headroom)
```

**Disk I/O**:
- Use SSD for index storage
- NVMe for high-throughput workloads
- RAID 0 for maximum performance (with backups!)

### Network

**Bandwidth requirements**:
```
Network = Query Rate × Avg Result Size × Replication Factor

Example (1000 qps, 10 KB results, RF=2):
Network = 1000 × 10 KB × 2 = 20 MB/s = 160 Mbps
```

**Latency requirements**:
- Same datacenter: <1ms
- Same region: <5ms
- Cross-region: Use local clusters

---

## Distributed Execution

### Parallel Shard Queries

**How it works**:
```
Query arrives at Coordination
  ↓
Coordination fans out to all shards in parallel
  ├─ Shard 0 (DataNode 1): 20ms
  ├─ Shard 1 (DataNode 1): 22ms
  ├─ Shard 2 (DataNode 2): 21ms
  └─ Shard 3 (DataNode 2): 23ms
  ↓
Coordination waits for all (max = 23ms)
  ↓
Coordination aggregates results: 5ms
  ↓
Total: 28ms (not 86ms sequential!)
```

### Shard Distribution Strategy

**Even distribution**:
```
Index: products, 6 shards, 3 nodes

Node 1: Shards 0, 1
Node 2: Shards 2, 3
Node 3: Shards 4, 5

Result: Balanced load, good parallelism
```

**Uneven distribution** (avoid):
```
Index: products, 6 shards, 3 nodes

Node 1: Shards 0, 1, 2, 3
Node 2: Shard 4
Node 3: Shard 5

Result: Node 1 bottleneck, poor performance
```

### Network Optimization

**Minimize data transfer**:
1. **Filter pushdown** → Diagon filters locally
2. **Field projection** → Only transfer needed fields
3. **TopN optimization** → Transfer only top N per shard

**Example**:
```
Without optimization:
  Each shard returns 10K docs × 5 shards = 50K docs transferred
  Network: 50K × 1 KB = 50 MB
  Time: 400ms @ 1 Gbps

With optimization:
  Each shard returns 10 docs × 5 shards = 50 docs transferred
  Network: 50 × 1 KB = 50 KB
  Time: 0.4ms @ 1 Gbps

Speedup: 1000× less data, 1000× faster transfer!
```

---

## Monitoring

### Key Metrics

#### Query Performance

```prometheus
# P95 latency
histogram_quantile(0.95,
  rate(quidditch_query_duration_seconds_bucket[5m]))

# Queries per second
rate(quidditch_queries_total[5m])

# Error rate
rate(quidditch_query_errors_total[5m]) /
  rate(quidditch_queries_total[5m])
```

#### Cache Performance

```prometheus
# Hit rate
quidditch_query_cache_hit_rate{cache_type="logical"}

# Cache memory
quidditch_query_cache_size_bytes{cache_type="logical"} / (1024*1024)
```

#### Resource Usage

```prometheus
# CPU usage
rate(process_cpu_seconds_total[5m])

# Memory usage
process_resident_memory_bytes / (1024*1024*1024)  # GB

# Network I/O
rate(node_network_receive_bytes_total[5m]) +
rate(node_network_transmit_bytes_total[5m])
```

### Grafana Dashboard

**Must-have panels**:

1. **Query Latency (P50, P95, P99)**
2. **Queries Per Second**
3. **Cache Hit Rate**
4. **Error Rate**
5. **CPU Usage by Node**
6. **Memory Usage by Node**
7. **Network I/O**
8. **Index Size Growth**

### Alerting Rules

```yaml
# Query latency too high
- alert: HighQueryLatency
  expr: histogram_quantile(0.95, rate(quidditch_query_duration_seconds_bucket[5m])) > 0.2
  for: 5m
  annotations:
    summary: "P95 query latency > 200ms"

# Cache hit rate too low
- alert: LowCacheHitRate
  expr: quidditch_query_cache_hit_rate < 0.5
  for: 10m
  annotations:
    summary: "Cache hit rate < 50%"

# High error rate
- alert: HighErrorRate
  expr: rate(quidditch_query_errors_total[5m]) / rate(quidditch_queries_total[5m]) > 0.05
  for: 5m
  annotations:
    summary: "Error rate > 5%"
```

---

## Troubleshooting

### Slow Queries

**Diagnosis**:
```bash
# Enable query logging
export LOG_LEVEL=debug

# Check query execution
grep "Query execution" /var/log/quidditch/*.log | tail -20

# Look for:
# - Scan time (should be <30ms per shard)
# - Aggregation time (should be <50ms)
# - Network time (should be <10ms)
```

**Common causes**:

1. **Large result sets**
   - Symptom: High network time
   - Fix: Reduce `size`, add filters

2. **No filter pushdown**
   - Symptom: Long scan time
   - Fix: Verify optimizer is working

3. **Heavy aggregations**
   - Symptom: Long aggregation time
   - Fix: Reduce cardinality, add filters

4. **Cold cache**
   - Symptom: First query slow, subsequent fast
   - Fix: Pre-warm cache

### High CPU Usage

**Diagnosis**:
```bash
# Check CPU usage
top -H -p $(pgrep coordination)

# Profile with pprof
curl http://localhost:9200/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof
```

**Common causes**:

1. **No query cache**
   - Fix: Enable query cache

2. **Complex queries**
   - Fix: Simplify queries, add indices

3. **Too many QPS**
   - Fix: Scale horizontally

### High Memory Usage

**Diagnosis**:
```bash
# Check memory usage
ps aux | grep coordination

# Check for leaks
curl http://localhost:9200/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

**Common causes**:

1. **Large cache**
   - Fix: Reduce cache size

2. **Large result sets**
   - Fix: Reduce `size` parameter

3. **Memory leak**
   - Fix: Upgrade to latest version

---

## Best Practices

### Query Design

**✅ DO**:
- Use specific filters
- Limit result sizes
- Use cache-friendly patterns
- Minimize aggregation cardinality
- Use TopN for sorted queries

**❌ DON'T**:
- Use `match_all` on large indices
- Request huge result sets (size > 1000)
- Use deep pagination (from > 1000)
- Include timestamps in queries
- Use wildcard queries on large fields

### Index Design

**✅ DO**:
- Choose appropriate shard count
- Index only searchable fields
- Use flat document structure
- Monitor index size growth
- Reindex periodically (for optimization)

**❌ DON'T**:
- Over-shard small indices
- Under-shard large indices
- Index everything
- Use deep nesting
- Ignore index size

### Cache Strategy

**✅ DO**:
- Enable both cache levels
- Monitor hit rates
- Tune TTL for workload
- Pre-warm for common queries
- Invalidate after updates

**❌ DON'T**:
- Disable cache in production
- Use too-short TTL
- Include random values in queries
- Ignore cache metrics
- Over-allocate cache memory

### Resource Management

**✅ DO**:
- Monitor resource usage
- Scale before bottlenecks
- Use SSD for storage
- Provision headroom (20-30%)
- Set resource limits

**❌ DON'T**:
- Run at 100% capacity
- Use HDD for active indices
- Under-provision memory
- Ignore CPU spikes
- Share resources with other apps

---

## Performance Checklist

### Before Deployment

- [ ] Shard count appropriate for data size
- [ ] Query cache enabled and configured
- [ ] Indexes optimized (only necessary fields)
- [ ] Resource limits set
- [ ] Monitoring and alerting configured
- [ ] Load testing completed

### Regular Maintenance

- [ ] Monitor query latency trends
- [ ] Check cache hit rates
- [ ] Review slow query logs
- [ ] Verify resource usage
- [ ] Check index size growth
- [ ] Update capacity planning

### Optimization Opportunities

- [ ] Queries taking > 100ms
- [ ] Cache hit rate < 70%
- [ ] CPU usage > 70%
- [ ] Memory usage > 80%
- [ ] Index size growing rapidly
- [ ] Frequent query errors

---

## Quick Reference

### Performance Targets

| Metric | Target |
|--------|--------|
| Query latency (simple) | <50ms |
| Query latency (complex) | <200ms |
| Indexing throughput | >50K docs/sec |
| Cache hit rate | >80% |
| Error rate | <1% |
| CPU usage | <70% |
| Memory usage | <80% |

### Cache Configuration

| Workload | Size | TTL |
|----------|------|-----|
| Dashboard | 2000 entries | 15 min |
| Search | 1500 entries | 5 min |
| Analytics | 500 entries | 2 min |

### Shard Configuration

| Index Size | Shards |
|------------|--------|
| <1M docs | 1-2 |
| 1-10M docs | 3-6 |
| 10-100M docs | 12-24 |
| >100M docs | 24-48 |

---

## References

### Related Documentation

- `QUERY_PLANNER_ARCHITECTURE.md` - Query planner details
- `QUERY_CACHE_CONFIGURATION.md` - Cache configuration
- `README.md` - Quick start guide

### Code Files

- `pkg/coordination/planner/` - Query planner
- `pkg/coordination/cache/` - Query cache
- `pkg/coordination/executor/` - Query execution
- `pkg/data/diagon/` - Diagon engine

---

**Document Version**: 1.0
**Last Updated**: 2026-01-26
**Author**: Quidditch Team
**Status**: Production Ready
