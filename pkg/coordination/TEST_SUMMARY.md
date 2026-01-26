# Coordination Node Test Suite

**Last Updated**: 2026-01-25
**Total Test Files**: 2 (coordination_test.go + parser tests)
**Total Lines**: ~1,250 lines
**Test Coverage**: REST API, Query Parsing, Endpoints

---

## Overview

Comprehensive test suite for the coordination node components, covering:
- REST API endpoints (OpenSearch-compatible)
- Query DSL parsing and validation
- HTTP request/response handling
- Error handling and validation

---

## Test Files

### 1. Coordination Tests (`pkg/coordination/coordination_test.go`)

**Tests**: 35 test functions + 2 benchmarks
**Coverage**: REST API endpoints, lifecycle, query integration

#### Construction Tests
- ✅ `TestNewCoordinationNode` - Basic node creation
- ✅ `TestNewCoordinationNodeNilLogger` - Error handling for nil logger

#### Endpoint Tests - Root & Cluster APIs
- ✅ `TestHandleRoot` - Root endpoint (/)
- ✅ `TestHandleClusterHealth` - Cluster health (with/without index)
- ✅ `TestHandleClusterState` - Cluster state
- ✅ `TestHandleClusterStats` - Cluster statistics
- ✅ `TestHandleClusterSettings` - Cluster settings update

#### Index Management Tests
- ✅ `TestHandleCreateIndex` - Create index with settings
- ✅ `TestHandleDeleteIndex` - Delete index
- ✅ `TestHandleGetIndex` - Get index metadata
- ✅ `TestHandleIndexExists` - HEAD request for index existence
- ✅ `TestHandleOpenIndex` - Open closed index
- ✅ `TestHandleCloseIndex` - Close index
- ✅ `TestHandleRefreshIndex` - Refresh index
- ✅ `TestHandleFlushIndex` - Flush index

#### Mapping & Settings Tests
- ✅ `TestHandleGetMapping` - Get index mappings
- ✅ `TestHandlePutMapping` - Update mappings
- ✅ `TestHandleGetSettings` - Get index settings
- ✅ `TestHandlePutSettings` - Update settings

#### Document API Tests
- ✅ `TestHandleIndexDocument` - Index document (PUT/POST)
- ✅ `TestHandleGetDocument` - Get document by ID
- ✅ `TestHandleDeleteDocument` - Delete document
- ✅ `TestHandleUpdateDocument` - Update document

#### Bulk & Search Tests
- ✅ `TestHandleBulk` - Bulk operations (with/without index)
- ✅ `TestHandleSearch` - Search API (GET/POST, with/without index)
- ✅ `TestHandleSearchWithComplexQuery` - Match query parsing
- ✅ `TestHandleSearchWithBoolQuery` - Bool query parsing
- ✅ `TestHandleSearchWithInvalidQuery` - Error handling for invalid JSON
- ✅ `TestHandleMultiSearch` - Multi-search API
- ✅ `TestHandleCount` - Count API (GET/POST)

#### Nodes API Tests
- ✅ `TestHandleNodes` - Nodes information
- ✅ `TestHandleNodesStats` - Nodes statistics

#### Lifecycle Tests
- ✅ `TestCoordinationNodeStopWithoutStart` - Safe stop without start
- ✅ `TestCoordinationNodeStartStop` - Full start/stop cycle (integration)

#### Route Configuration Tests
- ✅ `TestRouteSetup` - Verify all routes are registered

#### Benchmark Tests
- ✅ `BenchmarkHandleSearch` - Search endpoint performance
- ✅ `BenchmarkHandleClusterHealth` - Cluster health performance

---

### 2. Parser Tests (`pkg/coordination/parser/parser_test.go`)

**Tests**: 15+ test functions
**Coverage**: Query DSL parsing, validation, complexity estimation

See parser_test.go for complete parser test coverage:
- Match queries
- Term queries
- Bool queries (nested)
- Range queries
- Wildcard queries
- Prefix queries
- And 7 more query types

---

## Test Execution

### Run All Coordination Tests

```bash
# Run all tests
go test ./pkg/coordination/...

# Verbose output
go test -v ./pkg/coordination/...

# Short mode (skip integration tests)
go test -short ./pkg/coordination/...

# With coverage
go test -cover ./pkg/coordination/...
go test -coverprofile=coverage.out ./pkg/coordination/...
go tool cover -html=coverage.out
```

### Run Specific Test Files

```bash
# Coordination tests only
go test ./pkg/coordination -run TestCoordination

# Parser tests only
go test ./pkg/coordination/parser

# Search endpoint tests only
go test ./pkg/coordination -run TestHandleSearch
```

### Run Benchmarks

```bash
# All benchmarks
go test -bench=. ./pkg/coordination/...

# Specific benchmark
go test -bench=BenchmarkHandleSearch ./pkg/coordination
```

---

## Test Coverage Areas

### Covered ✅

1. **REST API Endpoints**
   - All 30+ OpenSearch-compatible endpoints
   - Request/response format validation
   - HTTP method handling (GET, POST, PUT, DELETE, HEAD)
   - Content-Type validation

2. **Query Parsing**
   - DSL query parsing (13 query types)
   - Query validation
   - Error handling for malformed JSON
   - Complex nested queries (bool with must/should/filter)

