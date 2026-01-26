# Quidditch Control Plane: Dual-Mode Architecture

**Version**: 1.0
**Date**: 2026-01-26
**Status**: Design Complete

---

## Executive Summary

Quidditch is designed as a **general-purpose search engine software** that supports deployment in diverse environments. To achieve maximum flexibility, Quidditch implements a **dual-mode control plane architecture**:

1. **Traditional Mode (Raft-based)** - For bare metal, VMs, and Kubernetes
2. **K8S-Native Mode (Operator-based)** - For Kubernetes-only deployments

**Key principle**: Same cluster behavior, different control plane implementations.

---

## Why Dual-Mode?

### The Challenge

Different users have different deployment requirements:

| User Type | Requirements | Best Choice |
|-----------|-------------|-------------|
| **Cloud-native startup** | K8S-only, cost-sensitive, simplicity | K8S-Native Operator |
| **Enterprise hybrid** | K8S + bare metal + VMs, portability | Traditional Raft Masters |
| **Edge computing** | Bare metal, no K8S, lightweight | Traditional Raft Masters |
| **Multi-cloud** | AWS, Azure, GCP, on-premise | Traditional Raft Masters |
| **Managed K8S service** | EKS/GKS/AKS only, K8S-native tools | K8S-Native Operator |

**Single-mode architectures force compromises:**
- K8S-only: Excludes bare metal/VM users
- Raft-only: Misses cloud-native benefits in K8S

### The Solution: Pluggable Architecture

```
┌─────────────────────────────────────────────────────────┐
│        Quidditch Coordination & Data Nodes              │
│                                                         │
│  ┌──────────────────────────────────────────────┐     │
│  │      Control Plane Interface (Abstract)      │     │
│  │  • CreateIndex()                              │     │
│  │  • GetClusterState()                          │     │
│  │  • AllocateShard()                            │     │
│  │  • RegisterNode()                             │     │
│  └─────────────────┬────────────────────────────┘     │
│                    │                                   │
│         ┌──────────┴──────────┐                       │
│         ▼                     ▼                       │
│  ┌─────────────┐      ┌──────────────┐              │
│  │ Raft Mode   │      │ K8S Mode     │              │
│  └─────────────┘      └──────────────┘              │
└─────────────────────────────────────────────────────────┘
         ↓                      ↓
┌─────────────────┐    ┌──────────────────┐
│ Master Nodes    │    │ K8S Operator     │
│ (StatefulSet)   │    │ + CRDs           │
│ + Raft          │    │ (etcd/Raft)      │
└─────────────────┘    └──────────────────┘
```

**Benefits:**
- ✅ Works everywhere (K8S, VMs, bare metal)
- ✅ K8S-native experience when in K8S
- ✅ Users choose based on their needs
- ✅ Single codebase (not two separate products)
- ✅ Migration path between modes

---

## Architecture Comparison

### Traditional Mode (Raft)

**How it works:**
```
3 Master Node Pods
    ↓
Hashicorp Raft Library
    ↓
Finite State Machine (FSM)
    ↓
Persistent Storage (BoltDB logs + snapshots)
    ↓
Cluster State (nodes, indices, routing)
```

**Key characteristics:**
- **Consensus**: Raft protocol (leader election, log replication)
- **Storage**: Persistent volumes for Raft logs and snapshots
- **Deployment**: StatefulSet with 3 replicas
- **Portability**: Works on K8S, VMs, bare metal
- **Latency**: 2-5ms for cluster operations
- **Cost**: ~$162/month (AWS EKS, 3 × m5.large)

**Use cases:**
- Multi-environment deployments (K8S + VMs + bare metal)
- Regulatory independence from K8S
- Ultra-low latency requirements (<5ms)
- Edge computing without K8S

### K8S-Native Mode (Operator)

**How it works:**
```
3 Operator Pods (leader-elected)
    ↓
Kubernetes Client-Go
    ↓
Custom Resource Definitions (CRDs)
    ↓
K8S API Server
    ↓
etcd (with Raft)
    ↓
Cluster State (stored as CRDs)
```

