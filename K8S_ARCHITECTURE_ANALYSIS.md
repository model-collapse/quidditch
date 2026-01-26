# Kubernetes Architecture Analysis: Is Master Node Still Required?

**Date:** 2026-01-26
**Question:** When deploying on Kubernetes, is the Quidditch master node still required?
**Answer:** **It depends** - there are multiple architectural options with different tradeoffs.

---

## Current Master Node Responsibilities

Based on the codebase analysis, the Quidditch master node currently provides:

### 1. **Cluster State Management** (Raft Consensus)
- **What it does**: Maintains authoritative cluster state across 3-5 master nodes
- **Implementation**: HashiCorp Raft library, custom FSM
- **State stored**:
  - Cluster UUID and version
  - Index metadata (indices map)
  - Node registry (nodes map)
  - Shard routing table (shard allocations)

### 2. **Index Lifecycle Management**
- Create/delete indices
- Update index settings and mappings
- Index state transitions (open, closed, deleting)
- Schema validation and evolution

### 3. **Shard Allocation & Routing**
- Decide which data nodes host which shards
- Primary shard placement
- Replica distribution
- Rebalancing on node failures

### 4. **Node Discovery & Health Monitoring**
- Detect node joins/leaves
- Track node health (healthy, degraded, offline)
- Trigger rebalancing on failures
- Heartbeat tracking

### 5. **Leader Election**
- Raft-based leader election among master nodes
- All writes go through leader
- Followers forward requests to leader

---

## What Kubernetes Provides

Kubernetes offers several primitives that overlap with master node functionality:

### 1. **Service Discovery**
- **K8s**: DNS-based service discovery (CoreDNS)
- **Master**: Node registry with gRPC addresses
- **Overlap**: 80% - K8s handles service endpoints

### 2. **Leader Election**
- **K8s**: Lease-based leader election API (`coordination.k8s.io/v1`)
- **Master**: Raft consensus for leader election
- **Overlap**: 100% - K8s can handle leader election

### 3. **Configuration Management**
- **K8s**: ConfigMaps, Secrets
- **Master**: Cluster settings in Raft state
- **Overlap**: 50% - K8s handles config distribution, not validation

### 4. **State Storage**
- **K8s**: etcd (used by K8s API server)
- **Master**: Embedded etcd or custom Raft
- **Overlap**: Partial - K8s etcd is for K8s resources, not application state

### 5. **Health Checking & Failover**
- **K8s**: Liveness/readiness probes, automatic pod restart
- **Master**: Heartbeat tracking, rebalancing triggers
- **Overlap**: 70% - K8s handles node-level health, not shard-level

### 6. **Scheduling**
- **K8s**: Pod placement based on resources, affinity, taints/tolerations
- **Master**: Shard allocation based on storage tier, capacity, existing shards
- **Overlap**: 30% - K8s handles pod placement, not shard placement

---

## Architectural Options on Kubernetes

### Option 1: Keep Master Node (Recommended for Production)

**Architecture:**
```
┌─────────────────────────────────────────────────────┐
│              Kubernetes Cluster                      │
├─────────────────────────────────────────────────────┤
│                                                      │
│  ┌────────────────────────────────────────────┐   │
│  │  Master StatefulSet (3-5 replicas)         │   │
│  │  • Raft consensus for cluster state        │   │
│  │  • Index/shard management                   │   │
│  │  • Custom allocation logic                  │   │
│  │  • Persistent storage for Raft log         │   │
│  └────────────────────────────────────────────┘   │
│                      ↓                              │
│  ┌────────────────────────────────────────────┐   │
│  │  Coordination Deployment (auto-scaling)     │   │
│  │  • Query planning & execution               │   │
│  │  • Result aggregation                       │   │
│  │  • Gets routing from master                 │   │
│  └────────────────────────────────────────────┘   │
│                      ↓                              │
│  ┌────────────────────────────────────────────┐   │
│  │  Data StatefulSet (10-1000+ replicas)      │   │
│  │  • Local storage (PVs)                      │   │
│  │  • Shard hosting                            │   │
│  │  • Registers with master                    │   │
│  └────────────────────────────────────────────┘   │
│                                                      │
└─────────────────────────────────────────────────────┘
```

**Pros:**
✅ **Full control over shard allocation logic**
- Custom placement strategies (storage tiers, zones, rack awareness)
- Optimization for search workloads (co-location, data skew)
- Gradual rebalancing control

✅ **Application-level state management**
- Index metadata, mappings, settings
- Shard versioning and history
- Custom cluster metadata

✅ **Elasticsearch/OpenSearch compatibility**
- Familiar architecture for users migrating from ES/OS
- Standard master node APIs
- Existing monitoring/tooling works

✅ **Independent of K8s API server**
- Works if K8s control plane is down (read-only)
- No dependency on K8s etcd performance
- Can run outside K8s (Docker Compose, bare metal)