3. **Document Operations**
   - Index document (with/without ID)
   - Get document
   - Update document
   - Delete document
   - Bulk operations

4. **Index Management**
   - Create/delete index
   - Open/close index
   - Refresh/flush
   - Mappings and settings CRUD

5. **Cluster Operations**
   - Health checks
   - Cluster state queries
   - Node statistics
   - Settings management

6. **Error Handling**
   - Invalid JSON parsing
   - Malformed queries
   - Nil logger validation

### Not Covered Yet ⚠️

1. **Master Integration**
   - Actual gRPC calls to master node
   - Cluster state synchronization
   - Index creation through master

2. **Data Node Integration**
   - Query routing to data nodes
   - Result aggregation from shards
   - Document routing to shards

3. **Complex Scenarios**
   - Concurrent request handling
   - Large bulk operations (1000+ docs)
   - Query timeout handling
   - Circuit breaker patterns

---

## Integration Test Notes

Some tests are marked with `testing.Short()` check:

```go
if testing.Short() {
    t.Skip("Skipping integration test in short mode")
}
```

**Integration tests** (require actual HTTP server):
- `TestCoordinationNodeStartStop`

**Run integration tests**:
```bash
go test ./pkg/coordination/... -timeout 30s
```

**Skip integration tests**:
```bash
go test -short ./pkg/coordination/...
```

---

## Test Helpers

### createTestNode
Creates a test coordination node with default config:
```go
func createTestNode(t *testing.T) *CoordinationNode {
    logger, _ := zap.NewDevelopment()
    cfg := &config.CoordinationConfig{
        NodeID:     "test-coord",
        BindAddr:   "127.0.0.1",
        RESTPort:   9200,
        MasterAddr: "127.0.0.1:9300",
    }
    node, err := NewCoordinationNode(cfg, logger)
    // ...
}
```

### HTTP Test Pattern
Standard pattern for testing endpoints:
```go
req, _ := http.NewRequest("GET", "/endpoint", nil)
w := httptest.NewRecorder()
node.router.ServeHTTP(w, req)

if w.Code != http.StatusOK {
    t.Errorf("Expected status 200, got %d", w.Code)
}

var response map[string]interface{}
json.Unmarshal(w.Body.Bytes(), &response)
```

---

## Test Data Patterns

### Sample Index Creation Request
```json
{
    "settings": {
        "number_of_shards": 5,
        "number_of_replicas": 1
    },
    "mappings": {
        "properties": {
            "title": {"type": "text"},
            "age": {"type": "integer"}
        }
    }
}
```

### Sample Search Request
```json
{
    "query": {
        "match": {
            "title": "test query"
        }
    },
    "size": 10,
    "from": 0
}
```

### Sample Bool Query
```json
{
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
}
```

---

## Code Quality

### Test Best Practices ✅
- Table-driven tests where appropriate
- Clear test names describing what is tested
- HTTP test recorder for API testing
- Proper JSON marshaling/unmarshaling
- Isolated tests (no shared state)
- Benchmark tests for performance-critical endpoints

### Test Organization
```
pkg/coordination/
├── coordination.go
├── coordination_test.go       ← 35 tests, 2 benchmarks
├── parser/
│   ├── parser.go
│   ├── types.go
│   └── parser_test.go        ← 15+ tests
└── TEST_SUMMARY.md           ← This file
```

---

## Next Steps

### Immediate
1. ✅ Coordination API tests complete
2. ✅ Parser tests complete
3. **Add integration tests** (NEXT)
   - Master gRPC integration
   - Data node query routing
   - End-to-end search flow

### Short Term
4. **Coverage Analysis**
   - Run coverage report
   - Target 90%+ coverage
   - Add missing edge cases

5. **Performance Tests**
   - Benchmark all endpoints
   - Load testing with wrk/ab
   - Concurrent request handling

### Medium Term
6. **Chaos Testing**
   - Master node failures
   - Network timeouts
   - Invalid response handling

---

## CI/CD Integration

### GitHub Actions Workflow

```yaml
name: Coordination Node Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.22'

      - name: Run tests
        run: go test -v -race -coverprofile=coverage.txt ./pkg/coordination/...

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.txt
```

---

## Summary Statistics

| Component | Test Functions | Lines | Coverage Target |
|-----------|---------------|-------|--------------------|
| Coordination API | 35 | ~750 | 85%+ |
| Parser | 15+ | ~500 | 95%+ |
| **Total** | **50+** | **~1,250** | **90%+** |

---

## Test Coverage by Category

| Category | Tests | Status |
|----------|-------|--------|
| Construction | 2 | ✅ Complete |
| Cluster APIs | 5 | ✅ Complete |
| Index Management | 8 | ✅ Complete |
| Mappings & Settings | 4 | ✅ Complete |
| Document APIs | 4 | ✅ Complete |
| Search APIs | 5 | ✅ Complete |
| Bulk & Multi-search | 2 | ✅ Complete |
| Nodes APIs | 2 | ✅ Complete |
| Lifecycle | 2 | ✅ Complete |
| Route Config | 1 | ✅ Complete |
| Benchmarks | 2 | ✅ Complete |

---

**Status**: ✅ Complete
**Test Quality**: High
**Ready for**: CI/CD integration, code review, integration testing

---

Last updated: 2026-01-25