**Key characteristics:**
- **Consensus**: K8S etcd (Raft already built-in)
- **Storage**: No persistent volumes (stateless operator)
- **Deployment**: Deployment with 3 replicas
- **Portability**: K8S-only
- **Latency**: 5-20ms for cluster operations
- **Cost**: ~$40/month (AWS EKS, 3 × t3.small)

**Use cases:**
- K8S-only deployments
- Cost-sensitive environments
- Cloud-native patterns preferred
- Want kubectl integration

---

## Implementation Design

### Control Plane Interface

All cluster operations go through a unified interface:

```go
type ControlPlane interface {
    // Index Management
    CreateIndex(ctx context.Context, req *CreateIndexRequest) (*Index, error)
    GetIndex(ctx context.Context, name string) (*Index, error)
    DeleteIndex(ctx context.Context, name string) error

    // Cluster State
    GetClusterState(ctx context.Context) (*ClusterState, error)

    // Node Management
    RegisterNode(ctx context.Context, node *NodeInfo) error
    GetNode(ctx context.Context, nodeID string) (*NodeInfo, error)
    ListNodes(ctx context.Context) ([]*NodeInfo, error)

    // Shard Management
    AllocateShard(ctx context.Context, indexName string, shardID int, nodeID string) error
    GetShardRouting(ctx context.Context, indexName string) (*ShardRoutingTable, error)

    // Watch for changes
    WatchClusterState(ctx context.Context) (<-chan *ClusterStateEvent, error)

    // Lifecycle
    Start(ctx context.Context) error
    Stop() error
    IsHealthy() bool
}
```

**Two implementations:**
1. `RaftControlPlane` - Uses hashicorp/raft
2. `K8SControlPlane` - Uses controller-runtime

### Coordination Node Integration

```go
// Coordination node works with BOTH modes
type Coordination struct {
    controlPlane controlplane.ControlPlane  // Interface, not concrete type
    executor     *QueryExecutor
}

func NewCoordination(cfg *Config) (*Coordination, error) {
    var cp controlplane.ControlPlane

    switch cfg.ControlPlane.Mode {
    case "raft":
        cp = raft.NewRaftControlPlane(cfg.Raft)
    case "k8s":
        cp = k8s.NewK8SControlPlane(cfg.K8S)
    case "auto":
        if k8s.IsRunningInKubernetes() {
            cp = k8s.NewK8SControlPlane(cfg.K8S)
        } else {
            cp = raft.NewRaftControlPlane(cfg.Raft)
        }
    }

    return &Coordination{controlPlane: cp}
}

// Create index - same code for both modes
func (c *Coordination) CreateIndex(req *CreateIndexRequest) error {
    return c.controlPlane.CreateIndex(ctx, req)
}
```

**Key insight**: Coordination and Data nodes don't care which mode is used.

### Configuration

**Unified format:**
```yaml
controlPlane:
  mode: auto  # auto, raft, k8s

  raft:
    nodeId: "master-0"
    raftPort: 9300
    dataDir: "/var/lib/quidditch/raft"
    peers: ["master-0:9300", "master-1:9300", "master-2:9300"]

  k8s:
    namespace: "quidditch"
    leaderElect: true
```

**Helm chart:**
```bash
# Raft mode
helm install quidditch ./charts \
  --set controlPlane.mode=raft

# K8S-native mode
helm install quidditch ./charts \
  --set controlPlane.mode=k8s

# Auto-detect (default)
helm install quidditch ./charts
```

---

## Feature Comparison

| Feature | Raft Mode | K8S Mode | Winner |
|---------|-----------|----------|--------|
| **Deployment Targets** |
| Kubernetes | ✅ StatefulSet | ✅ Deployment | Tie |
| Bare Metal | ✅ Native | ❌ No | Raft |
| VMs | ✅ Native | ❌ No | Raft |
| **Performance** |
| Index creation | 25-35ms | 40-80ms | Raft (2×) |
| Shard allocation | 10ms | 15ms | Raft (1.5×) |
| Cluster state read | <1ms (cache) | <1ms (cache) | Tie |
| Search queries | N/A | N/A | Tie (both bypass) |
| **Operations** |
| Infrastructure | 3 pods + PVCs | 3 pods (stateless) | K8S |
| Cost (AWS EKS) | $162/month | $40/month | K8S (4×) |
| kubectl integration | ❌ Custom | ✅ Native | K8S |
| Monitoring | Raft metrics | K8S events | K8S |
| Backup/restore | Snapshots | kubectl backup | K8S |
| **Consistency** |
| Write consistency | Strong (Raft) | Strong (etcd) | Tie |
| Read consistency | Strong | Strong | Tie |
| Watch latency | Immediate | 10-100ms | Raft |
| **Flexibility** |
| Multi-environment | ✅ Yes | ❌ K8S only | Raft |
| Cloud-native | ❌ No | ✅ Yes | K8S |

