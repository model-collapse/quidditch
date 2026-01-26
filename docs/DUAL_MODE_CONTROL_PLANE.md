# Dual-Mode Control Plane Architecture

**Version**: 1.0
**Date**: 2026-01-26
**Status**: Design Document

---

## Executive Summary

Quidditch supports **two control plane modes** for maximum deployment flexibility:

1. **Traditional Mode** (Raft-based) - For bare metal, VMs, and K8S
2. **K8S-Native Mode** (Operator-based) - For K8S-only deployments

**Key principle**: Same Quidditch cluster behavior, different control plane implementations.

---

## Architecture Overview

### Pluggable Control Plane Design

```
┌─────────────────────────────────────────────────────────────┐
│              Quidditch Coordination/Data Nodes              │
│                                                             │
│  ┌───────────────────────────────────────────────────┐    │
│  │          Control Plane Interface                   │    │
│  │  - CreateIndex()                                   │    │
│  │  - GetClusterState()                               │    │
│  │  - AllocateShard()                                 │    │
│  │  - RegisterNode()                                  │    │
│  │  - GetShardRouting()                               │    │
│  └─────────────────┬─────────────────────────────────┘    │
│                    │                                        │
│         ┌──────────┴──────────┐                           │
│         ▼                     ▼                           │
│  ┌─────────────┐      ┌──────────────┐                   │
│  │ Raft Mode   │      │ K8S Mode     │                   │
│  │ (etcd-go)   │      │ (Operator)   │                   │
│  └─────────────┘      └──────────────┘                   │
└─────────────────────────────────────────────────────────────┘
         ↓                      ↓
┌─────────────────┐    ┌──────────────────┐
│ Traditional     │    │ K8S API Server   │
│ Master Nodes    │    │ + etcd           │
│ (StatefulSet)   │    │ (Built-in)       │
└─────────────────┘    └──────────────────┘
```

**Key insight**: Coordination and Data nodes use the same interface, regardless of control plane mode.

---

## Control Plane Interface

### Core Interface Definition

```go
// pkg/controlplane/interface.go
package controlplane

import (
    "context"
    "time"
)

// ControlPlane defines the interface for cluster state management
// Implementations: RaftControlPlane, K8SControlPlane
type ControlPlane interface {
    // Index Management
    CreateIndex(ctx context.Context, req *CreateIndexRequest) (*Index, error)
    GetIndex(ctx context.Context, name string) (*Index, error)
    ListIndices(ctx context.Context) ([]*Index, error)
    DeleteIndex(ctx context.Context, name string) error
    UpdateIndexSettings(ctx context.Context, name string, settings *IndexSettings) error

    // Cluster State
    GetClusterState(ctx context.Context) (*ClusterState, error)
    GetClusterHealth(ctx context.Context) (*ClusterHealth, error)

    // Node Management
    RegisterNode(ctx context.Context, node *NodeInfo) error
    UnregisterNode(ctx context.Context, nodeID string) error
    GetNode(ctx context.Context, nodeID string) (*NodeInfo, error)
    ListNodes(ctx context.Context) ([]*NodeInfo, error)
    UpdateNodeStatus(ctx context.Context, nodeID string, status *NodeStatus) error

    // Shard Management
    AllocateShard(ctx context.Context, indexName string, shardID int, nodeID string) error
    DeallocateShard(ctx context.Context, indexName string, shardID int, nodeID string) error
    GetShardRouting(ctx context.Context, indexName string) (*ShardRoutingTable, error)
    UpdateShardState(ctx context.Context, indexName string, shardID int, state ShardState) error

    // Watch for changes (returns channel that emits events)
    WatchClusterState(ctx context.Context) (<-chan *ClusterStateEvent, error)

    // Lifecycle
    Start(ctx context.Context) error
    Stop() error

    // Health check
    IsHealthy() bool
}

// Cluster State Types
type ClusterState struct {
    Nodes   map[string]*NodeInfo
    Indices map[string]*Index
    Version int64  // Monotonically increasing
}

type Index struct {
    Name          string
    Settings      *IndexSettings
    Mappings      *IndexMappings
    ShardRouting  *ShardRoutingTable
    State         IndexState  // CREATING, ACTIVE, DELETING
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

type IndexSettings struct {
    Shards         int
    Replicas       int
    RefreshInterval string
    Codec          string
}

type NodeInfo struct {
    NodeID    string
    Address   string
    GRPCPort  int
    HTTPPort  int
    NodeType  NodeType  // MASTER, COORDINATION, DATA
    Resources *NodeResources
    State     NodeState  // JOINING, ACTIVE, LEAVING, DOWN
    LastSeen  time.Time
}

type NodeResources struct {
    CPUCores      int
    MemoryBytes   int64
    DiskBytes     int64
    DiskAvailable int64
}

type ShardRoutingTable struct {
    IndexName   string
    Allocations map[int]*ShardAllocation  // shardID -> allocation
}

type ShardAllocation struct {
    ShardID     int
    NodeID      string
    State       ShardState  // INITIALIZING, STARTED, RELOCATING, UNASSIGNED
    IsPrimary   bool
}

type ShardState string
const (
    ShardStateInitializing ShardState = "INITIALIZING"
    ShardStateStarted      ShardState = "STARTED"
    ShardStateRelocating   ShardState = "RELOCATING"
    ShardStateUnassigned   ShardState = "UNASSIGNED"
)

type ClusterStateEvent struct {
    Type      EventType  // NODE_JOINED, NODE_LEFT, INDEX_CREATED, SHARD_ALLOCATED
    Timestamp time.Time
    Details   interface{}
}

type CreateIndexRequest struct {
    Name     string
    Settings *IndexSettings
    Mappings *IndexMappings
}
```

---

## Implementation: Raft Mode (Traditional)

### Architecture

