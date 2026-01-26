# Quidditch Kubernetes Quick Start

Get Quidditch up and running in Kubernetes in under 5 minutes.

---

## Prerequisites

- Kubernetes cluster (minikube, kind, EKS, GKE, AKS, etc.)
- kubectl configured and connected
- 4+ GB available RAM
- Storage provisioner configured

---

## One-Command Deploy

```bash
# Deploy with auto-detected control plane mode
cd quidditch
./scripts/deploy-k8s.sh --profile dev
```

That's it! The script will:
1. ✓ Validate your environment
2. ✓ Auto-detect control plane mode (K8S-native or Raft)
3. ✓ Deploy all components
4. ✓ Wait for pods to be ready
5. ✓ Show you the cluster endpoint

---

## Step-by-Step (5 Minutes)

### Step 1: Clone and Navigate (30 seconds)

```bash
git clone https://github.com/yourorg/quidditch.git
cd quidditch
```

### Step 2: Deploy (2 minutes)

```bash
./scripts/deploy-k8s.sh --profile dev
```

**Expected output:**
```
    ___       _     _     _ _ _       _
   / _ \ _  _(_)_ _| |_ _| (_) |_ ___| |__
  | (_) | || | | '_| |  _  | |  _/ __| '_ \
   \__\_\\_,_|_|_| |_|\__,_|_|\__\___|_.__/

  Distributed Search Engine - K8S Deployment

===================================================
 Validating Environment
===================================================

✓ kubectl found: v1.28.0
✓ Connected to Kubernetes cluster: minikube
✓ Control plane mode: k8s
✓ Deployment profile: dev
✓ Environment validation passed

...

===================================================
 Deployment Complete!
===================================================

✓ Quidditch cluster deployed successfully!
```

### Step 3: Get Endpoint (30 seconds)

```bash
# Get the coordination service endpoint
kubectl get svc quidditch-coordination -n quidditch

# Example output:
# NAME                      TYPE           CLUSTER-IP      EXTERNAL-IP     PORT(S)          AGE
# quidditch-coordination    LoadBalancer   10.96.123.45    192.168.1.100   9200:30123/TCP   2m
```

**For minikube/kind:**
```bash
# Use port-forward instead
kubectl port-forward -n quidditch svc/quidditch-coordination 9200:9200
```

### Step 4: Test the Cluster (1 minute)

```bash
# Set endpoint (replace with your EXTERNAL-IP or use localhost:9200 for port-forward)
ENDPOINT="http://192.168.1.100:9200"

# Check cluster health
curl $ENDPOINT/_cluster/health

# Create an index
curl -X PUT "$ENDPOINT/products" \
  -H 'Content-Type: application/json' \
  -d '{
    "settings": {
      "number_of_shards": 3,
      "number_of_replicas": 1
    }
  }'

# Index a document
curl -X PUT "$ENDPOINT/products/_doc/1" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Quidditch Search Engine",
    "price": 99.99,
    "category": "software"
  }'

# Search
curl -X GET "$ENDPOINT/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {
      "match": {
        "name": "search"
      }
    }
  }'
```

### Step 5: Explore (1 minute)

```bash
# View pods
kubectl get pods -n quidditch

# View logs
kubectl logs -f deployment/quidditch-coordination -n quidditch

# View resources
kubectl top pods -n quidditch
```

---

## Deployment Modes Explained

The script auto-detects the best control plane mode for your environment:

### K8S-Native Mode (Auto-Selected for K8S)

**What it is:**
- Uses Kubernetes Operator pattern
- Stores cluster state in K8S CRDs
- Leverages K8S etcd (Raft built-in)

**When used:**
- When deploying to Kubernetes
- `--mode auto` (default) on K8S cluster
- Explicitly: `--mode k8s`

**Components:**
```
Namespace: quidditch
  ├─ Deployment: quidditch-operator (3 pods)
  ├─ Deployment: quidditch-coordination (1+ pods)
  └─ StatefulSet: quidditch-data (1+ pods)
```

**Characteristics:**
- Cost: ~$40/month (AWS EKS)
- Latency: 5-20ms for cluster ops
- Storage: Stateless operator (no PVCs)
- kubectl integration: ✓ Native

### Raft Mode (Fallback for Non-K8S)

**What it is:**
- Traditional master nodes with Raft consensus
- Stores cluster state in master pods
- Independent control plane

**When used:**
- On bare metal or VMs without K8S
- Explicitly: `--mode raft`
- Multi-environment consistency

**Components:**
```
Namespace: quidditch
  ├─ StatefulSet: quidditch-master (3 pods + PVCs)
  ├─ Deployment: quidditch-coordination (1+ pods)
  └─ StatefulSet: quidditch-data (1+ pods)
```

**Characteristics:**
- Cost: ~$162/month (AWS EKS)
- Latency: 2-5ms for cluster ops
- Storage: 50GB per master (PVCs)
- Portability: ✓ Works everywhere

---

## Deployment Profiles

### Development (Default)

```bash
./scripts/deploy-k8s.sh --profile dev
```

**Resources:**
- 1 master/operator pod
- 1 coordination pod
- 1 data pod
- Minimal CPU/RAM

**Use for:** Testing, development, CI/CD

### Staging

```bash
./scripts/deploy-k8s.sh --profile staging
```

**Resources:**
- 3 master/operator pods
- 2 coordination pods
- 3 data pods
- Moderate CPU/RAM