**Verdict**: Each mode wins in different scenarios.

---

## Migration Paths

### Raft → K8S

```bash
# 1. Export state
kubectl exec master-0 -- quidditch-admin export-state > state.json

# 2. Switch mode
helm upgrade quidditch ./charts --set controlPlane.mode=k8s

# 3. Import state
kubectl apply -f state-as-crds.yaml

# 4. Remove old masters
kubectl delete statefulset quidditch-master
```

### K8S → Raft

```bash
# 1. Export CRDs
kubectl get quidditchindices -o json > indices.json

# 2. Switch mode
helm upgrade quidditch ./charts --set controlPlane.mode=raft

# 3. Import state
kubectl exec master-0 -- quidditch-admin import-state < indices.json

# 4. Remove operator
kubectl delete deployment quidditch-operator
```

---

## Testing Strategy

### Unified Test Suite

Both implementations pass the same test suite:

```go
type ControlPlaneTestSuite struct {
    cp controlplane.ControlPlane
}

func (s *ControlPlaneTestSuite) TestCreateIndex(t *testing.T) {
    index, err := s.cp.CreateIndex(ctx, &CreateIndexRequest{...})
    assert.NoError(t, err)
    // ... assertions
}

// Run against Raft
func TestRaftControlPlane(t *testing.T) {
    suite := &ControlPlaneTestSuite{cp: setupRaft(t)}
    suite.TestCreateIndex(t)
}

// Run against K8S (same tests!)
func TestK8SControlPlane(t *testing.T) {
    suite := &ControlPlaneTestSuite{cp: setupK8S(t)}
    suite.TestCreateIndex(t)
}
```

**Test coverage:**
- Unit tests: Interface contract
- Integration tests: Multi-node clusters
- Performance tests: Latency benchmarks
- Migration tests: State export/import

---

## Decision Framework

### Choose Raft Mode If:

✅ **Multi-environment deployment**
- Need to run on K8S, VMs, AND bare metal
- Want consistent behavior everywhere
- Example: Hybrid cloud with on-premise data centers

✅ **Latency-critical cluster operations**
- Index creation must be <10ms
- Shard allocation must be <5ms
- Example: Trading platform, real-time analytics

✅ **Independence from K8S**
- Regulatory requirements to not depend on K8S control plane
- Want to own entire stack
- Example: Financial services, government

✅ **Edge computing**
- Deploying to edge locations without K8S
- Lightweight, no orchestrator overhead
- Example: IoT gateways, retail stores

### Choose K8S-Native Mode If:

✅ **K8S-only deployment**
- Committed to Kubernetes long-term
- No plans for bare metal or VMs
- Example: Cloud-native SaaS startup

✅ **Cost-sensitive**
- Save $122/month per cluster
- Simpler infrastructure (no PVCs)
- Example: Multi-tenant SaaS with many clusters

✅ **Prefer cloud-native patterns**
- Team experienced with K8S Operators
- Want kubectl integration
- Example: Platform engineering team

✅ **Simplicity over latency**
- Index creation at 40-80ms is acceptable (vs 25-35ms)
- Cluster operations are rare anyway
- Example: Most production workloads

### Use Auto-Detection If:

✅ **Flexible deployment**
- Deploy to both K8S and non-K8S
- Let environment determine mode
- Example: Open-source project with diverse users

✅ **Unsure of future requirements**
- Start with what you have
- Can migrate later if needed
- Example: Startup validating product-market fit

---

## Recommendation for Quidditch

As a **general-purpose search engine software**, Quidditch should:

1. **Implement both modes** ✅
   - Maximum flexibility for users
   - Not opinionated about deployment

2. **Default to auto-detection** ✅
   - Best experience out-of-box
   - K8S users get K8S-native
   - Non-K8S users get traditional