```
┌──────────────────────────────────────────────────────┐
│            Master Node (3 replicas)                  │
│                                                      │
│  ┌────────────────────────────────────────────┐    │
│  │        RaftControlPlane                     │    │
│  │  (Implements ControlPlane interface)        │    │
│  └──────────────────┬─────────────────────────┘    │
│                     │                               │
│  ┌──────────────────▼─────────────────────────┐    │
│  │         etcd-go/raft                        │    │
│  │  - Leader election                          │    │
│  │  - Log replication                          │    │
│  │  - State machine                            │    │
│  └──────────────────┬─────────────────────────┘    │
│                     │                               │
│  ┌──────────────────▼─────────────────────────┐    │
│  │    ClusterStateFSM                          │    │
│  │  (Finite State Machine)                     │    │
│  │  - Nodes map                                │    │
│  │  - Indices map                              │    │
│  │  - Routing table                            │    │
│  └──────────────────┬─────────────────────────┘    │
│                     │                               │
│  ┌──────────────────▼─────────────────────────┐    │
│  │    Persistent Storage                       │    │
│  │  - Raft logs (append-only)                  │    │
│  │  - Snapshots (periodic)                     │    │
│  └─────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────┘
```

### Implementation

```go
// pkg/controlplane/raft/raft_control_plane.go
package raft

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "time"

    "github.com/hashicorp/raft"
    raftboltdb "github.com/hashicorp/raft-boltdb"
    "github.com/yourorg/quidditch/pkg/controlplane"
)

type RaftControlPlane struct {
    nodeID    string
    raftNode  *raft.Raft
    fsm       *ClusterStateFSM
    transport *raft.NetworkTransport
    config    *RaftConfig

    // For watch functionality
    watchers     map[string]chan *controlplane.ClusterStateEvent
    watcherMutex sync.RWMutex

    logger *zap.Logger
}

type RaftConfig struct {
    NodeID           string
    BindAddr         string
    RaftPort         int
    DataDir          string
    BootstrapCluster bool
    Peers            []string
}

func NewRaftControlPlane(cfg *RaftConfig, logger *zap.Logger) (*RaftControlPlane, error) {
    fsm := NewClusterStateFSM()

    // Create Raft configuration
    raftConfig := raft.DefaultConfig()
    raftConfig.LocalID = raft.ServerID(cfg.NodeID)
    raftConfig.HeartbeatTimeout = 1000 * time.Millisecond
    raftConfig.ElectionTimeout = 1000 * time.Millisecond
    raftConfig.CommitTimeout = 50 * time.Millisecond
    raftConfig.SnapshotInterval = 120 * time.Second
    raftConfig.SnapshotThreshold = 8192

    // Create transport
    addr := fmt.Sprintf("%s:%d", cfg.BindAddr, cfg.RaftPort)
    transport, err := raft.NewTCPTransport(addr, nil, 3, 10*time.Second, os.Stderr)
    if err != nil {
        return nil, fmt.Errorf("failed to create transport: %w", err)
    }

    // Create log store
    logStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-log.db"))
    if err != nil {
        return nil, fmt.Errorf("failed to create log store: %w", err)
    }

    // Create stable store (for Raft metadata)
    stableStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-stable.db"))
    if err != nil {
        return nil, fmt.Errorf("failed to create stable store: %w", err)
    }

    // Create snapshot store
    snapshotStore, err := raft.NewFileSnapshotStore(cfg.DataDir, 3, os.Stderr)
    if err != nil {
        return nil, fmt.Errorf("failed to create snapshot store: %w", err)
    }

    // Create Raft node
    raftNode, err := raft.NewRaft(raftConfig, fsm, logStore, stableStore, snapshotStore, transport)
    if err != nil {
        return nil, fmt.Errorf("failed to create raft node: %w", err)
    }

    cp := &RaftControlPlane{
        nodeID:    cfg.NodeID,
        raftNode:  raftNode,
        fsm:       fsm,
        transport: transport,
        config:    cfg,
        watchers:  make(map[string]chan *controlplane.ClusterStateEvent),
        logger:    logger,
    }

    // Bootstrap cluster if needed
    if cfg.BootstrapCluster {
        configuration := raft.Configuration{
            Servers: []raft.Server{
                {
                    ID:      raft.ServerID(cfg.NodeID),
                    Address: transport.LocalAddr(),
                },
            },
        }
        raftNode.BootstrapCluster(configuration)
    }

    // Start event dispatcher
    go cp.dispatchEvents()

    return cp, nil
}

func (cp *RaftControlPlane) CreateIndex(ctx context.Context, req *controlplane.CreateIndexRequest) (*controlplane.Index, error) {
    // Serialize command
    cmd := &Command{
        Type: CommandTypeCreateIndex,
        Data: req,
    }

    data, err := json.Marshal(cmd)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal command: %w", err)
    }

    // Submit to Raft (blocks until committed)
    future := cp.raftNode.Apply(data, 10*time.Second)
    if err := future.Error(); err != nil {
        return nil, fmt.Errorf("raft apply failed: %w", err)
    }

    // Get result from state machine
    result := future.Response().(*ApplyResult)
    if result.Error != nil {
        return nil, result.Error
    }

    return result.Index, nil
}

func (cp *RaftControlPlane) GetClusterState(ctx context.Context) (*controlplane.ClusterState, error) {
    // Read from FSM (no Raft needed for reads)
    return cp.fsm.GetClusterState(), nil
}

func (cp *RaftControlPlane) RegisterNode(ctx context.Context, node *controlplane.NodeInfo) error {
    cmd := &Command{
        Type: CommandTypeRegisterNode,
        Data: node,
    }

    data, err := json.Marshal(cmd)
    if err != nil {
        return fmt.Errorf("failed to marshal command: %w", err)
    }

    future := cp.raftNode.Apply(data, 10*time.Second)
    if err := future.Error(); err != nil {
        return fmt.Errorf("raft apply failed: %w", err)
    }

    result := future.Response().(*ApplyResult)
    return result.Error
}

func (cp *RaftControlPlane) GetShardRouting(ctx context.Context, indexName string) (*controlplane.ShardRoutingTable, error) {
    return cp.fsm.GetShardRouting(indexName), nil
}

func (cp *RaftControlPlane) WatchClusterState(ctx context.Context) (<-chan *controlplane.ClusterStateEvent, error) {
    ch := make(chan *controlplane.ClusterStateEvent, 100)

    watcherID := fmt.Sprintf("watcher-%d", time.Now().UnixNano())

    cp.watcherMutex.Lock()
    cp.watchers[watcherID] = ch
    cp.watcherMutex.Unlock()

    // Cleanup on context cancellation
    go func() {
        <-ctx.Done()
        cp.watcherMutex.Lock()
        delete(cp.watchers, watcherID)
        close(ch)
        cp.watcherMutex.Unlock()
    }()

    return ch, nil
}

func (cp *RaftControlPlane) dispatchEvents() {
    // Listen to FSM events and dispatch to watchers
    for event := range cp.fsm.EventChannel() {
        cp.watcherMutex.RLock()
        for _, watcher := range cp.watchers {
            select {
            case watcher <- event:
            default:
                // Skip if watcher is slow
            }
        }
        cp.watcherMutex.RUnlock()
    }
}

func (cp *RaftControlPlane) IsHealthy() bool {
    return cp.raftNode.State() == raft.Leader || cp.raftNode.State() == raft.Follower
}

func (cp *RaftControlPlane) Start(ctx context.Context) error {
    cp.logger.Info("Starting Raft control plane", zap.String("node_id", cp.nodeID))
    return nil
}

func (cp *RaftControlPlane) Stop() error {
    cp.logger.Info("Stopping Raft control plane")
    return cp.raftNode.Shutdown().Error()
}

// ClusterStateFSM implements raft.FSM
type ClusterStateFSM struct {
    mu       sync.RWMutex
    nodes    map[string]*controlplane.NodeInfo
    indices  map[string]*controlplane.Index
    version  int64
    events   chan *controlplane.ClusterStateEvent
}

func NewClusterStateFSM() *ClusterStateFSM {
    return &ClusterStateFSM{
        nodes:   make(map[string]*controlplane.NodeInfo),
        indices: make(map[string]*controlplane.Index),
        version: 0,
        events:  make(chan *controlplane.ClusterStateEvent, 1000),
    }
}

func (fsm *ClusterStateFSM) Apply(log *raft.Log) interface{} {
    fsm.mu.Lock()
    defer fsm.mu.Unlock()

    var cmd Command
    if err := json.Unmarshal(log.Data, &cmd); err != nil {
        return &ApplyResult{Error: err}
    }

    fsm.version++

    switch cmd.Type {
    case CommandTypeCreateIndex:
        req := cmd.Data.(*controlplane.CreateIndexRequest)
        index := &controlplane.Index{
            Name:      req.Name,
            Settings:  req.Settings,
            Mappings:  req.Mappings,
            State:     controlplane.IndexStateActive,
            CreatedAt: time.Now(),
            UpdatedAt: time.Now(),
            ShardRouting: &controlplane.ShardRoutingTable{
                IndexName:   req.Name,
                Allocations: make(map[int]*controlplane.ShardAllocation),
            },
        }

        fsm.indices[req.Name] = index

        // Emit event
        fsm.events <- &controlplane.ClusterStateEvent{
            Type:      controlplane.EventTypeIndexCreated,
            Timestamp: time.Now(),
            Details:   index,
        }

        return &ApplyResult{Index: index}

    case CommandTypeRegisterNode:
        node := cmd.Data.(*controlplane.NodeInfo)
        node.State = controlplane.NodeStateActive
        node.LastSeen = time.Now()

        fsm.nodes[node.NodeID] = node

        fsm.events <- &controlplane.ClusterStateEvent{
            Type:      controlplane.EventTypeNodeJoined,
            Timestamp: time.Now(),
            Details:   node,
        }

        return &ApplyResult{}

    // ... other command types
    }

    return &ApplyResult{Error: fmt.Errorf("unknown command type: %s", cmd.Type)}
}

func (fsm *ClusterStateFSM) Snapshot() (raft.FSMSnapshot, error) {
    fsm.mu.RLock()
    defer fsm.mu.RUnlock()

    // Deep copy state for snapshot
    snapshot := &ClusterStateSnapshot{
        Nodes:   fsm.nodes,
        Indices: fsm.indices,
        Version: fsm.version,
    }

    return snapshot, nil
}

func (fsm *ClusterStateFSM) Restore(snapshot io.ReadCloser) error {
    fsm.mu.Lock()
    defer fsm.mu.Unlock()

    var snap ClusterStateSnapshot
    if err := json.NewDecoder(snapshot).Decode(&snap); err != nil {
        return err
    }

    fsm.nodes = snap.Nodes
    fsm.indices = snap.Indices
    fsm.version = snap.Version

    return nil
}

func (fsm *ClusterStateFSM) GetClusterState() *controlplane.ClusterState {
    fsm.mu.RLock()
    defer fsm.mu.RUnlock()

    return &controlplane.ClusterState{
        Nodes:   fsm.nodes,
        Indices: fsm.indices,
        Version: fsm.version,
    }
}

func (fsm *ClusterStateFSM) EventChannel() <-chan *controlplane.ClusterStateEvent {
    return fsm.events
}

type Command struct {
    Type CommandType
    Data interface{}
}

type CommandType string

const (
    CommandTypeCreateIndex    CommandType = "CreateIndex"
    CommandTypeDeleteIndex    CommandType = "DeleteIndex"
    CommandTypeRegisterNode   CommandType = "RegisterNode"
    CommandTypeUnregisterNode CommandType = "UnregisterNode"
    CommandTypeAllocateShard  CommandType = "AllocateShard"
)

type ApplyResult struct {
    Index *controlplane.Index
    Node  *controlplane.NodeInfo
    Error error
}
```