✅ **Multi-region/multi-cluster support**
- Master nodes can coordinate across K8s clusters
- Cross-region replication control
- Federation support

**Cons:**
❌ Additional operational complexity (3-5 master pods)
❌ Need to manage Raft cluster lifecycle
❌ Extra resource usage (~500MB RAM per master)
❌ Another consensus system to monitor

**When to use:**
- Production deployments
- Multi-cluster/multi-region setups
- Need for custom shard allocation logic
- Elasticsearch/OpenSearch migration
- Large clusters (100+ nodes)

---

### Option 2: Master-less with Operator Pattern

**Architecture:**
```
┌─────────────────────────────────────────────────────┐
│              Kubernetes Cluster                      │
├─────────────────────────────────────────────────────┤
│                                                      │
│  ┌────────────────────────────────────────────┐   │
│  │  Quidditch Operator (K8s Controller)        │   │
│  │  • Watches QuidditchCluster CRD             │   │
│  │  • Creates/updates data node StatefulSets   │   │
│  │  • Manages shard allocation via labels      │   │
│  │  • Stores state in K8s resources (CRDs)     │   │
│  └────────────────────────────────────────────┘   │
│                      ↓                              │
│  ┌────────────────────────────────────────────┐   │
│  │  Custom Resources (CRDs)                    │   │
│  │  • QuidditchCluster                         │   │
│  │  • QuidditchIndex                           │   │
│  │  • QuidditchShard                           │   │
│  └────────────────────────────────────────────┘   │
│                      ↓                              │
│  ┌────────────────────────────────────────────┐   │
│  │  Coordination Deployment                    │   │
│  │  • Queries K8s API for routing              │   │
│  │  • Service discovery via K8s DNS            │   │
│  └────────────────────────────────────────────┘   │
│                      ↓                              │
│  ┌────────────────────────────────────────────┐   │
│  │  Data StatefulSet                           │   │
│  │  • Labels indicate shard assignments        │   │
│  │  • Register via K8s API (annotations)       │   │
│  └────────────────────────────────────────────┘   │
│                                                      │
└─────────────────────────────────────────────────────┘
```

**How it works:**
1. **CRDs define cluster resources**:
   ```yaml
   apiVersion: quidditch.io/v1
   kind: QuidditchIndex
   metadata:
     name: products
   spec:
     shards: 10
     replicas: 2
     settings:
       refresh_interval: "1s"
   ```

2. **Operator reconciles state**:
   - Watches QuidditchIndex resources
   - Creates/updates StatefulSets for data nodes
   - Assigns shards via pod labels/annotations
   - Updates status in CRD

3. **Coordination queries K8s API**:
   - Gets shard routing from pod labels
   - Uses K8s DNS for service discovery
   - No master node needed

**Pros:**
✅ **Kubernetes-native** - leverages K8s primitives
✅ **Simpler deployment** - no master StatefulSet
✅ **Lower resource usage** - no master pods
✅ **GitOps-friendly** - CRDs are declarative
✅ **Better K8s integration** - standard tooling works

**Cons:**
❌ **K8s API dependency** - can't work without K8s
❌ **Less control** - limited custom allocation logic
❌ **CRD performance** - K8s etcd not optimized for high write rates
❌ **Not Elasticsearch-compatible** - different architecture
❌ **Single-cluster only** - can't coordinate across clusters

**When to use:**
- Small-medium deployments (<100 nodes)
- K8s-only environments
- Prefer K8s-native approach
- No multi-cluster requirements
- Greenfield projects

---

### Option 3: Hybrid Approach

**Architecture:**
```
┌─────────────────────────────────────────────────────┐
│              Kubernetes Cluster                      │
├─────────────────────────────────────────────────────┤
│                                                      │
│  ┌────────────────────────────────────────────┐   │
│  │  Master StatefulSet (3 replicas)            │   │
│  │  • Index metadata & mappings only           │   │
│  │  • No shard routing (delegated to operator) │   │
│  │  • Smaller state, simpler consensus         │   │
│  └────────────────────────────────────────────┘   │
│           ↓                          ↓              │
│  ┌────────────────┐     ┌────────────────────┐   │
│  │    Operator    │     │  Coordination       │   │
│  │  • Shard alloc │     │  • Queries both     │   │
│  │  • Via CRDs    │     │    master & K8s API │   │
│  └────────────────┘     └────────────────────┘   │
│           ↓                          ↓              │
│  ┌────────────────────────────────────────────┐   │
│  │  Data StatefulSet                           │   │
│  └────────────────────────────────────────────┘   │
│                                                      │
└─────────────────────────────────────────────────────┘
```

**Division of responsibilities:**
- **Master**: Index lifecycle, mappings, settings, cluster metadata
- **Operator**: Shard allocation, node management, scaling
- **K8s**: Service discovery, health checks, pod management

