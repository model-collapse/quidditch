#!/bin/bash
#
# Start Quidditch Cluster for Testing/Benchmarking
#

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Directories
PROJECT_ROOT="/home/ubuntu/quidditch"
BIN_DIR="$PROJECT_ROOT/bin"
DATA_DIR="${DATA_DIR:-/tmp/quidditch-test}"
LOG_DIR="$DATA_DIR/logs"

echo "=========================================="
echo " Starting Quidditch Cluster"
echo "=========================================="
echo ""

# Check if cluster is already running
if pgrep -f "quidditch-master" > /dev/null; then
    echo -e "${YELLOW}⚠ Cluster appears to be already running${NC}"
    echo "  Use stop_cluster.sh to stop it first"
    exit 1
fi

# Create directories
mkdir -p "$DATA_DIR"/{master,coordination,data,logs,config}

# Create config files
cat > "$DATA_DIR/config/master.yaml" <<EOF
node_id: "test-master-1"
bind_addr: "127.0.0.1"
raft_port: 9301
grpc_port: 9400
data_dir: "$DATA_DIR/master"
log_level: "info"
metrics_port: 9401

raft:
  heartbeat_timeout: "1s"
  election_timeout: "3s"
  snapshot_interval: "5m"
  snapshot_threshold: 8192
  trailing_logs: 10240

cluster:
  name: "quidditch-test"
  initial_master_nodes:
    - "test-master-1"
EOF

cat > "$DATA_DIR/config/data.yaml" <<EOF
node_id: "test-data-1"
bind_addr: "127.0.0.1"
grpc_port: 9500
data_dir: "$DATA_DIR/data"
master_addr: "127.0.0.1:9400"
log_level: "info"
metrics_port: 9501
storage_tier: "hot"
max_shards: 100
simd_enabled: true
EOF

cat > "$DATA_DIR/config/coordination.yaml" <<EOF
node_id: "test-coord-1"
bind_addr: "127.0.0.1"
rest_port: 9200
grpc_port: 9600
data_dir: "$DATA_DIR/coordination"
master_addr: "127.0.0.1:9400"
log_level: "info"
metrics_port: 9601

query:
  max_result_window: 10000
  default_size: 10
  max_timeout: "60s"
EOF

# Start master node
echo "Starting Master Node..."
$BIN_DIR/quidditch-master --config "$DATA_DIR/config/master.yaml" \
    > "$LOG_DIR/master.log" 2>&1 &
MASTER_PID=$!
echo "  Master PID: $MASTER_PID"
sleep 4

if ! ps -p $MASTER_PID > /dev/null 2>&1; then
    echo -e "${RED}✗ Master node failed to start${NC}"
    echo "Logs:"
    tail -20 "$LOG_DIR/master.log"
    exit 1
fi
echo -e "${GREEN}✓ Master node started${NC}"
echo ""

# Start data node
echo "Starting Data Node..."
export LD_LIBRARY_PATH="$PROJECT_ROOT/pkg/data/diagon/build:$LD_LIBRARY_PATH"
$BIN_DIR/quidditch-data --config "$DATA_DIR/config/data.yaml" \
    > "$LOG_DIR/data.log" 2>&1 &
DATA_PID=$!
echo "  Data Node PID: $DATA_PID"
sleep 4

if ! ps -p $DATA_PID > /dev/null 2>&1; then
    echo -e "${RED}✗ Data node failed to start${NC}"
    echo "Logs:"
    tail -20 "$LOG_DIR/data.log"
    kill $MASTER_PID 2>/dev/null || true
    exit 1
fi
echo -e "${GREEN}✓ Data node started${NC}"
echo ""

# Start coordination node
echo "Starting Coordination Node..."
$BIN_DIR/quidditch-coordination --config "$DATA_DIR/config/coordination.yaml" \
    > "$LOG_DIR/coordination.log" 2>&1 &
COORD_PID=$!
echo "  Coordination PID: $COORD_PID"
sleep 4

if ! ps -p $COORD_PID > /dev/null 2>&1; then
    echo -e "${RED}✗ Coordination node failed to start${NC}"
    echo "Logs:"
    tail -20 "$LOG_DIR/coordination.log"
    kill $MASTER_PID $DATA_PID 2>/dev/null || true
    exit 1
fi
echo -e "${GREEN}✓ Coordination node started${NC}"
echo ""

# Verify cluster health
echo "Verifying cluster health..."
sleep 2
HEALTH=$(curl -s --max-time 5 http://localhost:9200/_cluster/health 2>&1 || echo "{}")

if echo "$HEALTH" | grep -q "curl:"; then
    echo -e "${RED}✗ Cannot connect to coordination node${NC}"
    kill $MASTER_PID $DATA_PID $COORD_PID 2>/dev/null || true
    exit 1
fi

echo -e "${GREEN}✓ Cluster is healthy${NC}"
echo ""

# Save PIDs
echo "$MASTER_PID" > "$DATA_DIR/master.pid"
echo "$DATA_PID" > "$DATA_DIR/data.pid"
echo "$COORD_PID" > "$DATA_DIR/coordination.pid"

echo "=========================================="
echo -e "${GREEN}✓ CLUSTER STARTED SUCCESSFULLY${NC}"
echo "=========================================="
echo ""
echo "Cluster PIDs:"
echo "  Master:       $MASTER_PID"
echo "  Data:         $DATA_PID"
echo "  Coordination: $COORD_PID"
echo ""
echo "Endpoints:"
echo "  REST API:     http://localhost:9200"
echo "  Master gRPC:  localhost:9400"
echo "  Data gRPC:    localhost:9500"
echo ""
echo "Logs: $LOG_DIR/"
echo "PIDs saved to: $DATA_DIR/*.pid"
echo ""
echo "To stop the cluster, run: test/stop_cluster.sh"
echo ""

exit 0