### Kubernetes Deployment (Raft Mode)

```yaml
# deployments/kubernetes/raft-mode/master-statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: quidditch-master
  namespace: quidditch
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
        image: quidditch/master:1.0.0
        env:
        - name: CONTROL_PLANE_MODE
          value: "raft"
        - name: NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: RAFT_PORT
          value: "9300"
        - name: BOOTSTRAP_CLUSTER
          value: "false"  # Only first node bootstraps
        - name: RAFT_PEERS
          value: "quidditch-master-0.quidditch-master:9300,quidditch-master-1.quidditch-master:9300,quidditch-master-2.quidditch-master:9300"
        ports:
        - name: raft
          containerPort: 9300
        - name: grpc
          containerPort: 9301
        volumeMounts:
        - name: data
          mountPath: /var/lib/quidditch
        resources:
          requests:
            memory: "4Gi"
            cpu: "2"
          limits:
            memory: "8Gi"
            cpu: "4"
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: gp3
      resources:
        requests:
          storage: 50Gi
---
apiVersion: v1
kind: Service
metadata:
  name: quidditch-master
  namespace: quidditch
spec:
  clusterIP: None  # Headless service
  selector:
    app: quidditch-master
  ports:
  - name: raft
    port: 9300
  - name: grpc
    port: 9301
```

