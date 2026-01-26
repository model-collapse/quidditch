# Integration Test Framework

**Last Updated**: 2026-01-25
**Purpose**: Multi-node cluster integration testing
**Coverage**: Cluster formation, leader election, REST API, index operations

---

## Overview

The integration test framework provides comprehensive testing for multi-node Quidditch clusters. It handles the lifecycle of master nodes, coordination nodes, and data nodes, allowing you to test real distributed scenarios.

---

## Features

### Test Cluster Management
- **Automatic Node Creation**: Master, coordination, and data nodes
- **Port Allocation**: Automatic port assignment to avoid conflicts
- **Lifecycle Management**: Start, stop, and cleanup
- **Leader Election**: Automatic waiting for Raft leader election
- **Temporary Directories**: Isolated test data directories

### Test Utilities
- **HTTP Client**: REST API testing helpers
- **Retry Logic**: Exponential backoff for flaky operations
- **Assertions**: HTTP status, JSON fields, eventual conditions
- **Helper Functions**: Index creation, search, document operations

### Test Coverage
- **Cluster Formation**: Multi-node cluster setup (3-1-2 topology)
- **Leader Election**: Raft consensus testing
- **Index Operations**: Create, read, delete indices
- **Search API**: Query parsing and execution
- **REST API**: All OpenSearch-compatible endpoints
- **State Consistency**: Cross-node state verification

---

## Quick Start

### Run All Integration Tests

```bash
# Run all integration tests
go test ./test/integration/... -v

# Run with timeout (recommended)
go test ./test/integration/... -v -timeout 5m

# Run in short mode (skips integration tests)
go test -short ./test/integration/... -v
```

### Run Specific Tests

```bash
# Test cluster formation only
go test ./test/integration -run TestClusterFormation -v

# Test leader election
go test ./test/integration -run TestLeaderElection -v

# Test REST API
go test ./test/integration -run TestRESTAPI -v
```

---

## Architecture

### Test Cluster Structure

```
TestCluster
├── Master Nodes (3)
│   ├── master-0 (Raft Port: 19300, gRPC Port: 19400)
│   ├── master-1 (Raft Port: 19301, gRPC Port: 19401)
│   └── master-2 (Raft Port: 19302, gRPC Port: 19402)
├── Coordination Nodes (1)
│   └── coord-0 (REST Port: 19500)
└── Data Nodes (2)
    ├── data-0 (gRPC Port: 19600)
    └── data-1 (gRPC Port: 19601)
```

### File Structure

```
test/integration/
├── framework.go       ← Test cluster management
├── cluster_test.go    ← Cluster-level tests
├── api_test.go        ← REST API tests
├── helpers.go         ← Test utilities
└── README.md          ← This file
```

---

## Usage Examples

### Example 1: Basic Cluster Test

```go
func TestMyCluster(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // Create cluster with default config (3-1-2)
    cfg := DefaultClusterConfig()
    cluster, err := NewTestCluster(t, cfg)
    if err != nil {
        t.Fatalf("Failed to create cluster: %v", err)
    }
    defer cluster.Stop()

    ctx := context.Background()

    // Start all nodes
    if err := cluster.Start(ctx); err != nil {
        t.Fatalf("Failed to start cluster: %v", err)
    }

    // Wait for cluster to be ready
    if err := cluster.WaitForClusterReady(10 * time.Second); err != nil {
        t.Fatalf("Cluster not ready: %v", err)
    }

    // Get the leader
    leader := cluster.GetLeader()
    t.Logf("Leader: %s", leader.Config.NodeID)

    // Your test logic here...
}
```

### Example 2: Custom Cluster Configuration

```go
func TestCustomCluster(t *testing.T) {
    // Create a small cluster (1-1-1)
    cfg := &ClusterConfig{
        NumMasters:      1,
        NumCoordination: 1,
        NumData:         1,
        StartPorts: PortRange{
            MasterRaftBase: 20300,
            MasterGRPCBase: 20400,
            CoordRESTBase:  20500,
            DataGRPCBase:   20600,
        },
    }

    cluster, err := NewTestCluster(t, cfg)
    // ... rest of test
}
```

### Example 3: REST API Testing

```go
func TestAPIEndpoint(t *testing.T) {
    // Set up cluster...
    cluster, _ := NewTestCluster(t, DefaultClusterConfig())
    defer cluster.Stop()

    cluster.Start(context.Background())
    cluster.WaitForClusterReady(10 * time.Second)

    // Create HTTP client
    coordURL := GetCoordNodeURL(cluster, 0)
    client := NewHTTPClient(t, coordURL)

    // Create an index
    err := CreateTestIndex(t, client, "test-index", 5, 1)
    if err != nil {
        t.Fatalf("Failed to create index: %v", err)
    }

    // Search
    query := map[string]interface{}{
        "match_all": map[string]interface{}{},
    }
    results, err := SearchIndex(t, client, "test-index", query)
    t.Logf("Search took: %d ms", results.Took)
}
```

