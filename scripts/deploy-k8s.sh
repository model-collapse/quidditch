#!/usr/bin/env bash

#############################################################################
# Quidditch Kubernetes Deployment Script
#
# A comprehensive deployment script for Quidditch search engine that supports:
# - Both control plane modes (Raft and K8S-native)
# - Auto-detection and validation
# - Health checks and rollout status
# - Production and development configurations
#
# Usage:
#   ./deploy-k8s.sh [OPTIONS]
#
# Options:
#   --mode MODE           Control plane mode: raft, k8s, auto (default: auto)
#   --namespace NS        Kubernetes namespace (default: quidditch)
#   --profile PROFILE     Deployment profile: dev, staging, prod (default: dev)
#   --cluster-name NAME   Cluster name (default: quidditch)
#   --wait                Wait for all pods to be ready (default: true)
#   --dry-run             Print manifests without applying
#   --upgrade             Upgrade existing installation
#   --uninstall           Uninstall Quidditch
#   --help                Show this help message
#
# Examples:
#   # Deploy development cluster with auto-detection
#   ./deploy-k8s.sh --profile dev
#
#   # Deploy production cluster with Raft mode
#   ./deploy-k8s.sh --mode raft --profile prod --namespace quidditch-prod
#
#   # Deploy with K8S-native operator
#   ./deploy-k8s.sh --mode k8s --profile staging
#
#   # Dry run to see manifests
#   ./deploy-k8s.sh --mode raft --dry-run
#
#############################################################################

set -euo pipefail

#############################################################################
# Colors and Formatting
#############################################################################

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

#############################################################################
# Configuration
#############################################################################

# Default values
MODE="${MODE:-auto}"
NAMESPACE="${NAMESPACE:-quidditch}"
PROFILE="${PROFILE:-dev}"
CLUSTER_NAME="${CLUSTER_NAME:-quidditch}"
WAIT="${WAIT:-true}"
DRY_RUN="${DRY_RUN:-false}"
UPGRADE="${UPGRADE:-false}"
UNINSTALL="${UNINSTALL:-false}"

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MANIFESTS_DIR="$PROJECT_ROOT/deployments/kubernetes"

#############################################################################
# Helper Functions
#############################################################################

log() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

success() {
    echo -e "${GREEN}✓${NC} $*"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

fatal() {
    error "$*"
    exit 1
}

section() {
    echo ""
    echo -e "${CYAN}${BOLD}===================================================${NC}"
    echo -e "${CYAN}${BOLD} $*${NC}"
    echo -e "${CYAN}${BOLD}===================================================${NC}"
    echo ""
}

#############################################################################
# Usage and Help
#############################################################################

usage() {
    cat << EOF
${BOLD}Quidditch Kubernetes Deployment Script${NC}

${BOLD}USAGE:${NC}
    $0 [OPTIONS]

${BOLD}OPTIONS:${NC}
    --mode MODE           Control plane mode: raft, k8s, auto (default: auto)
    --namespace NS        Kubernetes namespace (default: quidditch)
    --profile PROFILE     Deployment profile: dev, staging, prod (default: dev)
    --cluster-name NAME   Cluster name (default: quidditch)
    --wait                Wait for all pods to be ready (default: true)
    --dry-run             Print manifests without applying
    --upgrade             Upgrade existing installation
    --uninstall           Uninstall Quidditch
    --help                Show this help message

${BOLD}CONTROL PLANE MODES:${NC}
    ${GREEN}raft${NC}    - Traditional master nodes with Raft consensus
              Works on: K8S, VMs, bare metal
              Latency: 2-5ms | Cost: ~\$162/month

    ${GREEN}k8s${NC}     - K8S-native operator with CRDs
              Works on: K8S only
              Latency: 5-20ms | Cost: ~\$40/month

    ${GREEN}auto${NC}    - Auto-detect environment (default)
              K8S → uses k8s mode
              Non-K8S → uses raft mode

${BOLD}DEPLOYMENT PROFILES:${NC}
    ${GREEN}dev${NC}     - Development (1 master, 1 coord, 1 data)
    ${GREEN}staging${NC} - Staging (3 masters, 2 coords, 3 data nodes)
    ${GREEN}prod${NC}    - Production (3/5 masters, 3+ coords, 5+ data nodes)

${BOLD}EXAMPLES:${NC}
    # Deploy development cluster
    $0 --profile dev

    # Deploy production with Raft mode
    $0 --mode raft --profile prod --namespace quidditch-prod

    # Deploy staging with K8S-native operator
    $0 --mode k8s --profile staging

    # Dry run to preview manifests
    $0 --mode raft --dry-run

    # Upgrade existing installation
    $0 --upgrade

    # Uninstall
    $0 --uninstall

EOF
}

#############################################################################
# Parse Arguments
#############################################################################

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --mode)
                MODE="$2"
                shift 2
                ;;
            --namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            --profile)
                PROFILE="$2"
                shift 2
                ;;
            --cluster-name)
                CLUSTER_NAME="$2"
                shift 2
                ;;
            --wait)
                WAIT=true
                shift
                ;;
            --no-wait)
                WAIT=false
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --upgrade)
                UPGRADE=true
                shift
                ;;
            --uninstall)
                UNINSTALL=true
                shift
                ;;
            --help|-h)
                usage
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
}

