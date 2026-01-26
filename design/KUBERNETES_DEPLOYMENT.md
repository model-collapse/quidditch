# Quidditch Kubernetes Deployment Guide

**Version**: 1.0.0
**Date**: 2026-01-25

---

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Operator Installation](#operator-installation)
4. [Cluster Deployment](#cluster-deployment)
5. [Storage Configuration](#storage-configuration)
6. [Networking](#networking)
7. [Security](#security)
8. [Monitoring](#monitoring)
9. [Scaling](#scaling)
10. [Backup & Restore](#backup--restore)

---

## Overview

Quidditch uses the Kubernetes Operator pattern for cluster lifecycle management. The operator automates:
- Cluster provisioning
- Rolling upgrades
- Scaling
- Backup/restore
- Self-healing

---

## Prerequisites

### Required
- Kubernetes 1.24+
- kubectl configured
- Persistent storage provider (NVMe CSI recommended)
- Object storage (S3/MinIO/Ceph) for cold tier
- Load balancer (for external access)

### Recommended
- Prometheus operator (for monitoring)
- Cert-manager (for TLS)
- External-DNS (for DNS automation)

---

## Operator Installation

### Install Quidditch Operator

```bash
# Add Helm repository
helm repo add quidditch https://quidditch.io/charts
helm repo update

# Install operator
kubectl create namespace quidditch-system
helm install quidditch-operator quidditch/operator \
  --namespace quidditch-system \
  --version 1.0.0

# Verify installation
kubectl get pods -n quidditch-system
```

**Expected Output**:
```
NAME                                  READY   STATUS    RESTARTS   AGE
quidditch-operator-7d6b5c8f9d-abcde   1/1     Running   0          30s
```

---

## Cluster Deployment

### Minimal Single-Node Cluster (Development)

```yaml
# quidditch-dev.yaml
apiVersion: quidditch.io/v1
kind: QuidditchCluster
metadata:
  name: quidditch-dev
  namespace: default
spec:
  version: "1.0.0"

  # Single node with all roles
  master:
    replicas: 1
    resources:
      requests:
        memory: "2Gi"
        cpu: "1"
      limits:
        memory: "4Gi"
        cpu: "2"

  coordination:
    replicas: 1
    resources:
      requests:
        memory: "2Gi"
        cpu: "1"
      limits:
        memory: "4Gi"
        cpu: "2"

  data:
    replicas: 1
    storage:
      class: "local-path"
      size: "10Gi"
    resources:
      requests:
        memory: "4Gi"
        cpu: "2"
      limits:
        memory: "8Gi"
        cpu: "4"

  # Development settings
  settings:
    index:
      number_of_shards: 1
      number_of_replicas: 0
    translog:
      durability: "async"
      sync_interval: "5s"
```

**Deploy**:
```bash
kubectl apply -f quidditch-dev.yaml

# Wait for ready
kubectl wait --for=condition=Ready quidditchcluster/quidditch-dev --timeout=300s

# Get cluster info
kubectl get quidditchcluster quidditch-dev -o yaml
```

---

### Production Cluster (High Availability)

```yaml
# quidditch-prod.yaml
apiVersion: quidditch.io/v1
kind: QuidditchCluster
metadata:
  name: quidditch-prod
  namespace: quidditch
spec:
  version: "1.0.0"

  # 3 master nodes for HA
  master:
    replicas: 3
    resources:
      requests:
        memory: "4Gi"
        cpu: "2"
      limits:
        memory: "8Gi"
        cpu: "4"
    storage:
      class: "fast-ssd"
      size: "20Gi"
    nodeSelector:
      node-role: master
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              app: quidditch
              role: master
          topologyKey: kubernetes.io/hostname
        - labelSelector:
            matchLabels:
              app: quidditch
              role: master
          topologyKey: topology.kubernetes.io/zone

  # 5 coordination nodes with auto-scaling
  coordination:
    replicas: 5
    autoscaling:
      enabled: true
      minReplicas: 5
      maxReplicas: 20
      targetCPUUtilization: 70
      targetMemoryUtilization: 80
    resources:
      requests:
        memory: "8Gi"
        cpu: "4"
      limits:
        memory: "16Gi"
        cpu: "8"
    nodeSelector:
      node-role: coordination
    python:
      enabled: true
      virtualenv: "/opt/quidditch/venv"
      packages:
        - "numpy==1.24.0"
        - "scikit-learn==1.2.0"
        - "onnxruntime==1.14.0"

  # 10 data nodes with dedicated NVMe storage
  data:
    replicas: 10
    storage:
      class: "nvme-ssd"
      size: "1Ti"
      tiering:
        hot:
          class: "nvme-ssd"
          size: "500Gi"
        warm:
          class: "ssd"
          size: "500Gi"
        cold:
          enabled: true
          s3:
            endpoint: "s3.amazonaws.com"
            bucket: "quidditch-prod-cold"
            region: "us-east-1"
    resources:
      requests:
        memory: "32Gi"
        cpu: "16"
      limits:
        memory: "64Gi"
        cpu: "32"
    nodeSelector:
      node-role: data
      storage-type: nvme
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchLabels:
                app: quidditch
                role: data
            topologyKey: topology.kubernetes.io/zone

  # Global settings
  settings:
    cluster:
      name: "quidditch-prod"
      routing:
        allocation:
          enable: "all"
          awareness:
            attributes: ["zone", "rack"]
          disk:
            threshold_enabled: true
            low_watermark: "85%"
            high_watermark: "90%"
            flood_stage: "95%"
    index:
      number_of_shards: 5
      number_of_replicas: 1
      refresh_interval: "1s"
      codec: "diagon_best_compression"
    translog:
      durability: "request"
      flush_threshold_size: "512mb"

  # TLS configuration
  tls:
    enabled: true
    certificateRef:
      name: quidditch-tls-cert
      namespace: quidditch

  # External access
  service:
    type: LoadBalancer
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
      external-dns.alpha.kubernetes.io/hostname: "quidditch-prod.example.com"

  # Monitoring
  monitoring:
    prometheus:
      enabled: true
      serviceMonitor:
        interval: "30s"
        scrapeTimeout: "10s"

  # Backup
  backup:
    enabled: true
    schedule: "0 2 * * *"  # 2 AM daily
    repository:
      type: s3
      bucket: "quidditch-prod-backups"
      region: "us-east-1"
    retention:
      days: 30
      minCount: 5
```

**Deploy**:
```bash
# Create namespace
kubectl create namespace quidditch

# Create S3 credentials secret
kubectl create secret generic quidditch-s3-creds \
  --namespace quidditch \
  --from-literal=access-key-id=AKIAIOSFODNN7EXAMPLE \
  --from-literal=secret-access-key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

# Deploy cluster
kubectl apply -f quidditch-prod.yaml

# Monitor deployment
kubectl get pods -n quidditch -w

# Get service endpoint
kubectl get svc -n quidditch quidditch-prod-coordination
```

---

## Storage Configuration

### NVMe CSI Driver (AWS EBS)

```yaml
# storageclass-nvme.yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: nvme-ssd
provisioner: ebs.csi.aws.com
parameters:
  type: io2  # Provisioned IOPS SSD
  iopsPerGB: "500"
  encrypted: "true"
  fsType: ext4
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
```

### Local Path Provisioner (Development)

```yaml
# storageclass-local.yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: local-path
provisioner: rancher.io/local-path
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
```

---

## Networking

### Service Configuration

```yaml
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: quidditch-prod-coordination
  namespace: quidditch
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: nlb
spec:
  type: LoadBalancer
  selector:
    app: quidditch
    role: coordination
  ports:
  - name: http
    port: 9200
    targetPort: 9200
  - name: transport
    port: 9300
    targetPort: 9300
```

### Ingress (Optional)

```yaml
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: quidditch-ingress
  namespace: quidditch
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/proxy-body-size: "100m"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - quidditch.example.com
    secretName: quidditch-tls
  rules:
  - host: quidditch.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: quidditch-prod-coordination
            port:
              number: 9200
```

---

## Security

### RBAC

```yaml
# rbac.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: quidditch
  namespace: quidditch

---

apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: quidditch
  namespace: quidditch
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]

---

apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: quidditch
  namespace: quidditch
subjects:
- kind: ServiceAccount
  name: quidditch
  namespace: quidditch
roleRef:
  kind: Role
  name: quidditch
  apiGroup: rbac.authorization.k8s.io
```

### TLS Certificate

```yaml
# certificate.yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: quidditch-tls-cert
  namespace: quidditch
spec:
  secretName: quidditch-tls-cert
  issuer:
    name: letsencrypt-prod
    kind: ClusterIssuer
  commonName: quidditch.example.com
  dnsNames:
  - quidditch.example.com
  - "*.quidditch.example.com"
```

### Network Policies

```yaml
# networkpolicy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: quidditch-data-nodes
  namespace: quidditch
spec:
  podSelector:
    matchLabels:
      app: quidditch
      role: data
  policyTypes:
  - Ingress
  ingress:
  # Allow from coordination nodes
  - from:
    - podSelector:
        matchLabels:
          app: quidditch
          role: coordination
    ports:
    - protocol: TCP
      port: 9300
  # Allow from master nodes
  - from:
    - podSelector:
        matchLabels:
          app: quidditch
          role: master
    ports:
    - protocol: TCP
      port: 9300
```

---

## Monitoring

### Prometheus ServiceMonitor

```yaml
# servicemonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: quidditch
  namespace: quidditch
  labels:
    app: quidditch
spec:
  selector:
    matchLabels:
      app: quidditch
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

### Grafana Dashboard

```json
{
  "dashboard": {
    "title": "Quidditch Cluster Overview",
    "panels": [
      {
        "title": "Query Rate",
        "targets": [
          {
            "expr": "rate(quidditch_query_total[5m])"
          }
        ]
      },
      {
        "title": "Query Latency (p99)",
        "targets": [
          {
            "expr": "histogram_quantile(0.99, rate(quidditch_query_latency_seconds_bucket[5m]))"
          }
        ]
      },
      {
        "title": "Indexing Rate",
        "targets": [
          {
            "expr": "rate(quidditch_indexing_docs_total[5m])"
          }
        ]
      },
      {
        "title": "Disk Usage",
        "targets": [
          {
            "expr": "quidditch_disk_usage_bytes / quidditch_disk_capacity_bytes"
          }
        ]
      }
    ]
  }
}
```

---

## Scaling

### Manual Scaling

```bash
# Scale data nodes
kubectl patch quidditchcluster quidditch-prod \
  --namespace quidditch \
  --type merge \
  --patch '{"spec":{"data":{"replicas":20}}}'

# Scale coordination nodes
kubectl patch quidditchcluster quidditch-prod \
  --namespace quidditch \
  --type merge \
  --patch '{"spec":{"coordination":{"replicas":10}}}'
```

### Horizontal Pod Autoscaler (HPA)

```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: quidditch-coordination
  namespace: quidditch
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: quidditch-prod-coordination
  minReplicas: 5
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
      - type: Percent
        value: 100
        periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Percent
        value: 50
        periodSeconds: 60
```

---

## Backup & Restore

### Configure Backup Repository

```http
PUT /_snapshot/k8s-s3-repo
{
  "type": "s3",
  "settings": {
    "bucket": "quidditch-prod-backups",
    "region": "us-east-1",
    "base_path": "snapshots",
    "compress": true,
    "chunk_size": "100mb"
  }
}
```

### Create Snapshot Policy

```yaml
# snapshotpolicy.yaml
apiVersion: quidditch.io/v1
kind: SnapshotPolicy
metadata:
  name: daily-backups
  namespace: quidditch
spec:
  clusterRef:
    name: quidditch-prod
  schedule: "0 2 * * *"  # 2 AM UTC
  repository: k8s-s3-repo
  indices: ["*"]
  retention:
    expireAfter: "30d"
    minCount: 7
    maxCount: 30
```

### Manual Snapshot

```bash
# Create snapshot
curl -X PUT "http://quidditch-prod.quidditch:9200/_snapshot/k8s-s3-repo/snapshot_$(date +%Y%m%d_%H%M%S)" \
  -H 'Content-Type: application/json' \
  -d '{
    "indices": "*",
    "ignore_unavailable": true,
    "include_global_state": true
  }'

# List snapshots
curl "http://quidditch-prod.quidditch:9200/_snapshot/k8s-s3-repo/_all"

# Restore snapshot
curl -X POST "http://quidditch-prod.quidditch:9200/_snapshot/k8s-s3-repo/snapshot_20260125_020000/_restore" \
  -H 'Content-Type: application/json' \
  -d '{
    "indices": "logs-*",
    "ignore_unavailable": true
  }'
```

---

## Troubleshooting

### Check Cluster Health

```bash
# Cluster status
kubectl get quidditchcluster -n quidditch

# Pod status
kubectl get pods -n quidditch

# Logs
kubectl logs -n quidditch -l app=quidditch,role=master
kubectl logs -n quidditch -l app=quidditch,role=coordination
kubectl logs -n quidditch -l app=quidditch,role=data

# Events
kubectl get events -n quidditch --sort-by='.lastTimestamp'
```

### Common Issues

**Issue**: Pods stuck in Pending
```bash
# Check PVC status
kubectl get pvc -n quidditch

# Check storage class
kubectl get storageclass

# Describe pod
kubectl describe pod <pod-name> -n quidditch
```

**Issue**: Cluster not ready
```bash
# Check master nodes
kubectl exec -n quidditch <master-pod> -- curl localhost:9300/_cluster/health

# Check discovery
kubectl logs -n quidditch <master-pod> | grep -i discovery
```

**Issue**: High memory usage
```bash
# Check resource limits
kubectl top pods -n quidditch

# Adjust limits
kubectl patch quidditchcluster quidditch-prod \
  --namespace quidditch \
  --type merge \
  --patch '{"spec":{"data":{"resources":{"limits":{"memory":"128Gi"}}}}}'
```

---

## Best Practices

1. **Resource Planning**:
   - Data nodes: 64 GB RAM per node minimum
   - Coordination nodes: 16 GB RAM per node
   - Master nodes: 8 GB RAM per node

2. **Storage**:
   - Use NVMe SSDs for hot tier
   - Enable volume expansion
   - Monitor disk usage (alert at 80%)

3. **High Availability**:
   - Always run 3 master nodes
   - Use pod anti-affinity for replicas
   - Distribute across availability zones

4. **Security**:
   - Enable TLS for all communication
   - Use RBAC for access control
   - Rotate credentials regularly
   - Use network policies

5. **Monitoring**:
   - Set up Prometheus + Grafana
   - Alert on high latency, errors, disk usage
   - Monitor cluster health

---

## Reference

- [Quidditch Architecture](QUIDDITCH_ARCHITECTURE.md)
- [Python Pipeline Guide](PYTHON_PIPELINE_GUIDE.md)
- [Kubernetes Documentation](https://kubernetes.io/docs/)
- [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator)

---

**Version**: 1.0.0
**Last Updated**: 2026-01-25