---

## Implementation: K8S-Native Mode

### Architecture

```
┌──────────────────────────────────────────────────────┐
│         Quidditch Operator (3 replicas)              │
│                                                      │
│  ┌────────────────────────────────────────────┐    │
│  │        K8SControlPlane                      │    │
│  │  (Implements ControlPlane interface)        │    │
│  └──────────────────┬─────────────────────────┘    │
│                     │                               │
│  ┌──────────────────▼─────────────────────────┐    │
│  │      Kubernetes Client-Go                   │    │
│  │  - CRD client (QuidditchIndex, etc.)        │    │
│  │  - Watch API                                │    │
│  │  - Informers & Listers                      │    │
│  └──────────────────┬─────────────────────────┘    │
│                     │                               │
│  ┌──────────────────▼─────────────────────────┐    │
│  │      Controller Reconciliation              │    │
│  │  - Index reconciler                         │    │
│  │  - Shard allocator                          │    │
│  │  - Node discovery                           │    │
│  └─────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────┘
                      ↓
┌──────────────────────────────────────────────────────┐
│         Kubernetes API Server + etcd                 │
│                                                      │
│  Custom Resources (stored in etcd):                 │
│  - QuidditchIndex                                   │
│  - QuidditchCluster                                 │
│  - ShardAllocation                                  │
└──────────────────────────────────────────────────────┘
```

### Custom Resource Definitions

```yaml
# config/crd/quidditch.io_quidditchindices.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: quidditchindices.quidditch.io
spec:
  group: quidditch.io
  names:
    kind: QuidditchIndex
    listKind: QuidditchIndexList
    plural: quidditchindices
    singular: quidditchindex
    shortNames:
    - qidx
  scope: Namespaced
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
            required:
            - shards
            properties:
              shards:
                type: integer
                minimum: 1
                maximum: 10000
              replicas:
                type: integer
                minimum: 0
                maximum: 10
                default: 1
              refreshInterval:
                type: string
                default: "1s"
              codec:
                type: string
                enum: ["default", "best_compression"]
                default: "default"
              mappings:
                type: object
                x-kubernetes-preserve-unknown-fields: true
          status:
            type: object
            properties:
              state:
                type: string
                enum: ["CREATING", "ACTIVE", "DELETING", "ERROR"]
              message:
                type: string
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
                      enum: ["INITIALIZING", "STARTED", "RELOCATING", "UNASSIGNED"]
                    isPrimary:
                      type: boolean
              createdAt:
                type: string
                format: date-time
              updatedAt:
                type: string
                format: date-time
    subresources:
      status: {}
    additionalPrinterColumns:
    - name: State
      type: string
      jsonPath: .status.state
    - name: Shards
      type: integer
      jsonPath: .spec.shards
    - name: Replicas
      type: integer
      jsonPath: .spec.replicas
    - name: Age
      type: date
      jsonPath: .metadata.creationTimestamp
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: quidditchclusters.quidditch.io
spec:
  group: quidditch.io
  names:
    kind: QuidditchCluster
    plural: quidditchclusters
    singular: quidditchcluster
    shortNames:
    - qcluster
  scope: Namespaced
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
              controlPlane:
                type: object
                properties:
                  mode:
                    type: string
                    enum: ["raft", "k8s"]
                    default: "k8s"
              coordination:
                type: object
                properties:
                  replicas:
                    type: integer
                    minimum: 1
                    default: 3
              data:
                type: object
                properties:
                  replicas:
                    type: integer
                    minimum: 1
          status:
            type: object
            properties:
              state:
                type: string
              nodes:
                type: array
                items:
                  type: object
                  properties:
                    nodeId:
                      type: string
                    nodeType:
                      type: string
                    state:
                      type: string
    subresources:
      status: {}
```

### Implementation