#############################################################################
# Validation
#############################################################################

validate_environment() {
    section "Validating Environment"

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        fatal "kubectl not found. Please install kubectl first."
    fi
    success "kubectl found: $(kubectl version --client --short 2>/dev/null || kubectl version --client)"

    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        fatal "Cannot connect to Kubernetes cluster. Please configure kubectl."
    fi
    success "Connected to Kubernetes cluster: $(kubectl config current-context)"

    # Validate mode
    if [[ ! "$MODE" =~ ^(raft|k8s|auto)$ ]]; then
        fatal "Invalid mode: $MODE. Must be one of: raft, k8s, auto"
    fi
    success "Control plane mode: $MODE"

    # Validate profile
    if [[ ! "$PROFILE" =~ ^(dev|staging|prod)$ ]]; then
        fatal "Invalid profile: $PROFILE. Must be one of: dev, staging, prod"
    fi
    success "Deployment profile: $PROFILE"

    # Check if running in K8S (for auto mode)
    if [[ "$MODE" == "auto" ]]; then
        if kubectl get nodes &> /dev/null; then
            MODE="k8s"
            log "Auto-detected K8S environment, using K8S-native mode"
        else
            MODE="raft"
            log "Auto-detected non-K8S environment, using Raft mode"
        fi
    fi

    success "Environment validation passed"
}

validate_resources() {
    section "Validating Cluster Resources"

    # Get cluster resources
    local nodes_count=$(kubectl get nodes --no-headers 2>/dev/null | wc -l || echo 0)

    if [[ $nodes_count -eq 0 ]]; then
        fatal "No nodes found in cluster"
    fi
    success "Found $nodes_count nodes in cluster"

    # Check storage classes
    local storage_classes=$(kubectl get storageclass --no-headers 2>/dev/null | wc -l || echo 0)
    if [[ $storage_classes -eq 0 ]]; then
        warn "No storage classes found. You may need to configure persistent storage."
    else
        success "Found $storage_classes storage class(es)"
        kubectl get storageclass -o custom-columns=NAME:.metadata.name,DEFAULT:.metadata.annotations."storageclass\.kubernetes\.io/is-default-class" | head -5
    fi

    # Minimum node requirements by profile
    case $PROFILE in
        dev)
            local min_nodes=1
            ;;
        staging)
            local min_nodes=3
            ;;
        prod)
            local min_nodes=5
            ;;
    esac

    if [[ $nodes_count -lt $min_nodes ]]; then
        warn "Profile '$PROFILE' recommends at least $min_nodes nodes, but only $nodes_count found"
        warn "Cluster may not have sufficient resources"
    fi
}

#############################################################################
# Configuration Generation
#############################################################################

get_profile_config() {
    local profile=$1

    case $profile in
        dev)
            cat << EOF
