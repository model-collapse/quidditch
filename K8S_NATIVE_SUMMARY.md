# K8S-Native vs Traditional Masters: Summary

**Date**: 2026-01-26
**Status**: Architectural Decision Document

---

## The Question Revisited

**"If Kubernetes already provides status sync and cluster management, why shouldn't Quidditch be K8S-native?"**

**Short answer**: You're right - **it should be K8S-native by default** (for K8S deployments).

---

## Key Realizations

### 1. K8S Already Has Raft

```
Your Kubernetes cluster:
    API Server
        ↓
    etcd cluster (3-5 nodes)
        ↓
    Raft consensus ← YOU ALREADY HAVE THIS!
        ↓
    Strong consistency
```

**Insight**: When you use K8S API, you're using Raft indirectly. Building another Raft cluster (master nodes) is **redundant**.

### 2. Strong Consistency Is Not Lost

**Myth**: "K8S is eventually consistent"
**Reality**: K8S API server is **strongly consistent** (linearizable)

**Proof**:
```go
// All K8S API operations guarantee:
index, _ := client.Create(ctx, index)   // Write
fetched, _ := client.Get(ctx, index.Name)  // Read
// fetched ALWAYS reflects the write (read-your-writes)

// Optimistic locking via resourceVersion
index.ResourceVersion = "12345"
client.Update(ctx, index)  // Fails if version changed
```

**Trade-off**: Watch events have 10-100ms latency, but all writes/reads are strongly consistent.

### 3. The Operator Pattern Is Standard

Modern distributed systems in K8S:

| System | Architecture | Control Plane |
|--------|--------------|---------------|
| Vitess | K8S Operator | No separate masters |
| TiDB | K8S Operator | CRDs for state |
| Strimzi (Kafka) | K8S Operator | No separate masters |
| Zalando Postgres | K8S Operator | K8S-native |
| K8ssandra | K8S Operator | CRDs for cluster state |

**Pattern**: Use CRDs + Controller, not separate StatefulSet for "masters"

### 4. Cost and Complexity Savings

**Traditional Masters**:
```
StatefulSet: 3 replicas
Resources: 3 × (4 GB RAM + 2 CPU + 50 GB storage)
Cost: ~$162/month + operational complexity
```

**K8S-Native Operator**:
```
Deployment: 3 replicas (for HA)
Resources: 3 × (1 GB RAM + 1 CPU)
Cost: ~$40/month
Savings: $122/month + simpler operations
```

### 5. Latency Is Acceptable

| Operation | Traditional | K8S-Native | Frequency |
|-----------|-------------|------------|-----------|
| Index create | 25-35ms | 40-80ms | Rare (<10/sec) |
| Routing query | 11ms (first) | 1ms (cached) | Every 5 min |
| Shard allocation | 10ms/shard | 15ms/shard | Rare |
| **Search query** | **NOT through masters** | **NOT through operator** | High throughput |

**Key**: Cluster operations are RARE. Extra 15-45ms doesn't matter.

**Critical path** (search queries): BYPASSES control plane entirely in both architectures.

---

## Revised Architecture

### K8S-Native (Recommended for K8S-only)

```yaml
# Single Operator Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: quidditch-operator
spec:
  replicas: 3  # For HA
  selector:
    matchLabels:
      app: quidditch-operator
  template:
    spec:
      containers:
      - name: operator
        image: quidditch/operator:1.0.0
        resources:
          requests:
            memory: 1Gi
            cpu: 1

---
# Custom Resources (stored in etcd)
apiVersion: quidditch.io/v1
kind: QuidditchIndex
metadata:
  name: products
spec:
  shards: 10
  replicas: 1
status:
  state: ACTIVE
  shardAllocations:
    - shardId: 0
      nodeId: data-0
      state: STARTED
```

**Benefits**:
- ✅ No master StatefulSet
- ✅ No PVCs for Raft logs
- ✅ Leverages K8S etcd (Raft)
- ✅ `kubectl get quidditchindex` works
- ✅ Standard Operator pattern
- ✅ $122/month savings

### Traditional Masters (For Multi-Environment)

Use only if:
- Need bare metal deployment (no K8S)
- Need multi-cloud portability (same code everywhere)
- Need <5ms cluster operation latency
- Want independence from K8S control plane

---

## Implementation: K8S-Native

### Step 1: Define CRDs

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
              replicas:
                type: integer
          status:
            type: object
            properties:
              state:
                type: string
              shardAllocations:
                type: array
```

### Step 2: Implement Operator

```go
// Reconciliation loop
func (r *IndexReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch QuidditchIndex CRD
    index := &quidditchv1.QuidditchIndex{}
    r.Get(ctx, req.NamespacedName, index)

    // 2. Get actual state from data nodes
    actualShards := r.getCurrentShardState(ctx, index)

    // 3. Reconcile: make actual match desired
    if needsReconciliation(index.Spec, actualShards) {
        r.allocateShards(ctx, index)
    }

    // 4. Update status
    index.Status.ShardAllocations = actualShards
    r.Status().Update(ctx, index)

    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}
