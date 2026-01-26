# Quidditch Deployment Scripts

This directory contains deployment and management scripts for Quidditch.

---

## Quick Start

```bash
# Deploy development cluster (auto-detects control plane mode)
./deploy-k8s.sh --profile dev

# Deploy production cluster with Raft masters
./deploy-k8s.sh --mode raft --profile prod --namespace quidditch-prod

# Deploy with K8S-native operator
./deploy-k8s.sh --mode k8s --profile staging

# Preview manifests without applying
./deploy-k8s.sh --mode raft --dry-run

# Uninstall
./deploy-k8s.sh --uninstall
```

---

## deploy-k8s.sh

**Comprehensive Kubernetes deployment script for Quidditch**

### Features

✅ **Dual-Mode Support**
- Raft mode (traditional masters)
- K8S-native mode (operator)
- Auto-detection based on environment

✅ **Multiple Deployment Profiles**
- `dev` - Single-node development
- `staging` - Multi-node staging
- `prod` - Production-grade HA

✅ **Complete Validation**
- Environment checks (kubectl, cluster connectivity)
- Resource validation
- Configuration verification

✅ **Health Monitoring**
- Rollout status tracking
- Pod health verification
- Service endpoint discovery

✅ **Production Features**
- HorizontalPodAutoscaler for coordination nodes
- StatefulSets for stateful components
- Proper resource limits and requests
- Liveness and readiness probes

### Usage

```bash
./deploy-k8s.sh [OPTIONS]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `--mode MODE` | Control plane mode: `raft`, `k8s`, `auto` | `auto` |
| `--namespace NS` | Kubernetes namespace | `quidditch` |
| `--profile PROFILE` | Deployment profile: `dev`, `staging`, `prod` | `dev` |
| `--cluster-name NAME` | Cluster name | `quidditch` |
| `--wait` | Wait for all pods to be ready | `true` |
| `--dry-run` | Print manifests without applying | `false` |
| `--upgrade` | Upgrade existing installation | `false` |
| `--uninstall` | Uninstall Quidditch | `false` |
| `--help` | Show help message | - |

### Control Plane Modes

#### Raft Mode (Traditional)

```bash
./deploy-k8s.sh --mode raft
```

**Architecture:**
- 3 master node pods (StatefulSet)
- Raft consensus via hashicorp/raft
- Persistent volumes for Raft logs
- Works on K8S, VMs, bare metal

**Characteristics:**
- Latency: 2-5ms for cluster operations
- Cost: ~$162/month (AWS EKS)
- Storage: 50GB per master pod
- Portability: Multi-environment

**Use when:**
- Need bare metal/VM deployment
- Require <5ms cluster operation latency
- Want independence from K8S control plane
- Multi-environment consistency needed

#### K8S-Native Mode (Operator)

```bash
./deploy-k8s.sh --mode k8s
```

**Architecture:**
- 3 operator pods (Deployment)
- Uses K8S etcd for state (Raft built-in)
- Custom Resource Definitions (CRDs)
- Leader election via Lease

**Characteristics:**
- Latency: 5-20ms for cluster operations
- Cost: ~$40/month (AWS EKS)
- Storage: Stateless (no PVCs)
- Portability: K8S-only

**Use when:**
- K8S-only deployment
- Cost-sensitive ($122/month savings)
- Prefer cloud-native patterns
- Want kubectl integration

#### Auto Mode (Recommended)

```bash
./deploy-k8s.sh --mode auto
```

Automatically detects environment:
- Running in K8S → Uses K8S-native mode
- Non-K8S environment → Uses Raft mode

### Deployment Profiles

#### Development (`dev`)

**Configuration:**
- 1 master/operator pod
- 1 coordination pod
- 1 data node pod
- Minimal resources

**Resources:**
- Master: 1 CPU, 2GB RAM, 20GB storage
- Coordination: 1 CPU, 2GB RAM
- Data: 2 CPU, 8GB RAM, 50GB storage

**Use for:**
- Local testing
- CI/CD pipelines
- Development clusters

```bash
./deploy-k8s.sh --profile dev
```

#### Staging (`staging`)

**Configuration:**
- 3 master/operator pods
- 2 coordination pods (+ HPA)
- 3 data nodes

**Resources:**
- Master: 2 CPU, 4GB RAM, 50GB storage
- Coordination: 2 CPU, 4GB RAM
- Data: 4 CPU, 16GB RAM, 200GB storage

**Use for:**
- Pre-production testing
- Load testing
- Integration testing

```bash
./deploy-k8s.sh --profile staging
```

#### Production (`prod`)

**Configuration:**
- 3 master/operator pods
- 3+ coordination pods (HPA to 20)
- 5+ data nodes

**Resources:**
- Master: 2 CPU, 4GB RAM, 50GB storage
- Coordination: 2 CPU, 4GB RAM
- Data: 8 CPU, 16GB RAM, 500GB storage

**Use for:**
- Production workloads
- High availability
- Large-scale deployments

```bash
./deploy-k8s.sh --profile prod --namespace quidditch-prod
```

### Examples

#### 1. Deploy Development Cluster

```bash
# Auto-detect mode, use development profile
./deploy-k8s.sh --profile dev