**Pros:**
✅ Mix of control and simplicity
✅ Smaller Raft state (faster)
✅ Still ES-compatible for index management
✅ Operator handles K8s-specific concerns

**Cons:**
❌ Split brain risk (two sources of truth)
❌ More complex to reason about
❌ Both systems need coordination

**When to use:**
- Migration path from master-based to operator-based
- Gradual adoption of K8s patterns
- Experimentation phase

---

## Detailed Comparison

### State Storage

| Aspect | Master Node | K8s Operator | Hybrid |
|--------|-------------|--------------|--------|
| **Index metadata** | Raft FSM | CRD (etcd) | Raft FSM |
| **Shard routing** | Raft FSM | Pod labels/CRD | CRD |
| **Node registry** | Raft FSM | Pod list | Both |
| **Settings** | Raft FSM | ConfigMap | Both |
| **Write rate** | High (optimized) | Medium (K8s API limits) | Medium |
| **Consistency** | Strong (Raft) | Strong (etcd) | Eventual |

### Shard Allocation

| Aspect | Master Node | K8s Operator | Hybrid |
|--------|-------------|--------------|--------|
| **Placement strategy** | Custom Go logic | K8s scheduler + operator | Mix |
| **Storage tier awareness** | ✅ Full control | ⚠️ Via node selectors | ⚠️ Via node selectors |
| **Rack awareness** | ✅ Full control | ⚠️ Via topology constraints | ⚠️ Via topology constraints |
| **Gradual rebalancing** | ✅ Custom algorithms | ❌ Pod recreate | ⚠️ Operator-controlled |
| **Data skew handling** | ✅ Custom logic | ❌ Limited | ⚠️ Limited |

### Operational Complexity

| Aspect | Master Node | K8s Operator | Hybrid |
|--------|-------------|--------------|--------|
| **Setup complexity** | High | Medium | High |
| **Monitoring** | Custom + Raft metrics | K8s standard | Both |
| **Debugging** | Need Raft knowledge | K8s standard tools | Both |
| **Upgrades** | Rolling restart | Operator upgrade | Both |
| **Backup/restore** | Raft snapshots | K8s resource backup | Both |

### Performance

| Aspect | Master Node | K8s Operator | Hybrid |
|--------|-------------|--------------|--------|
| **State update latency** | Low (Raft) | Medium (K8s API) | Medium |
| **Query routing lookup** | Fast (in-memory) | Medium (K8s API call) | Fast |
| **Cluster size limit** | 1000+ nodes | ~500 nodes (CRD limits) | ~750 nodes |
| **Failover time** | Seconds (Raft election) | Seconds (pod restart) | Seconds |

---

## Recommendation Matrix

### Choose **Master Node (Option 1)** if:

✅ Production deployment with >100 data nodes
✅ Multi-cluster or multi-region deployment
✅ Need custom shard allocation logic
✅ Migrating from Elasticsearch/OpenSearch
✅ Storage tier awareness is critical
✅ Want to work outside K8s (Docker, bare metal)
✅ Need sub-second failover times
✅ High state update rates (>1000/sec)

**Example: Large e-commerce platform**
- 500 data nodes across 3 regions
- Complex allocation: SSD tier for hot data, HDD for cold
- 10K+ indices with frequent mapping updates
- Need ES compatibility for existing tools

---

### Choose **K8s Operator (Option 2)** if:

✅ Small-medium deployment (<100 data nodes)
✅ Single K8s cluster only
✅ Prefer K8s-native approach
✅ Want simpler operational model
✅ GitOps workflow (declarative CRDs)
✅ Limited ops team (leverage K8s expertise)
✅ Don't need ES compatibility

**Example: SaaS application logging**
- 20 data nodes in single K8s cluster
- Simple deployment, standard allocation
- <1000 indices
- Ops team experienced with K8s, not Elasticsearch

---

### Choose **Hybrid (Option 3)** if:

⚠️ Migration from master-based to operator-based
⚠️ Experimentation phase
⚠️ Need both ES compatibility and K8s integration

**Note:** Hybrid is generally not recommended for production due to complexity.

---

## Implementation Guide for Option 2 (K8s Operator)

If you decide to go master-less with K8s operator, here's the high-level approach:

### 1. Define CRDs

```yaml
# pkg/operator/crds/quidditchcluster_crd.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: quidditchclusters.quidditch.io
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
                version:
                  type: string
                coordinationNodes:
                  type: integer
                dataNodes:
                  type: integer
                masterNodes:
                  type: integer  # 0 for master-less
            status:
              type: object
              properties:
                phase:
                  type: string
                nodes:
                  type: array

---

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
                settings:
                  type: object
            status:
              type: object
              properties:
                state:
                  type: string  # creating, active, deleting
                shardAllocation:
                  type: object  # shard -> node mapping
```