### Example 4: Index Operations

```go
func TestIndexOperations(t *testing.T) {
    // Set up cluster...

    // Get leader
    leader := cluster.GetLeader()
    ctx := context.Background()

    // Create index through Raft
    err := leader.Node.CreateIndex(ctx, "my-index", 5, 1)
    if err != nil {
        t.Fatalf("Failed to create index: %v", err)
    }

    // Verify through cluster state
    state, _ := leader.Node.GetClusterState(ctx)
    if _, exists := state.Indices["my-index"]; !exists {
        t.Error("Index not found in cluster state")
    }

    // Delete index
    err = leader.Node.DeleteIndex(ctx, "my-index")
    if err != nil {
        t.Fatalf("Failed to delete index: %v", err)
    }
}
```

---

## Test Framework API

### TestCluster Methods

#### Lifecycle
- `NewTestCluster(t, cfg)` - Create new test cluster
- `Start(ctx)` - Start all nodes
- `Stop()` - Stop all nodes and cleanup

#### Node Access
- `GetLeader()` - Get current Raft leader
- `GetMasterNode(index)` - Get master node by index
- `GetCoordNode(index)` - Get coordination node by index
- `GetDataNode(index)` - Get data node by index

#### Status & Waiting
- `WaitForLeader(timeout)` - Wait for leader election
- `WaitForClusterReady(timeout)` - Wait for full cluster readiness
- `Uptime()` - Get cluster uptime

#### Queries
- `NumMasterNodes()` - Number of master nodes
- `NumCoordNodes()` - Number of coordination nodes
- `NumDataNodes()` - Number of data nodes

### Helper Functions

#### HTTP Operations
- `NewHTTPClient(t, baseURL)` - Create HTTP client
- `Get(path)` - HTTP GET request
- `Post(path, body)` - HTTP POST request
- `Put(path, body)` - HTTP PUT request
- `Delete(path)` - HTTP DELETE request

#### High-Level Operations
- `CreateTestIndex(t, client, name, shards, replicas)` - Create index
- `SearchIndex(t, client, index, query)` - Perform search
- `IndexDocument(t, client, index, id, doc)` - Index document
- `GetClusterHealth(t, client)` - Get cluster health

#### Assertions
- `AssertHTTPStatus(t, resp, status)` - Assert HTTP status code
- `AssertJSONField(t, data, field, expected)` - Assert JSON field value
- `AssertEventually(t, timeout, condition, msg)` - Assert condition becomes true

#### Utilities
- `RetryWithBackoff(ctx, maxRetries, delay, fn)` - Retry with backoff
- `WaitForCondition(t, timeout, interval, condition)` - Wait for condition

---

## Test Scenarios

### 1. Cluster Formation Tests (`cluster_test.go`)
- **TestClusterFormation**: Basic 3-1-2 cluster setup
- **TestLeaderElection**: Raft leader election
- **TestSmallCluster**: Minimal 1-1-1 cluster

### 2. Index Management Tests
- **TestCreateIndex**: Index creation through Raft
- **TestDeleteIndex**: Index deletion
- **TestMultipleIndices**: Multiple index management
- **TestRegisterDataNode**: Data node registration

### 3. State Consistency Tests
- **TestClusterStateConsistency**: Cross-node state verification

### 4. REST API Tests (`api_test.go`)
- **TestRESTAPIRootEndpoint**: Root endpoint (/)
- **TestRESTAPIClusterHealth**: Cluster health API
- **TestRESTAPIClusterState**: Cluster state API
- **TestRESTAPIIndexCRUD**: Index create/read/delete
- **TestRESTAPISearch**: Search with match_all
- **TestRESTAPISearchWithQuery**: Search with match and bool queries
- **TestRESTAPINodes**: Nodes info and stats
- **TestRESTAPICount**: Count API

---

## Configuration

### Default Cluster Config

```go
ClusterConfig{
    NumMasters:      3,
    NumCoordination: 1,
    NumData:         2,
    StartPorts: PortRange{
        MasterRaftBase: 19300,
        MasterGRPCBase: 19400,
        CoordRESTBase:  19500,
        DataGRPCBase:   19600,
    },
}
```

### Custom Configuration