# Wait for rollout
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/instance=quidditch -n quidditch --timeout=600s

# Get coordination service endpoint
kubectl get svc quidditch-coordination -n quidditch
```

#### 2. Deploy Production with Raft

```bash
# Force Raft mode for multi-environment consistency
./deploy-k8s.sh \
  --mode raft \
  --profile prod \
  --namespace quidditch-prod \
  --cluster-name quidditch-prod

# Check master pods
kubectl get pods -n quidditch-prod -l app.kubernetes.io/component=master

# Verify Raft cluster
kubectl logs -n quidditch-prod quidditch-prod-master-0 | grep -i "raft"
```

#### 3. Deploy Staging with K8S-Native

```bash
# Use K8S-native operator
./deploy-k8s.sh \
  --mode k8s \
  --profile staging \
  --namespace quidditch-staging

# Check operator
kubectl get pods -n quidditch-staging -l app.kubernetes.io/component=operator

# View CRDs
kubectl get quidditchindices -n quidditch-staging
```

#### 4. Dry Run (Preview Manifests)

```bash
# See what would be deployed without applying
./deploy-k8s.sh --mode raft --profile prod --dry-run > manifests.yaml

# Review manifests
less manifests.yaml

# Apply manually if satisfied
kubectl apply -f manifests.yaml
```

#### 5. Upgrade Existing Cluster

```bash
# Upgrade with new configuration
./deploy-k8s.sh --upgrade --profile prod

# Monitor rollout
kubectl rollout status deployment/quidditch-coordination -n quidditch
kubectl rollout status statefulset/quidditch-data -n quidditch
```

#### 6. Multi-Cluster Deployment

```bash
# Production cluster
./deploy-k8s.sh \
  --mode raft \
  --profile prod \
  --namespace quidditch-prod \
  --cluster-name prod-cluster

# Staging cluster (same K8S)
./deploy-k8s.sh \
  --mode k8s \
  --profile staging \
  --namespace quidditch-staging \
  --cluster-name staging-cluster

# Dev cluster (same K8S)
./deploy-k8s.sh \
  --profile dev \
  --namespace quidditch-dev \
  --cluster-name dev-cluster
```

### Post-Deployment

#### 1. Verify Deployment

```bash
# Check all pods
kubectl get pods -n quidditch -l app.kubernetes.io/instance=quidditch

# Check services
kubectl get svc -n quidditch

# Check persistent volumes
kubectl get pvc -n quidditch

# Check cluster health
kubectl logs -n quidditch deployment/quidditch-coordination --tail=50
```

#### 2. Access the Cluster

```bash
# Get external IP
EXTERNAL_IP=$(kubectl get svc quidditch-coordination -n quidditch -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

echo "Quidditch endpoint: http://$EXTERNAL_IP:9200"

# Test connectivity
curl http://$EXTERNAL_IP:9200/_cluster/health
```

#### 3. Create First Index

```bash
# Create index with 5 shards
curl -X PUT "http://$EXTERNAL_IP:9200/products" \
  -H 'Content-Type: application/json' \
  -d '{
    "settings": {
      "number_of_shards": 5,
      "number_of_replicas": 1
    },
    "mappings": {
      "properties": {
        "name": {"type": "text"},
        "price": {"type": "float"},
        "category": {"type": "keyword"}
      }
    }
  }'
```

#### 4. Index Documents

```bash
# Index a document
curl -X PUT "http://$EXTERNAL_IP:9200/products/_doc/1" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Quidditch Search Engine",
    "price": 99.99,
    "category": "software"
  }'

# Bulk index
curl -X POST "http://$EXTERNAL_IP:9200/products/_bulk" \
  -H 'Content-Type: application/x-ndjson' \
  --data-binary @products.ndjson
```

#### 5. Search

```bash
# Simple search
curl -X GET "http://$EXTERNAL_IP:9200/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {
      "match": {
        "name": "search"
      }
    }
  }'

# Aggregations
curl -X GET "http://$EXTERNAL_IP:9200/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "size": 0,
    "aggs": {
      "categories": {
        "terms": {"field": "category"}
      },
      "price_stats": {
        "stats": {"field": "price"}
      }
    }
  }'
```

### Monitoring

#### 1. Pod Logs

```bash
# Coordination node logs
kubectl logs -f deployment/quidditch-coordination -n quidditch

# Data node logs
kubectl logs -f quidditch-data-0 -n quidditch

# Master/Operator logs (Raft mode)
kubectl logs -f quidditch-master-0 -n quidditch

# Operator logs (K8S mode)
kubectl logs -f deployment/quidditch-operator -n quidditch
```

#### 2. Metrics

```bash
# Pod resource usage
kubectl top pods -n quidditch

# Node resource usage
kubectl top nodes

