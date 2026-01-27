#!/bin/bash
# Quick cluster test - starts cluster and runs basic operations

set -e

PROJECT_ROOT="/home/ubuntu/quidditch"
DATA_DIR="/tmp/quidditch-quick"
LOG_DIR="$DATA_DIR/logs"

# Clean old processes
for port in 9301 9400 9500 9600 9200 9401 9501 9601; do
    lsof -ti:$port | xargs kill -9 2>/dev/null || true
done
sleep 2

# Setup
rm -rf "$DATA_DIR"
mkdir -p "$DATA_DIR"/{master,data,coordination,logs,config}

# Master config
cat > "$DATA_DIR/config/master.yaml" <<EOF
node_id: "quick-master-1"
bind_addr: "127.0.0.1"
raft_port: 9301
grpc_port: 9400
data_dir: "$DATA_DIR/master"
log_level: "debug"
metrics_port: 9401
EOF

# Data config
cat > "$DATA_DIR/config/data.yaml" <<EOF
node_id: "quick-data-1"
bind_addr: "127.0.0.1"
grpc_port: 9500
data_dir: "$DATA_DIR/data"
log_level: "debug"
metrics_port: 9501
master_addr: "127.0.0.1:9400"
simd_enabled: true
EOF

# Coordination config
cat > "$DATA_DIR/config/coordination.yaml" <<EOF
node_id: "quick-coord-1"
bind_addr: "127.0.0.1"
rest_port: 9200
grpc_port: 9600
data_dir: "$DATA_DIR/coordination"
log_level: "debug"
metrics_port: 9601
master_addr: "127.0.0.1:9400"
EOF

echo "Starting cluster..."
echo ""

# Start master
$PROJECT_ROOT/bin/quidditch-master --config "$DATA_DIR/config/master.yaml" \
    > "$LOG_DIR/master.log" 2>&1 &
MASTER_PID=$!
echo "Master started (PID: $MASTER_PID)"
sleep 5

# Start data node
export LD_LIBRARY_PATH="$PROJECT_ROOT/pkg/data/diagon/build:$PROJECT_ROOT/pkg/data/diagon/upstream/build/src/core"
$PROJECT_ROOT/bin/quidditch-data --config "$DATA_DIR/config/data.yaml" \
    > "$LOG_DIR/data.log" 2>&1 &
DATA_PID=$!
echo "Data node started (PID: $DATA_PID)"
sleep 5

# Start coordination
$PROJECT_ROOT/bin/quidditch-coordination --config "$DATA_DIR/config/coordination.yaml" \
    > "$LOG_DIR/coordination.log" 2>&1 &
COORD_PID=$!
echo "Coordination started (PID: $COORD_PID)"
sleep 5

echo ""
echo "Cluster running!"
echo "PIDs: Master=$MASTER_PID Data=$DATA_PID Coord=$COORD_PID"
echo ""
echo "Testing..."
echo ""

# Check health
echo "1. Cluster health:"
curl -s http://localhost:9200/_cluster/health | jq '.' || echo "  Health check failed"
echo ""

# Create index
echo "2. Creating index..."
CREATE_RESP=$(curl -s -X PUT http://localhost:9200/test_index \
    -H 'Content-Type: application/json' \
    -d '{
        "settings": {"number_of_shards": 1, "number_of_replicas": 0},
        "mappings": {"properties": {"title": {"type": "text"}}}
    }')
echo "  $CREATE_RESP"
echo ""
sleep 2

# Check cluster state
echo "3. Cluster state (indices):"
curl -s http://localhost:9200/_cluster/state | jq '.metadata.indices' || echo "  Failed"
echo ""

# Index document
echo "4. Indexing document..."
DOC_RESP=$(curl -s -X PUT http://localhost:9200/test_index/_doc/1 \
    -H 'Content-Type: application/json' \
    -d '{"title": "Test Document"}')
echo "  $DOC_RESP"
echo ""

echo "=========================================="
echo "Cluster is running. Check logs at:"
echo "  Master: $LOG_DIR/master.log"
echo "  Data:   $LOG_DIR/data.log"
echo "  Coord:  $LOG_DIR/coordination.log"
echo ""
echo "To stop: kill $MASTER_PID $DATA_PID $COORD_PID"
echo "=========================================="