Customize cluster topology and ports:

```go
cfg := &ClusterConfig{
    NumMasters:      5,  // 5-node master cluster
    NumCoordination: 2,  // 2 coordination nodes
    NumData:         4,  // 4 data nodes
    StartPorts: PortRange{
        MasterRaftBase: 25000,
        MasterGRPCBase: 25100,
        CoordRESTBase:  25200,
        DataGRPCBase:   25300,
    },
}
```

---

## Best Practices

### 1. Always Use Short Mode Check

```go
func TestMyIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    // ... test code
}
```

### 2. Always Defer Cleanup

```go
cluster, err := NewTestCluster(t, cfg)
if err != nil {
    t.Fatalf("Failed to create cluster: %v", err)
}
defer cluster.Stop()  // Always cleanup
```

### 3. Use Appropriate Timeouts

```go
// Wait for leader election (typically < 5s)
cluster.WaitForLeader(10 * time.Second)

// Wait for full cluster readiness (may take longer)
cluster.WaitForClusterReady(30 * time.Second)
```

### 4. Log Important Events

```go
leader := cluster.GetLeader()
t.Logf("Leader elected: %s", leader.Config.NodeID)
t.Logf("Cluster uptime: %v", cluster.Uptime())
```

### 5. Check Errors Properly

```go
if err != nil {
    t.Fatalf("Operation failed: %v", err)  // Use Fatalf for setup
}

if condition {
    t.Errorf("Unexpected state: %v", value)  // Use Errorf for assertions
}
```

---

## Troubleshooting

### Tests Hang or Timeout

**Problem**: Tests hang waiting for leader election or cluster readiness.

**Solutions**:
- Increase timeout values
- Check port conflicts (use different port ranges)
- Verify Go is properly installed
- Check system resources (file descriptors, memory)

### Port Already in Use

**Problem**: Tests fail with "address already in use" error.

**Solutions**:
- Run tests sequentially: `go test -p 1 ./test/integration/...`
- Use different port ranges in `ClusterConfig`
- Kill stale processes: `killall quidditch-*`

### Flaky Tests

**Problem**: Tests pass sometimes and fail other times.

**Solutions**:
- Use `RetryWithBackoff` for operations that may be temporarily unavailable
- Increase wait times for leader election
- Add `time.Sleep()` after critical operations
- Use `AssertEventually` instead of direct assertions

### Cluster Won't Start

**Problem**: `cluster.Start()` returns an error.

**Solutions**:
- Check logs for specific error messages
- Verify data directories are writable
- Ensure no previous test instances are running
- Check that all ports are available

---

## Performance Considerations

### Test Execution Time

Typical execution times:
- **Small cluster (1-1-1)**: 5-10 seconds
- **Standard cluster (3-1-2)**: 10-20 seconds
- **Large cluster (5-2-4)**: 20-40 seconds

### Resource Usage

Per node approximate usage:
- **Memory**: 50-100 MB
- **CPU**: 5-10% during startup
- **Disk**: 10-50 MB (temporary)
- **File Descriptors**: 20-50

### Optimization Tips

1. **Use Small Clusters**: Test with 1-1-1 when possible
2. **Run in Parallel**: Use `go test -parallel N` (carefully)
3. **Skip in Short Mode**: Always implement short mode checks
4. **Reuse Clusters**: Share cluster across multiple test cases when safe

---

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Integration Tests

on: [push, pull_request]

jobs:
  integration:
    runs-on: ubuntu-latest
    timeout-minutes: 30

    steps:
      - uses: actions/checkout@v3

      - uses: actions/setup-go@v4
        with:
          go-version: '1.22'

      - name: Run integration tests
        run: go test ./test/integration/... -v -timeout 20m

      - name: Upload test logs
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: test-logs
          path: /tmp/quidditch-test-*
```

---

## Future Enhancements

### Planned Features
- [ ] Docker-based test clusters
- [ ] Kubernetes test environment
- [ ] Chaos testing (node failures, network partitions)
- [ ] Performance benchmarking
- [ ] Load testing utilities
- [ ] Multi-datacenter simulation
- [ ] Backup/restore testing

---

## Summary

| Component | Tests | Status |
|-----------|-------|--------|
| Framework | Complete | ✅ |
| Cluster Tests | 8 tests | ✅ |
| API Tests | 9 tests | ✅ |
| Helpers | Complete | ✅ |
| **Total** | **17 tests** | ✅ |

**Test Coverage**: Cluster formation, leader election, index operations, REST API, state consistency

**Ready for**: Development, CI/CD, regression testing

---

Last updated: 2026-01-25