# HPA status
kubectl get hpa quidditch-coordination -n quidditch
```

#### 3. Events

```bash
# Cluster events
kubectl get events -n quidditch --sort-by='.lastTimestamp'

# Watch events
kubectl get events -n quidditch --watch
```

### Troubleshooting

#### Pods Not Starting

```bash
# Check pod status
kubectl describe pod <pod-name> -n quidditch

# Check events
kubectl get events -n quidditch | grep <pod-name>

# Check logs
kubectl logs <pod-name> -n quidditch --previous
```

#### PVC Issues

```bash
# Check PVC status
kubectl get pvc -n quidditch

# Describe PVC
kubectl describe pvc data-quidditch-data-0 -n quidditch

# Check storage class
kubectl get storageclass
```

#### LoadBalancer Pending

```bash
# Check service
kubectl describe svc quidditch-coordination -n quidditch

# If using minikube/kind, use port-forward
kubectl port-forward -n quidditch svc/quidditch-coordination 9200:9200
```

#### Raft Issues (Raft Mode)

```bash
# Check Raft cluster status
kubectl exec -it quidditch-master-0 -n quidditch -- /bin/sh -c "curl localhost:9400/raft/stats"

# Check Raft logs
kubectl logs quidditch-master-0 -n quidditch | grep -i raft

# Check peer connectivity
kubectl exec -it quidditch-master-0 -n quidditch -- nc -zv quidditch-master-1.quidditch-master 9300
```

#### Operator Issues (K8S Mode)

```bash
# Check operator status
kubectl get deployment quidditch-operator -n quidditch

# Check leader election
kubectl get lease -n quidditch

# Check CRDs
kubectl get crd | grep quidditch

# Check operator logs
kubectl logs deployment/quidditch-operator -n quidditch
```

### Uninstall

```bash
# Uninstall cluster (prompts for confirmation)
./deploy-k8s.sh --uninstall

# Force delete with PVCs
./deploy-k8s.sh --uninstall
# When prompted, type "yes" to delete data

# Manual cleanup if needed
kubectl delete namespace quidditch
```

### Advanced Usage

#### Custom Resource Limits

Edit the script and modify resource values in `get_profile_config()` function, or set environment variables:

```bash
# Custom dev profile
MASTER_REPLICAS=1 \
COORDINATION_REPLICAS=2 \
DATA_REPLICAS=2 \
DATA_CPU_REQUEST=4 \
DATA_MEMORY_REQUEST=16Gi \
./deploy-k8s.sh --profile dev
```

#### Custom Storage Class

```bash
# Check available storage classes
kubectl get storageclass

# Use specific storage class
# Edit script: STORAGE_CLASS="fast-ssd"
```

#### Node Affinity

Add node selectors to the generated manifests:

```yaml
spec:
  template:
    spec:
      nodeSelector:
        workload-type: search-engine
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: app.kubernetes.io/component
                operator: In
                values:
                - data
            topologyKey: kubernetes.io/hostname
```

### Integration with CI/CD

#### GitLab CI

```yaml
deploy:
  stage: deploy
  image: bitnami/kubectl:latest
  script:
    - kubectl config set-cluster k8s --server="$KUBE_URL" --insecure-skip-tls-verify=true
    - kubectl config set-credentials admin --token="$KUBE_TOKEN"
    - kubectl config set-context default --cluster=k8s --user=admin
    - kubectl config use-context default
    - ./scripts/deploy-k8s.sh --mode k8s --profile $CI_ENVIRONMENT_NAME --no-wait
  only:
    - main
```

#### GitHub Actions

```yaml
name: Deploy Quidditch
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: azure/setup-kubectl@v3
      - uses: azure/k8s-set-context@v3
        with:
          kubeconfig: ${{ secrets.KUBE_CONFIG }}
      - name: Deploy
        run: |
          ./scripts/deploy-k8s.sh --mode k8s --profile staging --no-wait
```

### Best Practices

1. **Use namespaces** to isolate environments
2. **Enable RBAC** for production deployments
3. **Use PodDisruptionBudgets** for HA
4. **Configure resource quotas** for namespaces
5. **Enable monitoring** (Prometheus/Grafana)
6. **Set up alerting** for critical components
7. **Regular backups** of persistent data
8. **Use network policies** for security
9. **Enable pod security policies**
10. **Regular updates** and security patches

---

## Related Documentation

- [Dual-Mode Control Plane](../docs/DUAL_MODE_CONTROL_PLANE.md)
- [Kubernetes Deployment Guide](../docs/KUBERNETES_DEPLOYMENT_GUIDE.md)
- [K8S-Native Deep Dive](../docs/K8S_NATIVE_DEEP_DIVE.md)
- [Architecture Overview](../QUIDDITCH_ARCHITECTURE.md)

---

## Support

For issues or questions:
- GitHub Issues: https://github.com/yourorg/quidditch/issues
- Documentation: https://docs.quidditch.io
- Community: https://community.quidditch.io
