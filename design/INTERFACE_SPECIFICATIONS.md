# Quidditch Interface Specifications

**Technical Specifications for Inter-Component Communication**

**Version**: 1.0.0
**Date**: 2026-01-25

---

## Table of Contents

1. [Overview](#overview)
2. [Protocol Specifications](#protocol-specifications)
3. [Master Node Interfaces](#master-node-interfaces)
4. [Coordination Node Interfaces](#coordination-node-interfaces)
5. [Data Node Interfaces](#data-node-interfaces)
6. [Client Interfaces](#client-interfaces)
7. [Storage Interfaces](#storage-interfaces)
8. [Error Handling](#error-handling)

---

## Overview

### Communication Patterns

```
┌─────────────────────────────────────────────────────────┐
│                 Communication Matrix                     │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  Client ←─ HTTP/REST ─→ Coordination Node                │
│                                                           │
│  Coordination ←─ gRPC ─→ Master Node                     │
│                                                           │
│  Coordination ←─ gRPC ─→ Data Node                       │
│                                                           │
│  Master ←─ Raft ─→ Master (cluster state)                │
│                                                           │
│  Data Node ←─ gRPC ─→ Data Node (shard operations)       │
│                                                           │
│  All Nodes ←─ HTTP ─→ Monitoring (metrics)               │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

### Protocol Summary

| Interface | Protocol | Port | Use Case |
|-----------|----------|------|----------|
| **Client → Coordination** | HTTP/REST | 9200 | OpenSearch API |
| **Coordination → Master** | gRPC | 9300 | Cluster metadata |
| **Coordination → Data** | gRPC | 9300 | Query execution |
| **Master → Master** | Raft/gRPC | 9301 | Consensus |
| **Data → Data** | gRPC | 9300 | Shard operations |
| **All → Monitoring** | HTTP | 9600 | Prometheus metrics |

---

## Protocol Specifications

### 1. gRPC Service Definitions

#### Master Service (master.proto)

```protobuf
syntax = "proto3";

package quidditch.master;

service MasterService {
  // Cluster state operations
  rpc GetClusterState(GetClusterStateRequest) returns (ClusterStateResponse);
  rpc UpdateClusterState(UpdateClusterStateRequest) returns (UpdateClusterStateResponse);

  // Index operations
  rpc CreateIndex(CreateIndexRequest) returns (CreateIndexResponse);
  rpc DeleteIndex(DeleteIndexRequest) returns (DeleteIndexResponse);
  rpc UpdateMapping(UpdateMappingRequest) returns (UpdateMappingResponse);

  // Shard operations
  rpc AllocateShard(AllocateShardRequest) returns (AllocateShardResponse);
  rpc RelocateShard(RelocateShardRequest) returns (RelocateShardResponse);

  // Node operations
  rpc RegisterNode(RegisterNodeRequest) returns (RegisterNodeResponse);
  rpc NodeHeartbeat(NodeHeartbeatRequest) returns (NodeHeartbeatResponse);
}

message ClusterStateResponse {
  int64 version = 1;
  repeated IndexMetadata indices = 2;
  RoutingTable routing_table = 3;
  repeated NodeInfo nodes = 4;
  ClusterMetadata metadata = 5;
}

message IndexMetadata {
  string name = 1;
  string uuid = 2;
  IndexSettings settings = 3;
  MappingMetadata mappings = 4;
  int32 number_of_shards = 5;
  int32 number_of_replicas = 6;
  IndexState state = 7;
  int64 creation_time = 8;
}

message IndexSettings {
  string codec = 1;
  string refresh_interval = 2;
  int32 max_result_window = 3;
  map<string, string> custom_settings = 4;
}

message MappingMetadata {
  map<string, FieldMapping> properties = 1;
}

message FieldMapping {
  string type = 1;  // text, keyword, float, etc.
  string analyzer = 2;
  bool store = 3;
  bool index = 4;
  map<string, FieldMapping> fields = 5;  // Multi-fields
  map<string, string> custom_attributes = 6;
}

message RoutingTable {
  map<string, ShardRouting> shards = 1;  // key: "index_name:shard_id"
}

message ShardRouting {
  string index = 1;
  int32 shard_id = 2;
  bool primary = 3;
  string node_id = 4;
  ShardState state = 5;
  string relocating_node_id = 6;
}

enum ShardState {
  UNASSIGNED = 0;
  INITIALIZING = 1;
  STARTED = 2;
  RELOCATING = 3;
  CLOSED = 4;
}

enum IndexState {
  OPEN = 0;
  CLOSE = 1;
  DELETE = 2;
}

message NodeInfo {
  string node_id = 1;
  string node_name = 2;
  string host = 3;
  int32 port = 4;
  repeated NodeRole roles = 5;
  map<string, string> attributes = 6;
}

enum NodeRole {
  MASTER = 0;
  COORDINATION = 1;
  DATA = 2;
  INGEST = 3;
}
```

---

#### Query Service (query.proto)

```protobuf
syntax = "proto3";

package quidditch.query;

service QueryService {
  // Search operations
  rpc Search(SearchRequest) returns (SearchResponse);
  rpc MultiSearch(MultiSearchRequest) returns (MultiSearchResponse);
  rpc Count(CountRequest) returns (CountResponse);

  // Document operations
  rpc Get(GetRequest) returns (GetResponse);
  rpc MultiGet(MultiGetRequest) returns (MultiGetResponse);
  rpc Index(IndexRequest) returns (IndexResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
  rpc Update(UpdateRequest) returns (UpdateResponse);
  rpc Bulk(BulkRequest) returns (BulkResponse);
}

message SearchRequest {
  string index = 1;
  QueryNode query = 2;
  repeated FilterNode filters = 3;
  repeated SortField sort = 4;
  int32 size = 5;
  int32 from = 6;
  map<string, Aggregation> aggregations = 7;
  HighlightConfig highlight = 8;
  repeated string source_includes = 9;
  repeated string source_excludes = 10;
  string pipeline = 11;  // Python pipeline name
  UserContext user = 12;
}

message QueryNode {
  oneof query_type {
    MatchAllQuery match_all = 1;
    MatchQuery match = 2;
    TermQuery term = 3;
    RangeQuery range = 4;
    BoolQuery bool = 5;
    FunctionScoreQuery function_score = 6;
  }
}

message MatchQuery {
  string field = 1;
  string query = 2;
  string operator = 3;  // "and" or "or"
  string fuzziness = 4;
  int32 prefix_length = 5;
}

message BoolQuery {
  repeated QueryNode must = 1;
  repeated QueryNode filter = 2;
  repeated QueryNode should = 3;
  repeated QueryNode must_not = 4;
  int32 minimum_should_match = 5;
}

message SearchResponse {
  int64 took_millis = 1;
  bool timed_out = 2;
  ShardInfo shards = 3;
  Hits hits = 4;
  map<string, AggregationResult> aggregations = 5;
}

message Hits {
  TotalHits total = 1;
  float max_score = 2;
  repeated Hit hits = 3;
}

message Hit {
  string index = 1;
  string id = 2;
  float score = 3;
  bytes source = 4;  // JSON document
  map<string, HighlightResult> highlight = 5;
  repeated SortValue sort = 6;
}
```

---

#### Data Node Service (data.proto)

```protobuf
syntax = "proto3";

package quidditch.data;

service DataNodeService {
  // Shard operations
  rpc CreateShard(CreateShardRequest) returns (CreateShardResponse);
  rpc DeleteShard(DeleteShardRequest) returns (DeleteShardResponse);
  rpc RecoverShard(RecoverShardRequest) returns (RecoverShardResponse);

  // Query execution
  rpc ExecuteShardQuery(ShardQueryRequest) returns (ShardQueryResponse);
  rpc ExecuteShardAggregation(ShardAggregationRequest) returns (ShardAggregationResponse);

  // Document operations
  rpc IndexDocument(IndexDocumentRequest) returns (IndexDocumentResponse);
  rpc GetDocument(GetDocumentRequest) returns (GetDocumentResponse);
  rpc DeleteDocument(DeleteDocumentRequest) returns (DeleteDocumentResponse);

  // Shard management
  rpc FlushShard(FlushShardRequest) returns (FlushShardResponse);
  rpc RefreshShard(RefreshShardRequest) returns (RefreshShardResponse);
  rpc MergeShard(MergeShardRequest) returns (MergeShardResponse);

  // Statistics
  rpc GetShardStats(GetShardStatsRequest) returns (GetShardStatsResponse);
}

message ShardQueryRequest {
  string index = 1;
  int32 shard_id = 2;
  QueryNode query = 3;
  repeated FilterNode filters = 4;
  int32 size = 5;
  int32 from = 6;
  repeated SortField sort = 7;
  int64 timeout_millis = 8;
}

message ShardQueryResponse {
  int64 took_millis = 1;
  int32 total_hits = 2;
  repeated ShardHit hits = 3;
  bool timed_out = 4;
}

message ShardHit {
  int32 doc_id = 1;
  float score = 2;
  bytes source = 3;
  repeated SortValue sort = 4;
}

message GetShardStatsResponse {
  int64 doc_count = 1;
  int64 deleted_doc_count = 2;
  int64 size_bytes = 3;
  int32 segment_count = 4;
  map<string, int64> field_stats = 5;
}
```

---

### 2. REST API Specifications

#### OpenSearch-Compatible Endpoints

```yaml
# OpenAPI 3.0 Specification
openapi: 3.0.0
info:
  title: Quidditch REST API
  version: 1.0.0
  description: OpenSearch-compatible REST API

servers:
  - url: http://localhost:9200
    description: Quidditch cluster

paths:
  /{index}:
    put:
      summary: Create index
      parameters:
        - name: index
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateIndexRequest'
      responses:
        '200':
          description: Index created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/CreateIndexResponse'

  /{index}/_search:
    post:
      summary: Search documents
      parameters:
        - name: index
          in: path
          required: true
          schema:
            type: string
        - name: pipeline
          in: query
          required: false
          schema:
            type: string
          description: Python pipeline to apply
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/SearchRequest'
      responses:
        '200':
          description: Search results
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SearchResponse'

  /_bulk:
    post:
      summary: Bulk operations
      requestBody:
        required: true
        content:
          application/x-ndjson:
            schema:
              type: string
              format: ndjson
      responses:
        '200':
          description: Bulk response
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/BulkResponse'

components:
  schemas:
    CreateIndexRequest:
      type: object
      properties:
        settings:
          type: object
          properties:
            number_of_shards:
              type: integer
            number_of_replicas:
              type: integer
            codec:
              type: string
        mappings:
          type: object
          properties:
            properties:
              type: object
              additionalProperties:
                $ref: '#/components/schemas/FieldMapping'

    FieldMapping:
      type: object
      properties:
        type:
          type: string
          enum: [text, keyword, integer, float, date, boolean, object, nested]
        analyzer:
          type: string
        store:
          type: boolean
        index:
          type: boolean
        fields:
          type: object
          additionalProperties:
            $ref: '#/components/schemas/FieldMapping'
```

---

## Master Node Interfaces

### 1. Cluster State Management

**Endpoint**: `MasterService.GetClusterState()`

**Request**:
```json
{
  "include_routing_table": true,
  "include_nodes": true,
  "include_metadata": true,
  "wait_for_version": 123
}
```

**Response**:
```json
{
  "version": 124,
  "indices": {
    "my-index": {
      "uuid": "abc-123",
      "settings": {...},
      "mappings": {...},
      "number_of_shards": 5,
      "number_of_replicas": 1,
      "state": "OPEN"
    }
  },
  "routing_table": {
    "my-index:0": {
      "primary": {"node_id": "node-1", "state": "STARTED"},
      "replicas": [{"node_id": "node-2", "state": "STARTED"}]
    }
  },
  "nodes": {
    "node-1": {
      "name": "data-1",
      "host": "10.0.1.5",
      "port": 9300,
      "roles": ["DATA"]
    }
  }
}
```

---

### 2. Shard Allocation

**Endpoint**: `MasterService.AllocateShard()`

**Request**:
```json
{
  "index": "my-index",
  "shard_id": 0,
  "primary": true,
  "node_id": "node-1",
  "reason": "initial_allocation"
}
```

**Response**:
```json
{
  "acknowledged": true,
  "allocation_id": "alloc-123",
  "node_id": "node-1",
  "shard_routing": {
    "state": "INITIALIZING",
    "primary": true
  }
}
```

---

## Coordination Node Interfaces

### 1. Query Planning

**Internal Interface**: `QueryPlanner.Plan()`

**Input**:
```go
type PlanRequest struct {
    Index       string
    Query       QueryNode
    Filters     []FilterNode
    Aggregations map[string]Aggregation
    Sort        []SortField
    Size        int
    From        int
    User        *UserContext
}
```

**Output**:
```go
type PhysicalPlan struct {
    Stages      []ExecutionStage
    ShardTasks  []ShardTask
    Aggregations []AggregationTask
    Estimated   PlanCost
}

type ShardTask struct {
    Index       string
    ShardID     int
    NodeID      string
    Query       QueryNode
    Filters     []FilterNode
    Size        int
    From        int
    Timeout     time.Duration
}
```

---

### 2. Python Pipeline Execution

**Interface**: Go ↔ Python Bridge

**Go Side**:
```go
type PipelineExecutor interface {
    // Pre-processing
    ProcessRequest(req *SearchRequest) (*SearchRequest, error)

    // Post-processing
    ProcessResponse(resp *SearchResponse, req *SearchRequest) (*SearchResponse, error)
}

// CGO wrapper
func ExecutePipeline(pipelineName string, request *SearchRequest) (*SearchRequest, error) {
    // Convert Go struct to Python dict
    pyRequest := goToPython(request)

    // Call Python function via CGO
    pyModule := C.PyImport_ImportModule(C.CString(pipelineName))
    pyFunc := C.PyObject_GetAttrString(pyModule, C.CString("process_request"))
    pyResult := C.PyObject_CallObject(pyFunc, pyRequest)

    // Convert back
    return pythonToGo(pyResult), nil
}
```

**Python Side**:
```python
from quidditch.pipeline import Processor

class MyProcessor(Processor):
    def process_request(self, request: dict) -> dict:
        # request = {"query": {...}, "filters": [...], "user": {...}}
        # Modify and return
        return request

    def process_response(self, response: dict, request: dict) -> dict:
        # response = {"hits": {...}, "aggregations": {...}}
        # Modify and return
        return response
```

---

## Data Node Interfaces

### 1. Shard Query Execution

**Endpoint**: `DataNodeService.ExecuteShardQuery()`

**Request**:
```protobuf
message ShardQueryRequest {
  string index = "products";
  int32 shard_id = 0;
  QueryNode query = {
    bool: {
      must: [{match: {field: "title", query: "search"}}],
      filter: [{range: {field: "price", gte: 10, lte: 100}}]
    }
  };
  int32 size = 10;
  int32 from = 0;
  int64 timeout_millis = 5000;
}
```

**Response**:
```protobuf
message ShardQueryResponse {
  int64 took_millis = 15;
  int32 total_hits = 152;
  repeated ShardHit hits = [
    {doc_id: 5, score: 3.14, source: {...}},
    {doc_id: 12, score: 2.87, source: {...}},
    ...
  ];
  bool timed_out = false;
}
```

---

### 2. Diagon Core Interface

**C API Wrapper** (Go → C++ bridge):

```c
// diagon_c_api.h

typedef void* DiagonShardHandle;

// Create shard
int diagon_create_shard(
    const char* path,
    const DiagonShardConfig* config,
    DiagonShardHandle* handle
);

// Index document
int diagon_index_document(
    DiagonShardHandle handle,
    int32_t doc_id,
    const char* json_doc,
    size_t json_len
);

// Search
int diagon_search(
    DiagonShardHandle handle,
    const DiagonQuery* query,
    DiagonSearchResults* results
);

// Query structure
typedef struct {
    DiagonQueryType type;  // TERM, BOOL, RANGE, etc.
    union {
        DiagonTermQuery term;
        DiagonBoolQuery bool_query;
        DiagonRangeQuery range;
    } query;
} DiagonQuery;

// Results structure
typedef struct {
    int32_t total_hits;
    DiagonHit* hits;
    int32_t num_hits;
} DiagonSearchResults;

typedef struct {
    int32_t doc_id;
    float score;
    char* source_json;
    size_t source_len;
} DiagonHit;
```

---

## Client Interfaces

### 1. HTTP Client Libraries

**Go Client**:
```go
package quidditch

import "github.com/opensearch-project/opensearch-go"

// Quidditch client is OpenSearch-compatible
type Client struct {
    *opensearch.Client
}

func NewClient(config *Config) (*Client, error) {
    osClient, err := opensearch.NewClient(opensearch.Config{
        Addresses: config.Hosts,
        Username:  config.Username,
        Password:  config.Password,
    })

    return &Client{Client: osClient}, err
}

// All OpenSearch client methods work
func (c *Client) Search(index string, body map[string]interface{}) (*SearchResponse, error) {
    return c.Client.Search(
        c.Client.Search.WithIndex(index),
        c.Client.Search.WithBody(toJSON(body)),
    )
}
```

**Python Client**:
```python
from opensearchpy import OpenSearch

# Quidditch is OpenSearch-compatible
client = OpenSearch(
    hosts=[{'host': 'localhost', 'port': 9200}],
    http_auth=('admin', 'admin'),
    use_ssl=False
)

# All OpenSearch Python client methods work
response = client.search(
    index='my-index',
    body={
        'query': {'match': {'title': 'search'}}
    }
)
```

---

## Storage Interfaces

### 1. Object Storage (S3)

**Interface**: Standard S3 API

**Cold Tier Storage**:
```go
type S3Storage struct {
    client *s3.Client
    bucket string
}

func (s *S3Storage) WriteSegment(segment *Segment) error {
    key := fmt.Sprintf("segments/%s/%s", segment.Index, segment.Name)

    _, err := s.client.PutObject(context.TODO(), &s3.PutObjectInput{
        Bucket: aws.String(s.bucket),
        Key:    aws.String(key),
        Body:   segment.Reader,
    })

    return err
}

func (s *S3Storage) ReadSegment(index, segmentName string) (*Segment, error) {
    key := fmt.Sprintf("segments/%s/%s", index, segmentName)

    result, err := s.client.GetObject(context.TODO(), &s3.GetObjectInput{
        Bucket: aws.String(s.bucket),
        Key:    aws.String(key),
    })

    if err != nil {
        return nil, err
    }

    return &Segment{
        Index:  index,
        Name:   segmentName,
        Reader: result.Body,
    }, nil
}
```

---

### 2. Local Storage

**Interface**: POSIX filesystem

**Directory Structure**:
```
/data/nodes/0/
├── indices/
│   ├── my-index/
│   │   ├── 0/              # Shard 0
│   │   │   ├── diagon/     # Diagon segments
│   │   │   │   ├── segment_0/
│   │   │   │   │   ├── inverted/
│   │   │   │   │   ├── columns/
│   │   │   │   │   └── _metadata.json
│   │   │   │   └── ...
│   │   │   ├── wal/        # Write-ahead log
│   │   │   └── _state/     # Shard state
│   │   └── 1/              # Shard 1
│   └── ...
└── cluster_state/
    └── state.json
```

---

## Error Handling

### Error Codes

```go
const (
    // Client errors (4xx)
    ErrBadRequest          = 400
    ErrUnauthorized        = 401
    ErrForbidden           = 403
    ErrNotFound            = 404
    ErrConflict            = 409
    ErrTooManyRequests     = 429

    // Server errors (5xx)
    ErrInternalServer      = 500
    ErrServiceUnavailable  = 503
    ErrGatewayTimeout      = 504
)
```

### Error Response Format

```json
{
  "error": {
    "type": "index_not_found_exception",
    "reason": "no such index [my-index]",
    "index": "my-index",
    "index_uuid": "_na_"
  },
  "status": 404
}
```

### Retry Policy

```go
type RetryConfig struct {
    MaxRetries      int
    InitialBackoff  time.Duration
    MaxBackoff      time.Duration
    Multiplier      float64
}

// Default retry policy
var DefaultRetryPolicy = RetryConfig{
    MaxRetries:     3,
    InitialBackoff: 100 * time.Millisecond,
    MaxBackoff:     30 * time.Second,
    Multiplier:     2.0,
}

// Retryable errors
func IsRetryable(err error) bool {
    switch grpc.Code(err) {
    case codes.Unavailable,
         codes.DeadlineExceeded,
         codes.ResourceExhausted:
        return true
    default:
        return false
    }
}
```

---

## Versioning & Compatibility

### API Versioning

```http
# Version in URL
GET /v1/my-index/_search

# Version in header
GET /my-index/_search
X-Quidditch-Version: 1
```

### Protocol Buffer Compatibility

```protobuf
// Use field numbers consistently
message IndexMetadata {
  string name = 1;           // Never change field number
  string uuid = 2;           // Never change field number

  // New fields get new numbers
  int64 creation_time = 8;   // Added in v1.1

  // Deprecated fields (don't reuse numbers)
  reserved 7;                // Removed old field
  reserved "old_field_name"; // Name reservation
}
```

---

## Performance Considerations

### Connection Pooling

```go
// gRPC connection pool
type ConnectionPool struct {
    pools map[string]*grpc.ClientConn
    mu    sync.RWMutex
}

func (p *ConnectionPool) GetConnection(target string) (*grpc.ClientConn, error) {
    p.mu.RLock()
    conn, exists := p.pools[target]
    p.mu.RUnlock()

    if exists {
        return conn, nil
    }

    // Create new connection
    conn, err := grpc.Dial(target,
        grpc.WithKeepaliveParams(keepalive.ClientParameters{
            Time:                10 * time.Second,
            Timeout:             3 * time.Second,
            PermitWithoutStream: true,
        }),
    )

    if err != nil {
        return nil, err
    }

    p.mu.Lock()
    p.pools[target] = conn
    p.mu.Unlock()

    return conn, nil
}
```

### Request Timeouts

```go
// Context with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Call with timeout
response, err := client.Search(ctx, &SearchRequest{...})
```

---

## Monitoring Interfaces

### Metrics Endpoint (Prometheus)

```http
GET /metrics HTTP/1.1
Host: localhost:9600

# HELP quidditch_query_total Total number of queries
# TYPE quidditch_query_total counter
quidditch_query_total{index="my-index",node="coord-1"} 12345

# HELP quidditch_query_latency_seconds Query latency in seconds
# TYPE quidditch_query_latency_seconds histogram
quidditch_query_latency_seconds_bucket{le="0.01"} 8932
quidditch_query_latency_seconds_bucket{le="0.05"} 11234
quidditch_query_latency_seconds_bucket{le="0.1"} 12000
```

---

## Security

### Authentication

```http
# Basic Auth
GET /my-index/_search HTTP/1.1
Authorization: Basic dXNlcjpwYXNzd29yZA==

# JWT
GET /my-index/_search HTTP/1.1
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### TLS Configuration

```go
// Server TLS
creds, err := credentials.NewServerTLSFromFile("server.crt", "server.key")
if err != nil {
    log.Fatal(err)
}

grpcServer := grpc.NewServer(grpc.Creds(creds))

// Client TLS
creds, err := credentials.NewClientTLSFromFile("ca.crt", "")
if err != nil {
    log.Fatal(err)
}

conn, err := grpc.Dial("localhost:9300", grpc.WithTransportCredentials(creds))
```

---

**Version**: 1.0.0
**Last Updated**: 2026-01-25
**Status**: Design Specification
