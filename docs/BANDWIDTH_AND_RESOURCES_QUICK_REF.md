# Bandwidth and Resources Quick Reference

**Version**: 1.0
**Date**: 2026-01-26

---

## Master Node Bandwidth

### Per Master Node

| Direction | Source/Dest | Bandwidth | Notes |
|-----------|-------------|-----------|-------|
| **Incoming** | Data nodes (heartbeats) | 1 KB/sec | 100 nodes × 10 bytes/sec |
| | Data nodes (shard health) | 83 bytes/sec | 100 nodes × 50 bytes/min |
| | Coord nodes (routing queries) | 0.7 KB/sec | Cached for 5 min |
| | Raft peers (heartbeats) | 2 KB/sec | 2 peers × 1 KB/sec |
| | API calls (index ops) | 0.1 KB/sec | Low frequency |
| **Incoming Total** | | **~4 KB/sec** | |
| **Outgoing** | Data nodes (allocation) | 0.5 KB/sec | Rare operations |
| | Data nodes (heartbeat acks) | 300 bytes/sec | 100 nodes × 3 bytes/sec |
| | Coord nodes (routing responses) | 0.7 KB/sec | Cached for 5 min |
| | Raft peers (log replication) | 5-10 KB/sec | Variable |
| **Outgoing Total** | | **~12 KB/sec** | |
| **TOTAL** | | **~16 KB/sec = 128 Kbps** | **Extremely low** |

### Master Cluster (3 nodes)

- **Total bandwidth**: 48 KB/sec = 384 Kbps
- **1 Gbps network can handle**: 60,000+ master nodes worth of traffic
- **Bandwidth is NOT a constraint**

---

## Hardware Requirements

### Master Nodes

| Cluster Size | CPU | Memory | Disk | Network | Replicas |
|--------------|-----|--------|------|---------|----------|
| Small (1-3 data nodes) | 2 cores | 2 GB | 20 GB SSD | 100 Mbps | 1 |
| Medium (3-20 data nodes) | 4 cores | 4 GB | 50 GB SSD | 1 Gbps | 3 |
| Large (20-100 data nodes) | 4 cores | 4 GB | 50 GB SSD | 1 Gbps | 3 |
| Very Large (100-500 data nodes) | 8 cores | 8 GB | 100 GB SSD | 1 Gbps | 3 or 5 |

**Why don't masters scale with data nodes?**
- Masters manage **metadata**, not data
- Typical large cluster: ~30 MB cluster state
- 3 master nodes can handle 1000+ data nodes

### Coordination Nodes

| Cluster Size | CPU | Memory | Network | Replicas |
|--------------|-----|--------|---------|----------|
| Small | 2 cores | 4 GB | 1 Gbps | 1 |
| Medium | 2 cores | 4 GB | 10 Gbps | 3 |
| Large | 4 cores | 8 GB | 10 Gbps | 3-5 |
| Very Large | 8 cores | 16 GB | 10 Gbps | 5-10 |

**Bandwidth dominated by search traffic**: 10-100 MB/sec per node

### Data Nodes

| Workload | CPU | Memory | Disk | Network | Replicas |
|----------|-----|--------|------|---------|----------|
| Hot tier (recent data) | 8 cores | 16 GB | 500 GB NVMe | 10 Gbps | 3-20 |
| Warm tier (older data) | 4 cores | 8 GB | 1 TB SSD | 10 Gbps | 3-20 |
| Cold tier (archive) | 2 cores | 4 GB | 2 TB HDD | 1 Gbps | 3-20 |

**Bandwidth dominated by search traffic**: 10-100 MB/sec per node

---

## Cluster State Size

### Estimation Formula

```
Master state = (nodes × 1KB) + (indices × 25KB) + (shards × 200 bytes) + 2KB

Example (large cluster):
  100 nodes × 1KB = 100 KB
  1000 indices × 25KB = 25 MB
  10000 shards × 200 bytes = 2 MB
  Settings = 2 KB
  Total = ~27 MB
```

### Growth Patterns

| Component | Grows With | Rate |
|-----------|-----------|------|
| Node registry | Data node count | 1 KB per node |
| Index metadata | Index count | 5-50 KB per index |
| Shard routing | Shard count | 200 bytes per shard |
| Settings | Configuration | ~2 KB total |

**Typical cluster state**: 10-50 MB

---

## Bandwidth Breakdown by Operation

### Node Registration

```
DataNode → Master: Register (1 KB)
Master → Raft peers: Replicate (2 × 1 KB = 2 KB)
Master → DataNode: Ack (200 bytes)
Total: ~3.2 KB per registration (once per node startup)
```