MASTER_REPLICAS=1
COORDINATION_REPLICAS=1
DATA_REPLICAS=1
MASTER_CPU_REQUEST="1"
MASTER_MEMORY_REQUEST="2Gi"
MASTER_STORAGE="20Gi"
COORDINATION_CPU_REQUEST="1"
COORDINATION_MEMORY_REQUEST="2Gi"
DATA_CPU_REQUEST="2"
DATA_MEMORY_REQUEST="8Gi"
DATA_STORAGE="50Gi"
STORAGE_CLASS="standard"
EOF
            ;;
        staging)
            cat << EOF
MASTER_REPLICAS=3
COORDINATION_REPLICAS=2
DATA_REPLICAS=3
MASTER_CPU_REQUEST="2"
MASTER_MEMORY_REQUEST="4Gi"
MASTER_STORAGE="50Gi"
COORDINATION_CPU_REQUEST="2"
COORDINATION_MEMORY_REQUEST="4Gi"
DATA_CPU_REQUEST="4"
DATA_MEMORY_REQUEST="16Gi"
DATA_STORAGE="200Gi"
STORAGE_CLASS="gp3"
EOF
            ;;
        prod)
            cat << EOF
MASTER_REPLICAS=3
COORDINATION_REPLICAS=3
DATA_REPLICAS=5
MASTER_CPU_REQUEST="2"
MASTER_MEMORY_REQUEST="4Gi"
MASTER_STORAGE="50Gi"
COORDINATION_CPU_REQUEST="2"
COORDINATION_MEMORY_REQUEST="4Gi"
DATA_CPU_REQUEST="8"
DATA_MEMORY_REQUEST="16Gi"
DATA_STORAGE="500Gi"
STORAGE_CLASS="gp3"
EOF
            ;;
    esac
}

#############################################################################
# Manifest Generation
#############################################################################

generate_namespace() {
    cat << EOF
apiVersion: v1
kind: Namespace
metadata:
  name: $NAMESPACE
  labels:
    app.kubernetes.io/name: quidditch
    app.kubernetes.io/instance: $CLUSTER_NAME
    app.kubernetes.io/component: namespace
EOF
}

generate_raft_masters() {
    cat << EOF
---
apiVersion: v1
kind: Service
metadata:
  name: ${CLUSTER_NAME}-master
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/name: quidditch
    app.kubernetes.io/instance: $CLUSTER_NAME
    app.kubernetes.io/component: master
spec:
  clusterIP: None  # Headless service
  selector:
    app.kubernetes.io/name: quidditch
    app.kubernetes.io/instance: $CLUSTER_NAME
    app.kubernetes.io/component: master
  ports:
  - name: raft
    port: 9300
    targetPort: 9300
  - name: grpc
    port: 9301
    targetPort: 9301
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: ${CLUSTER_NAME}-master
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/name: quidditch
    app.kubernetes.io/instance: $CLUSTER_NAME
    app.kubernetes.io/component: master
spec:
  serviceName: ${CLUSTER_NAME}-master
  replicas: $MASTER_REPLICAS
  selector:
    matchLabels:
      app.kubernetes.io/name: quidditch
      app.kubernetes.io/instance: $CLUSTER_NAME
      app.kubernetes.io/component: master
  template:
    metadata:
      labels:
        app.kubernetes.io/name: quidditch
        app.kubernetes.io/instance: $CLUSTER_NAME
        app.kubernetes.io/component: master
    spec:
      containers:
      - name: master
        image: quidditch/master:latest
        imagePullPolicy: IfNotPresent
        env:
        - name: CONTROL_PLANE_MODE
          value: "raft"
        - name: NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: CLUSTER_NAME
          value: "$CLUSTER_NAME"
        - name: RAFT_PORT
          value: "9300"
        - name: GRPC_PORT
          value: "9301"
        - name: DATA_DIR
          value: "/var/lib/quidditch/raft"
        ports:
        - name: raft
          containerPort: 9300
        - name: grpc
          containerPort: 9301
        - name: metrics
          containerPort: 9400
        volumeMounts:
        - name: data
          mountPath: /var/lib/quidditch
        resources:
          requests:
            memory: "$MASTER_MEMORY_REQUEST"
            cpu: "$MASTER_CPU_REQUEST"
          limits:
            memory: "$(echo $MASTER_MEMORY_REQUEST | sed 's/Gi$//')Gi"
            cpu: "$((${MASTER_CPU_REQUEST} * 2))"
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
      storageClassName: "$STORAGE_CLASS"
      resources:
        requests:
          storage: "$MASTER_STORAGE"
EOF
}