### 2. Operator Controller

```go
// pkg/operator/controller/index_controller.go
package controller

import (
    "context"
    quidditchv1 "github.com/quidditch/quidditch/pkg/operator/api/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type IndexReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func (r *IndexReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Get QuidditchIndex resource
    var index quidditchv1.QuidditchIndex
    if err := r.Get(ctx, req.NamespacedName, &index); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Allocate shards to data nodes
    allocation := r.allocateShards(&index)

    // Update pod labels with shard assignments
    if err := r.updateDataNodeLabels(ctx, allocation); err != nil {
        return ctrl.Result{}, err
    }

    // Update index status
    index.Status.State = "active"
    index.Status.ShardAllocation = allocation
    if err := r.Status().Update(ctx, &index); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}

func (r *IndexReconciler) allocateShards(index *quidditchv1.QuidditchIndex) map[string]string {
    // Get list of data node pods
    pods := &corev1.PodList{}
    r.List(context.Background(), pods,
        client.MatchingLabels{"app": "quidditch-data"})

    // Simple round-robin allocation
    allocation := make(map[string]string)
    nodeIndex := 0
    for i := 0; i < int(index.Spec.Shards); i++ {
        shardKey := fmt.Sprintf("%s:%d", index.Name, i)
        allocation[shardKey] = pods.Items[nodeIndex].Name
        nodeIndex = (nodeIndex + 1) % len(pods.Items)
    }

    return allocation
}
```

### 3. Coordination Node Queries K8s API

```go
// pkg/coordination/routing/k8s_router.go
package routing

import (
    "context"
    "fmt"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/client-go/kubernetes"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type K8sRouter struct {
    k8sClient client.Client
}

func (r *K8sRouter) GetShardLocation(indexName string, shardID int) (string, error) {
    // Get QuidditchIndex resource
    var index quidditchv1.QuidditchIndex
    key := client.ObjectKey{
        Namespace: "default",
        Name:      indexName,
    }
    if err := r.k8sClient.Get(context.Background(), key, &index); err != nil {
        return "", err
    }

    // Lookup shard allocation from status
    shardKey := fmt.Sprintf("%s:%d", indexName, shardID)
    nodeName, ok := index.Status.ShardAllocation[shardKey]
    if !ok {
        return "", fmt.Errorf("shard not allocated")
    }

    // Get pod details
    var pod corev1.Pod
    podKey := client.ObjectKey{
        Namespace: "default",
        Name:      nodeName,
    }
    if err := r.k8sClient.Get(context.Background(), podKey, &pod); err != nil {
        return "", err
    }

    // Return pod IP:port
    return fmt.Sprintf("%s:9300", pod.Status.PodIP), nil
}
```

---

## Migration Path

If you want to migrate from master-based to operator-based:

### Phase 1: Dual-Write (Month 1)
- Keep master nodes running
- Operator watches master state and mirrors to CRDs
- Coordination reads from master (primary) and K8s API (backup)

### Phase 2: Dual-Read (Month 2)
- Coordination reads from both sources, prefers K8s API
- Validate consistency between master and CRDs
- Fix any discrepancies

### Phase 3: Operator Primary (Month 3)
- Operator becomes source of truth
- Master becomes read-only (for ES compatibility only)
- Coordination reads only from K8s API

### Phase 4: Deprecate Master (Month 4)
- Remove master pods
- Update documentation
- Cleanup code

---

## Conclusion

### Recommended Approach: **Keep Master Node (Option 1)** for Production

**Rationale:**
1. **Proven architecture** - Elasticsearch/OpenSearch use similar design
2. **Full control** - Custom shard allocation for search workloads
3. **Multi-cluster support** - Essential for large deployments
4. **Independence** - Not tied to K8s (can run elsewhere)
5. **Performance** - Optimized for high write rates

**Deploy on K8s with:**
- Master nodes: StatefulSet (3-5 replicas)
- Coordination nodes: Deployment (auto-scaling)
- Data nodes: StatefulSet (10-1000+ replicas)
- Use K8s for: service discovery, health checks, pod management
- Use master for: cluster state, shard allocation, index management

### When to Reconsider

Consider the operator approach (Option 2) **only if**:
- Small deployment (<50 nodes)
- Single K8s cluster
- No ES compatibility needed
- Ops team prefers K8s-native tools
- Willing to sacrifice some control for simplicity

### Next Steps

1. **Create K8s manifests** for master, coordination, data StatefulSets
2. **Develop Helm chart** for easy deployment
3. **Write K8s operator** (future) for day-2 operations (scaling, upgrades)
4. **Keep master node** as core architecture component

---

**Bottom Line:** The master node provides significant value even on Kubernetes. Keep it for production deployments. Use K8s for pod management, not application-level state management.
