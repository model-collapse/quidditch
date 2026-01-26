# Master Node Architecture

**Version**: 1.0
**Date**: 2026-01-26
**Status**: Production Design

---

## Table of Contents

1. [Overview](#overview)
2. [Master Node Responsibilities](#master-node-responsibilities)
3. [Raft Consensus](#raft-consensus)
4. [Bandwidth Allocation](#bandwidth-allocation)
5. [Traditional Deployment](#traditional-deployment)
6. [Kubernetes Deployment](#kubernetes-deployment)
7. [Architecture Comparison](#architecture-comparison)
8. [Production Recommendations](#production-recommendations)

---

## Overview

Quidditch/Diagon implements a distributed search cluster architecture inspired by Elasticsearch/OpenSearch. The master nodes form the **control plane** that manages cluster state, while coordination and data nodes form the **data plane** that handles search traffic.

### Key Design Principles

1. **Separation of Control and Data Planes**: Master nodes never handle user search traffic
2. **Raft Consensus**: Provides strong consistency for cluster state
3. **Lightweight Metadata**: Minimize state stored in master nodes
4. **Cloud-Native**: Designed to work in both traditional and Kubernetes deployments

---

## Master Node Responsibilities

Master nodes are responsible for **cluster coordination only**, never data operations.

### 1. Cluster State Management

**What it stores:**
```go
type ClusterState struct {
    // Node registry
    Nodes map[string]*NodeInfo  // ~1KB per node

    // Index metadata (mappings, settings, shard count)
    Indices map[string]*IndexMetadata  // ~5-50KB per index

    // Shard allocation map
    ShardRouting map[string]map[int32]*ShardRouting  // ~200 bytes per shard

    // Cluster settings
    Settings *ClusterSettings  // ~2KB

    // Node health status
    NodeHealth map[string]*HealthStatus  // ~500 bytes per node
}
```

**Size estimation:**
- 100 nodes: ~100KB
- 1000 indices: ~25MB
- 10,000 shards: ~2MB
- **Total cluster state**: ~30MB (typical large cluster)

**Update frequency:**
- Node registration: Once per node startup (~minutes)
- Heartbeats: Every 30 seconds per node
- Shard allocation: Index creation + rebalancing (~minutes to hours)
- Index metadata: Index creation/update (~minutes)

### 2. Shard Allocation

Master nodes decide which data nodes host which shards.

**Allocation algorithm:**
```
1. Create index request arrives
2. Master determines shard count (from settings)
3. Select data nodes based on:
   - Available capacity (disk space, memory)
   - Current shard count (balance)
   - Node attributes (hot/warm/cold tiers)
   - Rack/zone awareness (if configured)
4. Record allocation in cluster state (Raft commit)
5. Send CreateShard RPC to selected data nodes
6. Wait for acknowledgments
7. Mark shards as STARTED in cluster state
```

**Bandwidth per shard allocation:**
- Allocation decision: ~200 bytes (Raft log entry)
- CreateShard RPC: ~500 bytes per shard
- Acknowledgment: ~200 bytes

**Example**: Creating 100-shard index = ~90KB total bandwidth

### 3. Node Registration & Discovery

**Node registration flow:**
```
DataNode Startup
    ↓
Register with Master (gRPC call)
    - NodeID, Address, Port
    - Capabilities (disk, memory, tier)
    - ~1KB payload
    ↓
Master validates and adds to cluster state
    ↓
Raft commit (~1.5KB with replication)
    ↓
Acknowledgment to DataNode
```

**Heartbeat flow:**
```
Every 30 seconds:
    DataNode → Master: Heartbeat (200 bytes)
        - NodeID
        - Shard health summary
        - Resource utilization
    Master → DataNode: Ack (100 bytes)
```

**Bandwidth per node:**
- Registration: 1KB (once)
- Heartbeats: 300 bytes / 30 seconds = **10 bytes/sec per node**

### 4. Routing Information

Coordination nodes query master for shard routing:

```go
type GetShardRoutingRequest struct {
    IndexName string  // ~50 bytes
}

type GetShardRoutingResponse struct {
    Routing map[int32]*ShardRouting  // ~200 bytes per shard
}
```

**Query frequency:**
- Coordination nodes cache routing (5 min TTL)
- Queries only on cache miss or invalidation
- **Typical**: 1 query per index per 5 minutes

**Bandwidth**: 200 bytes × shards × (1/300 seconds)
- 100 shards: **0.07 KB/sec**

---

## Raft Consensus

Master nodes use Raft for strong consistency.

### Raft Configuration

```yaml
# Recommended settings
raft:
  election_timeout: 1000ms      # Time before election
  heartbeat_interval: 100ms     # Leader → follower heartbeats
  snapshot_interval: 10000       # Entries before snapshot
  max_append_entries: 100       # Batch size for replication
```

### Raft Bandwidth

**Heartbeats (leader → followers):**
- Interval: 100ms
- Size: ~100 bytes
- Bandwidth per follower: **1 KB/sec**
- 3-node cluster: Leader sends 2 KB/sec

**Log replication (writes):**
- Each cluster state change → Raft log entry
- Entry size: 500 bytes - 5KB (depending on operation)
- Replicated to all followers
- **Write amplification**: N × entry size (N = cluster size)

**Example bandwidth (3 master nodes):**
```
Operation: Create index with 10 shards
    - Raft entry: ~2KB
    - Replicated to 2 followers: 2 × 2KB = 4KB
    - Acknowledgments: 2 × 200 bytes = 400 bytes
    - Total: ~4.4KB per index creation

Operation: Heartbeat cycle (all nodes)
    - Leader → 2 followers: 2 × 100 bytes = 200 bytes
    - Per second (10 cycles): 2KB/sec
```

### Raft Performance Characteristics

| Metric | Value | Notes |
|--------|-------|-------|
| Write latency | 2-10ms | Round-trip to quorum |
| Read latency | <1ms | Local read from leader |
| Throughput | 1000+ ops/sec | Cluster state changes |
| Snapshot size | 30-50MB | Full cluster state |
| Snapshot time | 1-5 seconds | Depends on size |

---

## Bandwidth Allocation

### Per-Node Bandwidth Budget

Assuming 100 data nodes, 1000 indices, 10,000 shards:

#### Master Node (Incoming)

| Source | Operation | Frequency | Bandwidth |
|--------|-----------|-----------|-----------|
| Data nodes | Heartbeats | 30s | 100 nodes × 10 bytes/sec = **1 KB/sec** |
| Data nodes | Shard health | 60s | 100 nodes × 50 bytes/min = **83 bytes/sec** |
| Coord nodes | Routing queries | 5min cache | 10 coord × 20 KB / 300s = **0.7 KB/sec** |
| Raft peers | Raft heartbeats | 100ms | 2 peers × 1 KB/sec = **2 KB/sec** |
| API calls | Index create/delete | Low freq | **~0.1 KB/sec** |
| **Total incoming** | | | **~4 KB/sec** |

#### Master Node (Outgoing)

| Destination | Operation | Frequency | Bandwidth |
|-------------|-----------|-----------|-----------|
| Data nodes | Shard allocation | Rare | **~0.5 KB/sec** |
| Data nodes | Heartbeat acks | 30s | 100 nodes × 3 bytes/sec = **300 bytes/sec** |
| Coord nodes | Routing responses | 5min | 10 coord × 20 KB / 300s = **0.7 KB/sec** |
| Raft peers | Log replication | Variable | **5-10 KB/sec** |
| **Total outgoing** | | | **~12 KB/sec** |

**Master node total bandwidth: ~16 KB/sec = 128 Kbps**

This is **extremely low** - a 1 Gbps network link can handle 60,000+ master nodes worth of traffic.

### Coordination Node Bandwidth

| Source | Operation | Bandwidth |
|--------|-----------|-----------|
| Clients | HTTP requests | **High** (1-10 MB/sec typical) |
| Data nodes | Search queries (gRPC) | **Very High** (10-100 MB/sec) |
| Data nodes | Search responses | **Very High** (10-100 MB/sec) |
| Master | Routing queries | **Negligible** (0.7 KB/sec) |

**Coordination node bandwidth dominated by search traffic, not master communication.**

### Data Node Bandwidth

| Source | Operation | Bandwidth |
|--------|-----------|-----------|
| Coord nodes | Search queries | **Very High** (10-100 MB/sec) |
| Coord nodes | Search responses | **Very High** (10-100 MB/sec) |
| Master | Shard allocation | **Negligible** (0.5 KB/sec) |
| Master | Heartbeats | **Negligible** (10 bytes/sec) |

**Data node bandwidth dominated by search traffic, not master communication.**

---

## Traditional Deployment

### Architecture Overview

```
                    ┌─────────────────────────────────────┐
                    │      Load Balancer (HAProxy)        │
                    │    (Client traffic: HTTPS/9200)     │
                    └──────────────┬──────────────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                    │
    ┌─────────▼─────────┐ ┌───────▼────────┐ ┌────────▼────────┐
    │ Coordination Node  │ │ Coordination    │ │ Coordination    │
    │   coord-1:9200    │ │  Node coord-2   │ │  Node coord-3   │
    │  (HTTP + gRPC)    │ │   :9200         │ │   :9200         │
    └─────────┬─────────┘ └───────┬────────┘ └────────┬────────┘
              │                    │                    │
              └────────────────────┼────────────────────┘
                                   │
                    ┌──────────────┴───────────────┐
                    │     Master Cluster           │
                    │   (Raft Consensus)           │
                    │                              │
              ┌─────▼──────┐  ┌────────────┐  ┌──────────────┐
              │ Master-1   │  │ Master-2   │  │  Master-3    │
              │  (Leader)  │←─┤ (Follower) │─→│ (Follower)   │
              │  :9301     │  │  :9301     │  │   :9301      │
              └─────┬──────┘  └────────────┘  └──────────────┘
                    │
                    │ (Shard routing, allocation)
                    │
       ┌────────────┼────────────┬─────────────┬──────────────┐
       │            │            │             │              │
  ┌────▼────┐  ┌───▼────┐  ┌────▼────┐  ┌────▼────┐  ┌──────▼──┐
  │ Data-1  │  │ Data-2 │  │ Data-3  │  │ Data-4  │  │ Data-5  │
  │ :9303   │  │ :9303  │  │ :9303   │  │ :9303   │  │ :9303   │
  │ Shards  │  │ Shards │  │ Shards  │  │ Shards  │  │ Shards  │
  │ 0,5,10  │  │ 1,6,11 │  │ 2,7,12  │  │ 3,8,13  │  │ 4,9,14  │
  └─────────┘  └────────┘  └─────────┘  └─────────┘  └─────────┘
```

### Master Node Requirements

#### Hardware Specifications

| Component | Minimum | Recommended | Large Cluster |
|-----------|---------|-------------|---------------|
| CPU | 2 cores | 4 cores | 8 cores |
| Memory | 2 GB | 4 GB | 8 GB |
| Disk | 20 GB SSD | 50 GB SSD | 100 GB SSD |
| Network | 100 Mbps | 1 Gbps | 1 Gbps |

**Why such low requirements?**
- Master nodes only store metadata (~30MB for large cluster)
- No data indexing or search operations
- CPU used for Raft consensus and allocation decisions
- Disk used for Raft logs and snapshots

#### Cluster Sizing

| Cluster Size | Data Nodes | Master Nodes | Reason |
|--------------|-----------|--------------|--------|
| Small | 1-3 | 1 | Single node acceptable for testing |
| Medium | 3-20 | 3 | Quorum with 1 failure tolerance |
| Large | 20-100 | 3 | Same (master nodes don't scale with data) |
| Very Large | 100-500 | 3 or 5 | 5 if you need 2 failure tolerance |

**Why doesn't master count scale with data nodes?**
- Master nodes manage **metadata**, not data
- Metadata size grows with indices/shards, not data volume
- A 3-node master cluster can manage 1000+ data nodes
- Adding more masters increases write latency (Raft quorum)

### Network Topology

#### Physical Network Layout

```yaml
# Production network topology
networks:
  management:
    subnet: 10.0.0.0/24
    purpose: SSH, monitoring, administration

  control_plane:
    subnet: 10.0.1.0/24
    purpose: Master node communication (Raft)
    bandwidth: 1 Gbps
    latency: <2ms

  data_plane:
    subnet: 10.0.2.0/24
    purpose: Coordination ↔ Data node search traffic
    bandwidth: 10 Gbps
    latency: <1ms

  client_facing:
    subnet: 10.0.3.0/24
    purpose: Load balancer ↔ Coordination nodes
    bandwidth: 10 Gbps
```

#### Port Allocation

| Component | Port | Protocol | Purpose |
|-----------|------|----------|---------|
| Master | 9300 | TCP | Raft peer-to-peer |
| Master | 9301 | gRPC | API for coord/data nodes |
| Master | 9400 | HTTP | Prometheus metrics |
| Coordination | 9200 | HTTP | Client REST API |
| Coordination | 9302 | gRPC | Master client |
| Coordination | 9401 | HTTP | Prometheus metrics |
| Data | 9303 | gRPC | Search/indexing API |
| Data | 9402 | HTTP | Prometheus metrics |

### Deployment Example (Bare Metal / VMs)

#### 1. Deploy Master Nodes

```bash
# master-1.example.com (Leader candidate)
./quidditch-master \
  --node-id=master-1 \
  --bind-addr=10.0.1.11 \
  --raft-port=9300 \
  --grpc-port=9301 \
  --data-dir=/var/lib/quidditch/master \
  --peers=10.0.1.11:9300,10.0.1.12:9300,10.0.1.13:9300

# master-2.example.com
./quidditch-master \
  --node-id=master-2 \
  --bind-addr=10.0.1.12 \
  --raft-port=9300 \
  --grpc-port=9301 \
  --data-dir=/var/lib/quidditch/master \
  --peers=10.0.1.11:9300,10.0.1.12:9300,10.0.1.13:9300

# master-3.example.com
./quidditch-master \
  --node-id=master-3 \
  --bind-addr=10.0.1.13 \
  --raft-port=9300 \
  --grpc-port=9301 \
  --data-dir=/var/lib/quidditch/master \
  --peers=10.0.1.11:9300,10.0.1.12:9300,10.0.1.13:9300
```

#### 2. Deploy Coordination Nodes

```bash
# coord-1.example.com
./quidditch-coordination \
  --node-id=coord-1 \
  --bind-addr=10.0.3.21 \
  --rest-port=9200 \
  --master-addr=10.0.1.11:9301,10.0.1.12:9301,10.0.1.13:9301

# coord-2.example.com
./quidditch-coordination \
  --node-id=coord-2 \
  --bind-addr=10.0.3.22 \
  --rest-port=9200 \
  --master-addr=10.0.1.11:9301,10.0.1.12:9301,10.0.1.13:9301
```

#### 3. Deploy Data Nodes

```bash
# data-1.example.com
./quidditch-data \
  --node-id=data-1 \
  --bind-addr=10.0.2.31 \
  --grpc-port=9303 \
  --master-addr=10.0.1.11:9301 \
  --data-dir=/var/lib/quidditch/data \
  --storage-tier=hot

# data-2.example.com
./quidditch-data \
  --node-id=data-2 \
  --bind-addr=10.0.2.32 \
  --grpc-port=9303 \
  --master-addr=10.0.1.11:9301 \
  --data-dir=/var/lib/quidditch/data \
  --storage-tier=hot
```

### Why Master Nodes Are Required

**1. Cluster Coordination**
- Who decides where shards go?
- Who tracks which nodes are alive?
- Who manages index metadata?
- **Answer**: Master nodes via Raft consensus

**2. Strong Consistency**
- Raft guarantees linearizable reads/writes
- No split-brain scenarios
- Deterministic shard allocation

**3. Fault Tolerance**
- 3 master nodes: Tolerates 1 failure
- 5 master nodes: Tolerates 2 failures
- Continues operating with quorum

**4. Centralized Metadata**
- Single source of truth for cluster state
- Coordination/data nodes are stateless (from cluster perspective)
- Can restart any coord/data node without state loss

---

## Kubernetes Deployment

### Option 1: Traditional Masters in K8S (Recommended)

**Use StatefulSets for master nodes, just like bare metal.**

#### Architecture

```yaml
# Master nodes as StatefulSet
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: quidditch-master
spec:
  serviceName: quidditch-master
  replicas: 3
  selector:
    matchLabels:
      app: quidditch-master
  template:
    metadata:
      labels:
        app: quidditch-master
    spec:
      containers:
      - name: master
        image: quidditch/master:latest
        ports:
        - containerPort: 9300
          name: raft
        - containerPort: 9301
          name: grpc
        volumeMounts:
        - name: data
          mountPath: /var/lib/quidditch/master
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 50Gi
```

#### Why StatefulSet?

1. **Stable network identity**: `quidditch-master-0`, `quidditch-master-1`, `quidditch-master-2`
2. **Persistent storage**: Each master has its own PVC
3. **Ordered deployment**: Masters start in sequence
4. **Raft peer discovery**: Use StatefulSet DNS for peer addresses

#### Discovery Configuration

```yaml
# Coordination deployment (discovers master via Service)
env:
- name: MASTER_ADDRESSES
  value: "quidditch-master-0.quidditch-master.default.svc.cluster.local:9301,
          quidditch-master-1.quidditch-master.default.svc.cluster.local:9301,
          quidditch-master-2.quidditch-master.default.svc.cluster.local:9301"
```

#### Advantages

✅ **Proven pattern**: Same as Elasticsearch/OpenSearch Helm charts
✅ **Battle-tested**: Raft consensus is mature
✅ **Simple**: No Kubernetes-specific logic in application
✅ **Portable**: Same code works in K8S and bare metal
✅ **Debuggable**: Standard Raft debugging tools work

#### Disadvantages

❌ **Extra pods**: 3 master pods that don't serve traffic
❌ **Persistent volumes**: Need PVCs for Raft logs
❌ **More complexity**: Additional StatefulSet to manage

### Option 2: Kubernetes-Native Control Plane

**Use K8S primitives instead of master nodes.**

#### Architecture

```
┌────────────────────────────────────────────────────┐
│            Kubernetes Control Plane                │
│                                                    │
│  ┌──────────────┐  ┌──────────────┐              │
│  │  ConfigMap   │  │   Service    │              │
│  │ (Index Meta) │  │  Discovery   │              │
│  └──────────────┘  └──────────────┘              │
│                                                    │
│  ┌──────────────┐  ┌──────────────┐              │
│  │ Leader       │  │  Custom      │              │
│  │ Election     │  │  Resources   │              │
│  └──────────────┘  └──────────────┘              │
└────────────────────────────────────────────────────┘
                      │
        ┌─────────────┼──────────────┐
        │             │              │
   ┌────▼─────┐  ┌───▼──────┐  ┌───▼──────┐
   │  Coord   │  │  Coord   │  │  Coord   │
   │ (Leader) │  │          │  │          │
   └────┬─────┘  └───┬──────┘  └───┬──────┘
        │            │             │
        └────────────┼─────────────┘
                     │
         ┌───────────┼───────────┐
         │           │           │
    ┌────▼────┐ ┌───▼────┐ ┌───▼────┐
    │ Data    │ │ Data   │ │ Data   │
    └─────────┘ └────────┘ └────────┘
```

#### Implementation

```go
// Coordination node with leader election
import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/tools/leaderelection"
)

func (c *Coordination) Start() error {
    // Use K8S leader election
    elector := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
        Lock: &resourcelock.LeaseLock{
            LeaseMeta: metav1.ObjectMeta{
                Name:      "quidditch-coordination-leader",
                Namespace: "default",
            },
        },
        LeaseDuration: 15 * time.Second,
        RenewDeadline: 10 * time.Second,
        RetryPeriod:   2 * time.Second,
        Callbacks: leaderelection.LeaderCallbacks{
            OnStartedLeading: c.becomeLeader,
            OnStoppedLeading: c.stopLeading,
        },
    })

    go elector.Run(ctx)
    return nil
}

func (c *Coordination) becomeLeader(ctx context.Context) {
    // Leader handles cluster coordination
    c.isLeader = true

    // Start shard allocation
    go c.shardAllocator.Run(ctx)

    // Start node health monitoring
    go c.nodeMonitor.Run(ctx)
}
```

#### Cluster State Storage

```yaml
# Store index metadata in ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: quidditch-indices
data:
  products.json: |
    {
      "name": "products",
      "shards": 10,
      "replicas": 1,
      "mapping": {...}
    }
  users.json: |
    {
      "name": "users",
      "shards": 5,
      "replicas": 2,
      "mapping": {...}
    }

---
# Store shard routing in ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: quidditch-shard-routing
data:
  routing.json: |
    {
      "products": {
        "0": {"node": "data-1", "state": "started"},
        "1": {"node": "data-2", "state": "started"}
      }
    }
```

#### Node Discovery

```go
// Watch K8S Service endpoints
import "k8s.io/client-go/informers"

func (c *Coordination) discoverDataNodes() {
    // Watch quidditch-data Service
    informer := factory.Core().V1().Endpoints().Informer()
    informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
        AddFunc: func(obj interface{}) {
            endpoint := obj.(*v1.Endpoints)
            for _, subset := range endpoint.Subsets {
                for _, addr := range subset.Addresses {
                    c.registerDataNode(addr.IP)
                }
            }
        },
    })
}
```

#### Advantages

✅ **No master pods**: Saves resources
✅ **K8S-native**: Uses proven K8S patterns
✅ **Auto-scaling**: Data nodes can scale via HPA
✅ **Simpler ops**: Fewer moving parts
✅ **Cost-effective**: 3 fewer pods running

#### Disadvantages

❌ **K8S-only**: Won't work in bare metal deployments
❌ **ConfigMap limits**: 1MB limit per ConfigMap
❌ **Eventually consistent**: K8S watch events have delays
❌ **No Raft**: Lose strong consistency guarantees
❌ **Custom code**: Need to implement coordination logic
❌ **Debugging**: Can't use standard Raft tools

### Comparison Table

| Aspect | Traditional Masters | K8S-Native |
|--------|---------------------|------------|
| **Deployment** | StatefulSet (3 pods) | Deployment (0 extra pods) |
| **Storage** | PVC (50GB × 3) | ConfigMap/Secrets |
| **Consistency** | Strong (Raft) | Eventual (K8S API) |
| **Portability** | Works everywhere | K8S only |
| **Complexity** | Standard Raft | Custom K8S logic |
| **Resource cost** | ~6 GB RAM, 6 CPU | ~0 (no extra pods) |
| **Latency** | <5ms (Raft) | 10-50ms (K8S API) |
| **Failure recovery** | Automatic (Raft) | Manual (K8S controller) |
| **Scalability** | 1000+ data nodes | 1000+ data nodes |
| **Operations** | Proven playbooks | Custom tooling |

---

## Architecture Comparison

### When to Use Master Nodes

✅ **Use master nodes if:**
- Deploying in bare metal / VMs / mixed environments
- Need strong consistency for cluster state
- Want production-proven architecture (Elasticsearch-style)
- Team familiar with distributed consensus
- Multi-datacenter deployment (Raft across DCs)
- Strict SLAs requiring <5ms coordination latency

### When to Use K8S-Native

✅ **Use K8S-native if:**
- K8S-only deployment forever
- Cost-sensitive (save 3 pods)
- Simple cluster (<50 data nodes, <100 indices)
- Can tolerate eventual consistency
- Team familiar with K8S controllers
- Don't need multi-datacenter coordination

---

## Production Recommendations

### Default Recommendation: **Traditional Master Nodes**

**Rationale:**
1. **Proven architecture**: Used by Elasticsearch, OpenSearch, Consul, etcd
2. **Strong consistency**: No split-brain, deterministic shard allocation
3. **Portable**: Same code works in K8S, VMs, bare metal
4. **Low cost**: 16 KB/sec bandwidth, 4 GB RAM per master
5. **Battle-tested**: Raft is mature and well-understood

### Master Node Best Practices

#### 1. Always Use Odd Number

```
✅ 3 masters: Tolerates 1 failure (quorum = 2)
✅ 5 masters: Tolerates 2 failures (quorum = 3)
❌ 2 masters: Tolerates 0 failures (quorum = 2)
❌ 4 masters: Tolerates 1 failure (quorum = 3) - same as 3!
❌ 6 masters: Tolerates 2 failures (quorum = 4) - same as 5!
```

**Why odd numbers?**
- Quorum = (N/2) + 1
- 3 masters: Need 2 for quorum
- 4 masters: Need 3 for quorum (same fault tolerance, more latency)

#### 2. Separate Networks

```yaml
# Isolate control plane traffic
master_network: 10.0.1.0/24    # Master ↔ Master (Raft)
api_network: 10.0.2.0/24       # Master ↔ Coord/Data (API)
data_network: 10.0.3.0/24      # Coord ↔ Data (search traffic)
```

#### 3. Dedicated Master Nodes

```bash
# Don't run masters on data nodes
# Don't run masters on coordination nodes
# Run masters on dedicated hardware/VMs

# Why?
# - Resource contention affects Raft latency
# - Master failures shouldn't correlate with data node failures
# - Security isolation (control plane vs data plane)
```

#### 4. Monitor Raft Metrics

```promql
# Leader election duration (should be <1s)
quidditch_raft_leader_election_duration_seconds

# Raft log replication lag (should be <10ms)
quidditch_raft_replication_lag_seconds

# Raft commit latency (should be <5ms)
quidditch_raft_commit_latency_seconds

# Cluster state size (should be <50MB)
quidditch_cluster_state_size_bytes
```

#### 5. Backup Raft State

```bash
# Snapshot Raft state daily
./quidditch-admin backup-cluster-state \
  --master=master-1:9301 \
  --output=/backups/cluster-state-$(date +%Y%m%d).snapshot

# Restore from snapshot
./quidditch-admin restore-cluster-state \
  --master=master-1:9301 \
  --snapshot=/backups/cluster-state-20260126.snapshot
```

### Kubernetes Deployment Recipe

#### Full Stack Helm Chart

```yaml
# values.yaml
master:
  enabled: true
  replicaCount: 3
  resources:
    requests:
      memory: 4Gi
      cpu: 2
    limits:
      memory: 8Gi
      cpu: 4
  storage:
    size: 50Gi
    storageClass: fast-ssd

coordination:
  replicaCount: 3
  resources:
    requests:
      memory: 4Gi
      cpu: 2

data:
  replicaCount: 5
  resources:
    requests:
      memory: 16Gi
      cpu: 8
  storage:
    size: 500Gi
```

#### Deploy

```bash
helm repo add quidditch https://charts.quidditch.io
helm install quidditch quidditch/quidditch \
  --namespace quidditch \
  --create-namespace \
  --values values.yaml
```

---

## Conclusion

### Key Takeaways

1. **Master nodes are lightweight**: Only 16 KB/sec bandwidth, 4 GB RAM
2. **Master count doesn't scale with cluster**: 3 masters can handle 1000+ data nodes
3. **Master bandwidth is negligible**: Search traffic dominates (10-100 MB/sec)
4. **Strong consistency matters**: Raft prevents split-brain and ensures correctness
5. **Traditional deployment**: Master nodes are **required** for production
6. **Kubernetes deployment**: Master nodes are **recommended** (K8S-native possible but not advised)

### Decision Matrix

| Scenario | Recommendation |
|----------|----------------|
| Production (any environment) | ✅ 3 master nodes with Raft |
| Development/Testing | ✅ 1 master node (no Raft) |
| K8S cost-sensitive | ⚠️ K8S-native (eventual consistency trade-off) |
| Multi-cloud | ✅ 5 master nodes (2 failure tolerance) |
| Very large cluster (500+ nodes) | ✅ 5 master nodes (more quorum bandwidth) |

**When in doubt: Use traditional master nodes. They're proven, portable, and cheap.**

---

**Document Version**: 1.0
**Last Updated**: 2026-01-26
**Author**: Claude Sonnet 4.5
**Status**: Production Design