generate_k8s_operator() {
    cat << EOF
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${CLUSTER_NAME}-operator
  namespace: $NAMESPACE
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ${CLUSTER_NAME}-operator
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
  name: ${CLUSTER_NAME}-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ${CLUSTER_NAME}-operator
subjects:
- kind: ServiceAccount
  name: ${CLUSTER_NAME}-operator
  namespace: $NAMESPACE
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${CLUSTER_NAME}-operator
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/name: quidditch
    app.kubernetes.io/instance: $CLUSTER_NAME
    app.kubernetes.io/component: operator
spec:
  replicas: 3  # HA with leader election
  selector:
    matchLabels:
      app.kubernetes.io/name: quidditch
      app.kubernetes.io/instance: $CLUSTER_NAME
      app.kubernetes.io/component: operator
  template:
    metadata:
      labels:
        app.kubernetes.io/name: quidditch
        app.kubernetes.io/instance: $CLUSTER_NAME
        app.kubernetes.io/component: operator
    spec:
      serviceAccountName: ${CLUSTER_NAME}-operator
      containers:
      - name: operator
        image: quidditch/operator:latest
        imagePullPolicy: IfNotPresent
        env:
        - name: CONTROL_PLANE_MODE
          value: "k8s"
        - name: NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: CLUSTER_NAME
          value: "$CLUSTER_NAME"
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
EOF
}

generate_coordination() {
    cat << EOF
---
apiVersion: v1
kind: Service
metadata:
  name: ${CLUSTER_NAME}-coordination
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/name: quidditch
    app.kubernetes.io/instance: $CLUSTER_NAME
    app.kubernetes.io/component: coordination
spec:
  type: LoadBalancer
  selector:
    app.kubernetes.io/name: quidditch
    app.kubernetes.io/instance: $CLUSTER_NAME
    app.kubernetes.io/component: coordination
  ports:
  - name: http
    port: 9200
    targetPort: 9200
  - name: grpc
    port: 9302
    targetPort: 9302
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${CLUSTER_NAME}-coordination
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/name: quidditch
    app.kubernetes.io/instance: $CLUSTER_NAME
    app.kubernetes.io/component: coordination
spec:
  replicas: $COORDINATION_REPLICAS
  selector:
    matchLabels:
      app.kubernetes.io/name: quidditch
      app.kubernetes.io/instance: $CLUSTER_NAME
      app.kubernetes.io/component: coordination
  template:
    metadata:
      labels:
        app.kubernetes.io/name: quidditch
        app.kubernetes.io/instance: $CLUSTER_NAME
        app.kubernetes.io/component: coordination
    spec:
      containers:
      - name: coordination
        image: quidditch/coordination:latest
        imagePullPolicy: IfNotPresent
        env:
        - name: CONTROL_PLANE_MODE
          value: "$MODE"
        - name: NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: CLUSTER_NAME
          value: "$CLUSTER_NAME"
        - name: NAMESPACE
          value: "$NAMESPACE"
        - name: HTTP_PORT
          value: "9200"
        - name: GRPC_PORT
          value: "9302"
        ports:
        - name: http
          containerPort: 9200
        - name: grpc
          containerPort: 9302
        - name: metrics
          containerPort: 9401
        resources:
          requests:
            memory: "$COORDINATION_MEMORY_REQUEST"
            cpu: "$COORDINATION_CPU_REQUEST"
          limits:
            memory: "$(echo $COORDINATION_MEMORY_REQUEST | sed 's/Gi$//')Gi"
            cpu: "$((${COORDINATION_CPU_REQUEST} * 2))"
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
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: ${CLUSTER_NAME}-coordination
  namespace: $NAMESPACE
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: ${CLUSTER_NAME}-coordination
  minReplicas: $COORDINATION_REPLICAS
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
EOF
}