```go
// pkg/controlplane/k8s/k8s_control_plane.go
package k8s

import (
    "context"
    "fmt"
    "time"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/rest"
    "k8s.io/client-go/tools/cache"
    "k8s.io/client-go/util/workqueue"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"

    quidditchv1 "github.com/yourorg/quidditch/api/v1"
    "github.com/yourorg/quidditch/pkg/controlplane"
)

type K8SControlPlane struct {
    client    client.Client
    scheme    *runtime.Scheme
    namespace string

    // Cache for fast reads
    indexCache    map[string]*controlplane.Index
    nodeCache     map[string]*controlplane.NodeInfo
    cacheMutex    sync.RWMutex

    // For watch functionality
    watchers     map[string]chan *controlplane.ClusterStateEvent
    watcherMutex sync.RWMutex

    // Informers for watching K8S resources
    indexInformer cache.SharedIndexInformer
    podInformer   cache.SharedIndexInformer

    logger *zap.Logger
}

type K8SConfig struct {
    Namespace     string
    LeaderElect   bool
    LeaseDuration time.Duration
}

func NewK8SControlPlane(cfg *K8SConfig, logger *zap.Logger) (*K8SControlPlane, error) {
    // Get K8S config
    config, err := rest.InClusterConfig()
    if err != nil {
        return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
    }

    // Create controller-runtime client
    scheme := runtime.NewScheme()
    quidditchv1.AddToScheme(scheme)
    corev1.AddToScheme(scheme)

    k8sClient, err := client.New(config, client.Options{Scheme: scheme})
    if err != nil {
        return nil, fmt.Errorf("failed to create K8S client: %w", err)
    }

    cp := &K8SControlPlane{
        client:     k8sClient,
        scheme:     scheme,
        namespace:  cfg.Namespace,
        indexCache: make(map[string]*controlplane.Index),
        nodeCache:  make(map[string]*controlplane.NodeInfo),
        watchers:   make(map[string]chan *controlplane.ClusterStateEvent),
        logger:     logger,
    }

    return cp, nil
}

func (cp *K8SControlPlane) CreateIndex(ctx context.Context, req *controlplane.CreateIndexRequest) (*controlplane.Index, error) {
    // Create QuidditchIndex CRD
    qIndex := &quidditchv1.QuidditchIndex{
        ObjectMeta: metav1.ObjectMeta{
            Name:      req.Name,
            Namespace: cp.namespace,
        },
        Spec: quidditchv1.IndexSpec{
            Shards:          req.Settings.Shards,
            Replicas:        req.Settings.Replicas,
            RefreshInterval: req.Settings.RefreshInterval,
            Codec:           req.Settings.Codec,
            Mappings:        convertMappings(req.Mappings),
        },
    }

    if err := cp.client.Create(ctx, qIndex); err != nil {
        return nil, fmt.Errorf("failed to create index CRD: %w", err)
    }

    // Convert to internal format
    index := cp.qIndexToInternal(qIndex)

    // Update cache
    cp.cacheMutex.Lock()
    cp.indexCache[req.Name] = index
    cp.cacheMutex.Unlock()

    return index, nil
}

func (cp *K8SControlPlane) GetIndex(ctx context.Context, name string) (*controlplane.Index, error) {
    // Try cache first
    cp.cacheMutex.RLock()
    if index, ok := cp.indexCache[name]; ok {
        cp.cacheMutex.RUnlock()
        return index, nil
    }
    cp.cacheMutex.RUnlock()

    // Fetch from K8S API
    qIndex := &quidditchv1.QuidditchIndex{}
    key := client.ObjectKey{Namespace: cp.namespace, Name: name}
    if err := cp.client.Get(ctx, key, qIndex); err != nil {
        return nil, fmt.Errorf("failed to get index: %w", err)
    }

    index := cp.qIndexToInternal(qIndex)

    // Update cache
    cp.cacheMutex.Lock()
    cp.indexCache[name] = index
    cp.cacheMutex.Unlock()

    return index, nil
}

func (cp *K8SControlPlane) GetClusterState(ctx context.Context) (*controlplane.ClusterState, error) {
    cp.cacheMutex.RLock()
    defer cp.cacheMutex.RUnlock()

    state := &controlplane.ClusterState{
        Nodes:   make(map[string]*controlplane.NodeInfo),
        Indices: make(map[string]*controlplane.Index),
        Version: time.Now().Unix(),
    }

    // Copy from cache
    for k, v := range cp.nodeCache {
        state.Nodes[k] = v
    }
    for k, v := range cp.indexCache {
        state.Indices[k] = v
    }

    return state, nil
}

func (cp *K8SControlPlane) RegisterNode(ctx context.Context, node *controlplane.NodeInfo) error {
    // In K8S mode, nodes are discovered via Pod informers
    // This method just updates cache
    cp.cacheMutex.Lock()
    cp.nodeCache[node.NodeID] = node
    cp.cacheMutex.Unlock()

    // Emit event
    cp.emitEvent(&controlplane.ClusterStateEvent{
        Type:      controlplane.EventTypeNodeJoined,
        Timestamp: time.Now(),
        Details:   node,
    })

    return nil
}

func (cp *K8SControlPlane) GetShardRouting(ctx context.Context, indexName string) (*controlplane.ShardRoutingTable, error) {
    index, err := cp.GetIndex(ctx, indexName)
    if err != nil {
        return nil, err
    }

    return index.ShardRouting, nil
}

func (cp *K8SControlPlane) WatchClusterState(ctx context.Context) (<-chan *controlplane.ClusterStateEvent, error) {
    ch := make(chan *controlplane.ClusterStateEvent, 100)

    watcherID := fmt.Sprintf("watcher-%d", time.Now().UnixNano())

    cp.watcherMutex.Lock()
    cp.watchers[watcherID] = ch
    cp.watcherMutex.Unlock()

    // Cleanup on context cancellation
    go func() {
        <-ctx.Done()
        cp.watcherMutex.Lock()
        delete(cp.watchers, watcherID)
        close(ch)
        cp.watcherMutex.Unlock()
    }()

    return ch, nil
}

func (cp *K8SControlPlane) Start(ctx context.Context) error {
    cp.logger.Info("Starting K8S control plane")

    // Start informers for watching resources
    go cp.watchIndices(ctx)
    go cp.watchPods(ctx)

    return nil
}

func (cp *K8SControlPlane) watchIndices(ctx context.Context) {
    // Watch QuidditchIndex resources
    indexList := &quidditchv1.QuidditchIndexList{}

    watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
        return cp.client.Watch(ctx, indexList)
    }

    watcher, err := watchFunc(metav1.ListOptions{})
    if err != nil {
        cp.logger.Error("Failed to watch indices", zap.Error(err))
        return
    }

    for event := range watcher.ResultChan() {
        qIndex := event.Object.(*quidditchv1.QuidditchIndex)
        index := cp.qIndexToInternal(qIndex)

        cp.cacheMutex.Lock()
        cp.indexCache[index.Name] = index
        cp.cacheMutex.Unlock()

        var eventType controlplane.EventType
        switch event.Type {
        case watch.Added:
            eventType = controlplane.EventTypeIndexCreated
        case watch.Modified:
            eventType = controlplane.EventTypeIndexModified
        case watch.Deleted:
            eventType = controlplane.EventTypeIndexDeleted
            delete(cp.indexCache, index.Name)
        }

        cp.emitEvent(&controlplane.ClusterStateEvent{
            Type:      eventType,
            Timestamp: time.Now(),
            Details:   index,
        })
    }
}

func (cp *K8SControlPlane) watchPods(ctx context.Context) {
    // Watch data node pods for discovery
    podList := &corev1.PodList{}
    listOpts := &client.ListOptions{
        Namespace:     cp.namespace,
        LabelSelector: labels.SelectorFromSet(labels.Set{"app": "quidditch-data"}),
    }

    watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
        return cp.client.Watch(ctx, podList, listOpts)
    }

    watcher, err := watchFunc(metav1.ListOptions{})
    if err != nil {
        cp.logger.Error("Failed to watch pods", zap.Error(err))
        return
    }

    for event := range watcher.ResultChan() {
        pod := event.Object.(*corev1.Pod)

        if pod.Status.Phase != corev1.PodRunning {
            continue
        }

        node := &controlplane.NodeInfo{
            NodeID:   pod.Name,
            Address:  pod.Status.PodIP,
            NodeType: controlplane.NodeTypeData,
            State:    controlplane.NodeStateActive,
            LastSeen: time.Now(),
        }

        cp.cacheMutex.Lock()
        cp.nodeCache[node.NodeID] = node
        cp.cacheMutex.Unlock()

        var eventType controlplane.EventType
        switch event.Type {
        case watch.Added:
            eventType = controlplane.EventTypeNodeJoined
        case watch.Deleted:
            eventType = controlplane.EventTypeNodeLeft
            delete(cp.nodeCache, node.NodeID)
        }

        cp.emitEvent(&controlplane.ClusterStateEvent{
            Type:      eventType,
            Timestamp: time.Now(),
            Details:   node,
        })
    }
}

func (cp *K8SControlPlane) emitEvent(event *controlplane.ClusterStateEvent) {
    cp.watcherMutex.RLock()
    defer cp.watcherMutex.RUnlock()

    for _, watcher := range cp.watchers {
        select {
        case watcher <- event:
        default:
            // Skip if watcher is slow
        }
    }
}

func (cp *K8SControlPlane) qIndexToInternal(qIndex *quidditchv1.QuidditchIndex) *controlplane.Index {
    index := &controlplane.Index{
        Name: qIndex.Name,
        Settings: &controlplane.IndexSettings{
            Shards:          qIndex.Spec.Shards,
            Replicas:        qIndex.Spec.Replicas,
            RefreshInterval: qIndex.Spec.RefreshInterval,
            Codec:           qIndex.Spec.Codec,
        },
        State:     convertIndexState(qIndex.Status.State),
        CreatedAt: qIndex.CreationTimestamp.Time,
        UpdatedAt: time.Now(),
        ShardRouting: &controlplane.ShardRoutingTable{
            IndexName:   qIndex.Name,
            Allocations: make(map[int]*controlplane.ShardAllocation),
        },
    }

    // Convert shard allocations
    for _, alloc := range qIndex.Status.ShardAllocations {
        index.ShardRouting.Allocations[alloc.ShardID] = &controlplane.ShardAllocation{
            ShardID:   alloc.ShardID,
            NodeID:    alloc.NodeID,
            State:     convertShardState(alloc.State),
            IsPrimary: alloc.IsPrimary,
        }
    }

    return index
}

func (cp *K8SControlPlane) IsHealthy() bool {
    // In K8S mode, health is determined by K8S API reachability
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    // Try to list a resource
    indexList := &quidditchv1.QuidditchIndexList{}
    err := cp.client.List(ctx, indexList, &client.ListOptions{Namespace: cp.namespace})
    return err == nil
}

func (cp *K8SControlPlane) Stop() error {
    cp.logger.Info("Stopping K8S control plane")
    return nil
}
```