### Heartbeats

```
DataNode → Master: Heartbeat (200 bytes, every 30 sec)
Master → DataNode: Ack (100 bytes, every 30 sec)
Total per node: 300 bytes / 30 sec = 10 bytes/sec
Total for 100 nodes: 1 KB/sec
```

### Index Creation (100 shards)

```
Client → Coord: CreateIndex (5 KB)
Coord → Master: CreateIndex (5 KB)
Master → Raft peers: Replicate (2 × 5 KB = 10 KB)
Master → Data nodes: CreateShard × 100 (100 × 500 bytes = 50 KB)
Data nodes → Master: Acks × 100 (100 × 200 bytes = 20 KB)
Total: ~90 KB (one-time operation)
```

### Search Query

```
Client → Coord: Search (1 KB)
Coord → Master: GetShardRouting (cached, 0 bytes most of the time)
Coord → Data nodes: Search × N shards (N × 1 KB)
Data nodes → Coord: Results × N shards (N × 10-100 KB)
Coord → Client: Merged results (10-100 KB)
Total: Dominated by data plane (10-100 MB/sec), NOT master traffic
```

---

## Network Topology

### Recommended Layout

```
┌────────────────────────────────────────────────┐
│                                                │
│  Management Network (10.0.0.0/24)             │
│  Purpose: SSH, monitoring                     │
│  Bandwidth: 100 Mbps                          │
│                                                │
├────────────────────────────────────────────────┤
│                                                │
│  Control Plane Network (10.0.1.0/24)          │
│  Purpose: Master ↔ Master (Raft)              │
│  Bandwidth: 1 Gbps                            │
│  Latency: <2ms (critical for Raft)           │
│                                                │
├────────────────────────────────────────────────┤
│                                                │
│  API Network (10.0.2.0/24)                    │
│  Purpose: Coord/Data ↔ Master (gRPC APIs)     │
│  Bandwidth: 1 Gbps                            │
│  Latency: <5ms                                │
│                                                │
├────────────────────────────────────────────────┤
│                                                │
│  Data Plane Network (10.0.3.0/24)             │
│  Purpose: Coord ↔ Data (search traffic)       │
│  Bandwidth: 10 Gbps                           │
│  Latency: <1ms                                │
│                                                │
├────────────────────────────────────────────────┤
│                                                │
│  Client Network (10.0.4.0/24)                 │
│  Purpose: LoadBalancer ↔ Coord                │
│  Bandwidth: 10 Gbps                           │
│  Latency: <5ms                                │
│                                                │
└────────────────────────────────────────────────┘
```

---

## Port Allocation

| Component | Port | Protocol | Purpose | Bandwidth |
|-----------|------|----------|---------|-----------|
| Master | 9300 | TCP | Raft peer-to-peer | 5-10 KB/sec per peer |
| Master | 9301 | gRPC | Coord/Data → Master API | 2-5 KB/sec |
| Master | 9400 | HTTP | Prometheus metrics | Polling only |
| Coordination | 9200 | HTTP | Client REST API | 10-100 MB/sec |
| Coordination | 9302 | gRPC | Coord → Master client | 1-5 KB/sec |
| Coordination | 9401 | HTTP | Prometheus metrics | Polling only |
| Data | 9303 | gRPC | Coord → Data search API | 10-100 MB/sec |
| Data | 9402 | HTTP | Prometheus metrics | Polling only |

---

## Kubernetes Resource Requirements

### Master StatefulSet (3 replicas)

```yaml
resources:
  requests:
    memory: 4Gi      # 12 Gi total
    cpu: 2           # 6 vCPU total
    storage: 50Gi    # 150 Gi total
  limits:
    memory: 8Gi
    cpu: 4
```

**Cost (AWS EKS)**: ~$162/month for 3 master nodes

### Coordination Deployment (3 replicas)

```yaml
resources:
  requests:
    memory: 4Gi      # 12 Gi total
    cpu: 2           # 6 vCPU total
  limits:
    memory: 8Gi
    cpu: 4
```

### Data StatefulSet (5 replicas, hot tier)

```yaml
resources:
  requests:
    memory: 16Gi     # 80 Gi total
    cpu: 8           # 40 vCPU total
    storage: 500Gi   # 2.5 Ti total
  limits:
    memory: 32Gi
    cpu: 16
```

---

## Raft Performance Metrics

| Metric | Target | Critical | Notes |
|--------|--------|----------|-------|
| Leader election | <1 sec | <3 sec | Higher = longer unavailability |
| Raft commit latency | <5 ms | <20 ms | Cluster state write latency |
| Replication lag | <10 ms | <50 ms | Follower lag behind leader |
| Heartbeat interval | 100 ms | 500 ms | Leader → follower keepalive |
| Snapshot time | <5 sec | <30 sec | Full state snapshot |
| Snapshot size | <50 MB | <200 MB | Full cluster state |