generate_data() {
    cat << EOF
---
apiVersion: v1
kind: Service
metadata:
  name: ${CLUSTER_NAME}-data
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/name: quidditch
    app.kubernetes.io/instance: $CLUSTER_NAME
    app.kubernetes.io/component: data
spec:
  clusterIP: None  # Headless service
  selector:
    app.kubernetes.io/name: quidditch
    app.kubernetes.io/instance: $CLUSTER_NAME
    app.kubernetes.io/component: data
  ports:
  - name: grpc
    port: 9303
    targetPort: 9303
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: ${CLUSTER_NAME}-data
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/name: quidditch
    app.kubernetes.io/instance: $CLUSTER_NAME
    app.kubernetes.io/component: data
spec:
  serviceName: ${CLUSTER_NAME}-data
  replicas: $DATA_REPLICAS
  selector:
    matchLabels:
      app.kubernetes.io/name: quidditch
      app.kubernetes.io/instance: $CLUSTER_NAME
      app.kubernetes.io/component: data
  template:
    metadata:
      labels:
        app.kubernetes.io/name: quidditch
        app.kubernetes.io/instance: $CLUSTER_NAME
        app.kubernetes.io/component: data
    spec:
      containers:
      - name: data
        image: quidditch/data:latest
        imagePullPolicy: IfNotPresent
        env:
        - name: CONTROL_PLANE_MODE
          value: "$MODE"
        - name: NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: CLUSTER_NAME
          value: "$CLUSTER_NAME"
        - name: NAMESPACE
          value: "$NAMESPACE"
        - name: GRPC_PORT
          value: "9303"
        - name: DATA_DIR
          value: "/var/lib/quidditch/data"
        ports:
        - name: grpc
          containerPort: 9303
        - name: metrics
          containerPort: 9402
        volumeMounts:
        - name: data
          mountPath: /var/lib/quidditch
        resources:
          requests:
            memory: "$DATA_MEMORY_REQUEST"
            cpu: "$DATA_CPU_REQUEST"
          limits:
            memory: "$(echo $DATA_MEMORY_REQUEST | sed 's/Gi$//')Gi"
            cpu: "$((${DATA_CPU_REQUEST} * 2))"
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
      storageClassName: "$STORAGE_CLASS"
      resources:
        requests:
          storage: "$DATA_STORAGE"
EOF
}

#############################################################################
# Deployment Functions
#############################################################################

deploy_control_plane() {
    section "Deploying Control Plane ($MODE mode)"

    if [[ "$MODE" == "raft" ]]; then
        log "Generating Raft master StatefulSet..."
        if [[ "$DRY_RUN" == "true" ]]; then
            generate_raft_masters
        else
            generate_raft_masters | kubectl apply -f -
            success "Raft masters deployed"
        fi
    else
        log "Generating K8S-native operator..."
        if [[ "$DRY_RUN" == "true" ]]; then
            generate_k8s_operator
        else
            generate_k8s_operator | kubectl apply -f -
            success "K8S operator deployed"
        fi
    fi
}

deploy_components() {
    section "Deploying Cluster Components"

    log "Deploying coordination nodes..."
    if [[ "$DRY_RUN" == "true" ]]; then
        generate_coordination
    else
        generate_coordination | kubectl apply -f -
        success "Coordination nodes deployed"
    fi

    log "Deploying data nodes..."
    if [[ "$DRY_RUN" == "true" ]]; then
        generate_data
    else
        generate_data | kubectl apply -f -
        success "Data nodes deployed"
    fi
}