### Kubernetes Deployment (K8S-Native Mode)

```yaml
# deployments/kubernetes/k8s-native/operator-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: quidditch-operator
  namespace: quidditch-system
spec:
  replicas: 3  # HA with leader election
  selector:
    matchLabels:
      app: quidditch-operator
  template:
    metadata:
      labels:
        app: quidditch-operator
    spec:
      serviceAccountName: quidditch-operator
      containers:
      - name: operator
        image: quidditch/operator:1.0.0
        env:
        - name: CONTROL_PLANE_MODE
          value: "k8s"
        - name: NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: LEADER_ELECT
          value: "true"
        ports:
        - name: metrics
          containerPort: 8080
        - name: health
          containerPort: 8081
        resources:
          requests:
            memory: "1Gi"
            cpu: "1"
          limits:
            memory: "2Gi"
            cpu: "2"
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: quidditch-operator
  namespace: quidditch-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: quidditch-operator
rules:
- apiGroups: ["quidditch.io"]
  resources: ["quidditchindices", "quidditchclusters"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["quidditch.io"]
  resources: ["quidditchindices/status", "quidditchclusters/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["pods", "services", "endpoints"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "create", "update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: quidditch-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: quidditch-operator
subjects:
- kind: ServiceAccount
  name: quidditch-operator
  namespace: quidditch-system
```

---

## Coordination/Data Node Integration

### Using Control Plane Interface

Both Coordination and Data nodes use the same `ControlPlane` interface regardless of mode:

```go
// pkg/coordination/coordination.go
package coordination

import (
    "context"

    "github.com/yourorg/quidditch/pkg/controlplane"
    "github.com/yourorg/quidditch/pkg/controlplane/raft"
    "github.com/yourorg/quidditch/pkg/controlplane/k8s"
)

type Coordination struct {
    controlPlane controlplane.ControlPlane
    executor     *executor.QueryExecutor
    config       *config.Config
    logger       *zap.Logger
}

func NewCoordination(cfg *config.Config, logger *zap.Logger) (*Coordination, error) {
    // Create control plane based on configuration
    var cp controlplane.ControlPlane
    var err error

    switch cfg.ControlPlane.Mode {
    case "raft":
        logger.Info("Using Raft control plane")
        cp, err = raft.NewRaftControlPlane(cfg.ControlPlane.Raft, logger)

    case "k8s":
        logger.Info("Using K8S control plane")
        cp, err = k8s.NewK8SControlPlane(cfg.ControlPlane.K8S, logger)

    case "auto":
        // Auto-detect
        if k8s.IsRunningInKubernetes() {
            logger.Info("Auto-detected K8S environment, using K8S control plane")
            cp, err = k8s.NewK8SControlPlane(cfg.ControlPlane.K8S, logger)
        } else {
            logger.Info("Auto-detected non-K8S environment, using Raft control plane")
            cp, err = raft.NewRaftControlPlane(cfg.ControlPlane.Raft, logger)
        }

    default:
        return nil, fmt.Errorf("unknown control plane mode: %s", cfg.ControlPlane.Mode)
    }

    if err != nil {
        return nil, fmt.Errorf("failed to create control plane: %w", err)
    }

    coord := &Coordination{
        controlPlane: cp,
        config:       cfg,
        logger:       logger,
    }

    return coord, nil
}

func (c *Coordination) Start(ctx context.Context) error {
    // Start control plane
    if err := c.controlPlane.Start(ctx); err != nil {
        return fmt.Errorf("failed to start control plane: %w", err)
    }

    // Start query executor with auto-discovery
    c.executor = executor.NewQueryExecutor(c.controlPlane, c.logger)
    go c.executor.Start(ctx)

    // Start auto-discovery of data nodes
    go c.discoverDataNodes(ctx)

    return nil
}

func (c *Coordination) discoverDataNodes(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // Get cluster state from control plane (works with both modes)
            state, err := c.controlPlane.GetClusterState(ctx)
            if err != nil {
                c.logger.Error("Failed to get cluster state", zap.Error(err))
                continue
            }

            // Register data nodes with executor
            for _, node := range state.Nodes {
                if node.NodeType == controlplane.NodeTypeData {
                    if !c.executor.HasDataNodeClient(node.NodeID) {
                        c.logger.Info("Discovered new data node", zap.String("node_id", node.NodeID))
                        client := NewDataNodeClient(node, c.logger)
                        c.executor.RegisterDataNode(client)
                    }
                }
            }
        }
    }
}

// CreateIndex - Same code regardless of control plane mode
func (c *Coordination) CreateIndex(ctx context.Context, req *CreateIndexRequest) (*CreateIndexResponse, error) {
    // Call control plane (same interface for both modes)
    index, err := c.controlPlane.CreateIndex(ctx, &controlplane.CreateIndexRequest{
        Name:     req.Name,
        Settings: req.Settings,
        Mappings: req.Mappings,
    })

    if err != nil {
        return nil, err
    }

    return &CreateIndexResponse{
        Index: index.Name,
        Acknowledged: true,
    }, nil
}

// Search - Same code regardless of control plane mode
func (c *Coordination) Search(ctx context.Context, indexName string, query []byte) (*SearchResponse, error) {
    // Get shard routing from control plane (same interface for both modes)
    routing, err := c.controlPlane.GetShardRouting(ctx, indexName)
    if err != nil {
        return nil, err
    }

    // Execute distributed search (same for both modes)
    return c.executor.ExecuteSearch(ctx, indexName, query, routing)
}
```