---

## Capacity Planning

### Master Node Capacity

| Metric | Value | Notes |
|--------|-------|-------|
| Max data nodes | 1000+ | Limited by metadata size, not bandwidth |
| Max indices | 10,000+ | ~250 MB cluster state |
| Max shards | 100,000+ | ~20 MB in routing table |
| Cluster state size | <200 MB | Typical: 10-50 MB |
| Raft log size | <10 GB | With snapshots every 10K entries |

### When to Add 5th Master

Only if:
- ✅ Need 2 failure tolerance (vs 1 with 3 masters)
- ✅ Multi-datacenter with high inter-DC latency
- ✅ Regulatory requirement for extra redundancy

**Not needed for scale**: 3 masters handle 1000+ data nodes

---

## Monitoring Checklist

### Master Node Metrics

```promql
# Bandwidth (should be <20 KB/sec)
rate(master_network_bytes_total[5m])

# Raft commit latency (should be <5ms)
histogram_quantile(0.99, rate(raft_commit_latency_seconds_bucket[5m]))

# Leader election duration (should be <1s)
raft_leader_election_duration_seconds

# Cluster state size (should be <50 MB)
master_cluster_state_size_bytes

# Raft replication lag (should be <10ms)
raft_replication_lag_seconds
```

### Coordination Node Metrics

```promql
# Search query rate
rate(coordination_search_requests_total[5m])

# Search latency p99 (should be <100ms)
histogram_quantile(0.99, rate(coordination_search_duration_seconds_bucket[5m]))

# Master API call latency (should be <5ms)
histogram_quantile(0.99, rate(coordination_master_api_duration_seconds_bucket[5m]))
```

### Data Node Metrics

```promql
# Search operations per second
rate(data_search_operations_total[5m])

# Shard search latency (should be <50ms)
histogram_quantile(0.99, rate(data_shard_search_duration_seconds_bucket[5m]))

# Network bandwidth (actual search traffic)
rate(data_network_bytes_total[5m])
```

---

## Common Scenarios

### Scenario 1: Adding 10 New Data Nodes

**Bandwidth impact on master:**
```
Registration: 10 nodes × 3.2 KB = 32 KB (one-time)
Ongoing heartbeats: 10 nodes × 10 bytes/sec = 100 bytes/sec
Total ongoing: +100 bytes/sec (negligible)
```

### Scenario 2: Creating 100-Shard Index

**Bandwidth impact:**
```
Index creation: ~5 KB (one-time)
Shard allocation: 100 × 500 bytes = 50 KB (one-time)
Total: ~55 KB one-time, then 0 ongoing
```

### Scenario 3: 1000 Queries/Second

**Bandwidth to master:**
```
Routing queries (cached 5 min): ~1 KB/sec (negligible)
Most traffic: Coord ↔ Data (100+ MB/sec, NOT through master)
```

**Key insight**: Search traffic does NOT go through master nodes.

---

## Troubleshooting

### High Master Bandwidth (>100 KB/sec)

Possible causes:
- ❌ Routing cache disabled (check coordination config)
- ❌ Too many index creates/deletes
- ❌ Very large Raft log entries (check index settings)
- ❌ Excessive heartbeat frequency (should be 30 sec)

### Raft Commit Latency >20ms

Possible causes:
- ❌ Network latency between masters >5ms
- ❌ Masters not on dedicated hardware
- ❌ Disk I/O saturation (check Raft log writes)
- ❌ Too many concurrent cluster state changes

### Master Out of Memory

Possible causes:
- ❌ Cluster state >8 GB (extremely rare, check for bugs)
- ❌ Raft log not snapshotting (check snapshot interval)
- ❌ Memory leak (check master logs)

---

## Key Takeaways

1. **Master bandwidth is negligible**: 16 KB/sec per master
2. **Masters don't scale with cluster**: 3 masters handle 1000+ data nodes
3. **Search traffic bypasses masters**: Coord ↔ Data is the hot path
4. **Raft is cheap**: Strong consistency costs only ~10 KB/sec
5. **Masters are lightweight**: 4 GB RAM, 2 CPU cores sufficient
6. **Cost is minimal**: ~$162/month for 3 masters in AWS
7. **Always use 3 or 5 masters**: Odd numbers only
8. **Network isolation matters**: Separate control and data planes

---

**Document Version**: 1.0
**Last Updated**: 2026-01-26
