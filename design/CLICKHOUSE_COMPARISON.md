# ClickHouse vs Quidditch: Comprehensive Comparison

**Date**: 2026-01-25
**Purpose**: Learn from ClickHouse's architecture to identify gaps and enhancements for Quidditch
**Status**: Analysis Complete

---

## Executive Summary

**ClickHouse** is a high-performance columnar OLAP database optimized for analytical queries. Since Quidditch uses ClickHouse-inspired columnar storage (forward index), we should learn from their architecture.

**Key Findings**:
- ✅ **Already Included**: Columnar storage, compression, SIMD, skip indexes
- 🔶 **Should Add**: Materialized views, streaming ingestion, merge tree architecture
- ⚠️ **Consider**: Dictionaries, JOINs optimization, distributed query execution

---

## Table of Contents

1. [High-Level Comparison](#1-high-level-comparison)
2. [Storage Engine Architecture](#2-storage-engine-architecture)
3. [Query Processing & Execution](#3-query-processing--execution)
4. [UDF Support](#4-udf-support)
5. [Distributed Architecture](#5-distributed-architecture)
6. [Data Ingestion & Streaming](#6-data-ingestion--streaming)
7. [Compression & Encoding](#7-compression--encoding)
8. [Vectorization & SIMD](#8-vectorization--simd)
9. [Indexes & Data Skipping](#9-indexes--data-skipping)
10. [Materialized Views](#10-materialized-views)
11. [Replication & High Availability](#11-replication--high-availability)
12. [What We Should Add](#12-what-we-should-add)
13. [What We Can Skip](#13-what-we-can-skip)
14. [Implementation Priorities](#14-implementation-priorities)

---

## 1. High-Level Comparison

### Architecture Overview

| Aspect | ClickHouse | Quidditch | Gap Analysis |
|--------|-----------|-----------|--------------|
| **Primary Use Case** | OLAP analytics | Full-text search + analytics | Different focus |
| **Query Language** | SQL (ClickHouse dialect) | OpenSearch DSL + PPL | Different API |
| **Storage Engine** | MergeTree family | Inverted + Forward (columnar) | ✅ Similar approach |
| **Distributed** | Yes (shards + replicas) | Yes (shards + replicas) | ✅ Similar |
| **Node Types** | All nodes equal | Master, Coordination, Data | ✅ More specialized |
| **Real-time** | Near real-time | Real-time search | ✅ Better for search |
| **Compression** | Excellent (3-10×) | Planned (3-5×) | 🔶 Learn from CH |
| **SIMD** | Extensive | Planned for BM25 | 🔶 Expand to aggregations |
| **Vectorization** | Full columnar vectorization | Planned | 🔶 Critical for analytics |

---

## 2. Storage Engine Architecture

### ClickHouse: MergeTree Family

**MergeTree** is ClickHouse's flagship storage engine:

```
MergeTree Architecture:
┌─────────────────────────────────────────┐
│  Table                                   │
├─────────────────────────────────────────┤
│  Part 1 (2023-01-01 to 2023-01-07)     │
│    ├─ primary.idx (sparse index)        │
│    ├─ data.bin (columnar data)         │
│    ├─ marks.mrk (granule pointers)     │
│    └─ checksums.txt                     │
│                                          │
│  Part 2 (2023-01-08 to 2023-01-14)     │
│    ├─ primary.idx                       │
│    ├─ data.bin                          │
│    ├─ marks.mrk                         │
│    └─ checksums.txt                     │
│                                          │
│  [Background merge process]              │
└─────────────────────────────────────────┘
```

**Key Features**:
1. **Data Parts**: Immutable data parts merged in background
2. **Sparse Index**: Primary key index with 8192-row granules
3. **Columnar**: Each column stored separately
4. **Compression**: Per-column compression with codecs
5. **Skip Indexes**: Bloom filters, MinMax, Set, NGram
6. **Partitioning**: By date/key for data management
7. **Background Merges**: Continuously merge small parts

### Quidditch: Inverted + Forward Index

**Current Design**:
```
Quidditch Data Node:
┌─────────────────────────────────────────┐
│  Shard                                   │
├─────────────────────────────────────────┤
│  Inverted Index (Lucene-style)          │
│    ├─ Term Dictionary                   │
│    ├─ Posting Lists (doc IDs)           │
│    ├─ Term Frequencies                  │
│    └─ Positions (for phrase queries)    │
│                                          │
│  Forward Index (Columnar)                │
│    ├─ Column 1 (compressed)             │
│    ├─ Column 2 (compressed)             │
│    └─ Column N (compressed)             │
│                                          │
│  Translog (WAL)                         │
└─────────────────────────────────────────┘
```

### Comparison & Gaps

| Feature | ClickHouse | Quidditch | Gap |
|---------|-----------|-----------|-----|
| **Immutable Parts** | Yes (MergeTree) | No (Lucene segments) | 🔶 Consider for forward index |
| **Background Merges** | Yes, continuous | Yes (Lucene) | ✅ Have it |
| **Sparse Index** | Yes (primary key) | Yes (inverted index) | ✅ Have it |
| **Columnar Storage** | Yes | Yes (forward index) | ✅ Have it |
| **Skip Indexes** | Yes (multiple types) | Planned | 🔶 **MUST ADD** |
| **Partitioning** | Yes (by key/date) | By shard | 🔶 Add time-based partitioning |
| **Column Families** | No | No | ⚪ Not needed |

### **Recommendation**: Add MergeTree-style architecture to Forward Index

**Proposal**: Hybrid approach for Forward Index
```
Forward Index v2 (MergeTree-inspired):
┌─────────────────────────────────────────┐
│  Forward Index (Analytics)               │
├─────────────────────────────────────────┤
│  Immutable Data Parts:                   │
│    Part 1: docs 1-100k (sorted by PK)   │
│    Part 2: docs 100k-200k               │
│    Part 3: docs 200k-300k               │
│                                          │
│  Each Part has:                          │
│    ├─ sparse_index.idx (every 8192 rows)│
│    ├─ column_data/*.bin (compressed)    │
│    ├─ skip_indexes/*.idx (bloom, minmax)│
│    └─ metadata.json                     │
│                                          │
│  Background Merge Scheduler              │
│    → Merge small parts into larger      │
│    → Delete old parts after merge       │
└─────────────────────────────────────────┘

Inverted Index (Search) - Keep as is (Lucene-style)
```

**Benefits**:
- ✅ Better compression (immutable parts)
- ✅ Faster range queries (sorted data)
- ✅ Efficient updates (append new parts, merge later)
- ✅ Skip indexes on columnar data

---

## 3. Query Processing & Execution

### ClickHouse Query Pipeline

```
SQL Query
  ↓
Parser
  ↓
Analyzer (syntax tree)
  ↓
Query Plan (logical)
  ↓
Query Pipeline Builder
  ↓
Execution Pipeline (vectorized)
  ├→ Stage 1: Table Scan (with skip indexes)
  ├→ Stage 2: WHERE filter (vectorized)
  ├→ Stage 3: GROUP BY (hash aggregation)
  ├→ Stage 4: ORDER BY (merge sort)
  └→ Stage 5: LIMIT
  ↓
Results (columnar blocks)
```

**Key Features**:
1. **Vectorized Execution**: Process 8192-row blocks at a time
2. **Skip Indexes**: Prune granules before reading data
3. **Predicate Pushdown**: Push filters down to storage
4. **Lazy Materialization**: Only read needed columns
5. **Hash Aggregations**: Fast GROUP BY with SIMD
6. **Parallel Execution**: Multi-threaded pipeline stages

### Quidditch Query Pipeline

```
DSL/PPL Query
  ↓
Parser (coordination node)
  ↓
Query Planner (Go, custom or DataFusion)
  ↓
Physical Plan
  ↓
Distributed Execution
  ├→ Scatter: Send sub-queries to data nodes
  ├→ Data Node Execution:
  │   ├─ Inverted Index Scan (BM25 scoring)
  │   ├─ Forward Index Scan (aggregations)
  │   └─ WASM UDF execution
  └→ Gather: Aggregate results
  ↓
Results (JSON)
```

### Comparison & Gaps

| Feature | ClickHouse | Quidditch | Gap |
|---------|-----------|-----------|-----|
| **Vectorized Execution** | Yes (8192 rows) | Planned | 🔶 **CRITICAL** for analytics |
| **Skip Indexes** | Yes (multiple) | Planned | 🔶 **MUST ADD** |
| **Predicate Pushdown** | Yes | Yes | ✅ Have it |
| **Lazy Materialization** | Yes | Planned | 🔶 Add in Phase 3 |
| **Hash Aggregations** | Yes (SIMD) | Planned | 🔶 Add in Phase 3 |
| **Parallel Pipeline** | Yes | Yes | ✅ Have it |
| **Late Materialization** | Yes | No | 🔶 Consider adding |
| **Constant Folding** | Yes | Planner will do | ✅ Covered |

### **Recommendation**: Implement Vectorized Execution for Aggregations

**Proposal**: Add vectorized execution to Forward Index queries

```cpp
// Current (row-by-row):
for (auto& doc : matching_docs) {
    double price = doc.GetField("price");
    sum += price;
}

// Vectorized (block-by-block):
for (size_t offset = 0; offset < doc_count; offset += 8192) {
    size_t block_size = min(8192, doc_count - offset);

    // Load block of prices (SIMD-friendly)
    __m256d* prices = LoadColumnBlock("price", offset, block_size);

    // SIMD sum (8 doubles at once)
    for (size_t i = 0; i < block_size; i += 8) {
        __m256d block = _mm256_load_pd(&prices[i]);
        sum_vec = _mm256_add_pd(sum_vec, block);
    }
}
```

**Benefits**:
- ✅ 4-8× faster aggregations
- ✅ Better cache locality
- ✅ Lower memory bandwidth

---

## 4. UDF Support

### ClickHouse UDFs

**Types**:
1. **SQL UDFs** (Inline functions)
2. **Executable UDFs** (External programs)
3. **C++ Compiled UDFs** (Shared libraries)

**Example: SQL UDF**:
```sql
CREATE FUNCTION my_score AS (score, boost) -> score * boost * 1.5;

SELECT my_score(bm25_score, 1.2) FROM documents;
```

**Example: Executable UDF**:
```xml
<!-- /etc/clickhouse-server/config.xml -->
<functions>
    <function>
        <type>executable</type>
        <name>custom_score</name>
        <command>python3 /opt/udfs/custom_score.py</command>
        <format>TabSeparated</format>
    </function>
</functions>
```

**Example: C++ Compiled UDF**:
```cpp
// Custom aggregate function
class MyAggregateFunction : public IAggregateFunction {
    void add(AggregateDataPtr place,
             const IColumn** columns,
             size_t row_num) override {
        // Aggregate logic
    }
};
```

### Quidditch UDFs (Our Design)

**Types**:
1. **Expression Trees** (Built-in functions)
2. **WASM UDFs** (Custom logic, near-native)
3. **Python UDFs** (ML models, complex logic)

### Comparison

| Feature | ClickHouse | Quidditch | Gap |
|---------|-----------|-----------|-----|
| **Inline Functions** | Yes (SQL) | Expression Trees | ✅ Similar |
| **External Programs** | Yes (Executable UDFs) | No | 🔶 Consider adding |
| **Compiled UDFs** | Yes (C++) | Yes (WASM) | ✅ Better (sandboxed) |
| **Language Support** | Python, any exec | Rust, C, AssemblyScript | ✅ Modern languages |
| **Performance** | Fast (native C++) | Fast (WASM JIT) | ✅ Similar |
| **Sandboxing** | No (trust required) | Yes (WASM) | ✅ **Better** |
| **Deployment** | Config file | API | ✅ **Better** |

### **Verdict**: Quidditch UDF design is **superior** to ClickHouse

**Reasons**:
- ✅ Sandboxed execution (WASM)
- ✅ Better deployment (API vs config files)
- ✅ Modern languages (Rust, AssemblyScript)
- ✅ Faster warmup (tiered compilation)

**Optional**: Add executable UDFs in Phase 4+ for legacy compatibility

---

## 5. Distributed Architecture

### ClickHouse Cluster Architecture

```
ClickHouse Cluster:
┌─────────────────────────────────────────┐
│  ZooKeeper (Coordination)                │
│    ├─ Cluster metadata                  │
│    ├─ Replica status                    │
│    └─ Distributed DDL queue             │
└─────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────┐
│  Shard 1                                 │
│    ├─ Replica 1 (leader)                │
│    └─ Replica 2 (follower)              │
│                                          │
│  Shard 2                                 │
│    ├─ Replica 1 (leader)                │
│    └─ Replica 2 (follower)              │
└─────────────────────────────────────────┘
```

**Key Features**:
1. **Shared-Nothing**: Each shard independent
2. **ZooKeeper**: Centralized coordination
3. **Replication**: Async replication between replicas
4. **Distributed Table**: Virtual table spanning shards
5. **Local Table**: Physical table on each shard
6. **Distributed Queries**: Scatter-gather pattern

**Distributed Query**:
```sql
-- Query on distributed table
SELECT country, sum(revenue)
FROM distributed_sales
GROUP BY country;

-- Execution:
1. Query sent to any node (entry point)
2. Scatter: Sub-queries to all shards
3. Each shard executes locally
4. Gather: Merge results at entry point
5. Final aggregation
```

### Quidditch Cluster Architecture

```
Quidditch Cluster:
┌─────────────────────────────────────────┐
│  Master Nodes (Raft)                     │
│    ├─ Cluster state (in-memory)         │
│    ├─ Shard allocation                  │
│    └─ Index metadata                    │
└─────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────┐
│  Coordination Nodes                      │
│    ├─ Query routing                     │
│    ├─ Result aggregation                │
│    └─ Python pipelines                  │
└─────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────┐
│  Data Nodes (Shards)                     │
│    ├─ Shard 1 (primary + replicas)      │
│    ├─ Shard 2 (primary + replicas)      │
│    └─ Shard N (primary + replicas)      │
└─────────────────────────────────────────┘
```

### Comparison

| Feature | ClickHouse | Quidditch | Gap |
|---------|-----------|-----------|-----|
| **Coordination** | ZooKeeper | Raft (master nodes) | ✅ **Better** (no ZK) |
| **Node Roles** | All equal | Specialized (M/C/D) | ✅ **Better** |
| **Replication** | Async | Planned (Raft?) | 🔶 Design in Phase 1 |
| **Distributed Query** | Scatter-gather | Scatter-gather | ✅ Similar |
| **Distributed DDL** | Yes | Planned | 🔶 Add in Phase 1 |
| **Cluster Resize** | Manual | Planned (auto) | ✅ **Better** (K8S) |
| **Leader Election** | ZK | Raft | ✅ **Better** |

### **Verdict**: Quidditch architecture is **simpler and more cloud-native**

**Advantages**:
- ✅ No ZooKeeper dependency (Raft built-in)
- ✅ Specialized nodes (better resource utilization)
- ✅ Kubernetes-native (auto-scaling)

**Missing**: Need to design replication strategy (Phase 1)

---

## 6. Data Ingestion & Streaming

### ClickHouse Ingestion

**Methods**:
1. **INSERT INTO** (SQL)
2. **Kafka Engine** (Streaming)
3. **File Imports** (CSV, Parquet, JSON)
4. **Buffer Tables** (Batch inserts)
5. **Materialized Views** (Transform on insert)

**Kafka Integration**:
```sql
-- Kafka table engine
CREATE TABLE kafka_queue (
    timestamp DateTime,
    user_id UInt64,
    event String
) ENGINE = Kafka
SETTINGS
    kafka_broker_list = 'broker1:9092',
    kafka_topic_list = 'events',
    kafka_group_name = 'clickhouse_consumer',
    kafka_format = 'JSONEachRow';

-- Materialized view to persist
CREATE MATERIALIZED VIEW events_mv TO events AS
SELECT * FROM kafka_queue;
```

**Performance**: 100K-1M rows/sec per node

### Quidditch Ingestion

**Current Design**:
1. **Bulk API** (OpenSearch compatible)
2. **Index API** (Single document)
3. **Update API** (Partial update)

**Planned** (from docs):
- Real-time indexing
- Translog for durability
- Refresh interval for visibility

### Comparison & Gaps

| Feature | ClickHouse | Quidditch | Gap |
|---------|-----------|-----------|-----|
| **Bulk Insert** | Yes | Yes | ✅ Have it |
| **Streaming (Kafka)** | Yes (native) | No | 🔶 **SHOULD ADD** |
| **Buffer Tables** | Yes | No | 🔶 Consider for batching |
| **File Import** | Yes (many formats) | No | 🔶 Add Parquet import |
| **Transform on Insert** | Yes (MV) | Python pipelines | ✅ Similar |
| **Throughput** | 1M rows/sec | 100K docs/sec | 🔶 Optimize |

### **Recommendation**: Add Kafka/Streaming Ingestion

**Proposal**: Kafka Consumer for Quidditch

```yaml
# quidditch-kafka-consumer.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kafka-consumer-config
data:
  consumer.yaml: |
    kafka:
      brokers:
        - kafka-1:9092
        - kafka-2:9092
      topics:
        - events
        - logs
      consumer_group: quidditch-ingest

    quidditch:
      index: events-{yyyy.MM.dd}
      bulk_size: 10000
      flush_interval: 5s

    transform:
      pipeline: kafka_transform  # Python pipeline
```

**Implementation**: Phase 4-5 (Streaming ingestion)

---

## 7. Compression & Encoding

### ClickHouse Compression

**Codecs** (Per-column):
1. **LZ4** (default, fast)
2. **ZSTD** (better compression)
3. **Delta** (for sorted numeric data)
4. **DoubleDelta** (for time series)
5. **Gorilla** (for floating point)
6. **T64** (for integers)
7. **FPC** (for doubles)

**Example**:
```sql
CREATE TABLE metrics (
    timestamp DateTime CODEC(DoubleDelta, LZ4),
    user_id UInt64 CODEC(Delta, LZ4),
    temperature Float64 CODEC(Gorilla, ZSTD),
    location String CODEC(ZSTD(3))
);
```

**Compression Ratios**: 3-10× typical, 100× for sorted data

### Quidditch Compression (Planned)

From docs: LZ4, ZSTD, Delta, Gorilla codecs planned

### Comparison

| Codec | ClickHouse | Quidditch | Gap |
|-------|-----------|-----------|-----|
| **LZ4** | Yes | Planned | 🔶 Implement in Phase 0-1 |
| **ZSTD** | Yes | Planned | 🔶 Implement in Phase 0-1 |
| **Delta** | Yes | Planned | 🔶 Implement in Phase 2 |
| **DoubleDelta** | Yes | No | 🔶 Consider adding |
| **Gorilla** | Yes | Planned | 🔶 Implement in Phase 2 |
| **T64** | Yes | No | 🔶 Consider adding |
| **FPC** | Yes | No | 🔶 Consider adding |
| **Dictionary** | Yes | No | 🔶 **SHOULD ADD** |

### **Recommendation**: Add Dictionary Encoding

**Dictionary Encoding** (Critical for string columns):
```
Before (uncompressed strings):
["United States", "United States", "Canada", "United States", ...]
Size: 1000 × 13 bytes avg = 13 KB

After (dictionary encoded):
Dictionary: [0: "United States", 1: "Canada", 2: "Mexico"]
Values: [0, 0, 1, 0, 2, 1, 0, 0, ...]
Size: 3 × 13 bytes + 1000 × 1 byte = ~1 KB

Compression: 13× !
```

**Implementation**: Phase 1-2 (Critical for forward index)

---

## 8. Vectorization & SIMD

### ClickHouse Vectorization

**Extensive SIMD usage**:
1. **Column Scans**: AVX2/AVX-512 for filtering
2. **Aggregations**: SIMD sum, min, max
3. **String Operations**: SIMD strcmp, search
4. **Arithmetic**: Vectorized math operations
5. **Hashing**: SIMD CRC32, xxHash
6. **Compression**: SIMD LZ4, ZSTD

**Example: Vectorized Filter**
```cpp
// Filter: WHERE age > 30
void FilterGreater(const uint32_t* ages,
                   uint8_t* mask,
                   size_t count) {
    __m256i threshold = _mm256_set1_epi32(30);

    for (size_t i = 0; i < count; i += 8) {
        __m256i values = _mm256_loadu_si256((__m256i*)&ages[i]);
        __m256i cmp = _mm256_cmpgt_epi32(values, threshold);

        // Store mask (1 bit per element)
        uint32_t mask_bits = _mm256_movemask_ps(_mm256_castsi256_ps(cmp));
        mask[i / 8] = mask_bits;
    }
}
```

**Performance**: 4-16× faster than scalar

### Quidditch SIMD (Planned)

From docs: SIMD BM25 scoring (4-8× faster)

### Comparison & Gaps

| Operation | ClickHouse | Quidditch | Gap |
|-----------|-----------|-----------|-----|
| **Text Search** | No (not OLAP) | SIMD BM25 (planned) | ✅ Search-specific |
| **Column Filters** | Yes (SIMD) | Planned | 🔶 **CRITICAL** for analytics |
| **Aggregations** | Yes (SIMD) | Planned | 🔶 **CRITICAL** for analytics |
| **String Ops** | Yes (SIMD) | No | 🔶 Add if needed |
| **Hash Tables** | Yes (SIMD) | No | 🔶 Add for GROUP BY |
| **Compression** | Yes (SIMD) | Planned | 🔶 Add in Phase 1 |

### **Recommendation**: Expand SIMD beyond BM25

**Priority Order**:
1. **Phase 0**: SIMD BM25 (already planned) ✅
2. **Phase 2**: SIMD column filters (WHERE clauses)
3. **Phase 3**: SIMD aggregations (SUM, AVG, MIN, MAX)
4. **Phase 3**: SIMD hash tables (GROUP BY)
5. **Phase 4**: SIMD compression/decompression

**Example Implementation**:
```cpp
// SIMD aggregation: SUM
double VectorizedSum(const double* values, size_t count) {
    __m256d sum_vec = _mm256_setzero_pd();

    // Process 4 doubles at a time (AVX2)
    for (size_t i = 0; i < count; i += 4) {
        __m256d vals = _mm256_loadu_pd(&values[i]);
        sum_vec = _mm256_add_pd(sum_vec, vals);
    }

    // Horizontal sum
    double result[4];
    _mm256_storeu_pd(result, sum_vec);
    return result[0] + result[1] + result[2] + result[3];
}
```

---

## 9. Indexes & Data Skipping

### ClickHouse Skip Indexes

**Types**:
1. **MinMax** - Track min/max per granule
2. **Set** - Track unique values (cardinality limit)
3. **Bloom Filter** - Probabilistic membership test
4. **NGram Bloom Filter** - For LIKE queries
5. **Token Bloom Filter** - For full-text search

**Example**:
```sql
CREATE TABLE events (
    timestamp DateTime,
    user_id UInt64,
    url String,
    INDEX idx_user_id user_id TYPE minmax GRANULARITY 4,
    INDEX idx_url url TYPE bloom_filter GRANULARITY 4,
    INDEX idx_url_ngram url TYPE ngrambf_v1(4, 1024, 3, 0) GRANULARITY 1
) ENGINE = MergeTree()
ORDER BY timestamp;
```

**How it works**:
```
Query: SELECT * FROM events WHERE user_id = 12345

1. Load MinMax index for user_id
   Granule 1: [1, 10000] → Skip
   Granule 2: [10001, 20000] → Check (12345 in range)
   Granule 3: [20001, 30000] → Skip

2. Only read Granule 2 (saved 66% I/O)
```

**Performance**: 90-99% data skipping on selective queries

### Quidditch Skip Indexes (Planned)

From docs: Skip indexes planned (MinMax, Set, BloomFilter)

### Comparison

| Index Type | ClickHouse | Quidditch | Gap |
|------------|-----------|-----------|-----|
| **MinMax** | Yes | Planned | 🔶 Phase 2 |
| **Set** | Yes | Planned | 🔶 Phase 2 |
| **Bloom Filter** | Yes | Planned | 🔶 Phase 2 |
| **NGram BF** | Yes | Inverted index better | ✅ Not needed |
| **Token BF** | Yes | Inverted index better | ✅ Not needed |
| **Primary Index** | Sparse (8192 rows) | Inverted (full) | ✅ Better for search |

### **Recommendation**: Prioritize Skip Indexes in Phase 2

**Implementation Priority**:
1. **MinMax** (Phase 2) - Easy, high impact
2. **Bloom Filter** (Phase 2) - Medium difficulty, high impact
3. **Set** (Phase 2) - Easy, medium impact for low-cardinality columns

**Example Structure**:
```cpp
// diagon/src/forward_index/skip_index.h
class SkipIndex {
public:
    virtual bool CanSkipGranule(const Predicate& pred) = 0;
};

class MinMaxSkipIndex : public SkipIndex {
    struct GranuleMinMax {
        Value min;
        Value max;
    };

    std::vector<GranuleMinMax> granules_;

public:
    bool CanSkipGranule(const Predicate& pred) override {
        // If predicate is "age > 50" and granule max is 30, skip
        if (pred.op == GT && pred.value > granules_[idx].max) {
            return true;  // Skip this granule
        }
        return false;
    }
};
```

---

## 10. Materialized Views

### ClickHouse Materialized Views

**Purpose**: Pre-aggregate data for fast queries

**Example**:
```sql
-- Base table
CREATE TABLE events (
    timestamp DateTime,
    user_id UInt64,
    event_type String,
    revenue Decimal(10,2)
) ENGINE = MergeTree()
ORDER BY timestamp;

-- Materialized view (auto-updates on insert)
CREATE MATERIALIZED VIEW daily_revenue
ENGINE = SummingMergeTree()
ORDER BY (date, user_id)
AS SELECT
    toDate(timestamp) AS date,
    user_id,
    sum(revenue) AS daily_revenue
FROM events
GROUP BY date, user_id;

-- Fast query (uses MV)
SELECT user_id, sum(daily_revenue)
FROM daily_revenue
WHERE date >= '2023-01-01'
GROUP BY user_id;
```

**Benefits**:
- ✅ 100-1000× faster queries (pre-aggregated)
- ✅ Auto-updated on insert
- ✅ Transparent to queries (optimizer uses MV)

### Quidditch (Current)

**No materialized views in design**

### Comparison

| Feature | ClickHouse | Quidditch | Gap |
|---------|-----------|-----------|-----|
| **Materialized Views** | Yes | No | 🔶 **SHOULD ADD** |
| **Auto-Update on Insert** | Yes | No | 🔶 Phase 4+ |
| **Query Optimizer Uses** | Yes | No | 🔶 Phase 4+ |

### **Recommendation**: Add Materialized Views in Phase 4-5

**Use Cases**:
1. **Pre-aggregated metrics**: Daily/hourly rollups
2. **Flattened data**: Join results cached
3. **Sorted views**: Different sort orders

**Proposal**: OpenSearch-compatible Rollup Jobs + ClickHouse MV semantics

```json
PUT _rollup/daily-sales
{
  "source_index": "sales",
  "target_index": "sales-daily",
  "schedule": {
    "interval": "1h"
  },
  "groups": {
    "date": {
      "date_histogram": {
        "field": "timestamp",
        "interval": "1d"
      }
    },
    "product_id": { "terms": { "field": "product_id" } }
  },
  "metrics": [
    { "field": "revenue", "metrics": ["sum", "avg", "max"] },
    { "field": "quantity", "metrics": ["sum"] }
  ]
}
```

**Implementation**: Phase 4-5 (Nice-to-have, not critical for v1.0)

---

## 11. Replication & High Availability

### ClickHouse Replication

**Architecture**:
```
Shard 1:
  ├─ Replica 1 (CH Server 1) [Leader]
  ├─ Replica 2 (CH Server 2) [Follower]
  └─ Replica 3 (CH Server 3) [Follower]

ZooKeeper:
  ├─ /clickhouse/tables/{cluster}/table1/replicas/1
  ├─ /clickhouse/tables/{cluster}/table1/replicas/2
  └─ /clickhouse/tables/{cluster}/table1/replicas/3
```

**Replication Process**:
1. Insert to any replica
2. Data written to local ReplicatedMergeTree
3. Metadata logged to ZooKeeper
4. Other replicas fetch data in background
5. Async replication (eventual consistency)

**Failure Handling**:
- If leader dies, any replica can serve reads
- Writes go to any alive replica
- ZooKeeper tracks replica status

### Quidditch Replication (To Design)

**Current**: Not fully specified in architecture docs

**Proposal** (from architecture review):
- Primary/replica shards
- Master node tracks shard allocation
- Replication strategy TBD

### Comparison & Gaps

| Feature | ClickHouse | Quidditch | Gap |
|---------|-----------|-----------|-----|
| **Replication** | Yes (async) | To design | 🔶 **CRITICAL** for Phase 1 |
| **Consistency** | Eventual | TBD | 🔶 Design needed |
| **Failover** | Automatic | TBD | 🔶 Design needed |
| **Quorum Writes** | Yes (optional) | TBD | 🔶 Consider |
| **Sync Replication** | No | Consider | 🔶 Better for search |

### **Recommendation**: Design Replication Strategy in Phase 1

**Proposal**: Raft-based Replication per Shard

```
Shard 1 (Raft Group):
  ├─ Primary (Leader)    - Accepts writes
  ├─ Replica 1 (Follower) - Async replication
  └─ Replica 2 (Follower) - Async replication

Write Path:
  1. Write to Primary
  2. Raft log replication (synchronous to quorum)
  3. Apply to local storage
  4. Ack to client

Read Path:
  1. Read from Primary (consistent)
  2. Or read from Replica (may be slightly stale)
```

**Benefits**:
- ✅ Consistent reads from primary
- ✅ Automatic failover (Raft election)
- ✅ No ZooKeeper dependency
- ✅ Better than ClickHouse (sync option)

**Alternative**: Async replication like ClickHouse (faster writes, eventual consistency)

**Decision**: Design in Phase 1, implement in Phase 1-2

---

## 12. What We Should Add

### High Priority (Phase 1-2)

| Feature | Why | Effort | Impact |
|---------|-----|--------|--------|
| **Skip Indexes** | 90%+ data skipping | Medium | 🔥 Huge |
| **Dictionary Encoding** | 10-100× string compression | Low | 🔥 Huge |
| **Replication Design** | HA requirement | High | 🔥 Critical |
| **Vectorized Aggregations** | 4-8× faster analytics | Medium | 🔥 Huge |
| **MergeTree-style Parts** | Better compression & queries | High | 🔴 Large |

### Medium Priority (Phase 3-4)

| Feature | Why | Effort | Impact |
|---------|-----|--------|--------|
| **Kafka Integration** | Streaming ingestion | Medium | 🟡 Medium |
| **Parquet Import** | Bulk data loading | Low | 🟡 Medium |
| **Lazy Materialization** | Reduce memory | Medium | 🟡 Medium |
| **Buffer Tables** | Batch writes | Low | 🟢 Small |
| **More Codecs** | Better compression | Medium | 🟡 Medium |

### Low Priority (Phase 5+)

| Feature | Why | Effort | Impact |
|---------|-----|--------|--------|
| **Materialized Views** | Pre-aggregation | High | 🟡 Medium |
| **Executable UDFs** | Legacy compat | Low | 🟢 Small |
| **Advanced Joins** | Complex queries | High | 🟡 Medium |

---

## 13. What We Can Skip

### Not Needed (Quidditch is Search-First)

| Feature | ClickHouse Has | Skip Because | Alternative |
|---------|---------------|--------------|-------------|
| **SQL Interface** | Yes | OpenSearch DSL/PPL | Custom query lang |
| **Table Engines** | Many (50+) | Single engine | MergeTree-inspired forward index |
| **External Dictionaries** | Yes | Not OLAP-focused | Cache in app layer |
| **Advanced SQL** | Window functions, CTEs | Complex | Focus on search/agg |
| **Data Sampling** | Yes | Not needed | Full scan with skip indexes |
| **Quota Management** | Yes | Use K8S | Resource quotas in K8S |

### Already Better Than ClickHouse

| Feature | Quidditch Advantage |
|---------|---------------------|
| **Full-Text Search** | ✅ Native inverted index (ClickHouse: basic) |
| **UDF Security** | ✅ Sandboxed WASM (ClickHouse: trusted C++) |
| **Coordination** | ✅ Raft built-in (ClickHouse: ZooKeeper) |
| **Cloud-Native** | ✅ K8S operator (ClickHouse: manual) |
| **Node Specialization** | ✅ Master/Coord/Data (ClickHouse: all equal) |
| **Real-Time** | ✅ Real-time search (ClickHouse: near-real-time) |

---

## 14. Implementation Priorities

### Updated Roadmap with ClickHouse Learnings

#### Phase 0 (Months 1-2): Diagon Core ✅ As Planned
- Complete inverted index
- SIMD BM25 scoring
- Basic columnar storage
- Compression (LZ4, ZSTD)

#### Phase 1 (Months 3-5): Distributed + Replication
**Add from ClickHouse**:
- ✅ **Dictionary encoding** for strings
- ✅ **MinMax skip indexes** (basic)
- ✅ **Replication design** (Raft-based)
- ✅ **MergeTree-style immutable parts** (forward index)

#### Phase 2 (Months 6-8): Query Planning
**Add from ClickHouse**:
- ✅ **Skip indexes** (Bloom filters, Set indexes)
- ✅ **Vectorized execution** (column filters)
- ✅ **Late materialization**
- Expression trees + WASM UDFs (as planned)

#### Phase 3 (Months 9-10): Python + Analytics
**Add from ClickHouse**:
- ✅ **SIMD aggregations** (SUM, AVG, COUNT, etc.)
- ✅ **Hash aggregation** (GROUP BY with SIMD)
- Python UDFs (as planned)

#### Phase 4 (Months 11-13): Production Features
**Add from ClickHouse**:
- ✅ **Kafka integration** (streaming ingestion)
- ✅ **Parquet import** (bulk loading)
- PPL, security (as planned)

#### Phase 5 (Months 14-16): Cloud-Native
**As planned**, no major ClickHouse additions

#### Phase 6+ (Future): Advanced Features
**Consider from ClickHouse**:
- Materialized views
- More codecs (DoubleDelta, T64, FPC)
- Advanced query optimization

---

## 15. Architecture Diagram: Quidditch + ClickHouse Learnings

```
┌─────────────────────────────────────────────────────────────────┐
│                    Quidditch Architecture                        │
│            (Enhanced with ClickHouse Best Practices)             │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  Coordination Nodes (Go + Python)                                │
│    • DSL/PPL Query Parsing                                       │
│    • Custom Query Planner (Go)                                   │
│    • WASM UDF Management                                         │
│    • Python Pipeline Execution                                   │
│    • Result Aggregation (SIMD)          ← ClickHouse inspired   │
└─────────────────────────────────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│  Data Nodes (C++ Diagon)                                         │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Inverted Index (Lucene-style)                          │   │
│  │    • Term dictionary                                     │   │
│  │    • Posting lists                                       │   │
│  │    • SIMD BM25 scoring (4-8× faster)                    │   │
│  │    • Position information for phrases                    │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Forward Index (MergeTree-inspired) ← ClickHouse        │   │
│  │                                                          │   │
│  │    Immutable Data Parts:                                │   │
│  │      Part 1 [sorted, compressed]                        │   │
│  │        ├─ Sparse primary index (8192 rows/granule)     │   │
│  │        ├─ Column data (dictionary encoded) ← CH        │   │
│  │        ├─ Skip indexes:                     ← CH        │   │
│  │        │   ├─ MinMax per granule            ← CH        │   │
│  │        │   ├─ Bloom filters                 ← CH        │   │
│  │        │   └─ Set indexes                   ← CH        │   │
│  │        └─ Compression (LZ4, ZSTD, Delta, Gorilla)      │   │
│  │                                                          │   │
│  │      Part 2, Part 3, ... Part N                        │   │
│  │                                                          │   │
│  │    Background Merge Process:                ← CH        │   │
│  │      → Merge small parts into large                    │   │
│  │      → Re-sort and re-compress                         │   │
│  │      → Update skip indexes                             │   │
│  │                                                          │   │
│  │    Vectorized Execution:                    ← CH        │   │
│  │      → Process 8192-row blocks                         │   │
│  │      → SIMD filters (WHERE)                            │   │
│  │      → SIMD aggregations (GROUP BY)                    │   │
│  │      → 90%+ data skipping with indexes                 │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Computation Layer                                       │   │
│  │    • WASM UDF execution (20ns/call)                     │   │
│  │    • Python UDF execution (ML models)                   │   │
│  │    • Join processing                                     │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  Streaming Ingestion (Kafka) ← ClickHouse inspired              │
│    • Kafka consumers                                             │
│    • Transform pipelines                                         │
│    • Bulk indexing                                               │
└─────────────────────────────────────────────────────────────────┘
```

---

## 16. Key Takeaways

### What We Learned from ClickHouse

1. **Skip Indexes are Critical**: 90-99% data pruning on selective queries
2. **Dictionary Encoding**: 10-100× compression for string columns
3. **Vectorized Execution**: 4-8× faster aggregations
4. **MergeTree Pattern**: Immutable parts + background merges = better compression
5. **Kafka Integration**: Native streaming ingestion is valuable
6. **Granular Compression**: Per-column codecs matter

### Quidditch's Unique Strengths

1. **Full-Text Search**: Native inverted index (ClickHouse basic)
2. **Real-Time**: Immediate visibility (ClickHouse near-real-time)
3. **Sandboxed UDFs**: WASM security (ClickHouse trusts C++)
4. **No ZooKeeper**: Raft built-in (ClickHouse needs ZK)
5. **Cloud-Native**: K8S operator (ClickHouse manual)
6. **Specialized Nodes**: Better resource utilization

### Updated Technology Comparison

| Aspect | OpenSearch | ClickHouse | Quidditch |
|--------|-----------|------------|-----------|
| **Full-Text Search** | ⭐⭐⭐⭐⭐ | ⭐ | ⭐⭐⭐⭐⭐ |
| **Analytics** | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| **Compression** | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| **Vectorization** | ⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| **UDF Security** | ⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐⭐ |
| **Cloud-Native** | ⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐⭐ |
| **Real-Time** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| **Operational** | ⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

---

## 17. Action Items

### Immediate (Before Phase 1)

- [ ] Review this comparison with team
- [ ] Prioritize skip indexes for Phase 2
- [ ] Design replication strategy (Raft vs async)
- [ ] Plan dictionary encoding implementation

### Phase 1 Additions

- [ ] Implement dictionary encoding
- [ ] Add basic MinMax skip indexes
- [ ] Design MergeTree-style parts
- [ ] Finalize replication design

### Phase 2 Additions

- [ ] Implement Bloom filter skip indexes
- [ ] Add vectorized column filters
- [ ] Implement late materialization
- [ ] Optimize SIMD aggregations

### Phase 4+ Considerations

- [ ] Kafka integration design
- [ ] Parquet import support
- [ ] Materialized views (if needed)

---

## 18. Conclusion

**ClickHouse teaches us**:
- Analytics performance requires skip indexes, vectorization, and smart compression
- MergeTree's immutable parts + background merges is elegant
- Dictionary encoding is critical for strings
- Streaming ingestion matters for real-time analytics

**Quidditch's position**:
- ✅ Better at full-text search (core strength)
- ✅ Better security (WASM UDFs)
- ✅ Better cloud-native (K8S operator)
- 🔶 Need to add: Skip indexes, vectorization, dictionary encoding
- 🔶 Consider: Kafka integration, materialized views

**Updated vision**:
**"OpenSearch API compatibility + ClickHouse analytics performance + Modern cloud-native architecture"**

---

**Status**: ✅ Analysis Complete
**Next Steps**: Update IMPLEMENTATION_ROADMAP.md with ClickHouse learnings
**Priority**: Implement skip indexes and dictionary encoding in Phase 1-2

---

Made with ❤️ by the Quidditch team