---

## Configuration

### Unified Configuration Format

```yaml
# config/quidditch.yaml
cluster:
  name: "quidditch-prod"

# Control plane configuration
controlPlane:
  # Mode: "raft", "k8s", or "auto"
  # auto = detect environment and choose automatically
  mode: auto

  # Raft mode configuration (used if mode=raft or auto+non-K8S)
  raft:
    nodeId: "${NODE_ID}"
    bindAddr: "0.0.0.0"
    raftPort: 9300
    dataDir: "/var/lib/quidditch/raft"
    bootstrapCluster: false
    peers:
      - "master-0.master.svc.cluster.local:9300"
      - "master-1.master.svc.cluster.local:9300"
      - "master-2.master.svc.cluster.local:9300"
    # Raft tuning
    heartbeatTimeout: 1000ms
    electionTimeout: 3000ms
    commitTimeout: 50ms
    snapshotInterval: 120s
    snapshotThreshold: 8192

  # K8S mode configuration (used if mode=k8s or auto+K8S)
  k8s:
    namespace: "quidditch"
    leaderElect: true
    leaseDuration: 15s

# Node configuration
node:
  nodeId: "${NODE_ID}"
  nodeType: "coordination"  # coordination, data, master
  bindAddr: "0.0.0.0"
  grpcPort: 9302
  httpPort: 9200

# Data node specific
data:
  dataDir: "/var/lib/quidditch/data"
  diagonConfig:
    maxMemory: "16GB"
    cacheSize: "8GB"
```

### Helm Chart (Supports Both Modes)

```yaml
# charts/quidditch/values.yaml
controlPlane:
  # Mode: raft, k8s, auto
  mode: auto

  # Raft masters (only deployed if mode=raft)
  raft:
    enabled: false  # Auto-enabled if mode=raft
    replicas: 3
    resources:
      requests:
        memory: 4Gi
        cpu: 2
      limits:
        memory: 8Gi
        cpu: 4
    storage:
      class: gp3
      size: 50Gi
    config:
      heartbeatTimeout: 1000ms
      electionTimeout: 3000ms

  # K8S operator (only deployed if mode=k8s)
  k8s:
    enabled: false  # Auto-enabled if mode=k8s
    replicas: 3
    resources:
      requests:
        memory: 1Gi
        cpu: 1
      limits:
        memory: 2Gi
        cpu: 2

coordination:
  replicas: 3
  resources:
    requests:
      memory: 4Gi
      cpu: 2
  autoscaling:
    enabled: true
    minReplicas: 3
    maxReplicas: 20

data:
  replicas: 5
  resources:
    requests:
      memory: 16Gi
      cpu: 8
  storage:
    class: gp3
    size: 500Gi
```

### Installation Examples

**Raft Mode (Traditional):**
```bash
helm install quidditch ./charts/quidditch \
  --set controlPlane.mode=raft \
  --set controlPlane.raft.replicas=3
```

**K8S-Native Mode:**
```bash
helm install quidditch ./charts/quidditch \
  --set controlPlane.mode=k8s \
  --set controlPlane.k8s.replicas=3
```

**Auto-detect Mode:**
```bash
# Automatically uses K8S mode when deployed to K8S
helm install quidditch ./charts/quidditch \
  --set controlPlane.mode=auto
```

---

## Migration Between Modes

### Raft → K8S Migration

```bash
# Step 1: Export cluster state from Raft
kubectl exec -it quidditch-master-0 -- \
  /usr/local/bin/quidditch-admin export-state \
  --output /tmp/cluster-state.json

kubectl cp quidditch-master-0:/tmp/cluster-state.json ./cluster-state.json

# Step 2: Deploy K8S-native operator
helm upgrade quidditch ./charts/quidditch \
  --set controlPlane.mode=k8s \
  --reuse-values

# Step 3: Import state to K8S CRDs
kubectl apply -f - <<EOF
apiVersion: quidditch.io/v1
kind: QuidditchCluster
metadata:
  name: quidditch-prod
spec:
  importState: |
    $(cat cluster-state.json)
EOF

# Step 4: Wait for operator to reconcile
kubectl wait --for=condition=Ready quidditchcluster/quidditch-prod

# Step 5: Switch coordination nodes to K8S mode
kubectl set env deployment/quidditch-coordination CONTROL_PLANE_MODE=k8s

# Step 6: Remove Raft masters
kubectl delete statefulset quidditch-master
kubectl delete pvc -l app=quidditch-master
```

### K8S → Raft Migration

