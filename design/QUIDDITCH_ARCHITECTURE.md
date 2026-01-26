# Quidditch: Distributed Search Engine Architecture
## OpenSearch-Compatible Engine Based on Diagon

**Project Name**: Quidditch (Distributed search across nodes, like Quidditch players across the field)

**Version**: 1.0.0-design
**Date**: 2026-01-25
**Status**: Design Phase

---

## Executive Summary

**Quidditch** is a distributed, cloud-native search engine that provides 100% OpenSearch API compatibility while leveraging the high-performance Diagon search engine core. It combines:

- **Diagon Core**: Lucene-style inverted index + ClickHouse columnar storage
- **OpenSearch API**: Complete compatibility with Index Management, DSL, and PPL
- **Distributed Architecture**: Specialized node types for scalability
- **Query Planning**: Apache Calcite-based logical plan optimization
- **Cloud-Native**: Kubernetes-native deployment with operator pattern
- **Python Integration**: Embedded Python for search pipelines and ML inference

### Key Design Goals

1. **100% OpenSearch API Compatibility**: Index management, mappings, DSL queries
2. **90% PPL Support**: Piped Processing Language with Calcite translation
3. **Hybrid Architecture**: Text search + analytics in one system
4. **Horizontal Scalability**: 10-1000+ node clusters
5. **Python-First Pipelines**: Search pipelines as Python code
6. **Production-Ready**: Multi-tenancy, auth, monitoring, disaster recovery

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Node Types & Responsibilities](#2-node-types--responsibilities)
3. [API Compatibility](#3-api-compatibility)
4. [Query Processing Pipeline](#4-query-processing-pipeline)
5. [Distributed Coordination](#5-distributed-coordination)
6. [Storage Architecture](#6-storage-architecture)
7. [Python Integration](#7-python-integration)
8. [Deployment & Operations](#8-deployment--operations)
9. [Implementation Language Selection](#9-implementation-language-selection)
10. [Detailed Component Design](#10-detailed-component-design)
11. [Migration from OpenSearch](#11-migration-from-opensearch)
12. [Performance Targets](#12-performance-targets)

---

## 1. Architecture Overview

### 1.1 High-Level Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                         Quidditch Cluster                              │
├──────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │              API Layer (OpenSearch Compatible)               │    │
│  │  • REST API (/_search, /_bulk, /_mapping, /_cluster)        │    │
│  │  • Transport Protocol (inter-node communication)              │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                            ↓                                           │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                    Master Nodes (3-5)                         │    │
│  │  • Cluster state management (Raft consensus)                 │    │
│  │  • Index/shard allocation                                     │    │
│  │  • Schema & mapping management                                │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                            ↓                                           │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │                   Coordination Nodes (N)                        │ │
│  │  • Query parsing (DSL/PPL → Calcite logical plan)             │ │
│  │  • Query optimization & execution planning                     │ │
│  │  • Result aggregation from data nodes                          │ │
│  │  • Python pipeline execution                                   │ │
│  └────────────────────────────────────────────────────────────────┘ │
│                            ↓                                           │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │                      Data Nodes (10-1000+)                      │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌─────────────┐ │ │
│  │  │ Inverted Index   │  │  Forward Index   │  │ Computation │ │ │
│  │  │    Nodes         │  │  (Columnar) Nodes│  │    Nodes    │ │ │
│  │  ├──────────────────┤  ├──────────────────┤  ├─────────────┤ │ │
│  │  │ • Text search    │  │ • Sort/aggregate │  │ • Joins     │ │ │
│  │  │ • BM25 scoring   │  │ • Filters        │  │ • Analytics │ │ │
│  │  │ • Phrase queries │  │ • Doc values     │  │ • ML inference│ │
│  │  │ • Diagon core    │  │ • Columnar scan  │  │ • Python UDF│ │ │
│  │  └──────────────────┘  └──────────────────┘  └─────────────┘ │ │
│  └────────────────────────────────────────────────────────────────┘ │
│                            ↓                                           │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                  Persistent Storage Layer                     │    │
│  │  • Shared object storage (S3/MinIO/Ceph) for cold data      │    │
│  │  • Local NVMe/SSD for hot data                               │    │
│  │  • Write-Ahead Log (WAL) for durability                      │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                        │
└──────────────────────────────────────────────────────────────────────┘
```

### 1.2 Deployment Modes

#### Single-Process Mode
- All node types in one process
- Development & testing
- Small deployments (<1M documents)
- Embedded use cases

#### Distributed Mode
- Separate processes per node type
- Kubernetes-native with StatefulSets
- Horizontal scaling (10-1000+ nodes)
- Production deployments

---

## 2. Node Types & Responsibilities

### 2.1 Master Nodes

**Purpose**: Cluster state management and coordination

**Responsibilities**:
- **Cluster State**: Maintain authoritative cluster state (indices, shards, nodes, settings)
- **Shard Allocation**: Decide which data nodes host which shards
- **Index Management**: Handle create/delete index, update mappings, aliases
- **Node Discovery**: Detect node failures and trigger rebalancing
- **Consensus**: Use Raft for distributed consensus (3-5 master-eligible nodes)

**Implementation**:
- **Language**: Go (excellent for distributed systems, Raft libraries)
- **State Storage**: Embedded etcd or custom Raft implementation
- **API**: gRPC for inter-node communication

**Key Components**:
```
MasterNode
├── ClusterStateManager (Raft-based)
│   ├── Index metadata
│   ├── Shard routing table
│   └── Node registry
├── AllocationService
│   ├── Primary shard placement
│   └── Replica distribution
├── MappingService
│   └── Schema validation & evolution
└── DiscoveryService
    └── Node health checks
```

**Configuration**:
```yaml
master:
  election_timeout_ms: 5000
  heartbeat_interval_ms: 1000
  min_master_nodes: 2
  snapshot_interval: 10000
```

---

### 2.2 Coordination Nodes

**Purpose**: Query parsing, planning, and result aggregation

**Responsibilities**:
- **Query Parsing**: Parse OpenSearch DSL and PPL into AST
- **Logical Planning**: Convert AST to Calcite logical plan
- **Physical Planning**: Generate distributed execution plan
- **Query Routing**: Route sub-queries to appropriate data nodes
- **Result Aggregation**: Merge results from data nodes (top-K, aggregations)
- **Python Pipelines**: Execute search pipelines (pre/post-processing)
- **Authentication**: JWT/RBAC enforcement

**Implementation**:
- **Language**: Go (orchestration) + Embedded Python (pipelines)
- **Query Planning**: Calcite JVM integration via JNI/CGO or pure Go rewrite
- **Python Runtime**: Embedded CPython 3.11+ or PyPy

**Key Components**:
```
CoordinationNode
├── QueryParser
│   ├── DSLParser (OpenSearch Query DSL)
│   ├── PPLParser (Piped Processing Language)
│   └── SQLParser (optional, for SQL support)
├── LogicalPlanner (Calcite integration)
│   ├── Rule-based optimizer
│   ├── Cost-based optimizer
│   └── Physical plan generator
├── ExecutionEngine
│   ├── DistributedExecutor (fan-out to data nodes)
│   ├── ResultAggregator (merge, sort, aggregate)
│   └── PythonPipelineExecutor
├── AuthenticationService
│   └── JWT validation, RBAC
└── CacheManager
    ├── Query result cache
    └── Segment info cache
```

**Query Execution Flow**:
```
1. Parse DSL/PPL → AST
2. Validate against index mapping
3. Generate Calcite logical plan
4. Optimize (push-down filters, projection)
5. Generate physical plan (shard-level tasks)
6. Execute Python pre-processing pipeline
7. Fan-out to data nodes
8. Aggregate results
9. Execute Python post-processing pipeline
10. Return to client
```

---

### 2.3 Data Nodes

#### 2.3.1 Inverted Index Nodes

**Purpose**: Full-text search and BM25 scoring

**Responsibilities**:
- **Inverted Index**: Manage Diagon inverted index (FST term dict, VByte postings)
- **Text Search**: Execute TermQuery, PhraseQuery, BooleanQuery
- **BM25 Scoring**: Rank documents by relevance
- **Skip Lists**: Fast query evaluation with skip pointers
- **SIMD Acceleration**: AVX2/NEON for scoring

**Implementation**:
- **Language**: C++ (Diagon core)
- **Storage**: Diagon segments (immutable, log-structured)

**Key Features**:
- Lucene-compatible segment format
- Background merging (tiered merge policy)
- Compressed postings (VByte, Frame-of-Reference)
- SIMD-accelerated BM25

---

#### 2.3.2 Forward Index (Columnar) Nodes

**Purpose**: Analytical queries, sorting, aggregations

**Responsibilities**:
- **Columnar Storage**: Diagon ClickHouse-style columns
- **Doc Values**: Fast field access for sorting/aggregations
- **Range Filters**: Numeric/timestamp range queries
- **Aggregations**: Sum, avg, min, max, percentiles, cardinality
- **Skip Indexes**: MinMax, Set, BloomFilter for granule pruning

**Implementation**:
- **Language**: C++ (Diagon core)
- **Storage**: Wide/Compact data parts with granules (8192 rows)
- **Compression**: LZ4, ZSTD, Delta, Gorilla

**Key Features**:
- Granule-based I/O (adaptive granularity)
- Skip indexes (90%+ data skipping)
- SIMD-accelerated filters
- Multi-valued fields (arrays)

---

#### 2.3.3 Computation Nodes

**Purpose**: Complex analytics and ML inference

**Responsibilities**:
- **Joins**: Cross-index joins (nested, hash, merge)
- **Complex Analytics**: Window functions, graph analytics
- **ML Inference**: Run trained models (TensorFlow, PyTorch, ONNX)
- **Python UDFs**: User-defined functions in Python
- **Vector Search**: kNN search for embeddings (HNSW, IVF)

**Implementation**:
- **Language**: C++ (computation) + Python (ML/UDF)
- **ML Runtime**: ONNX Runtime, TensorFlow Lite
- **Vector Search**: FAISS, HNSWlib

**Key Features**:
- In-memory computation engine
- GPU acceleration (CUDA/ROCm) for ML
- Vector index (HNSW, IVF-PQ)

---

### 2.4 Node Specialization Strategy

**Hybrid Nodes** (default for small clusters):
- Single data node runs all three roles (inverted + forward + computation)
- Automatic based on query type

**Dedicated Nodes** (large clusters):
- Separate node pools for each role
- Master coordinates shard placement
- Query planner routes to appropriate nodes

**Configuration**:
```yaml
node:
  roles: [inverted_index, forward_index, computation]  # Hybrid
  # OR
  roles: [inverted_index]  # Dedicated
```

---

## 3. API Compatibility

### 3.1 Index Management APIs (100% OpenSearch Compatible)

#### 3.1.1 Index CRUD

```http
# Create Index
PUT /my-index
{
  "settings": {
    "number_of_shards": 5,
    "number_of_replicas": 1,
    "index": {
      "codec": "diagon_best_compression",
      "refresh_interval": "1s"
    }
  },
  "mappings": {
    "properties": {
      "title": {"type": "text", "analyzer": "standard"},
      "category": {"type": "keyword"},
      "price": {"type": "float"},
      "tags": {"type": "keyword", "multi_valued": true},
      "timestamp": {"type": "date"}
    }
  }
}

# Get Index
GET /my-index

# Delete Index
DELETE /my-index

# Update Mapping
PUT /my-index/_mapping
{
  "properties": {
    "new_field": {"type": "text"}
  }
}
```

#### 3.1.2 Aliases

```http
# Add Alias
POST /_aliases
{
  "actions": [
    {"add": {"index": "my-index", "alias": "my-alias"}}
  ]
}

# Get Alias
GET /my-alias/_alias

# Delete Alias
DELETE /my-index/_alias/my-alias
```

#### 3.1.3 Index Templates

```http
# Create Template
PUT /_index_template/logs-template
{
  "index_patterns": ["logs-*"],
  "template": {
    "settings": {
      "number_of_shards": 3
    },
    "mappings": {
      "properties": {
        "timestamp": {"type": "date"},
        "message": {"type": "text"}
      }
    }
  }
}
```

---

### 3.2 Document APIs (100% OpenSearch Compatible)

```http
# Index Document
PUT /my-index/_doc/1
{
  "title": "Diagon Search Engine",
  "category": "software",
  "price": 99.99,
  "tags": ["search", "analytics"],
  "timestamp": "2026-01-25T10:00:00Z"
}

# Bulk API
POST /_bulk
{"index": {"_index": "my-index", "_id": "1"}}
{"title": "Document 1", "price": 10.0}
{"index": {"_index": "my-index", "_id": "2"}}
{"title": "Document 2", "price": 20.0}

# Get Document
GET /my-index/_doc/1

# Update Document
POST /my-index/_update/1
{
  "doc": {"price": 89.99}
}

# Delete Document
DELETE /my-index/_doc/1

# Multi-Get
GET /_mget
{
  "docs": [
    {"_index": "my-index", "_id": "1"},
    {"_index": "my-index", "_id": "2"}
  ]
}
```

---

### 3.3 Search APIs (100% DSL Compatible)

#### 3.3.1 Query DSL

```http
# Full-Text Search
POST /my-index/_search
{
  "query": {
    "match": {
      "title": "search engine"
    }
  },
  "size": 10,
  "from": 0,
  "sort": [
    {"price": "asc"}
  ]
}

# Boolean Query
POST /my-index/_search
{
  "query": {
    "bool": {
      "must": [
        {"match": {"title": "search"}}
      ],
      "filter": [
        {"range": {"price": {"gte": 10, "lte": 100}}},
        {"term": {"category": "software"}}
      ],
      "should": [
        {"match": {"tags": "analytics"}}
      ],
      "must_not": [
        {"term": {"status": "archived"}}
      ]
    }
  }
}

# Aggregations
POST /my-index/_search
{
  "size": 0,
  "aggs": {
    "price_ranges": {
      "range": {
        "field": "price",
        "ranges": [
          {"to": 50},
          {"from": 50, "to": 100},
          {"from": 100}
        ]
      }
    },
    "avg_price": {
      "avg": {"field": "price"}
    }
  }
}
```

#### 3.3.2 Multi-Search

```http
POST /_msearch
{"index": "my-index"}
{"query": {"match": {"title": "search"}}}
{"index": "other-index"}
{"query": {"term": {"category": "software"}}}
```

---

### 3.4 PPL (Piped Processing Language) - 90% Compatible

**Example PPL Queries**:

```sql
-- Basic search with filters
source=logs
| where status >= 200 and status < 300
| fields timestamp, ip, status, response_time
| sort -response_time
| head 10

-- Aggregations
source=logs
| stats count() by status
| where count > 100

-- Time-series analysis
source=metrics
| where timestamp > now() - 1h
| stats avg(cpu_usage) by host, span(1m)
| sort -avg(cpu_usage)

-- Joins (90% support)
source=orders
| join type=left orders.user_id = users.id [search source=users]
| fields order_id, user_name, total_price
```

**PPL Translation Pipeline**:
```
PPL Query → PPLParser → Calcite SQL AST → Logical Plan → Optimization → Physical Plan
```

**Unsupported PPL Features** (10%):
- `dedup` command (deduplication - can be approximated with aggregations)
- `rare` command (rare terms - requires full scan, expensive)
- `top` with complex expressions
- Some advanced windowing functions

---

## 4. Query Processing Pipeline

### 4.1 DSL Query Processing

```
┌──────────────────────────────────────────────────────────────┐
│                     DSL Query Processing                      │
└──────────────────────────────────────────────────────────────┘

1. Client Request (REST API)
   ↓
2. Coordination Node: DSLParser
   • Parse JSON into QueryNode AST
   • Validate field names against mapping
   ↓
3. Mapping Lookup (Cache or Master Node)
   • Retrieve index mapping
   • Identify field types (text, keyword, numeric, etc.)
   ↓
4. Calcite Logical Plan Generation
   • Convert QueryNode → Calcite RelNode
   • Apply rule-based optimizations:
     - Filter push-down
     - Projection push-down
     - Predicate reordering (selectivity)
   ↓
5. Cost-Based Optimization (Calcite)
   • Estimate cardinality
   • Choose join algorithm (hash, merge, nested-loop)
   • Decide inverted vs columnar access path
   ↓
6. Physical Plan Generation
   • Shard-level TaskPlan
   • Determine node targets (inverted, forward, computation)
   • Generate execution DAG
   ↓
7. Python Pre-Processing Pipeline (optional)
   • Query rewriting
   • Query expansion (synonyms, spelling correction)
   • Access control (filter by permissions)
   ↓
8. Distributed Execution
   • Fan-out TaskPlan to data nodes (gRPC)
   • Parallel execution per shard
   • Streaming results back to coordinator
   ↓
9. Result Aggregation (Coordination Node)
   • Merge sorted results (top-K heap)
   • Aggregate metrics (sum, avg, percentiles)
   • Combine histograms, cardinality estimates
   ↓
10. Python Post-Processing Pipeline (optional)
    • Re-ranking (ML models)
    • Highlighting
    • Result transformation
    ↓
11. Response to Client (JSON)
```

### 4.2 PPL Query Processing

```
┌──────────────────────────────────────────────────────────────┐
│                     PPL Query Processing                      │
└──────────────────────────────────────────────────────────────┘

1. Client Request (PPL query string)
   ↓
2. Coordination Node: PPLParser
   • Tokenize pipe-separated commands
   • Parse into PPLCommand chain
   ↓
3. PPL → SQL Translation
   • Map PPL commands to SQL equivalents:
     - source → FROM
     - where → WHERE
     - stats → GROUP BY + aggregation
     - fields → SELECT
     - sort → ORDER BY
   ↓
4. Calcite SQL → RelNode
   • Use Calcite SQL parser
   • Generate logical plan
   ↓
5. [Continue with steps 5-11 from DSL processing]
```

**PPL Command Mapping**:

| PPL Command | SQL Equivalent | Notes |
|-------------|----------------|-------|
| `source=index` | `FROM index` | Index selection |
| `where expr` | `WHERE expr` | Filtering |
| `fields f1, f2` | `SELECT f1, f2` | Projection |
| `stats agg by f` | `SELECT f, agg GROUP BY f` | Aggregation |
| `sort +/-field` | `ORDER BY field ASC/DESC` | Sorting |
| `head N` | `LIMIT N` | Top-N |
| `eval f=expr` | `SELECT expr AS f` | Computed fields |
| `join` | `JOIN` | Join operations |

---

### 4.3 Query Optimization Rules

**Calcite Rules Applied**:

1. **FilterPushDownRule**: Push filters to data nodes (reduce data transfer)
2. **ProjectionPushDownRule**: Only read required fields
3. **PredicateReorderingRule**: Evaluate selective filters first
4. **JoinReorderRule**: Reorder joins for minimal intermediate size
5. **AggregationPushDownRule**: Partial aggregation at data nodes
6. **IndexSelectionRule**: Choose inverted vs columnar index
7. **SkipIndexPruningRule**: Use skip indexes (MinMax, BloomFilter)

**Custom Quidditch Rules**:

1. **DiagonIndexScanRule**: Use Diagon skip indexes for granule pruning
2. **SIMDFilterRule**: Use SIMD for numeric range filters
3. **HybridScanRule**: Combine inverted + columnar scans
4. **PythonUDFPushDownRule**: Execute Python UDFs at data nodes (if safe)

---

## 5. Distributed Coordination

### 5.1 Cluster State Management

**State Storage** (Master Nodes):
```go
type ClusterState struct {
    Version         int64                   // Monotonic version
    Indices         map[string]*IndexMetadata
    RoutingTable    *ShardRoutingTable
    Nodes           map[string]*NodeInfo
    Metadata        *ClusterMetadata
}

type IndexMetadata struct {
    Name            string
    UUID            string
    Settings        IndexSettings
    Mappings        *MappingMetadata
    Shards          int
    Replicas        int
    State           IndexState  // OPEN, CLOSE, DELETE
}

type ShardRoutingTable struct {
    Shards          map[ShardID][]ShardRouting
}

type ShardRouting struct {
    ShardID         ShardID
    State           ShardState  // UNASSIGNED, INITIALIZING, STARTED, RELOCATING
    Primary         bool
    NodeID          string
    RelocatingNodeID string
}
```

**Raft Consensus**:
- 3-5 master-eligible nodes
- Leader election (5s timeout)
- Log replication (cluster state changes)
- Snapshot every 10,000 operations

---

### 5.2 Shard Allocation

**Allocation Strategy**:

1. **Initial Allocation** (index creation):
   - Distribute primaries evenly across nodes
   - Place replicas on different nodes than primary
   - Consider node disk space, CPU, memory

2. **Rebalancing** (node join/leave):
   - Move shards to balance load
   - Minimize data movement
   - Respect allocation constraints (same node, same rack)

3. **Failure Handling**:
   - Detect node failure (heartbeat timeout)
   - Promote replica to primary
   - Allocate new replica on healthy node

**Allocation Constraints**:
```yaml
allocation:
  awareness:
    attributes: [zone, rack]  # Don't place primary + replica in same zone
  disk:
    threshold_enabled: true
    low_watermark: 85%        # Stop allocating shards
    high_watermark: 90%       # Relocate shards away
    flood_stage: 95%          # Block writes
```

---

### 5.3 Node Discovery & Health

**Discovery Mechanisms**:
- **Kubernetes**: Use K8S service discovery (StatefulSet headless service)
- **Standalone**: Seed hosts configuration + Zen discovery

**Health Checks**:
- Master → Data Nodes: Heartbeat every 1s, timeout 5s
- Coordination → Data Nodes: Query-level health (fail fast on errors)
- Liveness probe: TCP socket check
- Readiness probe: API endpoint `/cluster/health`

---

## 6. Storage Architecture

### 6.1 Shard Storage Layout

**Logical Structure**:
```
Index: logs-2026-01
├── Shard 0 (Primary on Node A, Replica on Node B)
├── Shard 1 (Primary on Node B, Replica on Node C)
├── Shard 2 (Primary on Node C, Replica on Node A)
└── ...
```

**Physical Storage** (per shard):
```
/data/nodes/0/indices/logs-2026-01/0/  # Shard 0
├── diagon/                             # Diagon segments
│   ├── segment_0/
│   │   ├── inverted/                   # Inverted index
│   │   │   ├── _terms.fst
│   │   │   ├── _postings.bin
│   │   │   └── _postings.skip
│   │   ├── columns/                    # Columnar storage
│   │   │   ├── timestamp.int64/
│   │   │   │   ├── data.bin
│   │   │   │   └── marks.mrk2
│   │   │   ├── message.string/
│   │   │   │   ├── data.bin
│   │   │   │   └── marks.mrk2
│   │   │   └── ...
│   │   ├── _metadata.json
│   │   └── _checksums.txt
│   ├── segment_1/
│   └── ...
├── wal/                                # Write-Ahead Log
│   ├── wal-0001.log
│   └── wal-0002.log
├── _state/                             # Shard metadata
│   └── state-1.st
└── translog/                           # Translog (pre-commit buffer)
    └── translog-1.tlog
```

---

### 6.2 Multi-Tier Storage

**Storage Tiers**:

| Tier | Storage | Use Case | Cost | Latency |
|------|---------|----------|------|---------|
| **Hot** | Local NVMe/SSD | Real-time ingest & search | $$$ | <10ms |
| **Warm** | Local SSD | Recent data (7-30 days) | $$ | <50ms |
| **Cold** | S3/MinIO/Ceph | Historical data (>30 days) | $ | <500ms |
| **Frozen** | Glacier/Archive | Compliance/backup | ¢ | Minutes |

**Lifecycle Policy** (Index Lifecycle Management):
```yaml
policy:
  name: logs-policy
  phases:
    hot:
      actions:
        rollover:
          max_size: 50gb
          max_age: 1d
    warm:
      min_age: 7d
      actions:
        allocate:
          require:
            tier: warm
        readonly: {}
    cold:
      min_age: 30d
      actions:
        searchable_snapshot:
          snapshot_repository: s3-repo
    delete:
      min_age: 90d
      actions:
        delete: {}
```

**Searchable Snapshots** (Cold tier):
- Store segments in S3
- Cache frequently accessed blocks locally
- On-demand segment download for queries

---

## 7. Python Integration

### 7.1 Search Pipeline Architecture

**Search Pipeline Phases**:
```
Request → [Pre-processors] → Query Execution → [Post-processors] → Response
```

**Pipeline Definition** (Python):
```python
# /pipelines/my_pipeline.py

from quidditch.pipeline import SearchPipeline, Processor

class SynonymExpansionProcessor(Processor):
    """Pre-processor: Expand query with synonyms"""

    def process_request(self, request):
        query_text = request.query.get('match', {}).get('title', '')
        synonyms = self.get_synonyms(query_text)  # Custom logic

        if synonyms:
            request.query = {
                'bool': {
                    'should': [
                        {'match': {'title': query_text}},
                        {'match': {'title': ' '.join(synonyms)}}
                    ]
                }
            }
        return request

class MLRerankProcessor(Processor):
    """Post-processor: Re-rank results with ML model"""

    def __init__(self):
        self.model = self.load_model('rerank_model.onnx')

    def process_response(self, response, request):
        hits = response['hits']['hits']

        # Extract features for ML model
        features = [self.extract_features(hit, request.query) for hit in hits]

        # Predict scores
        scores = self.model.predict(features)

        # Re-rank
        for hit, score in zip(hits, scores):
            hit['_score'] = score

        hits.sort(key=lambda x: x['_score'], reverse=True)
        return response

# Pipeline registration
pipeline = SearchPipeline(
    name='my_pipeline',
    processors=[
        SynonymExpansionProcessor(),
        MLRerankProcessor()
    ]
)
```

**Pipeline Deployment** (REST API):
```http
PUT /_search/pipeline/my_pipeline
{
  "description": "Synonym expansion + ML re-ranking",
  "processors": [
    {
      "type": "python",
      "module": "my_pipeline",
      "class": "SynonymExpansionProcessor"
    },
    {
      "type": "python",
      "module": "my_pipeline",
      "class": "MLRerankProcessor"
    }
  ]
}
```

**Using Pipeline**:
```http
POST /my-index/_search?pipeline=my_pipeline
{
  "query": {"match": {"title": "search engine"}}
}
```

---

### 7.2 Python Runtime Integration

**Embedded Python** (CPython):
- Coordination nodes embed CPython 3.11+
- Use Python C API for bi-directional calls
- Virtual environments per pipeline (isolation)

**Architecture**:
```
┌─────────────────────────────────────┐
│     Coordination Node (Go)          │
│                                     │
│  ┌───────────────────────────────┐ │
│  │   Query Execution Engine      │ │
│  └───────────────────────────────┘ │
│               ↓                     │
│  ┌───────────────────────────────┐ │
│  │  Embedded Python Runtime      │ │
│  │  (CPython 3.11 via CGO)       │ │
│  │                               │ │
│  │  ┌─────────────────────────┐ │ │
│  │  │  Pipeline Executor      │ │ │
│  │  └─────────────────────────┘ │ │
│  │               ↓               │ │
│  │  ┌─────────────────────────┐ │ │
│  │  │  User Pipelines         │ │ │
│  │  │  (my_pipeline.py)       │ │ │
│  │  └─────────────────────────┘ │ │
│  └───────────────────────────────┘ │
└─────────────────────────────────────┘
```

**Go ↔ Python Bridge** (CGO):
```go
// #cgo pkg-config: python3
// #include <Python.h>
import "C"

type PythonRuntime struct {
    interpreter *C.PyInterpreter
}

func (r *PythonRuntime) ExecutePipeline(pipelineName string, request *SearchRequest) error {
    // Convert Go struct to Python dict
    pyRequest := r.goToPython(request)

    // Call Python function
    pyModule := C.PyImport_ImportModule(C.CString(pipelineName))
    pyFunc := C.PyObject_GetAttrString(pyModule, C.CString("process_request"))
    pyResult := C.PyObject_CallObject(pyFunc, pyRequest)

    // Convert Python dict back to Go struct
    r.pythonToGo(pyResult, request)

    return nil
}
```

**Security**:
- Sandboxed execution (RestrictedPython or custom sandbox)
- Resource limits (CPU time, memory)
- No filesystem/network access (except approved APIs)

---

### 7.3 Python UDFs (User-Defined Functions)

**Use Cases**:
- Custom scoring functions
- Text processing (NLP, tokenization)
- Data transformations
- ML model inference

**Example UDF**:
```python
from quidditch.udf import udf

@udf(return_type='float')
def custom_relevance_score(doc_id: int, query: str, bm25_score: float) -> float:
    """Custom scoring combining BM25 + recency + popularity"""

    # Fetch document fields
    doc = get_document(doc_id)

    # Recency boost (exponential decay)
    age_days = (now() - doc['timestamp']).days
    recency_score = math.exp(-age_days / 30)  # 30-day half-life

    # Popularity boost
    popularity_score = math.log(1 + doc['view_count'])

    # Combined score
    return bm25_score * recency_score * (1 + 0.1 * popularity_score)
```

**Using UDF in Query**:
```http
POST /my-index/_search
{
  "query": {
    "function_score": {
      "query": {"match": {"title": "search"}},
      "functions": [
        {
          "script_score": {
            "script": {
              "source": "custom_relevance_score",
              "lang": "python",
              "params": {"query": "search"}
            }
          }
        }
      ]
    }
  }
}
```

---

## 8. Deployment & Operations

### 8.1 Kubernetes Deployment

**Architecture**:
```
┌──────────────────────────────────────────────────────────┐
│                 Kubernetes Cluster                        │
├──────────────────────────────────────────────────────────┤
│                                                            │
│  ┌────────────────────────────────────────────────────┐  │
│  │          Master Nodes (StatefulSet)                 │  │
│  │  • 3 replicas (high availability)                   │  │
│  │  • PersistentVolumeClaims for state                 │  │
│  │  • Headless Service for discovery                   │  │
│  └────────────────────────────────────────────────────┘  │
│                                                            │
│  ┌────────────────────────────────────────────────────┐  │
│  │      Coordination Nodes (Deployment)                │  │
│  │  • N replicas (auto-scaling)                        │  │
│  │  • Load Balancer Service (external access)          │  │
│  └────────────────────────────────────────────────────┘  │
│                                                            │
│  ┌────────────────────────────────────────────────────┐  │
│  │         Data Nodes (StatefulSet)                    │  │
│  │  • M replicas (one per shard)                       │  │
│  │  • PersistentVolumeClaims (NVMe/SSD)                │  │
│  │  • Pod affinity rules (spread across zones)         │  │
│  └────────────────────────────────────────────────────┘  │
│                                                            │
│  ┌────────────────────────────────────────────────────┐  │
│  │          Object Storage (S3/MinIO)                  │  │
│  │  • Cold tier storage                                │  │
│  │  • Snapshots & backups                              │  │
│  └────────────────────────────────────────────────────┘  │
│                                                            │
└──────────────────────────────────────────────────────────┘
```

**Operator Pattern**:
- Custom Resource Definitions (CRDs): QuidditchCluster, QuidditchIndex
- Operator controller watches CRDs and reconciles state
- Automated rolling upgrades, scaling, backup/restore

**Example CRD**:
```yaml
apiVersion: quidditch.io/v1
kind: QuidditchCluster
metadata:
  name: prod-cluster
spec:
  version: "1.0.0"
  master:
    replicas: 3
    resources:
      requests:
        memory: "4Gi"
        cpu: "2"
  coordination:
    replicas: 5
    autoscaling:
      enabled: true
      minReplicas: 5
      maxReplicas: 20
      targetCPUUtilization: 70
  data:
    replicas: 10
    storage:
      class: "nvme"
      size: "1Ti"
    nodeSelector:
      node-type: "data"
  s3:
    endpoint: "s3.amazonaws.com"
    bucket: "quidditch-cold-storage"
```

---

### 8.2 Monitoring & Observability

**Metrics** (Prometheus):
```
# Indexing
quidditch_indexing_rate{index="logs"}
quidditch_indexing_errors_total{index="logs"}
quidditch_segment_count{index="logs", shard="0"}
quidditch_merge_operations_total{index="logs"}

# Querying
quidditch_query_rate{index="logs"}
quidditch_query_latency_seconds{index="logs", quantile="0.99"}
quidditch_cache_hit_rate{cache_type="query_result"}

# Cluster
quidditch_node_count{role="master|coordination|data"}
quidditch_shard_count{state="started|initializing|relocating"}
quidditch_disk_usage_bytes{node="node-1", tier="hot"}

# Python
quidditch_pipeline_execution_time_seconds{pipeline="my_pipeline"}
quidditch_python_errors_total{pipeline="my_pipeline"}
```

**Logging** (Structured JSON):
```json
{
  "timestamp": "2026-01-25T10:00:00Z",
  "level": "INFO",
  "component": "QueryExecutor",
  "node": "coord-1",
  "query_id": "abc123",
  "index": "logs-2026-01",
  "message": "Query executed successfully",
  "duration_ms": 45,
  "hits": 1523
}
```

**Tracing** (OpenTelemetry):
- Distributed tracing across coordination → data nodes
- Query execution spans (parsing, planning, execution, aggregation)
- Python pipeline execution spans

---

### 8.3 Backup & Disaster Recovery

**Snapshot Repository**:
```http
PUT /_snapshot/s3-repo
{
  "type": "s3",
  "settings": {
    "bucket": "quidditch-backups",
    "region": "us-east-1",
    "compress": true,
    "chunk_size": "100mb"
  }
}
```

**Snapshot Policy**:
```yaml
snapshot_policy:
  name: daily-snapshots
  schedule: "0 2 * * *"  # 2 AM daily
  repository: s3-repo
  config:
    indices: ["logs-*", "metrics-*"]
    ignore_unavailable: true
    include_global_state: true
  retention:
    expire_after: "30d"
    min_count: 5
    max_count: 30
```

**Restore**:
```http
POST /_snapshot/s3-repo/snapshot_2026_01_25/_restore
{
  "indices": "logs-2026-01-*",
  "ignore_unavailable": true,
  "include_global_state": false,
  "rename_pattern": "logs-(.+)",
  "rename_replacement": "restored-logs-$1"
}
```

---

## 9. Implementation Language Selection

### 9.1 Language Analysis

| Component | Language | Rationale |
|-----------|----------|-----------|
| **Master Nodes** | **Go** | • Excellent concurrency (goroutines)<br>• Mature Raft libraries (etcd/raft, hashicorp/raft)<br>• Strong ecosystem for distributed systems<br>• Easy deployment (single binary) |
| **Coordination Nodes** | **Go** | • High-performance HTTP/gRPC<br>• Good for orchestration logic<br>• CGO for Python embedding<br>• JSON parsing performance |
| **Data Nodes (Diagon)** | **C++** | • Existing Diagon codebase<br>• Maximum performance (SIMD, memory control)<br>• Lucene/ClickHouse heritage<br>• No GC pauses |
| **Python Integration** | **Python** | • Native Python for pipelines<br>• Embedded CPython in Go (CGO)<br>• Rich ML/NLP ecosystem |
| **Alternative: Rust** | **Rust** | • Memory safety without GC<br>• Strong concurrency (async/await)<br>• Interop with C++ (easier than Go)<br>• Consideration for future rewrites |

**Recommendation**: **Go + C++ + Python**
- **Go**: Master & coordination nodes (95% of distributed logic)
- **C++**: Data nodes (Diagon core)
- **Python**: Pipelines & UDFs

**Alternative**: **Rust + C++ + Python**
- If starting fresh, Rust for coordination (better safety, async)
- C++ still needed for Diagon (already implemented)

---

### 9.2 Language Integration

#### Go ↔ C++ (Diagon)

**Option 1: CGO + C Wrapper**
```go
// #cgo LDFLAGS: -ldiagon
// #include "diagon_c_api.h"
import "C"

type DiagonIndex struct {
    handle C.DiagonIndexHandle
}

func (idx *DiagonIndex) Search(query string) ([]SearchHit, error) {
    cQuery := C.CString(query)
    defer C.free(unsafe.Pointer(cQuery))

    var results C.DiagonSearchResults
    status := C.diagon_search(idx.handle, cQuery, &results)

    if status != C.DIAGON_OK {
        return nil, fmt.Errorf("search failed: %d", status)
    }

    return convertResults(&results), nil
}
```

**C Wrapper** (diagon_c_api.h):
```c
#ifdef __cplusplus
extern "C" {
#endif

typedef void* DiagonIndexHandle;

typedef struct {
    int32_t doc_id;
    float score;
} DiagonSearchHit;

typedef struct {
    DiagonSearchHit* hits;
    int32_t num_hits;
} DiagonSearchResults;

int diagon_open_index(const char* path, DiagonIndexHandle* handle);
int diagon_search(DiagonIndexHandle handle, const char* query, DiagonSearchResults* results);
void diagon_free_results(DiagonSearchResults* results);
int diagon_close_index(DiagonIndexHandle handle);

#ifdef __cplusplus
}
#endif
```

**Option 2: gRPC Service**
- Diagon as separate process (C++)
- Go coordination node communicates via gRPC
- Better isolation, easier deployment
- Slight latency overhead (~1-2ms)

---

#### Go ↔ Python

**Embedded CPython** (CGO):
```go
// #cgo pkg-config: python3-embed
// #include <Python.h>
import "C"

type PythonVM struct {
    initialized bool
}

func NewPythonVM() *PythonVM {
    C.Py_Initialize()
    return &PythonVM{initialized: true}
}

func (vm *PythonVM) ExecuteScript(script string) (map[string]interface{}, error) {
    cScript := C.CString(script)
    defer C.free(unsafe.Pointer(cScript))

    pyMain := C.PyImport_AddModule(C.CString("__main__"))
    pyDict := C.PyModule_GetDict(pyMain)

    result := C.PyRun_String(cScript, C.Py_file_input, pyDict, pyDict)
    if result == nil {
        return nil, fmt.Errorf("Python execution failed")
    }

    return convertPyObject(result), nil
}

func (vm *PythonVM) Close() {
    if vm.initialized {
        C.Py_Finalize()
    }
}
```

---

### 9.3 Build System

**Multi-Language Build** (Bazel or Make):
```makefile
# Makefile

# Build C++ Diagon core
build-diagon:
    cd diagon && cmake -B build -S . && cmake --build build

# Build Go master/coordination nodes
build-go:
    go build -o bin/quidditch-master ./cmd/master
    go build -o bin/quidditch-coord ./cmd/coordination
    go build -o bin/quidditch-data ./cmd/data

# Build C wrapper for Diagon
build-c-wrapper:
    cd diagon-c-wrapper && make

# Package Python pipelines
package-python:
    cd pipelines && python setup.py sdist

# Docker images
docker-build:
    docker build -t quidditch/master:latest -f docker/Dockerfile.master .
    docker build -t quidditch/coord:latest -f docker/Dockerfile.coord .
    docker build -t quidditch/data:latest -f docker/Dockerfile.data .

all: build-diagon build-c-wrapper build-go package-python docker-build
```

---

## 10. Detailed Component Design

### 10.1 Calcite Integration

**Architecture**:
```
┌───────────────────────────────────────────────────────────┐
│           Calcite Logical Planner (JVM)                   │
├───────────────────────────────────────────────────────────┤
│                                                             │
│  1. Parse Query (DSL/PPL/SQL)                              │
│     ↓                                                       │
│  2. Validate Against Schema (RelOptTable)                  │
│     ↓                                                       │
│  3. Generate Logical Plan (RelNode tree)                   │
│     • TableScan                                             │
│     • Filter                                                │
│     • Project                                               │
│     • Aggregate                                             │
│     • Join                                                  │
│     • Sort                                                  │
│     ↓                                                       │
│  4. Apply Optimization Rules                               │
│     • FilterPushDownRule                                    │
│     • ProjectRemoveRule                                     │
│     • AggregateRemoveRule                                   │
│     • JoinCommuteRule                                       │
│     ↓                                                       │
│  5. Cost-Based Optimization                                │
│     • Cardinality estimation                               │
│     • Cost model (CPU, I/O, network)                       │
│     • Choose best plan                                      │
│     ↓                                                       │
│  6. Generate Physical Plan                                 │
│     • DiagonIndexScan                                       │
│     • DiagonColumnarScan                                    │
│     • HashJoin / MergeJoin                                 │
│     • Exchange (shuffle)                                    │
│     ↓                                                       │
│  7. Serialize Plan (JSON/Protobuf)                         │
│     • Send to Go coordination node                         │
│                                                             │
└───────────────────────────────────────────────────────────┘
```

**Integration Options**:

**Option 1: JVM Subprocess**
- Go spawns Calcite JVM process
- Communicate via JSON over stdin/stdout
- Pros: Standard Calcite, easy to upgrade
- Cons: JVM overhead, IPC latency

**Option 2: JNI via CGO**
- Embed JVM in Go process via CGO
- Pros: Lower latency
- Cons: Complex, memory management, JVM in same process

**Option 3: gRPC Service**
- Calcite as separate microservice
- Pros: Language-agnostic, scalable
- Cons: Network latency, extra deployment

**Recommendation**: **Option 3 (gRPC Service)**
- Deploy Calcite as separate StatefulSet (3-5 replicas)
- Coordination nodes call via gRPC
- Cache plans for repeated queries

---

### 10.2 Shard Management

**Shard Design**:
```
Index: logs-2026-01
├── Shard 0: [doc_0, doc_5, doc_10, ...]  (hash(doc_id) % num_shards == 0)
├── Shard 1: [doc_1, doc_6, doc_11, ...]  (hash(doc_id) % num_shards == 1)
├── Shard 2: [doc_2, doc_7, doc_12, ...]  (hash(doc_id) % num_shards == 2)
└── ...
```

**Routing**:
```go
func (idx *Index) routeDocument(docID string) int {
    hash := murmur3.Sum32([]byte(docID))
    return int(hash % uint32(idx.numShards))
}
```

**Shard Size**:
- Target: 20-40 GB per shard
- Max: 50 GB (triggers split)
- Reason: Balance query parallelism vs merge overhead

**Replica Strategy**:
- 1 replica (default): 2 copies total
- 2 replicas: 3 copies total
- Cross-zone distribution (Kubernetes zone affinity)

---

### 10.3 Translog & Durability

**Translog (Transaction Log)**:
- Append-only log of indexing operations
- Persisted before in-memory buffer
- Used for crash recovery

**Durability Guarantees**:
```yaml
translog:
  durability: REQUEST  # or ASYNC
  sync_interval: 5s    # ASYNC only
  flush_threshold: 512mb
```

**REQUEST Mode** (default):
- fsync() after every indexing request
- Guaranteed durability
- Lower throughput (~10k docs/sec)

**ASYNC Mode**:
- fsync() every 5s
- Higher throughput (~100k docs/sec)
- Risk: 5s data loss on crash

---

## 11. Migration from OpenSearch

### 11.1 Compatibility Layer

**Snapshot & Restore**:
```http
# Export from OpenSearch
POST /_snapshot/s3-repo/opensearch-snapshot/_snapshot

# Import to Quidditch
POST /_snapshot/s3-repo/opensearch-snapshot/_restore
{
  "indices": "*",
  "include_global_state": true
}
```

**Reindex API**:
```http
POST /_reindex
{
  "source": {
    "remote": {
      "host": "https://opensearch-cluster:9200",
      "username": "admin",
      "password": "admin"
    },
    "index": "source-index"
  },
  "dest": {
    "index": "dest-index"
  }
}
```

---

### 11.2 Migration Checklist

1. **Pre-Migration**:
   - [ ] Audit OpenSearch indices, mappings, settings
   - [ ] Identify custom plugins (ingest, analyzers, etc.)
   - [ ] Test Quidditch on sample data
   - [ ] Plan downtime window

2. **Migration**:
   - [ ] Snapshot OpenSearch indices
   - [ ] Deploy Quidditch cluster
   - [ ] Restore snapshots to Quidditch
   - [ ] Re-implement custom plugins as Python pipelines
   - [ ] Update application clients (API endpoints)

3. **Post-Migration**:
   - [ ] Run parallel queries (OpenSearch vs Quidditch)
   - [ ] Validate result correctness
   - [ ] Monitor performance metrics
   - [ ] Cutover traffic to Quidditch
   - [ ] Decommission OpenSearch

---

## 12. Performance Targets

### 12.1 Indexing Performance

| Metric | Target | Notes |
|--------|--------|-------|
| **Throughput** | 100k docs/sec/node | Bulk API, 100 bytes/doc |
| **Latency (p99)** | <500ms | Single document index |
| **Refresh Interval** | 1s (configurable) | Near real-time search |
| **Translog fsync** | <5ms | REQUEST mode |

### 12.2 Query Performance

| Metric | Target | Notes |
|--------|--------|-------|
| **TermQuery (p99)** | <10ms | Single shard |
| **BooleanQuery (p99)** | <50ms | 5 clauses, 10 shards |
| **Aggregation (p99)** | <100ms | Group by 1 field, 10 shards |
| **PPL (p99)** | <200ms | 3-stage pipeline |
| **Cache Hit Rate** | >80% | Query result cache |

### 12.3 SIMD Acceleration

| Operation | Speedup (vs scalar) | Target |
|-----------|---------------------|--------|
| **BM25 Scoring** | 4-8× | AVX2 |
| **Range Filters** | 2-4× | AVX2 |
| **Aggregations** | 2-3× | AVX2 |

### 12.4 Storage Efficiency

| Metric | Target | Notes |
|--------|--------|-------|
| **Compression Ratio** | 3-5× | ZSTD on text fields |
| **Storage Reduction** | 30-40% | vs uncompressed Lucene |
| **Skip Index Pruning** | 90%+ | Granules skipped on filters |

### 12.5 Scalability

| Metric | Target | Notes |
|--------|--------|-------|
| **Max Nodes** | 1000+ | Linear scaling |
| **Max Shards/Node** | 1000 | With resource limits |
| **Max Index Size** | 10+ TB | Distributed across shards |
| **Cluster State Size** | <100 MB | Efficient routing table |

---

## Appendix A: API Reference

### A.1 Cluster APIs

```http
# Cluster Health
GET /_cluster/health

# Cluster Stats
GET /_cluster/stats

# Node Info
GET /_nodes
GET /_nodes/{node_id}

# Node Stats
GET /_nodes/stats
GET /_nodes/{node_id}/stats

# Cluster Settings
GET /_cluster/settings
PUT /_cluster/settings
{
  "persistent": {
    "cluster.routing.allocation.enable": "all"
  }
}
```

### A.2 Index APIs

```http
# Create Index
PUT /my-index
DELETE /my-index
GET /my-index
GET /my-index/_settings
GET /my-index/_mapping

# Open/Close Index
POST /my-index/_open
POST /my-index/_close

# Refresh
POST /my-index/_refresh

# Flush
POST /my-index/_flush

# Force Merge
POST /my-index/_forcemerge?max_num_segments=1
```

### A.3 Search APIs

```http
# Search
GET /my-index/_search
POST /my-index/_search {...}

# Multi-Search
POST /_msearch

# Count
GET /my-index/_count

# Explain
GET /my-index/_explain/{doc_id}

# Field Capabilities
GET /my-index/_field_caps?fields=title,price
```

---

## Appendix B: Configuration Examples

### B.1 Single-Node Config

```yaml
# quidditch-single.yml
node:
  name: node-1
  roles: [master, coordination, data]

cluster:
  name: quidditch-dev
  initial_master_nodes: [node-1]

path:
  data: /var/lib/quidditch/data
  logs: /var/log/quidditch

http:
  port: 9200
  cors_enabled: true

transport:
  port: 9300

index:
  number_of_shards: 1
  number_of_replicas: 0
```

### B.2 Production Cluster Config

```yaml
# quidditch-master.yml
node:
  name: master-1
  roles: [master]

cluster:
  name: quidditch-prod
  initial_master_nodes:
    - master-1
    - master-2
    - master-3

discovery:
  seed_hosts:
    - master-1.quidditch.svc:9300
    - master-2.quidditch.svc:9300
    - master-3.quidditch.svc:9300

path:
  data: /var/lib/quidditch/data
  logs: /var/log/quidditch

transport:
  port: 9300

---

# quidditch-coord.yml
node:
  name: coord-1
  roles: [coordination]

cluster:
  name: quidditch-prod

discovery:
  seed_hosts:
    - master-1.quidditch.svc:9300
    - master-2.quidditch.svc:9300
    - master-3.quidditch.svc:9300

http:
  port: 9200

transport:
  port: 9300

python:
  enabled: true
  virtualenv: /opt/quidditch/venv

---

# quidditch-data.yml
node:
  name: data-1
  roles: [inverted_index, forward_index, computation]

cluster:
  name: quidditch-prod

discovery:
  seed_hosts:
    - master-1.quidditch.svc:9300
    - master-2.quidditch.svc:9300
    - master-3.quidditch.svc:9300

path:
  data: /mnt/nvme/quidditch/data
  logs: /var/log/quidditch

transport:
  port: 9300

diagon:
  ram_buffer_size_mb: 512
  merge_policy: tiered
  compression: zstd
```

---

## Appendix C: Python Pipeline Examples

### C.1 Query Expansion Pipeline

```python
from quidditch.pipeline import Processor
import requests

class QueryExpansionProcessor(Processor):
    """Expand query with synonyms from external service"""

    def __init__(self, synonym_service_url):
        self.synonym_service_url = synonym_service_url

    def process_request(self, request):
        query = request.query

        if 'match' in query:
            field, text = next(iter(query['match'].items()))
            synonyms = self.get_synonyms(text)

            if synonyms:
                request.query = {
                    'bool': {
                        'should': [
                            {'match': {field: text}},
                            {'match': {field: ' '.join(synonyms)}}
                        ],
                        'minimum_should_match': 1
                    }
                }

        return request

    def get_synonyms(self, text):
        response = requests.get(
            f"{self.synonym_service_url}/synonyms",
            params={'q': text}
        )
        return response.json().get('synonyms', [])
```

### C.2 ML Re-Ranking Pipeline

```python
from quidditch.pipeline import Processor
import onnxruntime as ort
import numpy as np

class MLRerankProcessor(Processor):
    """Re-rank results using ONNX ML model"""

    def __init__(self, model_path):
        self.session = ort.InferenceSession(model_path)

    def process_response(self, response, request):
        hits = response['hits']['hits']

        # Extract features for each hit
        features = []
        for hit in hits:
            features.append([
                hit['_score'],
                hit['_source'].get('view_count', 0),
                hit['_source'].get('like_count', 0),
                self.text_similarity(request.query_text, hit['_source']['title'])
            ])

        # Run inference
        features_array = np.array(features, dtype=np.float32)
        scores = self.session.run(None, {'features': features_array})[0]

        # Update scores and re-sort
        for hit, score in zip(hits, scores):
            hit['_score'] = float(score)

        hits.sort(key=lambda x: x['_score'], reverse=True)

        return response

    def text_similarity(self, query, title):
        # Simple overlap similarity
        query_terms = set(query.lower().split())
        title_terms = set(title.lower().split())
        if not query_terms:
            return 0.0
        return len(query_terms & title_terms) / len(query_terms)
```

---

## Appendix D: Glossary

| Term | Definition |
|------|------------|
| **BM25** | Best Match 25, a ranking function for text search |
| **Calcite** | Apache Calcite, SQL parser and query optimizer |
| **Diagon** | Underlying search engine core (Lucene + ClickHouse hybrid) |
| **DSL** | Domain Specific Language (OpenSearch Query DSL) |
| **FST** | Finite State Transducer, compact term dictionary structure |
| **Granule** | ClickHouse concept, fixed-size block of rows (default 8192) |
| **PPL** | Piped Processing Language, SQL-like query language |
| **Raft** | Consensus algorithm for distributed systems |
| **Shard** | Horizontal partition of an index |
| **SIMD** | Single Instruction Multiple Data, CPU parallel operations |
| **Skip Index** | Secondary index for granule pruning (MinMax, BloomFilter) |
| **Translog** | Transaction log for durability |
| **VByte** | Variable-byte encoding for integer compression |

---

## Next Steps

1. **Design Review**: Review this architecture with stakeholders
2. **Prototype**: Build proof-of-concept (single-node Go + Diagon)
3. **Distributed Prototype**: Add master/coordination nodes, shard routing
4. **Calcite Integration**: Integrate Apache Calcite for query planning
5. **Python Integration**: Embed CPython, build pipeline framework
6. **Kubernetes Operator**: Develop custom operator for deployment
7. **Production Hardening**: Monitoring, security, performance tuning
8. **Documentation**: API docs, deployment guides, migration guides

**Estimated Timeline**: 12-18 months with a team of 5-8 engineers

---

**Document Version**: 1.0.0-design
**Last Updated**: 2026-01-25
**Author**: Architecture Team
**Status**: Design Phase - Pending Review