wait_for_rollout() {
    if [[ "$WAIT" != "true" ]] || [[ "$DRY_RUN" == "true" ]]; then
        return
    fi

    section "Waiting for Rollout"

    local timeout=600  # 10 minutes

    if [[ "$MODE" == "raft" ]]; then
        log "Waiting for master StatefulSet..."
        if kubectl rollout status statefulset/${CLUSTER_NAME}-master -n $NAMESPACE --timeout=${timeout}s; then
            success "Masters are ready"
        else
            warn "Masters did not become ready within timeout"
        fi
    else
        log "Waiting for operator Deployment..."
        if kubectl rollout status deployment/${CLUSTER_NAME}-operator -n $NAMESPACE --timeout=${timeout}s; then
            success "Operator is ready"
        else
            warn "Operator did not become ready within timeout"
        fi
    fi

    log "Waiting for coordination Deployment..."
    if kubectl rollout status deployment/${CLUSTER_NAME}-coordination -n $NAMESPACE --timeout=${timeout}s; then
        success "Coordination nodes are ready"
    else
        warn "Coordination nodes did not become ready within timeout"
    fi

    log "Waiting for data StatefulSet..."
    if kubectl rollout status statefulset/${CLUSTER_NAME}-data -n $NAMESPACE --timeout=${timeout}s; then
        success "Data nodes are ready"
    else
        warn "Data nodes did not become ready within timeout"
    fi
}

verify_deployment() {
    if [[ "$DRY_RUN" == "true" ]]; then
        return
    fi

    section "Verifying Deployment"

    # Check pods
    log "Checking pod status..."
    kubectl get pods -n $NAMESPACE -l app.kubernetes.io/instance=$CLUSTER_NAME

    local total_pods=$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/instance=$CLUSTER_NAME --no-headers | wc -l)
    local running_pods=$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/instance=$CLUSTER_NAME --field-selector=status.phase=Running --no-headers | wc -l)

    echo ""
    if [[ $running_pods -eq $total_pods ]] && [[ $total_pods -gt 0 ]]; then
        success "All $total_pods pods are running"
    else
        warn "$running_pods/$total_pods pods are running"
    fi

    # Check services
    log "Checking services..."
    kubectl get svc -n $NAMESPACE -l app.kubernetes.io/instance=$CLUSTER_NAME

    # Get coordination service endpoint
    local coord_svc="${CLUSTER_NAME}-coordination"
    local external_ip=$(kubectl get svc $coord_svc -n $NAMESPACE -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "pending")

    if [[ "$external_ip" != "pending" ]] && [[ -n "$external_ip" ]]; then
        success "LoadBalancer IP: $external_ip"
    else
        warn "LoadBalancer IP is pending (this may take a few minutes)"
    fi
}

#############################################################################
# Uninstall Function
#############################################################################

uninstall() {
    section "Uninstalling Quidditch"

    warn "This will delete all Quidditch resources in namespace: $NAMESPACE"
    read -p "Are you sure? (yes/no): " confirm

    if [[ "$confirm" != "yes" ]]; then
        log "Uninstall cancelled"
        exit 0
    fi

    log "Deleting resources..."

    # Delete in reverse order
    kubectl delete statefulset ${CLUSTER_NAME}-data -n $NAMESPACE --ignore-not-found=true
    kubectl delete deployment ${CLUSTER_NAME}-coordination -n $NAMESPACE --ignore-not-found=true
    kubectl delete hpa ${CLUSTER_NAME}-coordination -n $NAMESPACE --ignore-not-found=true

    if [[ "$MODE" == "raft" ]]; then
        kubectl delete statefulset ${CLUSTER_NAME}-master -n $NAMESPACE --ignore-not-found=true
    else
        kubectl delete deployment ${CLUSTER_NAME}-operator -n $NAMESPACE --ignore-not-found=true
        kubectl delete clusterrolebinding ${CLUSTER_NAME}-operator --ignore-not-found=true
        kubectl delete clusterrole ${CLUSTER_NAME}-operator --ignore-not-found=true
        kubectl delete serviceaccount ${CLUSTER_NAME}-operator -n $NAMESPACE --ignore-not-found=true
    fi

    kubectl delete service ${CLUSTER_NAME}-data -n $NAMESPACE --ignore-not-found=true
    kubectl delete service ${CLUSTER_NAME}-coordination -n $NAMESPACE --ignore-not-found=true
    kubectl delete service ${CLUSTER_NAME}-master -n $NAMESPACE --ignore-not-found=true

    # Delete PVCs
    read -p "Delete persistent volumes? This will delete all data! (yes/no): " delete_pvcs
    if [[ "$delete_pvcs" == "yes" ]]; then
        kubectl delete pvc -n $NAMESPACE -l app.kubernetes.io/instance=$CLUSTER_NAME
        success "Persistent volumes deleted"
    else
        warn "Persistent volumes retained"
    fi

    success "Uninstall complete"
}

