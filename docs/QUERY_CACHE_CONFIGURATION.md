# Query Cache Configuration Guide

**Version**: 1.0
**Date**: 2026-01-26
**Status**: Production Ready

---

## Table of Contents

1. [Overview](#overview)
2. [Cache Architecture](#cache-architecture)
3. [Configuration Options](#configuration-options)
4. [Workload-Specific Tuning](#workload-specific-tuning)
5. [Monitoring and Metrics](#monitoring-and-metrics)
6. [Best Practices](#best-practices)
7. [Troubleshooting](#troubleshooting)

---

## Overview

Quidditch's query cache is a two-level caching system that stores optimized logical plans and cost-optimized physical plans. It provides **2-4× performance improvements** for repeated queries.

### Key Benefits

- **2-4× faster** query planning for cache hits
- **60% CPU savings** for cached queries
- **80-95% hit rates** for typical production workloads
- **Zero code changes** required (transparent caching)

### Cache Levels

```
Level 1: Logical Plan Cache
  ├─ Stores: Optimized logical plans
  ├─ Key: hash(index + query + shards)
  ├─ Saves: AST conversion + optimization (0.3-0.5ms)
  └─ Default: 1000 entries, 100MB, 5min TTL

Level 2: Physical Plan Cache
  ├─ Stores: Cost-optimized physical plans
  ├─ Key: hash(logical plan)
  ├─ Saves: Physical planning + costing (0.2-0.3ms)
  └─ Default: 500 entries, 50MB, 5min TTL
```

---

## Cache Architecture

### Cache Implementation

**Technology**: In-memory LRU (Least Recently Used) cache with TTL

**Key Features**:
- **Thread-safe**: Read/write locks for concurrent access
- **TTL-based expiration**: Auto-remove stale entries
- **Size limits**: Both count-based and byte-based limits
- **Statistics**: Hit/miss rates, sizes, eviction counts
- **Prometheus integration**: Real-time metrics

### Cache Flow

```
Query arrives
  ↓
[Logical Plan Cache]
  ├─ HIT: Return cached logical plan → Skip to Physical Cache
  └─ MISS: Parse → Convert → Optimize → Cache result
       ↓
[Physical Plan Cache]
  ├─ HIT: Return cached physical plan → Execute
  └─ MISS: Generate physical plan → Cache result → Execute
```

### Cache Key Generation

**Logical Plan Key**:
```go
// Components
- Index name
- Normalized query (canonical form)
- Shard IDs (sorted)

// Example
key = "products:term:status=active:[0,1,2]"
hash = SHA256(key) = "a3f2c1..."
```

**Physical Plan Key**:
```go
// Components
- Canonical logical plan string

// Example
key = "Scan(products, filter=status=active, shards=[0,1,2])"
hash = SHA256(key) = "b8e4d2..."
```

---

## Configuration Options

### Default Configuration

```go
// Default config (suitable for most workloads)
config := &QueryCacheConfig{
    // Logical plan cache
    LogicalCacheSize:      1000,              // Max 1000 entries
    LogicalCacheMaxBytes:  100 * 1024 * 1024, // Max 100 MB
    LogicalCacheTTL:       5 * time.Minute,   // 5 minute expiration

    // Physical plan cache
    PhysicalCacheSize:     500,               // Max 500 entries
    PhysicalCacheMaxBytes: 50 * 1024 * 1024,  // Max 50 MB
    PhysicalCacheTTL:      5 * time.Minute,   // 5 minute expiration

    // Feature flags
    EnableLogical:         true,              // Enable logical cache
    EnablePhysical:        true,              // Enable physical cache
}
```

### Configuration Parameters

#### LogicalCacheSize

**Type**: `int`
**Default**: `1000`
**Range**: `10 - 10000`

**Description**: Maximum number of logical plans to cache

**Tuning**:
- **Small** (100-500): Memory-constrained environments
- **Medium** (1000-2000): Standard production
- **Large** (2000-5000): High-traffic dashboards

**Memory Impact**: ~100KB per entry (varies by query complexity)

#### LogicalCacheMaxBytes

**Type**: `int64` (bytes)
**Default**: `100 MB` (104,857,600 bytes)
**Range**: `10 MB - 1 GB`

**Description**: Maximum memory for logical plan cache

**Tuning**:
- **Small** (10-50 MB): Tight memory budgets
- **Medium** (100-200 MB): Standard production
- **Large** (200 MB - 1 GB): Large-scale deployments

**When to increase**:
- High cache eviction rates
- Complex queries (nested bools, many aggregations)
- Large number of indices

#### LogicalCacheTTL

**Type**: `time.Duration`
**Default**: `5 * time.Minute`
**Range**: `30 seconds - 30 minutes`

**Description**: How long to keep cached logical plans

**Tuning**:
- **Short** (1-2 min): Rapidly changing indices
- **Medium** (5-10 min): Standard production
- **Long** (15-30 min): Static reference data

**Trade-offs**:
- Longer TTL = Higher hit rate, stale plans possible
- Shorter TTL = Always fresh, more cache misses

#### PhysicalCacheSize

**Type**: `int`
**Default**: `500`
**Range**: `10 - 5000`

**Description**: Maximum number of physical plans to cache

**Tuning**: Generally half of LogicalCacheSize

#### PhysicalCacheMaxBytes

**Type**: `int64` (bytes)
**Default**: `50 MB`
**Range**: `5 MB - 500 MB`

**Description**: Maximum memory for physical plan cache

**Tuning**: Generally half of LogicalCacheMaxBytes

#### PhysicalCacheTTL

**Type**: `time.Duration`
**Default**: `5 * time.Minute`
**Range**: `30 seconds - 30 minutes`

**Description**: How long to keep cached physical plans

**Tuning**: Same as LogicalCacheTTL

#### EnableLogical / EnablePhysical

**Type**: `bool`
**Default**: `true`

**Description**: Enable/disable each cache level

**Use cases for disabling**:
- Testing (verify without cache)
- Debugging (isolate cache issues)
- Memory-constrained (disable one level)

---

## Workload-Specific Tuning

### Dashboard Workload

**Characteristics**:
- Same queries repeated every 5-30 seconds
- Small number of unique queries (10-50)
- Very high repetition rate

**Configuration**:
```go
config := &QueryCacheConfig{
    LogicalCacheSize:      2000,
    LogicalCacheMaxBytes:  200 * 1024 * 1024,  // 200 MB
    LogicalCacheTTL:       15 * time.Minute,   // Longer TTL

    PhysicalCacheSize:     1000,
    PhysicalCacheMaxBytes: 100 * 1024 * 1024,  // 100 MB
    PhysicalCacheTTL:      15 * time.Minute,

    EnableLogical:         true,
    EnablePhysical:        true,
}
```

**Expected Performance**:
- Hit rate: 95-99%
- Query planning: <0.2ms (vs 0.8ms without cache)
- Speedup: 4× faster

### Analytics Workload

**Characteristics**:
- Ad-hoc queries with high variability
- Low repetition rate
- Complex aggregations

**Configuration**:
```go
config := &QueryCacheConfig{
    LogicalCacheSize:      500,                // Fewer entries
    LogicalCacheMaxBytes:  100 * 1024 * 1024,
    LogicalCacheTTL:       2 * time.Minute,    // Shorter TTL

    PhysicalCacheSize:     250,
    PhysicalCacheMaxBytes: 50 * 1024 * 1024,
    PhysicalCacheTTL:      2 * time.Minute,

    EnableLogical:         true,
    EnablePhysical:        true,
}
```

**Expected Performance**:
- Hit rate: 20-40%
- Still valuable for pagination and drill-downs

### Search/Discovery Workload

**Characteristics**:
- User search queries
- Medium repetition (popular searches)
- Real-time requirements

**Configuration**:
```go
config := &QueryCacheConfig{
    LogicalCacheSize:      1500,
    LogicalCacheMaxBytes:  150 * 1024 * 1024,
    LogicalCacheTTL:       5 * time.Minute,

    PhysicalCacheSize:     750,
    PhysicalCacheMaxBytes: 75 * 1024 * 1024,
    PhysicalCacheTTL:      5 * time.Minute,

    EnableLogical:         true,
    EnablePhysical:        true,
}
```

**Expected Performance**:
- Hit rate: 60-80%
- Common searches cached
- Long tail of unique searches uncached

### Memory-Constrained Environment

**Characteristics**:
- Limited RAM (< 1 GB for coordination node)
- Need to balance cache vs other operations

**Configuration**:
```go
config := &QueryCacheConfig{
    LogicalCacheSize:      300,
    LogicalCacheMaxBytes:  30 * 1024 * 1024,   // 30 MB
    LogicalCacheTTL:       3 * time.Minute,

    PhysicalCacheSize:     150,
    PhysicalCacheMaxBytes: 15 * 1024 * 1024,   // 15 MB
    PhysicalCacheTTL:      3 * time.Minute,

    EnableLogical:         true,
    EnablePhysical:        true,
}
```

**Expected Performance**:
- Hit rate: 50-70% (limited by size)
- Total cache memory: ~45 MB
- Still provides benefit for repeated queries

### High-Traffic Production

**Characteristics**:
- Thousands of queries per second
- Mix of dashboard and ad-hoc queries
- Need maximum performance

**Configuration**:
```go
config := &QueryCacheConfig{
    LogicalCacheSize:      5000,               // Large
    LogicalCacheMaxBytes:  500 * 1024 * 1024,  // 500 MB
    LogicalCacheTTL:       10 * time.Minute,

    PhysicalCacheSize:     2500,
    PhysicalCacheMaxBytes: 250 * 1024 * 1024,  // 250 MB
    PhysicalCacheTTL:      10 * time.Minute,

    EnableLogical:         true,
    EnablePhysical:        true,
}
```

**Expected Performance**:
- Hit rate: 85-95%
- Can handle 1000+ unique queries
- Total cache memory: ~750 MB

---

## Monitoring and Metrics

### Prometheus Metrics

#### Cache Hit/Miss Counters

```prometheus
# Logical cache
quidditch_query_cache_hits_total{cache_type="logical", index="products"}
quidditch_query_cache_misses_total{cache_type="logical", index="products"}

# Physical cache
quidditch_query_cache_hits_total{cache_type="physical", index="products"}
quidditch_query_cache_misses_total{cache_type="physical", index="products"}
```

**Usage**:
```promql
# Hit rate (last 5 minutes)
sum(rate(quidditch_query_cache_hits_total[5m])) /
  (sum(rate(quidditch_query_cache_hits_total[5m])) +
   sum(rate(quidditch_query_cache_misses_total[5m])))
```

#### Cache Size Gauges

```prometheus
# Entry count
quidditch_query_cache_size{cache_type="logical"}
quidditch_query_cache_size{cache_type="physical"}

# Memory usage
quidditch_query_cache_size_bytes{cache_type="logical"}
quidditch_query_cache_size_bytes{cache_type="physical"}
```

**Usage**:
```promql
# Memory usage
quidditch_query_cache_size_bytes{cache_type="logical"} / (1024*1024)  # MB
```

#### Cache Hit Rate Gauge

```prometheus
quidditch_query_cache_hit_rate{cache_type="logical"}
quidditch_query_cache_hit_rate{cache_type="physical"}
```

**Values**: 0.0 (0%) to 1.0 (100%)

### Grafana Dashboard

**Recommended panels**:

1. **Cache Hit Rate** (Time series)
   ```promql
   quidditch_query_cache_hit_rate
   ```

2. **Cache Hits vs Misses** (Stacked area)
   ```promql
   sum by (cache_type) (rate(quidditch_query_cache_hits_total[5m]))
   sum by (cache_type) (rate(quidditch_query_cache_misses_total[5m]))
   ```

3. **Cache Size** (Gauge)
   ```promql
   quidditch_query_cache_size
   ```

4. **Memory Usage** (Gauge)
   ```promql
   quidditch_query_cache_size_bytes / (1024*1024)
   ```

5. **Query Planning Time** (Histogram)
   ```promql
   histogram_quantile(0.95,
     rate(quidditch_query_planning_duration_seconds_bucket[5m]))
   ```

### Log Analysis

**Cache hits** (DEBUG level):
```
[DEBUG] Logical plan cache HIT for index=products
[DEBUG] Physical plan cache HIT
[DEBUG] Query planning time: 0.22ms (cached)
```

**Cache misses** (DEBUG level):
```
[DEBUG] Logical plan cache MISS for index=products
[DEBUG] Converting AST to logical plan
[DEBUG] Optimizing logical plan (7 rules)
[DEBUG] Caching logical plan
[DEBUG] Physical plan cache MISS
[DEBUG] Generating physical plan
[DEBUG] Caching physical plan
[DEBUG] Query planning time: 0.85ms
```

### Key Performance Indicators

| Metric | Target | Good | Poor |
|--------|--------|------|------|
| Logical cache hit rate | >80% | >70% | <50% |
| Physical cache hit rate | >80% | >70% | <50% |
| Cache memory usage | <80% of limit | <90% | >95% |
| Planning time (cached) | <0.3ms | <0.5ms | >1ms |
| Planning time (uncached) | <1ms | <1.5ms | >2ms |

---

## Best Practices

### Cache Key Optimization

**✅ DO**:
- Use consistent query formats
- Sort array parameters (shards, fields)
- Normalize whitespace and formatting
- Use canonical field ordering

**❌ DON'T**:
- Include timestamps in queries
- Use random sort parameters
- Change query structure for same logic
- Add cache-busting parameters

### Query Normalization

**Example**:
```javascript
// These queries produce the same cache key ✅
Query 1: {"term": {"status": "active"}}
Query 2: {"term": {"status":"active"}}
Query 3: {"term":{"status": "active"}}

// These produce different cache keys ❌
Query 4: {"term": {"status": "active"}, "timestamp": "2026-01-26"}
Query 5: {"term": {"status": "inactive"}}
```

### Index Design for Caching

**✅ DO**:
- Group related queries to same index
- Use consistent shard counts
- Design for cache-friendly access patterns

**❌ DON'T**:
- Frequently reindex (invalidates cache)
- Use wildcard index patterns
- Change shard assignments

### Cache Warming

**Pre-warm cache** for common queries:

```bash
# Script to warm cache
for query in common_queries.json; do
    curl -X POST http://localhost:9200/products/_search \
      -H 'Content-Type: application/json' \
      -d @"$query"
done
```

**Benefits**:
- Faster response for first users
- Predictable performance
- Reduced cold-start latency

### TTL Management

**Longer TTL when**:
- Stable reference data (products, categories)
- Infrequent updates
- Dashboard workloads

**Shorter TTL when**:
- Frequently updated indices
- Real-time requirements
- Analytics workloads

### Memory Management

**Monitor memory usage**:
```bash
# Check coordination node memory
ps aux | grep coordination

# Check cache metrics
curl http://localhost:9200/metrics | grep cache_size_bytes
```

**Adjust if**:
- Memory usage > 80% of limit
- Frequent cache evictions
- OOM errors

---

## Troubleshooting

### Problem: Low Cache Hit Rate (<50%)

**Symptoms**:
- `quidditch_query_cache_hit_rate` < 0.5
- No performance improvement from cache

**Diagnosis**:
```bash
# Check hit/miss rates
curl http://localhost:9200/metrics | grep cache_hits
curl http://localhost:9200/metrics | grep cache_misses

# Check query variability in logs
grep "cache MISS" /var/log/quidditch/coordination.log | wc -l
```

**Possible Causes**:

1. **High query variability**
   - Each query is unique
   - Solution: Normalize queries, remove timestamps

2. **TTL too short**
   - Cache expires before reuse
   - Solution: Increase `LogicalCacheTTL` and `PhysicalCacheTTL`

3. **Cache too small**
   - Frequent evictions
   - Solution: Increase `LogicalCacheSize` and `PhysicalCacheSize`

4. **Cache disabled**
   - Check `EnableLogical` and `EnablePhysical`
   - Solution: Enable both caches

### Problem: High Memory Usage

**Symptoms**:
- Coordination node using > 2GB RAM
- OOM errors
- Slow query planning

**Diagnosis**:
```bash
# Check cache memory usage
curl http://localhost:9200/metrics | grep cache_size_bytes

# Expected: logical ~100MB, physical ~50MB
```

**Solutions**:

1. **Reduce cache sizes**:
   ```go
   config.LogicalCacheMaxBytes = 50 * 1024 * 1024   // 50 MB
   config.PhysicalCacheMaxBytes = 25 * 1024 * 1024  // 25 MB
   ```

2. **Reduce entry counts**:
   ```go
   config.LogicalCacheSize = 500
   config.PhysicalCacheSize = 250
   ```

3. **Shorter TTL** (more aggressive eviction):
   ```go
   config.LogicalCacheTTL = 2 * time.Minute
   ```

4. **Disable physical cache** (save ~50%):
   ```go
   config.EnablePhysical = false
   ```

### Problem: Stale Query Results

**Symptoms**:
- Query returns old data
- Index updated but query returns previous results

**Diagnosis**:
```bash
# Check TTL settings
# Check last index update time
# Check cache timestamps in logs
```

**Solutions**:

1. **Shorter TTL**:
   ```go
   config.LogicalCacheTTL = 1 * time.Minute
   ```

2. **Manual invalidation after updates**:
   ```go
   // After index update
   cache.InvalidateIndex("products")
   ```

3. **Disable cache for real-time indices**:
   ```go
   // For specific index
   if index == "realtime_events" {
       return false  // Don't cache
   }
   ```

### Problem: Cache Not Working

**Symptoms**:
- Hit rate always 0%
- All queries show "cache MISS"

**Diagnosis**:
```bash
# Check cache is enabled
curl http://localhost:9200/metrics | grep cache_hits

# Check logs
grep "cache" /var/log/quidditch/coordination.log
```

**Solutions**:

1. **Verify cache is enabled**:
   ```go
   config.EnableLogical = true
   config.EnablePhysical = true
   ```

2. **Check cache initialization**:
   ```go
   // Verify cache is passed to QueryService
   queryService := NewQueryService(..., cache, ...)
   ```

3. **Verify metrics are working**:
   ```bash
   curl http://localhost:9200/metrics
   ```

### Problem: Inconsistent Hit Rates

**Symptoms**:
- Hit rate fluctuates wildly
- Sometimes 90%, sometimes 10%

**Diagnosis**:
```bash
# Check for bursty query patterns
# Monitor over longer period (hours, not minutes)
```

**Solutions**:

1. **Increase cache size** for burst capacity:
   ```go
   config.LogicalCacheSize = 2000  // Was 1000
   ```

2. **Longer TTL** for stability:
   ```go
   config.LogicalCacheTTL = 10 * time.Minute  // Was 5
   ```

3. **Pre-warm cache** before traffic spikes

---

## Configuration Examples

### Minimal Configuration (100 MB total)

```go
config := &QueryCacheConfig{
    LogicalCacheSize:      500,
    LogicalCacheMaxBytes:  50 * 1024 * 1024,
    LogicalCacheTTL:       3 * time.Minute,

    PhysicalCacheSize:     250,
    PhysicalCacheMaxBytes: 25 * 1024 * 1024,
    PhysicalCacheTTL:      3 * time.Minute,

    EnableLogical:         true,
    EnablePhysical:        true,
}
```

### Standard Configuration (200 MB total)

```go
config := &QueryCacheConfig{
    LogicalCacheSize:      1000,
    LogicalCacheMaxBytes:  100 * 1024 * 1024,
    LogicalCacheTTL:       5 * time.Minute,

    PhysicalCacheSize:     500,
    PhysicalCacheMaxBytes: 50 * 1024 * 1024,
    PhysicalCacheTTL:      5 * time.Minute,

    EnableLogical:         true,
    EnablePhysical:        true,
}
```

### Large Configuration (1 GB total)

```go
config := &QueryCacheConfig{
    LogicalCacheSize:      5000,
    LogicalCacheMaxBytes:  500 * 1024 * 1024,
    LogicalCacheTTL:       10 * time.Minute,

    PhysicalCacheSize:     2500,
    PhysicalCacheMaxBytes: 250 * 1024 * 1024,
    PhysicalCacheTTL:      10 * time.Minute,

    EnableLogical:         true,
    EnablePhysical:        true,
}
```

### Logical-Only Configuration

```go
config := &QueryCacheConfig{
    LogicalCacheSize:      2000,
    LogicalCacheMaxBytes:  200 * 1024 * 1024,
    LogicalCacheTTL:       5 * time.Minute,

    EnableLogical:         true,
    EnablePhysical:        false,  // Disabled
}
```

---

## Migration Guide

### From No Cache to Cached

**Step 1**: Add cache to QueryService
```go
// Create cache
cacheConfig := DefaultQueryCacheConfig()
cache := NewQueryCache(cacheConfig)

// Pass to QueryService
queryService := NewQueryService(
    masterClient,
    parser,
    planner,
    executor,
    cache,  // Add cache
    logger,
)
```

**Step 2**: Monitor performance
```bash
# Watch hit rate
watch -n 5 'curl -s http://localhost:9200/metrics | grep hit_rate'
```

**Step 3**: Tune configuration based on hit rate

### Upgrading Cache Size

**Step 1**: Update configuration
```go
config.LogicalCacheSize = 2000  // Was 1000
config.LogicalCacheMaxBytes = 200 * 1024 * 1024  // Was 100MB
```

**Step 2**: Restart coordination node
```bash
systemctl restart quidditch-coordination
```

**Step 3**: Monitor memory usage
```bash
watch -n 5 'curl -s http://localhost:9200/metrics | grep cache_size_bytes'
```

---

## References

### Related Documentation

- `QUERY_PLANNER_ARCHITECTURE.md` - Query planner overview
- `PERFORMANCE_TUNING.md` - Performance optimization
- `PHASE2_QUERY_CACHE_COMPLETE.md` - Cache implementation details

### Code Files

- `pkg/coordination/cache/lru_cache.go` - LRU cache implementation
- `pkg/coordination/cache/query_cache.go` - Query cache layer
- `pkg/coordination/query_service.go` - Cache integration

---

**Document Version**: 1.0
**Last Updated**: 2026-01-26
**Author**: Quidditch Team
**Status**: Production Ready