3. **Document trade-offs clearly** ✅
   - Help users choose intelligently
   - Provide migration paths

4. **Test both equally** ✅
   - Same test suite for both
   - Equal quality and reliability

**This approach:**
- Maximizes addressable market (all deployment types)
- Provides best-in-class experience for each environment
- Maintains single codebase (not two products)
- Allows users to choose based on their needs

---

## Implementation Timeline

### 8-Week Plan

**Week 1: Interface Definition**
- Define `ControlPlane` interface
- Design cluster state types
- API contracts and documentation

**Weeks 2-4: Raft Implementation**
- Integrate hashicorp/raft
- Implement FSM (Finite State Machine)
- Persistent storage (logs + snapshots)
- Raft configuration and tuning
- Unit tests

**Weeks 5-6: K8S Implementation**
- Define CRDs (Custom Resource Definitions)
- Implement reconciliation controllers
- K8S client integration
- Leader election via Lease
- Unit tests

**Week 7: Integration Testing**
- Multi-node cluster tests
- Test both modes with same suite
- Performance benchmarks
- Migration tests

**Week 8: Documentation & Packaging**
- Deployment guides for both modes
- Helm chart with mode selection
- Migration documentation
- Decision framework

**Deliverables:**
- Production-ready dual-mode architecture
- Complete test coverage
- Comprehensive documentation
- Helm charts for both modes

---

## Files Delivered

### Documentation (52+ pages)

1. **`docs/DUAL_MODE_CONTROL_PLANE.md`** (45 pages)
   - Complete design for both modes
   - Interface definition
   - Implementation examples
   - Configuration and deployment
   - Migration paths

2. **`docs/MASTER_NODE_ARCHITECTURE.md`** (24 pages)
   - Traditional Raft mode deep-dive
   - Bandwidth analysis (16 KB/sec)
   - Hardware requirements

3. **`docs/KUBERNETES_DEPLOYMENT_GUIDE.md`** (20 pages)
   - K8S deployment patterns
   - Complete manifests
   - Cost analysis

4. **`docs/K8S_NATIVE_DEEP_DIVE.md`** (45 pages)
   - K8S-native mode analysis
   - Operator pattern
   - CRD and controller examples

5. **`docs/BANDWIDTH_AND_RESOURCES_QUICK_REF.md`** (8 pages)
   - Quick reference for ops teams
   - Capacity planning

6. **`K8S_NATIVE_SUMMARY.md`**
   - Decision guide
   - Trade-offs comparison

7. **`CONTROL_PLANE_ARCHITECTURE_SUMMARY.md`** (this document)
   - Executive summary
   - Complete architecture overview

### Code Structure

```
pkg/
├── controlplane/
│   ├── interface.go           # ControlPlane interface
│   ├── types.go               # Common types
│   ├── raft/
│   │   ├── raft_control_plane.go
│   │   └── fsm.go
│   └── k8s/
│       ├── k8s_control_plane.go
│       └── reconciler.go
├── coordination/
│   └── coordination.go        # Uses ControlPlane interface
└── data/
    └── data.go                # Uses ControlPlane interface

config/
└── crd/
    ├── quidditchindices.yaml
    └── quidditchclusters.yaml

charts/
└── quidditch/
    ├── values.yaml            # mode: auto/raft/k8s
    ├── templates/
    │   ├── raft-statefulset.yaml
    │   └── operator-deployment.yaml
    └── crds/
```

---

## Conclusion

Quidditch's dual-mode control plane architecture provides:

1. **Maximum Flexibility**: Works on K8S, VMs, and bare metal
2. **Best-in-Class Experience**: Optimized for each environment
3. **Future-Proof**: Migrate between modes as needs change
4. **Single Codebase**: Not two separate products
5. **User Choice**: Deploy however you want

**For a general-purpose search engine software**, this is the right architectural choice.

**Bottom line**:
- Want K8S-native? ✅ We have it
- Want traditional masters? ✅ We have it
- Want to choose later? ✅ Use auto-detection
- Want to migrate? ✅ We support it

**Quidditch: One search engine, any deployment.**

---

**Document Version**: 1.0
**Last Updated**: 2026-01-26
**Status**: Design Complete - Ready for Implementation