#############################################################################
# Main Deployment Flow
#############################################################################

main() {
    parse_args "$@"

    # Handle uninstall
    if [[ "$UNINSTALL" == "true" ]]; then
        uninstall
        exit 0
    fi

    # Print banner
    cat << "EOF"
    ___       _     _     _ _ _       _
   / _ \ _  _(_)_ _| |_ _| (_) |_ ___| |__
  | (_) | || | | '_| |  _  | |  _/ __| '_ \
   \__\_\\_,_|_|_| |_|\__,_|_|\__\___|_.__/

  Distributed Search Engine - K8S Deployment
EOF
    echo ""

    # Validation
    validate_environment
    validate_resources

    # Load profile configuration
    eval "$(get_profile_config $PROFILE)"

    # Print configuration summary
    section "Deployment Configuration"
    cat << EOF
${BOLD}Cluster Configuration:${NC}
  Name:               $CLUSTER_NAME
  Namespace:          $NAMESPACE
  Profile:            $PROFILE
  Control Plane:      $MODE

${BOLD}Resource Configuration:${NC}
  Master Replicas:    $MASTER_REPLICAS
  Coord Replicas:     $COORDINATION_REPLICAS (up to 20 with HPA)
  Data Replicas:      $DATA_REPLICAS
  Storage Class:      $STORAGE_CLASS

${BOLD}Master Resources:${NC}
  CPU:                $MASTER_CPU_REQUEST cores
  Memory:             $MASTER_MEMORY_REQUEST
  Storage:            $MASTER_STORAGE per pod

${BOLD}Coordination Resources:${NC}
  CPU:                $COORDINATION_CPU_REQUEST cores
  Memory:             $COORDINATION_MEMORY_REQUEST

${BOLD}Data Node Resources:${NC}
  CPU:                $DATA_CPU_REQUEST cores
  Memory:             $DATA_MEMORY_REQUEST
  Storage:            $DATA_STORAGE per pod
EOF

    if [[ "$DRY_RUN" == "true" ]]; then
        warn "Dry run mode - printing manifests only"
    fi

    echo ""
    read -p "Proceed with deployment? (yes/no): " proceed
    if [[ "$proceed" != "yes" ]]; then
        log "Deployment cancelled"
        exit 0
    fi

    # Create namespace
    if [[ "$DRY_RUN" == "true" ]]; then
        section "Generated Manifests"
        generate_namespace
    else
        section "Creating Namespace"
        generate_namespace | kubectl apply -f -
        success "Namespace created: $NAMESPACE"
    fi

    # Deploy
    deploy_control_plane
    deploy_components
    wait_for_rollout
    verify_deployment

    # Success message
    if [[ "$DRY_RUN" != "true" ]]; then
        section "Deployment Complete!"

        cat << EOF
${GREEN}✓${NC} Quidditch cluster deployed successfully!

${BOLD}Next Steps:${NC}

1. Check cluster status:
   kubectl get pods -n $NAMESPACE

2. Get coordination service endpoint:
   kubectl get svc ${CLUSTER_NAME}-coordination -n $NAMESPACE

3. Create an index:
   curl -X PUT "http://<EXTERNAL-IP>:9200/my-index" \\
     -H 'Content-Type: application/json' \\
     -d '{"settings": {"number_of_shards": 3}}'

4. Search the index:
   curl -X GET "http://<EXTERNAL-IP>:9200/my-index/_search" \\
     -H 'Content-Type: application/json' \\
     -d '{"query": {"match_all": {}}}'

${BOLD}Monitoring:${NC}
  kubectl logs -f deployment/${CLUSTER_NAME}-coordination -n $NAMESPACE
  kubectl top pods -n $NAMESPACE

${BOLD}Documentation:${NC}
  Control Plane:  docs/DUAL_MODE_CONTROL_PLANE.md
  K8S Deployment: docs/KUBERNETES_DEPLOYMENT_GUIDE.md

EOF
    fi
}

#############################################################################
# Execute
#############################################################################

main "$@"
