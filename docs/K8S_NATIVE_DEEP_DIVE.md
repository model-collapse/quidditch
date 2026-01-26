# K8S-Native Control Plane: Deep Dive

**Version**: 1.0
**Date**: 2026-01-26
**Status**: Architectural Analysis

---

## Table of Contents

1. [The Case for K8S-Native](#the-case-for-k8s-native)
2. [What Kubernetes Already Provides](#what-kubernetes-already-provides)
3. [Strong Consistency Reality](#strong-consistency-reality)
4. [Custom Resource Definitions (CRDs)](#custom-resource-definitions-crds)
5. [Operator Pattern](#operator-pattern)
6. [Implementation Deep-Dive](#implementation-deep-dive)
7. [Performance Analysis](#performance-analysis)
8. [Trade-offs Re-examined](#trade-offs-re-examined)
9. [Hybrid Architecture](#hybrid-architecture)
10. [Recommendation Reconsidered](#recommendation-reconsidered)

---

## The Case for K8S-Native

### The Question

**If Kubernetes already provides:**
- Distributed state management (etcd)
- Leader election
- Service discovery
- Resource management
- Reconciliation loops
- Strong consistency

**Why build a separate Raft cluster for master nodes?**

### The Modern Cloud-Native Perspective

In 2026, the Operator pattern is the standard for distributed systems in Kubernetes:

| System | Pattern | Notes |
|--------|---------|-------|
| Vitess | K8S Operator | MySQL orchestration, no separate masters |
| TiDB | K8S Operator | Distributed database, uses CRDs |
| Kafka (Strimzi) | K8S Operator | No separate control plane |
| PostgreSQL (Zalando) | K8S Operator | Pure K8S-native |
| Cassandra (K8ssandra) | K8S Operator | CRDs for cluster state |
| Rook/Ceph | K8S Operator | Storage orchestration |

**Common pattern**: No separate StatefulSet for "master" nodes.

---

## What Kubernetes Already Provides

### 1. Distributed State: etcd

**Kubernetes API server is backed by etcd, which uses Raft!**

```
Your K8S Cluster Already Has:

    API Server
        ↓
    etcd cluster (3-5 nodes)
        ↓
    Raft consensus
        ↓
    Strong consistency
```

**Key insight**: When you use K8S API for state, you're using Raft indirectly.

### 2. Custom Resource Definitions (CRDs)

Store cluster metadata as Kubernetes resources:

```yaml
apiVersion: quidditch.io/v1
kind: QuidditchIndex
metadata:
  name: products
  namespace: quidditch
spec:
  shards: 10
  replicas: 2
  settings:
    refresh_interval: 1s
    number_of_shards: 10
  mappings:
    properties:
      title:
        type: text
      price:
        type: float
status:
  state: ACTIVE
  shardAllocations:
    - shardId: 0
      nodeId: quidditch-data-0
      state: STARTED
    - shardId: 1
      nodeId: quidditch-data-1
      state: STARTED
```

**Advantages over internal cluster state:**
- ✅ Stored in etcd (Raft-backed, strongly consistent)
- ✅ Versioned (resourceVersion for optimistic locking)
- ✅ RBAC integrated
- ✅ kubectl works natively: `kubectl get quidditchindex`
- ✅ No 1MB limit (unlike ConfigMaps)
- ✅ Watch API for real-time updates
- ✅ Admission webhooks for validation
- ✅ Finalizers for cleanup

### 3. Leader Election

Built-in leader election via Lease objects:

```go
import (
    "k8s.io/client-go/tools/leaderelection"
    "k8s.io/client-go/tools/leaderelection/resourcelock"
)

func (c *CoordinationController) Start(ctx context.Context) error {
    // K8S native leader election
    lock := &resourcelock.LeaseLock{
        LeaseMeta: metav1.ObjectMeta{
            Name:      "quidditch-controller-leader",
            Namespace: "quidditch",
        },
        Client: c.kubeClient.CoordinationV1(),
        LockConfig: resourcelock.ResourceLockConfig{
            Identity: c.nodeName,
        },
    }

    elector, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectorConfig{
        Lock:            lock,
        LeaseDuration:   15 * time.Second,
        RenewDeadline:   10 * time.Second,
        RetryPeriod:     2 * time.Second,
        ReleaseOnCancel: true,
        Callbacks: leaderelection.LeaderCallbacks{
            OnStartedLeading: c.onBecomeLeader,
            OnStoppedLeading: c.onLoseLeadership,
            OnNewLeader:      c.onNewLeader,
        },
    })

    go elector.Run(ctx)
    return nil
}
```

**Characteristics:**
- Based on etcd leases (Raft-backed)
- Automatic failover (<15 seconds)
- No split-brain (strong consistency from etcd)
- Battle-tested (used by all K8S controllers)

### 4. Service Discovery

Native K8S service discovery:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: quidditch-data
  namespace: quidditch
spec:
  clusterIP: None  # Headless service
  selector:
    app: quidditch-data
  ports:
  - port: 9303
    name: grpc
```

**Automatic DNS**:
```
quidditch-data-0.quidditch-data.quidditch.svc.cluster.local
quidditch-data-1.quidditch-data.quidditch.svc.cluster.local
quidditch-data-2.quidditch-data.quidditch.svc.cluster.local
```

**Endpoints API**:
```go
// Watch for data node additions/removals
informer := factory.Core().V1().Endpoints().Informer()
informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc: func(obj interface{}) {
        endpoint := obj.(*v1.Endpoints)
        for _, subset := range endpoint.Subsets {
            for _, addr := range subset.Addresses {
                c.registerDataNode(addr.IP, addr.TargetRef.Name)
            }
        }
    },
    DeleteFunc: func(obj interface{}) {
        // Handle node removal
    },
})
```

### 5. Resource Management

K8S tracks resource allocation natively:

```go
// Get node resource information
node, err := c.kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})

available := node.Status.Allocatable
used := node.Status.Capacity

diskPressure := node.Status.Conditions[...]
memoryPressure := node.Status.Conditions[...]
```

No need to track this separately.

### 6. Reconciliation Loops

Controller pattern handles eventual consistency automatically:

```go
func (c *IndexController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    // Get desired state (CRD)
    index := &quidditchv1.QuidditchIndex{}
    if err := c.Get(ctx, req.NamespacedName, index); err != nil {
        return reconcile.Result{}, client.IgnoreNotFound(err)
    }

    // Get actual state (from data nodes)
    actualShards := c.getCurrentShardAllocations(ctx, index.Name)

    // Reconcile: make actual match desired
    if !reflect.DeepEqual(index.Status.ShardAllocations, actualShards) {
        c.reconcileShards(ctx, index, actualShards)
    }

    return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}
```

**Advantages**:
- Declarative: Define desired state, controller ensures it
- Self-healing: Automatically recovers from drift
- Idempotent: Safe to re-run
- Standard pattern: All K8S operators work this way

---

## Strong Consistency Reality

### Myth: K8S Is Eventually Consistent

**Reality: K8S API server is strongly consistent.**

```
Client → API Server → etcd (Raft) → Commit → Response

All reads go through Raft leader
All writes require Raft quorum
```

**Proof**:
```go
// K8S API guarantees:
// 1. Read-your-writes consistency
index, err := client.Create(ctx, index)  // Write
fetched, err := client.Get(ctx, index.Name)  // Read
// fetched ALWAYS reflects the create (no stale reads)

// 2. Linearizability via resourceVersion
index.ResourceVersion  // Monotonically increasing
// Optimistic locking prevents conflicts
```

### What About Watch Delays?

**Watch API uses etcd watch (Raft log streaming)**:

```go
watcher, err := client.Watch(ctx, &quidditchv1.QuidditchIndex{})
for event := range watcher.ResultChan() {
    // Events arrive in Raft log order
    // No reordering, no duplicates
    // Eventual delivery: <100ms typically
}
```

**Characteristics**:
- Events delivered in Raft commit order
- No lost events (unless client disconnects)
- Typical latency: 10-100ms
- Guaranteed: Eventually all watchers see all events

### Strong vs Eventual Consistency

| Scenario | Internal Raft | K8S API (etcd) |
|----------|--------------|----------------|
| Write latency | 2-5ms | 5-20ms |
| Read latency | <1ms (leader) | 1-5ms (API server) |
| Consistency | Linearizable | Linearizable |
| Watch latency | N/A | 10-100ms |
| Split-brain prevention | ✓ | ✓ |
| Optimistic locking | Manual | Built-in (resourceVersion) |

**Verdict**: K8S API is strongly consistent, just slightly higher latency.

---

## Custom Resource Definitions (CRDs)

### QuidditchIndex CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: quidditchindices.quidditch.io
spec:
  group: quidditch.io
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              shards:
                type: integer
                minimum: 1
                maximum: 1000
              replicas:
                type: integer
                minimum: 0
                maximum: 5
              settings:
                type: object
              mappings:
                type: object
          status:
            type: object
            properties:
              state:
                type: string
                enum: [CREATING, ACTIVE, DELETING]
              shardAllocations:
                type: array
                items:
                  type: object
                  properties:
                    shardId:
                      type: integer
                    nodeId:
                      type: string
                    state:
                      type: string
                      enum: [INITIALIZING, STARTED, RELOCATING]
  scope: Namespaced
  names:
    plural: quidditchindices
    singular: quidditchindex
    kind: QuidditchIndex
    shortNames:
    - qidx
```

### Usage

```bash
# Create index
kubectl apply -f - <<EOF
apiVersion: quidditch.io/v1
kind: QuidditchIndex
metadata:
  name: products
spec:
  shards: 10
  replicas: 1
EOF

# Get indices
kubectl get quidditchindex
# NAME       SHARDS   STATE    AGE
# products   10       ACTIVE   5m

# Get details
kubectl describe quidditchindex products

# Watch for changes
kubectl get quidditchindex -w
```

### QuidditchNode CRD

```yaml
apiVersion: quidditch.io/v1
kind: QuidditchNode
metadata:
  name: quidditch-data-0
spec:
  nodeType: DATA
  tier: hot
  maxShards: 100
status:
  state: HEALTHY
  lastHeartbeat: "2026-01-26T10:30:00Z"
  allocatedShards: 8
  resources:
    diskUsed: 50Gi
    diskTotal: 500Gi
    memoryUsed: 12Gi
    memoryTotal: 16Gi
```

### ShardAllocation CRD (Optional)

```yaml
apiVersion: quidditch.io/v1
kind: ShardAllocation
metadata:
  name: products-shard-0
spec:
  indexName: products
  shardId: 0
  isPrimary: true
  assignedNode: quidditch-data-0
status:
  state: STARTED
  docsCount: 1000000
  sizeBytes: 500000000
```

### No 1MB Limit

**ConfigMaps**: Limited to 1MB
**CRDs**: No hard limit (stored in etcd, typically <1MB per resource)

**Strategy for large metadata**:
- Store index mappings in separate ConfigMap/CRD
- Reference by name in QuidditchIndex
- Split large objects across multiple resources

**Realistic limits**:
- etcd object size: 1.5 MB default (configurable to 10 MB)
- Typical QuidditchIndex: <50 KB
- 10,000 indices × 50 KB = 500 MB total (etcd can handle this)

---

## Operator Pattern

### Controller Architecture

```
┌─────────────────────────────────────────────────────┐
│            Quidditch Operator Pod                    │
│                                                      │
│  ┌────────────────────────────────────────────────┐ │
│  │         Controller Manager                      │ │
│  │                                                 │ │
│  │  ┌─────────────┐  ┌──────────────┐  ┌────────┐│ │
│  │  │   Index     │  │    Node      │  │ Shard  ││ │
│  │  │ Controller  │  │  Controller  │  │ Alloc  ││ │
│  │  └──────┬──────┘  └──────┬───────┘  └───┬────┘│ │
│  └─────────┼─────────────────┼──────────────┼─────┘ │
│            │                 │              │       │
│  ┌─────────▼─────────────────▼──────────────▼─────┐ │
│  │          Kubernetes API Client                  │ │
│  │          (Watches CRDs, Manages Resources)      │ │
│  └─────────┬───────────────────────────────────────┘ │
└────────────┼─────────────────────────────────────────┘
             │
┌────────────▼─────────────────────────────────────────┐
│           Kubernetes API Server                      │
│                     ↓                                │
│                   etcd                               │
│              (Raft Consensus)                        │
└──────────────────────────────────────────────────────┘
             │
┌────────────▼─────────────────────────────────────────┐
│         Data Node StatefulSet                        │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐             │
│  │ Data-0  │  │ Data-1  │  │ Data-2  │             │
│  └─────────┘  └─────────┘  └─────────┘             │
└──────────────────────────────────────────────────────┘
```

### Index Controller Implementation

```go
package controllers

import (
    "context"
    "time"

    quidditchv1 "github.com/quidditch/quidditch/api/v1"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"
)

type IndexReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    DataNodeClient DataNodeClientInterface
}

func (r *IndexReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // Fetch the QuidditchIndex
    index := &quidditchv1.QuidditchIndex{}
    if err := r.Get(ctx, req.NamespacedName, index); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion
    if !index.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, index)
    }

    // Ensure finalizer
    if !containsString(index.Finalizers, "quidditch.io/index-protection") {
        index.Finalizers = append(index.Finalizers, "quidditch.io/index-protection")
        if err := r.Update(ctx, index); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Reconcile based on state
    switch index.Status.State {
    case "":
        // New index, start creation
        return r.reconcileCreate(ctx, index)
    case "CREATING":
        // Check if shards are allocated
        return r.reconcileCreating(ctx, index)
    case "ACTIVE":
        // Ensure desired state
        return r.reconcileActive(ctx, index)
    default:
        logger.Info("Unknown state", "state", index.Status.State)
    }

    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *IndexReconciler) reconcileCreate(ctx context.Context, index *quidditchv1.QuidditchIndex) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    logger.Info("Creating index", "index", index.Name, "shards", index.Spec.Shards)

    // Get available data nodes
    nodes := &quidditchv1.QuidditchNodeList{}
    if err := r.List(ctx, nodes, client.InNamespace(index.Namespace)); err != nil {
        return ctrl.Result{}, err
    }

    // Filter healthy data nodes
    healthyNodes := filterHealthyDataNodes(nodes.Items)
    if len(healthyNodes) == 0 {
        logger.Info("No healthy data nodes available")
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    // Allocate shards using round-robin
    allocations := r.allocateShards(index.Spec.Shards, healthyNodes)

    // Create shards on data nodes
    for _, alloc := range allocations {
        if err := r.DataNodeClient.CreateShard(ctx, alloc.NodeID, index.Name, alloc.ShardID); err != nil {
            logger.Error(err, "Failed to create shard", "node", alloc.NodeID, "shard", alloc.ShardID)
            continue
        }
    }

    // Update status
    index.Status.State = "CREATING"
    index.Status.ShardAllocations = allocations
    if err := r.Status().Update(ctx, index); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *IndexReconciler) reconcileCreating(ctx context.Context, index *quidditchv1.QuidditchIndex) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // Check if all shards are started
    allStarted := true
    for i, alloc := range index.Status.ShardAllocations {
        info, err := r.DataNodeClient.GetShardInfo(ctx, alloc.NodeID, index.Name, alloc.ShardID)
        if err != nil {
            logger.Error(err, "Failed to get shard info")
            allStarted = false
            continue
        }

        // Update allocation state
        index.Status.ShardAllocations[i].State = info.State
        if info.State != "STARTED" {
            allStarted = false
        }
    }

    // Update status
    if err := r.Status().Update(ctx, index); err != nil {
        return ctrl.Result{}, err
    }

    // Transition to ACTIVE if all started
    if allStarted {
        index.Status.State = "ACTIVE"
        if err := r.Status().Update(ctx, index); err != nil {
            return ctrl.Result{}, err
        }
        logger.Info("Index active", "index", index.Name)
        return ctrl.Result{}, nil
    }

    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *IndexReconciler) reconcileActive(ctx context.Context, index *quidditchv1.QuidditchIndex) (ctrl.Result, error) {
    // Verify all shards are healthy
    // Rebalance if needed
    // Handle spec changes (scaling)
    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *IndexReconciler) reconcileDelete(ctx context.Context, index *quidditchv1.QuidditchIndex) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    logger.Info("Deleting index", "index", index.Name)

    // Delete shards from data nodes
    for _, alloc := range index.Status.ShardAllocations {
        if err := r.DataNodeClient.DeleteShard(ctx, alloc.NodeID, index.Name, alloc.ShardID); err != nil {
            logger.Error(err, "Failed to delete shard")
        }
    }

    // Remove finalizer
    index.Finalizers = removeString(index.Finalizers, "quidditch.io/index-protection")
    if err := r.Update(ctx, index); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IndexReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&quidditchv1.QuidditchIndex{}).
        Complete(r)
}
```

### Node Heartbeat Controller

```go
type NodeReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // Get QuidditchNode
    node := &quidditchv1.QuidditchNode{}
    if err := r.Get(ctx, req.NamespacedName, node); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Check last heartbeat
    lastHeartbeat, err := time.Parse(time.RFC3339, node.Status.LastHeartbeat)
    if err == nil {
        timeSinceHeartbeat := time.Since(lastHeartbeat)

        // Mark unhealthy if no heartbeat for 60 seconds
        if timeSinceHeartbeat > 60*time.Second {
            if node.Status.State != "UNHEALTHY" {
                logger.Info("Node unhealthy", "node", node.Name, "lastHeartbeat", lastHeartbeat)
                node.Status.State = "UNHEALTHY"
                if err := r.Status().Update(ctx, node); err != nil {
                    return ctrl.Result{}, err
                }
            }
        }
    }

    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}
```

---

## Performance Analysis

### Latency Comparison

#### Shard Allocation

**Internal Raft (Traditional Masters)**:
```
1. Client → Coord: CreateIndex (5ms network)
2. Coord → Master: CreateIndex RPC (5ms network)
3. Master: Raft commit (2-5ms consensus)
4. Master → Data nodes: CreateShard RPC (5ms × N shards)
5. Data nodes → Master: Ack (5ms × N)
Total: ~25-35ms + (10ms × N shards)
```

**K8S-Native**:
```
1. Client → Operator: CreateIndex CRD (5ms network)
2. K8S API → etcd: Raft commit (5-10ms consensus)
3. Operator watch: Event delivery (10-50ms)
4. Operator → Data nodes: CreateShard RPC (5ms × N shards)
5. Data nodes → Operator: Ack (5ms × N)
6. Operator: Update CRD status (5-10ms)
Total: ~40-80ms + (10ms × N shards)
```

**Difference**: +15-45ms (acceptable for index creation, which is rare)

#### Routing Query

**Internal Raft**:
```
1. Coord → Master: GetShardRouting (5ms network)
2. Master: Read from memory (< 1ms)
3. Master → Coord: Response (5ms network)
Total: ~11ms (cached for 5 min)
```

**K8S-Native**:
```
1. Operator: Watch QuidditchIndex CRD (always up-to-date)
2. Operator: In-memory cache (< 1ms)
3. Coord → Operator: GetShardRouting (5ms network if needed)
Total: ~1ms (if cached in operator) or ~11ms (if network call)
```

**Difference**: Same or better (operator can cache locally)

#### Heartbeat

**Internal Raft**:
```
1. Data node → Master: Heartbeat (5ms network)
2. Master: Record in memory (< 1ms)
3. Master → Data node: Ack (5ms network)
Total: ~11ms every 30 seconds
```

**K8S-Native**:
```
1. Data node → Operator: Update QuidditchNode CRD (5ms network)
2. K8S API → etcd: Write (5-10ms)
3. Operator watch: Event (10-50ms async)
Total: ~20-65ms every 30 seconds (but can be async)
```

**Difference**: +9-54ms (acceptable, not on critical path)

### Throughput Comparison

| Operation | Internal Raft | K8S-Native | Notes |
|-----------|--------------|------------|-------|
| Index creates | 100-200/sec | 50-100/sec | K8S API throttling |
| Routing queries | 10,000/sec | 10,000/sec | Cached in both |
| Heartbeats | 1000/sec | 500/sec | Async in K8S |
| Shard allocations | 1000/sec | 500/sec | Limited by etcd |

**Bottleneck**: K8S API throttling (5000 requests/sec default)

**Reality**: Cluster operations are NOT high-throughput
- Index creates: <10/sec (humans creating indices)
- Shard allocations: <100/sec (batch operations)
- Routing queries: Cached (1/5min per coordination node)

**Verdict**: K8S API throughput is sufficient.

---

## Trade-offs Re-examined

### Traditional Masters (Internal Raft)

#### Advantages

✅ **Portability**: Same code works in K8S, VMs, bare metal
✅ **Lower latency**: 2-5ms Raft commit vs 5-10ms K8S API
✅ **Higher throughput**: No K8S API throttling
✅ **Proven pattern**: Elasticsearch, OpenSearch, etcd, Consul
✅ **Custom data model**: Not constrained by K8S types
✅ **Independent**: Doesn't depend on K8S control plane

#### Disadvantages

❌ **Extra pods**: 3 master pods (cost: $162/month)
❌ **Persistent volumes**: 3 × 50 GB PVCs
❌ **Custom implementation**: Raft, cluster state, allocation logic
❌ **More operational complexity**: Separate control plane to monitor
❌ **Not cloud-native**: Doesn't leverage K8S primitives
❌ **No kubectl integration**: Need custom CLI for cluster management

### K8S-Native (Operator Pattern)

#### Advantages

✅ **Zero extra pods**: Operator runs as Deployment (1-3 replicas)
✅ **No persistent volumes**: etcd is K8S control plane's concern
✅ **Leverage K8S**: Leader election, CRDs, watches, RBAC
✅ **kubectl native**: `kubectl get quidditchindex` works
✅ **Cloud-native**: Standard Operator pattern
✅ **Less code**: K8S handles distribution, consensus, storage
✅ **Better observability**: K8S events, standard metrics
✅ **Simpler deployment**: One operator Deployment vs StatefulSet + Service

#### Disadvantages

❌ **K8S-only**: Can't deploy in bare metal without K8S
❌ **Higher latency**: +15-45ms for cluster operations
❌ **Lower throughput**: K8S API throttling (5000 req/sec)
❌ **Eventual watch delivery**: 10-100ms watch latency
❌ **CRD learning curve**: Team needs K8S expertise
❌ **API server dependency**: If API server down, control plane down

---

## Hybrid Architecture

### Best of Both Worlds

**Option 3: Hybrid with Auto-Detection**

```go
type ControlPlane interface {
    CreateIndex(ctx context.Context, name string, config IndexConfig) error
    GetShardRouting(ctx context.Context, indexName string) (map[int32]*ShardRouting, error)
    RegisterNode(ctx context.Context, node NodeInfo) error
    Heartbeat(ctx context.Context, nodeID string, health HealthStatus) error
}

// RaftControlPlane: Traditional master nodes
type RaftControlPlane struct {
    raft   *RaftCluster
    logger *zap.Logger
}

// K8SControlPlane: Operator pattern
type K8SControlPlane struct {
    client client.Client
    logger *zap.Logger
}

// Factory
func NewControlPlane(config Config) ControlPlane {
    if config.Mode == "auto" {
        if isRunningInK8S() {
            // Use K8S-native
            return NewK8SControlPlane(config)
        }
        // Fall back to Raft
        return NewRaftControlPlane(config)
    }

    // Explicit mode
    switch config.Mode {
    case "raft":
        return NewRaftControlPlane(config)
    case "k8s":
        return NewK8SControlPlane(config)
    default:
        panic("invalid mode")
    }
}
```

### Configuration

```yaml
# Helm values.yaml
controlPlane:
  mode: auto  # auto, raft, k8s

  # Raft configuration (if mode=raft or auto + not K8S)
  raft:
    enabled: true
    replicas: 3
    storage:
      size: 50Gi
    resources:
      requests:
        memory: 4Gi
        cpu: 2

  # K8S configuration (if mode=k8s or auto + K8S)
  k8s:
    enabled: true
    operator:
      replicas: 3  # Operator replicas (for HA)
      resources:
        requests:
          memory: 1Gi  # Much less than masters
          cpu: 1
```

### Migration Path

```bash
# Start with K8S-native
helm install quidditch quidditch/quidditch \
  --set controlPlane.mode=k8s

# Migrate to Raft (if need portability)
helm upgrade quidditch quidditch/quidditch \
  --set controlPlane.mode=raft \
  --set controlPlane.raft.enabled=true

# Controller migrates state from CRDs to Raft
# Waits for Raft cluster ready
# Switches traffic to Raft
# Keeps CRDs for read-only access
```

---

## Recommendation Reconsidered

### For Production K8S Deployments

**If you will ONLY EVER deploy in Kubernetes**: Use **K8S-native**

#### Rationale

1. **Cost**: Save $162/month + 150 GB storage
2. **Simplicity**: One operator Deployment vs StatefulSet + complexity
3. **Cloud-native**: Leverage K8S primitives (the modern way)
4. **Consistency**: K8S API is strongly consistent (backed by etcd/Raft)
5. **Latency**: Extra 15-45ms on rare operations (acceptable)
6. **kubectl integration**: Native K8S tooling works
7. **Standard pattern**: Operators are how distributed systems are built in K8S now

#### Trade-off

❌ Can't deploy to bare metal/VMs without K8S

**Question**: Do you actually need bare metal deployment?
- If answer is NO: Use K8S-native ✅
- If answer is MAYBE: Use Hybrid with auto-detection
- If answer is YES: Use traditional masters

### For Multi-Environment Deployments

**If you need bare metal, VMs, AND Kubernetes**: Use **Traditional Masters**

#### Rationale

1. **Portability**: Same architecture everywhere
2. **Proven**: Elasticsearch/OpenSearch pattern
3. **Lower latency**: Native Raft (2-5ms vs 5-10ms)
4. **Independence**: Not coupled to K8S
5. **Cost**: $162/month is negligible for multi-environment flexibility

### Decision Matrix

| Requirement | Traditional Masters | K8S-Native | Hybrid |
|-------------|---------------------|------------|--------|
| K8S-only deployment | ⚠️ Works but overkill | ✅ Recommended | ✅ Works |
| Bare metal needed | ✅ Required | ❌ Doesn't work | ✅ Works |
| Cost-sensitive | ⚠️ +$162/month | ✅ Minimal | ⚠️ Mode-dependent |
| Cloud-native philosophy | ❌ Brings own control plane | ✅ Leverages K8S | ⚠️ Mixed |
| Operational simplicity | ❌ More complex | ✅ Simpler | ⚠️ Mode-dependent |
| Latency-sensitive | ✅ 2-5ms | ⚠️ 5-20ms | ⚠️ Mode-dependent |
| kubectl integration | ❌ Need custom CLI | ✅ Native | ⚠️ Mode-dependent |

### Revised Recommendation

**2026 Cloud-Native Best Practice**:

1. **Default**: K8S-native operator (leverage K8S)
2. **If multi-environment**: Traditional masters (portability)
3. **If unsure**: Hybrid with auto-detection (flexibility)

**Key insight**: The "always use traditional masters" recommendation was based on 2020 thinking. In 2026, with Operator SDK maturity and CRD capabilities, K8S-native is the modern approach.

---

## Conclusion

### Why K8S-Native Should Be Considered

1. **K8S already has Raft**: etcd uses Raft, so you get strong consistency
2. **Operator pattern is standard**: Vitess, TiDB, Strimzi all use it
3. **Lower cost**: Save $162/month + storage
4. **Simpler**: Leverage K8S primitives instead of building your own
5. **Better tooling**: kubectl, K8S events, standard RBAC
6. **Cloud-native**: Aligns with modern architecture principles

### When Traditional Masters Still Make Sense

1. **Bare metal deployment required**
2. **Very latency-sensitive** (need <5ms cluster operations)
3. **Very high throughput** (>5000 cluster ops/sec)
4. **Independence from K8S control plane** (availability concern)

### The Answer

**"Is master node still required in K8S?"**

**No** - K8S provides all the primitives (etcd, leader election, CRDs, watches) needed for a control plane. The Operator pattern is the modern, cloud-native way.

**But** - Traditional masters are still a valid choice if you need:
- Portability to non-K8S environments
- Lower latency
- Independence from K8S API server

**Recommended approach**: Start with K8S-native, add traditional masters if portability becomes a requirement.

---

**Document Version**: 1.0
**Last Updated**: 2026-01-26
**Status**: Architectural Re-evaluation
