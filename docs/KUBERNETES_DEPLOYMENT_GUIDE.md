# Kubernetes Deployment Guide

**Version**: 1.0
**Date**: 2026-01-26

---

## Table of Contents

1. [Deployment Options](#deployment-options)
2. [Option 1: Traditional Masters (Recommended)](#option-1-traditional-masters-recommended)
3. [Option 2: K8S-Native Control Plane](#option-2-k8s-native-control-plane)
4. [Option 3: Hybrid Approach](#option-3-hybrid-approach)
5. [Production Patterns](#production-patterns)
6. [Cost Analysis](#cost-analysis)
7. [Migration Strategies](#migration-strategies)

---

## Deployment Options

Quidditch can be deployed in Kubernetes using three architectural patterns:

| Pattern | Master Nodes | Control Logic | Consistency | Portability | Cost |
|---------|--------------|---------------|-------------|-------------|------|
| **Traditional Masters** | Yes (StatefulSet) | Raft consensus | Strong | High | Medium |
| **K8S-Native** | No | K8S controllers | Eventual | K8S-only | Low |
| **Hybrid** | Optional | Mixed | Configurable | Medium | Medium |

---

## Option 1: Traditional Masters (Recommended)

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                    │
│                                                          │
│  ┌────────────────────────────────────────────────────┐│
│  │           Namespace: quidditch                      ││
│  │                                                     ││
│  │  ┌──────────────────────────────────────────────┐ ││
│  │  │  Service: quidditch-coordination             │ ││
│  │  │  Type: LoadBalancer                          │ ││
│  │  │  Port: 9200 (HTTPS)                          │ ││
│  │  └────────────┬─────────────────────────────────┘ ││
│  │               │                                    ││
│  │  ┌────────────▼─────────────────────────────────┐ ││
│  │  │  Deployment: quidditch-coordination          │ ││
│  │  │  Replicas: 3                                 │ ││
│  │  │  ┌──────┐  ┌──────┐  ┌──────┐               │ ││
│  │  │  │Coord1│  │Coord2│  │Coord3│               │ ││
│  │  │  └───┬──┘  └───┬──┘  └───┬──┘               │ ││
│  │  └──────┼─────────┼─────────┼───────────────────┘ ││
│  │         │         │         │                      ││
│  │  ┌──────▼─────────▼─────────▼───────────────────┐ ││
│  │  │  StatefulSet: quidditch-master               │ ││
│  │  │  Replicas: 3                                 │ ││
│  │  │  ┌────────┐  ┌────────┐  ┌────────┐         │ ││
│  │  │  │Master-0│  │Master-1│  │Master-2│         │ ││
│  │  │  │PVC:50Gi│  │PVC:50Gi│  │PVC:50Gi│         │ ││
│  │  │  └───┬────┘  └───┬────┘  └───┬────┘         │ ││
│  │  └──────┼───────────┼───────────┼──────────────┘ ││
│  │         │           │           │                 ││
│  │  ┌──────▼───────────▼───────────▼──────────────┐ ││
│  │  │  StatefulSet: quidditch-data                │ ││
│  │  │  Replicas: 5                                │ ││
│  │  │  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌────┐│ ││
│  │  │  │Data-0│ │Data-1│ │Data-2│ │Data-3│ │... ││ ││
│  │  │  │PVC:  │ │PVC:  │ │PVC:  │ │PVC:  │ │    ││ ││
│  │  │  │500Gi │ │500Gi │ │500Gi │ │500Gi │ │    ││ ││
│  │  │  └──────┘ └──────┘ └──────┘ └──────┘ └────┘│ ││
│  │  └────────────────────────────────────────────┘ ││
│  └─────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

### Complete Kubernetes Manifests

#### 1. Namespace

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: quidditch
```

#### 2. Master StatefulSet

```yaml
apiVersion: v1
kind: Service
metadata:
  name: quidditch-master
  namespace: quidditch
spec:
  clusterIP: None  # Headless service for StatefulSet
  selector:
    app: quidditch-master
  ports:
  - name: raft
    port: 9300
    targetPort: 9300
  - name: grpc
    port: 9301
    targetPort: 9301
  - name: metrics
    port: 9400
    targetPort: 9400
---
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
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9400"
    spec:
      affinity:
        # Spread masters across nodes
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: app
                operator: In
                values:
                - quidditch-master
            topologyKey: kubernetes.io/hostname
      containers:
      - name: master
        image: quidditch/master:1.0.0
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 9300
          name: raft
        - containerPort: 9301
          name: grpc
        - containerPort: 9400
          name: metrics
        env:
        - name: NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: RAFT_PEERS
          value: "quidditch-master-0.quidditch-master.quidditch.svc.cluster.local:9300,quidditch-master-1.quidditch-master.quidditch.svc.cluster.local:9300,quidditch-master-2.quidditch-master.quidditch.svc.cluster.local:9300"
        - name: BIND_ADDR
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        volumeMounts:
        - name: data
          mountPath: /var/lib/quidditch/master
        resources:
          requests:
            memory: "4Gi"
            cpu: "2"
          limits:
            memory: "8Gi"
            cpu: "4"
        livenessProbe:
          httpGet:
            path: /health
            port: 9400
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 9400
          initialDelaySeconds: 10
          periodSeconds: 5
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: fast-ssd  # Use your storage class
      resources:
        requests:
          storage: 50Gi
```

#### 3. Coordination Deployment

```yaml
apiVersion: v1
kind: Service
metadata:
  name: quidditch-coordination
  namespace: quidditch
spec:
  type: LoadBalancer
  selector:
    app: quidditch-coordination
  ports:
  - name: http
    port: 9200
    targetPort: 9200
  - name: metrics
    port: 9401
    targetPort: 9401
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: quidditch-coordination
  namespace: quidditch
spec:
  replicas: 3
  selector:
    matchLabels:
      app: quidditch-coordination
  template:
    metadata:
      labels:
        app: quidditch-coordination
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9401"
    spec:
      containers:
      - name: coordination
        image: quidditch/coordination:1.0.0
        ports:
        - containerPort: 9200
          name: http
        - containerPort: 9302
          name: grpc
        - containerPort: 9401
          name: metrics
        env:
        - name: NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: MASTER_ADDRESSES
          value: "quidditch-master-0.quidditch-master.quidditch.svc.cluster.local:9301,quidditch-master-1.quidditch-master.quidditch.svc.cluster.local:9301,quidditch-master-2.quidditch-master.quidditch.svc.cluster.local:9301"
        - name: BIND_ADDR
          value: "0.0.0.0"
        - name: REST_PORT
          value: "9200"
        resources:
          requests:
            memory: "4Gi"
            cpu: "2"
          limits:
            memory: "8Gi"
            cpu: "4"
        livenessProbe:
          httpGet:
            path: /health
            port: 9200
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 9200
          initialDelaySeconds: 10
          periodSeconds: 5
```

#### 4. Data Node StatefulSet

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
  - name: grpc
    port: 9303
    targetPort: 9303
  - name: metrics
    port: 9402
    targetPort: 9402
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: quidditch-data
  namespace: quidditch
spec:
  serviceName: quidditch-data
  replicas: 5
  selector:
    matchLabels:
      app: quidditch-data
  template:
    metadata:
      labels:
        app: quidditch-data
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9402"
    spec:
      affinity:
        # Spread data nodes across nodes
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - quidditch-data
              topologyKey: kubernetes.io/hostname
      containers:
      - name: data
        image: quidditch/data:1.0.0
        ports:
        - containerPort: 9303
          name: grpc
        - containerPort: 9402
          name: metrics
        env:
        - name: NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: MASTER_ADDR
          value: "quidditch-master-0.quidditch-master.quidditch.svc.cluster.local:9301"
        - name: BIND_ADDR
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        - name: STORAGE_TIER
          value: "hot"
        volumeMounts:
        - name: data
          mountPath: /var/lib/quidditch/data
        resources:
          requests:
            memory: "16Gi"
            cpu: "8"
          limits:
            memory: "32Gi"
            cpu: "16"
        livenessProbe:
          httpGet:
            path: /health
            port: 9402
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 9402
          initialDelaySeconds: 10
          periodSeconds: 5
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: fast-ssd
      resources:
        requests:
          storage: 500Gi
```

### Deployment Steps

```bash
# 1. Create namespace
kubectl apply -f namespace.yaml

# 2. Deploy master nodes (wait for all to be ready)
kubectl apply -f master-statefulset.yaml
kubectl rollout status statefulset/quidditch-master -n quidditch

# 3. Deploy coordination nodes
kubectl apply -f coordination-deployment.yaml
kubectl rollout status deployment/quidditch-coordination -n quidditch

# 4. Deploy data nodes
kubectl apply -f data-statefulset.yaml
kubectl rollout status statefulset/quidditch-data -n quidditch

# 5. Verify cluster
kubectl get pods -n quidditch
kubectl logs -n quidditch quidditch-master-0
```

### Scaling

```bash
# Scale coordination nodes (stateless)
kubectl scale deployment quidditch-coordination -n quidditch --replicas=5

# Scale data nodes (stateful)
kubectl scale statefulset quidditch-data -n quidditch --replicas=10

# NEVER scale master nodes beyond 3 or 5
# More masters = higher Raft latency, not higher capacity
```

---

## Option 2: K8S-Native Control Plane

**⚠️ Not recommended for production - documented for completeness**

### Architecture

No master StatefulSet. Coordination nodes use K8S API for coordination.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: quidditch-coordination
  namespace: quidditch
spec:
  replicas: 3
  selector:
    matchLabels:
      app: quidditch-coordination
  template:
    metadata:
      labels:
        app: quidditch-coordination
    spec:
      serviceAccountName: quidditch-coordination
      containers:
      - name: coordination
        image: quidditch/coordination-k8s:1.0.0
        env:
        - name: USE_K8S_COORDINATION
          value: "true"
        - name: LEADER_ELECTION_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: quidditch-coordination
  namespace: quidditch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: quidditch-coordination
  namespace: quidditch
rules:
- apiGroups: [""]
  resources: ["configmaps", "endpoints"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: quidditch-coordination
  namespace: quidditch
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: quidditch-coordination
subjects:
- kind: ServiceAccount
  name: quidditch-coordination
  namespace: quidditch
```

### Disadvantages

1. **K8S-only**: Can't deploy in VMs or bare metal
2. **Eventual consistency**: ConfigMap updates propagate with delays
3. **1MB limit**: ConfigMaps limited to 1MB
4. **API server dependency**: Cluster coordination depends on K8S API availability
5. **Custom implementation**: Need to write coordination logic yourself

**Verdict**: Only use for PoC or cost-constrained dev environments.

---

## Option 3: Hybrid Approach

Use master nodes, but make them optional in K8S.

### Configuration

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: quidditch-config
  namespace: quidditch
data:
  config.yaml: |
    coordination:
      mode: auto  # auto, master, k8s-native
      master_addresses:
        - quidditch-master-0.quidditch-master:9301
        - quidditch-master-1.quidditch-master:9301
        - quidditch-master-2.quidditch-master:9301
      k8s_fallback: true  # Use K8S if masters unavailable
```

### Logic

```go
func (c *Coordination) Start() error {
    mode := c.config.Mode

    if mode == "auto" {
        // Try master nodes first
        if c.canConnectToMasters() {
            return c.startWithMasters()
        }

        // Fall back to K8S-native if configured
        if c.config.K8SFallback && c.isRunningInK8S() {
            c.logger.Warn("Master nodes unavailable, using K8S-native mode")
            return c.startK8SNative()
        }

        return errors.New("no coordination backend available")
    }

    // Explicit mode
    switch mode {
    case "master":
        return c.startWithMasters()
    case "k8s-native":
        return c.startK8SNative()
    }
}
```

### Use Case

- Development: K8S-native (no master pods)
- Staging: Hybrid (masters optional)
- Production: Masters only (reliable)

---

## Production Patterns

### Pattern 1: Multi-Zone Deployment

```yaml
# Spread masters across availability zones
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: quidditch-master
spec:
  # ...
  template:
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: app
                operator: In
                values:
                - quidditch-master
            topologyKey: topology.kubernetes.io/zone
```

### Pattern 2: Node Selectors

```yaml
# Dedicate nodes for each component
---
# Master nodes on control-plane nodes
spec:
  nodeSelector:
    node-role: control-plane
    workload: quidditch-master

---
# Data nodes on storage-optimized nodes
spec:
  nodeSelector:
    node-role: data
    workload: quidditch-data
    storage-type: nvme
```

### Pattern 3: Resource Quotas

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: quidditch-quota
  namespace: quidditch
spec:
  hard:
    requests.cpu: "200"
    requests.memory: "400Gi"
    persistentvolumeclaims: "20"
    services.loadbalancers: "2"
```

### Pattern 4: PodDisruptionBudget

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: quidditch-master-pdb
  namespace: quidditch
spec:
  minAvailable: 2  # Always keep quorum
  selector:
    matchLabels:
      app: quidditch-master
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: quidditch-data-pdb
  namespace: quidditch
spec:
  maxUnavailable: 1  # One data node can be down
  selector:
    matchLabels:
      app: quidditch-data
```

---

## Cost Analysis

### Traditional Masters (3 nodes)

```
Master pods:
  - 3 × 4 GB RAM = 12 GB
  - 3 × 2 CPU = 6 vCPUs
  - 3 × 50 GB SSD = 150 GB

Estimated cost (AWS EKS):
  - Memory: 12 GB × $0.05/GB/month = $0.60/month
  - CPU: 6 vCPUs × $0.034/vCPU/hour = $146/month
  - Storage: 150 GB × $0.10/GB/month = $15/month
  - Total: ~$162/month
```

### K8S-Native (0 master nodes)

```
No additional pods needed.

Cost savings: $162/month

But:
  - No strong consistency
  - K8S-only deployment
  - Higher operational risk
```

### Is $162/month worth it?

**Yes**, if:
- Production cluster serving millions of queries
- Need strong consistency guarantees
- Want portability and proven architecture
- Total cluster cost is $10,000+/month

**Consider K8S-native** if:
- Development/testing only
- Very small cluster (<3 data nodes)
- Can tolerate eventual consistency
- Cost is extreme constraint

---

## Migration Strategies

### From K8S-Native to Traditional Masters

```bash
# 1. Deploy master StatefulSet
kubectl apply -f master-statefulset.yaml

# 2. Wait for masters to be ready
kubectl wait --for=condition=ready pod -l app=quidditch-master -n quidditch

# 3. Update coordination config to use masters
kubectl set env deployment/quidditch-coordination \
  -n quidditch \
  USE_K8S_COORDINATION=false \
  MASTER_ADDRESSES="quidditch-master-0.quidditch-master:9301,..."

# 4. Rolling restart coordination nodes
kubectl rollout restart deployment/quidditch-coordination -n quidditch

# 5. Verify master nodes are being used
kubectl logs -n quidditch -l app=quidditch-coordination | grep "connected to master"

# 6. Remove K8S permissions (optional)
kubectl delete rolebinding quidditch-coordination -n quidditch
kubectl delete role quidditch-coordination -n quidditch
```

### From Traditional Masters to K8S-Native

**⚠️ Not recommended - lose data consistency guarantees**

```bash
# 1. Enable K8S fallback in coordination
kubectl set env deployment/quidditch-coordination \
  -n quidditch \
  K8S_FALLBACK=true

# 2. Grant K8S permissions
kubectl apply -f rbac.yaml

# 3. Scale down masters
kubectl scale statefulset quidditch-master -n quidditch --replicas=0

# 4. Coordination nodes will auto-switch to K8S-native

# 5. Verify
kubectl logs -n quidditch -l app=quidditch-coordination | grep "using k8s-native"
```

---

## Helm Chart

### Install via Helm

```bash
helm repo add quidditch https://charts.quidditch.io
helm repo update

# Install with default (traditional masters)
helm install quidditch quidditch/quidditch \
  --namespace quidditch \
  --create-namespace \
  --set master.enabled=true \
  --set master.replicas=3 \
  --set coordination.replicas=3 \
  --set data.replicas=5

# Install K8S-native (not recommended for production)
helm install quidditch quidditch/quidditch \
  --namespace quidditch \
  --create-namespace \
  --set master.enabled=false \
  --set coordination.k8sNative=true
```

### Custom Values

```yaml
# values.yaml
master:
  enabled: true
  replicas: 3
  resources:
    requests:
      memory: 4Gi
      cpu: 2
  storage:
    size: 50Gi
    storageClass: fast-ssd

coordination:
  replicas: 3
  k8sNative: false
  resources:
    requests:
      memory: 4Gi
      cpu: 2

data:
  replicas: 5
  resources:
    requests:
      memory: 16Gi
      cpu: 8
  storage:
    size: 500Gi
    storageClass: fast-ssd
```

---

## Conclusion

### Recommended: Traditional Masters

Use master StatefulSet in Kubernetes for:
- ✅ Strong consistency (Raft)
- ✅ Production reliability
- ✅ Portability (same code as bare metal)
- ✅ Proven architecture
- ✅ Low cost ($162/month for 3 masters)

### When K8S-Native Might Be Acceptable

Only for:
- Development environments
- Cost-constrained PoCs
- Very small clusters
- Can tolerate eventual consistency

### Final Recommendation

**Always use traditional master nodes in Kubernetes.**
- Cost is negligible compared to data node costs
- Reliability gains are significant
- Operational simplicity (standard Raft patterns)

---

**Document Version**: 1.0
**Last Updated**: 2026-01-26
