#!/bin/bash
#
# Manual E2E Test - Start cluster and leave it running for inspection
#

set -e

PROJECT_ROOT="/home/ubuntu/quidditch"
DATA_DIR="/tmp/quidditch-manual"
LOG_DIR="$DATA_DIR/logs"

echo "Setting up directories..."
rm -rf "$DATA_DIR"
mkdir -p "$DATA_DIR"/{master,coordination,data,logs,config}

# Master config
cat > "$DATA_DIR/config/master.yaml" <<EOF
node_id: "manual-master-1"
bind_addr: "127.0.0.1"
raft_port: 9301
grpc_port: 9400
data_dir: "$DATA_DIR/master"
log_level: "info"
metrics_port: 9401
EOF

# Data config
cat > "$DATA_DIR/config/data.yaml" <<EOF
node_id: "manual-data-1"
bind_addr: "127.0.0.1"
grpc_port: 9500
data_dir: "$DATA_DIR/data"
master_addr: "127.0.0.1:9400"
log_level: "info"
metrics_port: 9501
simd_enabled: true
EOF

# Coordination config
cat > "$DATA_DIR/config/coordination.yaml" <<EOF
node_id: "manual-coord-1"
bind_addr: "127.0.0.1"
rest_port: 9200
grpc_port: 9600
data_dir: "$DATA_DIR/coordination"
master_addr: "127.0.0.1:9400"
log_level: "info"
metrics_port: 9601
EOF

echo ""
echo "Starting Master Node..."
$PROJECT_ROOT/bin/quidditch-master --config "$DATA_DIR/config/master.yaml" \
    > "$LOG_DIR/master.log" 2>&1 &
MASTER_PID=$!
echo "  PID: $MASTER_PID"

sleep 6
if ! ps -p $MASTER_PID > /dev/null; then
    echo "ERROR: Master failed to start!"
    echo "Logs:"
    cat "$LOG_DIR/master.log"
    exit 1
fi

echo "  ✓ Master running"
echo ""

echo "Starting Data Node..."
export LD_LIBRARY_PATH="$PROJECT_ROOT/pkg/data/diagon/build:$LD_LIBRARY_PATH"
$PROJECT_ROOT/bin/quidditch-data --config "$DATA_DIR/config/data.yaml" \
    > "$LOG_DIR/data.log" 2>&1 &
DATA_PID=$!
echo "  PID: $DATA_PID"

sleep 4
if ! ps -p $DATA_PID > /dev/null; then
    echo "ERROR: Data node failed to start!"
    echo "Logs:"
    cat "$LOG_DIR/data.log"
    kill $MASTER_PID 2>/dev/null || true
    exit 1
fi

echo "  ✓ Data node running"
echo ""

echo "Starting Coordination Node..."
$PROJECT_ROOT/bin/quidditch-coordination --config "$DATA_DIR/config/coordination.yaml" \
    > "$LOG_DIR/coordination.log" 2>&1 &
COORD_PID=$!
echo "  PID: $COORD_PID"

sleep 4
if ! ps -p $COORD_PID > /dev/null; then
    echo "ERROR: Coordination failed to start!"
    echo "Logs:"
    cat "$LOG_DIR/coordination.log"
    kill $MASTER_PID $DATA_PID 2>/dev/null || true
    exit 1
fi

echo "  ✓ Coordination running"
echo ""

echo "=========================================="
echo "Cluster is running!"
echo "=========================================="
echo ""
echo "PIDs:"
echo "  Master:       $MASTER_PID"
echo "  Data:         $DATA_PID"
echo "  Coordination: $COORD_PID"
echo ""
echo "Logs:"
echo "  Master:       $LOG_DIR/master.log"
echo "  Data:         $LOG_DIR/data.log"
echo "  Coordination: $LOG_DIR/coordination.log"
echo ""
echo "Testing cluster health..."
sleep 2

HEALTH=$(curl -s --max-time 3 http://localhost:9200/_cluster/health 2>&1 || echo "{}")
echo "Health: $HEALTH"
echo ""

echo "Cluster is ready! You can now test manually."
echo ""
echo "Example commands:"
echo "  curl http://localhost:9200/_cluster/health"
echo "  curl -X PUT http://localhost:9200/test_index"
echo "  curl -X PUT http://localhost:9200/test_index/_doc/1 -H 'Content-Type: application/json' -d '{\"title\":\"Test\"}'"
echo "  curl http://localhost:9200/test_index/_search"
echo ""
echo "To stop: pkill -f quidditch"
echo ""