```bash
# Step 1: Export CRDs
kubectl get quidditchindices -o json > indices.json

# Step 2: Deploy Raft masters
helm upgrade quidditch ./charts/quidditch \
  --set controlPlane.mode=raft \
  --reuse-values

# Step 3: Wait for Raft cluster to be ready
kubectl wait --for=condition=Ready pod -l app=quidditch-master

# Step 4: Import state
kubectl exec -it quidditch-master-0 -- \
  /usr/local/bin/quidditch-admin import-state \
  --input /tmp/indices.json

# Step 5: Switch coordination nodes to Raft mode
kubectl set env deployment/quidditch-coordination CONTROL_PLANE_MODE=raft

# Step 6: Remove K8S operator
kubectl delete deployment quidditch-operator -n quidditch-system
```

---

## Testing Both Modes

### Test Framework

```go
// pkg/controlplane/test/suite.go
package test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/yourorg/quidditch/pkg/controlplane"
)

// ControlPlaneTestSuite runs the same tests against both implementations
type ControlPlaneTestSuite struct {
    cp controlplane.ControlPlane
}

func (suite *ControlPlaneTestSuite) TestCreateIndex(t *testing.T) {
    ctx := context.Background()

    req := &controlplane.CreateIndexRequest{
        Name: "test-index",
        Settings: &controlplane.IndexSettings{
            Shards:   5,
            Replicas: 1,
        },
    }

    index, err := suite.cp.CreateIndex(ctx, req)
    assert.NoError(t, err)
    assert.Equal(t, "test-index", index.Name)
    assert.Equal(t, 5, index.Settings.Shards)
}

func (suite *ControlPlaneTestSuite) TestGetClusterState(t *testing.T) {
    ctx := context.Background()

    state, err := suite.cp.GetClusterState(ctx)
    assert.NoError(t, err)
    assert.NotNil(t, state)
    assert.NotNil(t, state.Nodes)
    assert.NotNil(t, state.Indices)
}

func (suite *ControlPlaneTestSuite) TestRegisterNode(t *testing.T) {
    ctx := context.Background()

    node := &controlplane.NodeInfo{
        NodeID:   "data-0",
        Address:  "192.168.1.10",
        GRPCPort: 9303,
        NodeType: controlplane.NodeTypeData,
    }

    err := suite.cp.RegisterNode(ctx, node)
    assert.NoError(t, err)

    // Verify node is in cluster state
    state, _ := suite.cp.GetClusterState(ctx)
    assert.Contains(t, state.Nodes, "data-0")
}

func (suite *ControlPlaneTestSuite) TestWatchClusterState(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    watch, err := suite.cp.WatchClusterState(ctx)
    assert.NoError(t, err)

    // Create an index in another goroutine
    go func() {
        time.Sleep(100 * time.Millisecond)
        suite.cp.CreateIndex(ctx, &controlplane.CreateIndexRequest{
            Name: "watch-test",
            Settings: &controlplane.IndexSettings{Shards: 1},
        })
    }()

    // Should receive event
    select {
    case event := <-watch:
        assert.Equal(t, controlplane.EventTypeIndexCreated, event.Type)
    case <-time.After(5 * time.Second):
        t.Fatal("Timeout waiting for watch event")
    }
}

// Run suite against Raft implementation
func TestRaftControlPlane(t *testing.T) {
    cp := setupRaftControlPlane(t)
    defer cp.Stop()

    suite := &ControlPlaneTestSuite{cp: cp}
    suite.TestCreateIndex(t)
    suite.TestGetClusterState(t)
    suite.TestRegisterNode(t)
    suite.TestWatchClusterState(t)
}

// Run suite against K8S implementation
func TestK8SControlPlane(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping K8S integration test")
    }

    cp := setupK8SControlPlane(t)
    defer cp.Stop()

    suite := &ControlPlaneTestSuite{cp: cp}
    suite.TestCreateIndex(t)
    suite.TestGetClusterState(t)
    suite.TestRegisterNode(t)
    suite.TestWatchClusterState(t)
}
```

---

## Comparison Matrix

| Feature | Raft Mode | K8S Mode | Notes |
|---------|-----------|----------|-------|
| **Deployment** |
| Kubernetes | ✅ StatefulSet | ✅ Deployment | Both supported |
| Bare Metal | ✅ Native | ❌ Not supported | Raft only |
| VMs | ✅ Native | ❌ Not supported | Raft only |
| **Performance** |
| Index creation | 25-35ms | 40-80ms | K8S 2× slower |
| Cluster state read | <1ms (cache) | <1ms (cache) | Same |
| Shard allocation | 10ms | 15ms | K8S 50% slower |
| Search query | N/A | N/A | Both bypass control plane |
| **Operational** |
| Infrastructure | 3 master pods + PVCs | 3 operator pods | K8S simpler |
| Cost (AWS EKS) | ~$162/month | ~$40/month | K8S 75% cheaper |
| Monitoring | Custom Raft metrics | K8S events | K8S integrated |
| Backup | Raft snapshots | kubectl backup | K8S native |
| **Consistency** |
| Write consistency | Strong (Raft) | Strong (etcd/Raft) | Same |
| Read consistency | Strong (FSM) | Strong (K8S API) | Same |
| Watch latency | Immediate | 10-100ms | Raft faster |
| **Flexibility** |
| Multi-environment | ✅ Yes | ❌ K8S only | Raft portable |
| kubectl integration | ❌ No | ✅ Yes | K8S native |
| K8S RBAC | ❌ Custom | ✅ Native | K8S integrated |

---

## Recommendation

**For a general-purpose search engine software:**

✅ **Implement both modes** with pluggable interface

**Default mode:**
- `auto` - Detect environment and choose automatically
- K8S → Use K8S-native
- Non-K8S → Use Raft

**This gives Quidditch:**
1. Maximum deployment flexibility
2. K8S-native experience when in K8S
3. Bare metal/VM support via Raft
4. Single codebase for both modes
5. Migration path between modes

**Implementation priority:**
1. Define `ControlPlane` interface (Week 1)
2. Implement Raft mode (Weeks 2-4)
3. Implement K8S mode (Weeks 5-6)
4. Test both with same test suite (Week 7)
5. Document and provide Helm chart (Week 8)

**Total: 8 weeks to production-ready dual-mode architecture**

---

**Document Version**: 1.0
**Last Updated**: 2026-01-26
**Status**: Design Complete - Ready for Implementation