**Use for:** Pre-production, integration testing

### Production

```bash
./scripts/deploy-k8s.sh --profile prod --namespace quidditch-prod
```

**Resources:**
- 3 master/operator pods
- 3+ coordination pods (auto-scales to 20)
- 5+ data pods
- High CPU/RAM

**Use for:** Production workloads, HA

---

## Common Tasks

### Check Status

```bash
kubectl get pods -n quidditch
kubectl get svc -n quidditch
kubectl top pods -n quidditch
```

### View Logs

```bash
# Coordination logs
kubectl logs -f deployment/quidditch-coordination -n quidditch

# Data node logs
kubectl logs -f quidditch-data-0 -n quidditch

# Operator logs (K8S-native mode)
kubectl logs -f deployment/quidditch-operator -n quidditch

# Master logs (Raft mode)
kubectl logs -f quidditch-master-0 -n quidditch
```

### Scale Coordination Nodes

```bash
# Manual scaling
kubectl scale deployment quidditch-coordination -n quidditch --replicas=5

# HPA (auto-scaling)
kubectl get hpa quidditch-coordination -n quidditch
```

### Scale Data Nodes

```bash
kubectl scale statefulset quidditch-data -n quidditch --replicas=10
```

### Upgrade

```bash
./scripts/deploy-k8s.sh --upgrade
```

### Uninstall

```bash
./scripts/deploy-k8s.sh --uninstall
```

---

## Local Development (minikube/kind)

### minikube

```bash
# Start minikube
minikube start --cpus=4 --memory=8192

# Deploy Quidditch
./scripts/deploy-k8s.sh --profile dev

# Access via port-forward
kubectl port-forward -n quidditch svc/quidditch-coordination 9200:9200

# Test
curl http://localhost:9200/_cluster/health
```

### kind (Kubernetes in Docker)

```bash
# Create cluster
kind create cluster --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
EOF

# Deploy Quidditch
./scripts/deploy-k8s.sh --profile dev

# Access via port-forward
kubectl port-forward -n quidditch svc/quidditch-coordination 9200:9200

# Test
curl http://localhost:9200/_cluster/health
```

---

## Cloud Deployment (5 minutes)

### AWS EKS

```bash
# Create EKS cluster (takes ~15 minutes)
eksctl create cluster \
  --name quidditch \
  --region us-west-2 \
  --nodes 3 \
  --node-type t3.large

# Deploy Quidditch
./scripts/deploy-k8s.sh --profile prod --namespace quidditch-prod

# Get LoadBalancer endpoint
kubectl get svc quidditch-coordination -n quidditch-prod
```

### Google GKE

```bash
# Create GKE cluster
gcloud container clusters create quidditch \
  --zone us-central1-a \
  --num-nodes 3 \
  --machine-type n1-standard-2

# Get credentials
gcloud container clusters get-credentials quidditch --zone us-central1-a

# Deploy Quidditch
./scripts/deploy-k8s.sh --profile prod --namespace quidditch-prod

# Get LoadBalancer endpoint
kubectl get svc quidditch-coordination -n quidditch-prod
```

### Azure AKS

```bash
# Create resource group
az group create --name quidditch-rg --location eastus

# Create AKS cluster
az aks create \
  --resource-group quidditch-rg \
  --name quidditch \
  --node-count 3 \
  --node-vm-size Standard_D2s_v3

# Get credentials
az aks get-credentials --resource-group quidditch-rg --name quidditch

# Deploy Quidditch
./scripts/deploy-k8s.sh --profile prod --namespace quidditch-prod

# Get LoadBalancer endpoint
kubectl get svc quidditch-coordination -n quidditch-prod
```

---

## Troubleshooting

### Pods Not Starting

```bash
# Check pod status
kubectl describe pod <pod-name> -n quidditch

# Check events
kubectl get events -n quidditch --sort-by='.lastTimestamp'

# Check logs
kubectl logs <pod-name> -n quidditch
```

### LoadBalancer Pending (Local)

```bash
# Use port-forward instead
kubectl port-forward -n quidditch svc/quidditch-coordination 9200:9200
```

### Out of Resources

```bash
# Check node resources
kubectl top nodes

# Reduce profile
./scripts/deploy-k8s.sh --profile dev --uninstall
./scripts/deploy-k8s.sh --profile dev
```

---

## Next Steps

1. **Index Data**: Start indexing your documents
2. **Configure**: Adjust settings for your workload
3. **Monitor**: Set up Prometheus and Grafana
4. **Scale**: Add more data nodes as needed
5. **Secure**: Enable authentication and TLS

---

## Resources

- **Full Documentation**: [docs/KUBERNETES_DEPLOYMENT_GUIDE.md](docs/KUBERNETES_DEPLOYMENT_GUIDE.md)
- **Control Plane Modes**: [docs/DUAL_MODE_CONTROL_PLANE.md](docs/DUAL_MODE_CONTROL_PLANE.md)
- **Architecture**: [QUIDDITCH_ARCHITECTURE.md](QUIDDITCH_ARCHITECTURE.md)
- **Script Details**: [scripts/README.md](scripts/README.md)

---

## Support

- **Issues**: https://github.com/yourorg/quidditch/issues
- **Discussions**: https://github.com/yourorg/quidditch/discussions
- **Docs**: https://docs.quidditch.io

---

**That's it! You're now running a production-ready distributed search engine.** 🎉