```

### Step 3: Leader Election

```go
// Built-in K8S leader election
func (c *Controller) Start(ctx context.Context) error {
    lock := &resourcelock.LeaseLock{
        LeaseMeta: metav1.ObjectMeta{
            Name:      "quidditch-leader",
            Namespace: "quidditch",
        },
    }

    elector, _ := leaderelection.NewLeaderElector(leaderelection.LeaderElectorConfig{
        Lock:          lock,
        LeaseDuration: 15 * time.Second,
        Callbacks: leaderelection.LeaderCallbacks{
            OnStartedLeading: c.becomeLeader,
            OnStoppedLeading: c.stopLeading,
        },
    })

    go elector.Run(ctx)
    return nil
}
```

### Step 4: Service Discovery

```go
// Watch K8S Endpoints for data nodes
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

---

## Trade-offs Comparison

### Traditional Masters

**Pros**:
- ✅ Portable (K8S, VMs, bare metal)
- ✅ Lower latency (2-5ms Raft)
- ✅ Independent of K8S
- ✅ Proven pattern (Elasticsearch)

**Cons**:
- ❌ Extra $162/month
- ❌ 3 master pods + PVCs
- ❌ More operational complexity
- ❌ Custom implementation (Raft, cluster state)
- ❌ Not cloud-native

### K8S-Native

**Pros**:
- ✅ Save $122/month
- ✅ No master pods
- ✅ No PVCs
- ✅ Leverage K8S primitives
- ✅ `kubectl` integration
- ✅ Standard Operator pattern
- ✅ Simpler operations
- ✅ Cloud-native

**Cons**:
- ❌ K8S-only (no bare metal)
- ❌ Higher latency (5-20ms vs 2-5ms)
- ❌ Watch events have 10-100ms delay
- ❌ K8S API throttling (5000 req/sec)

---

## Decision Framework

### Choose K8S-Native If:

✅ **Only deploying to Kubernetes**
- No plans for bare metal, VMs, or non-K8S clouds
- Team comfortable with K8S Operators
- Want to leverage cloud-native patterns

✅ **Cost-sensitive**
- Save $122/month + storage
- Simpler infrastructure

✅ **Prefer simplicity**
- One Operator Deployment vs StatefulSet + complexity
- Standard K8S tooling (kubectl, events, RBAC)

✅ **Latency-tolerant for cluster ops**
- Index creation, shard allocation are rare (<10/sec)
- Extra 15-45ms doesn't matter

### Choose Traditional Masters If:

✅ **Multi-environment deployment**
- Need bare metal, VMs, AND K8S
- Same codebase everywhere

✅ **Latency-critical cluster operations**
- Need <5ms for shard allocation
- Very high cluster operation throughput (>1000/sec)

✅ **Independence from K8S**
- Don't want control plane coupled to K8S API server
- Regulatory or compliance requirements

---

## Hybrid Approach (Best of Both)

```yaml
# Helm values.yaml
controlPlane:
  mode: auto  # auto, k8s, raft

  # K8S operator (if mode=k8s or auto+K8S)
  operator:
    enabled: true
    replicas: 3
    resources:
      memory: 1Gi
      cpu: 1

  # Raft masters (if mode=raft or auto+bare-metal)
  raft:
    enabled: false  # Auto-enabled if not in K8S
    replicas: 3
    resources:
      memory: 4Gi
      cpu: 2
```

**Behavior**:
- Running in K8S? → Use Operator
- Running on VMs? → Use Raft masters
- Need to migrate? → Switch modes with config flag

---

## Revised Recommendation

### For 2026 Cloud-Native Architecture

**Default**: **K8S-Native Operator** ← This is the modern approach

**Rationale**:
1. K8S already provides Raft (via etcd)
2. Operator pattern is standard (Vitess, TiDB, Strimzi)
3. Saves cost and complexity
4. Latency acceptable (cluster ops are rare)
5. Strong consistency preserved
6. Better K8S integration

**Fallback**: Traditional masters if multi-environment needed

### Migration Path

```
Phase 1: Launch with K8S-native (simplicity)
    ↓
Phase 2: If bare metal needed, add Raft mode
    ↓
Phase 3: Hybrid with auto-detection
```

---

## Conclusion

### Your Question Was Right

**"Why shouldn't Quidditch be K8S-native?"**

**Answer**: It should be.

**Key insights**:
1. K8S API is strongly consistent (backed by etcd/Raft)
2. Operator pattern is the 2026 standard
3. Latency trade-off is acceptable (cluster ops are rare)
4. Cost and complexity savings are significant
5. Search queries (hot path) bypass control plane anyway

### Recommendation Change

**Old recommendation**: Always use traditional master nodes

**New recommendation**:
- **K8S deployments**: Use K8S-native operator ✅
- **Multi-environment**: Use traditional masters
- **Unsure**: Hybrid with auto-detection

**Bottom line**: For a cloud-native system in 2026, leveraging K8S primitives (etcd, CRDs, Operators) is the right architectural choice.

---

**Document Version**: 1.0
**Last Updated**: 2026-01-26
**Conclusion**: K8S-native is recommended for K8S-only deployments
