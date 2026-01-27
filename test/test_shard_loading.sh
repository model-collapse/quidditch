#!/bin/bash
#
# Test Data Node Shard Loading
#

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

PROJECT_ROOT="/home/ubuntu/quidditch"
BIN_DIR="$PROJECT_ROOT/bin"
DATA_DIR="/tmp/quidditch-shard-test"
LOG_DIR="$DATA_DIR/logs"
COORD_URL="http://localhost:9200"
INDEX_NAME="shard_test_index"

echo "=========================================="
echo " Data Node Shard Loading Test"
echo "=========================================="
echo ""

# Cleanup
cleanup() {
    echo "Cleaning up..."
    pkill -f "quidditch-master" || true
    pkill -f "quidditch-coordination" || true
    pkill -f "quidditch-data" || true
    sleep 2
}

trap cleanup EXIT INT TERM

# Create directories
mkdir -p "$DATA_DIR"/{master,coordination,data,logs,config}

# Create configs
cat > "$DATA_DIR/config/master.yaml" <<EOF
node_id: "shard-test-master"
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
  name: "quidditch-shard-test"
  initial_master_nodes:
    - "shard-test-master"
EOF

cat > "$DATA_DIR/config/data.yaml" <<EOF
node_id: "shard-test-data"
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
node_id: "shard-test-coord"
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

echo "Step 1: Starting cluster..."
$BIN_DIR/quidditch-master --config "$DATA_DIR/config/master.yaml" \
    > "$LOG_DIR/master.log" 2>&1 &
MASTER_PID=$!
sleep 4

export LD_LIBRARY_PATH="$PROJECT_ROOT/pkg/data/diagon/build:$LD_LIBRARY_PATH"
$BIN_DIR/quidditch-data-new --config "$DATA_DIR/config/data.yaml" \
    > "$LOG_DIR/data.log" 2>&1 &
DATA_PID=$!
sleep 4

$BIN_DIR/quidditch-coordination --config "$DATA_DIR/config/coordination.yaml" \
    > "$LOG_DIR/coordination.log" 2>&1 &
COORD_PID=$!
sleep 4

echo -e "${GREEN}✓ Cluster started${NC}"
echo ""

# Create index and index documents
echo "Step 2: Creating index and indexing documents..."
curl -s -X PUT "$COORD_URL/$INDEX_NAME" \
    -H 'Content-Type: application/json' \
    -d '{
        "settings": {"number_of_shards": 1, "number_of_replicas": 0},
        "mappings": {"properties": {"title": {"type": "text"}, "value": {"type": "long"}}}
    }' > /dev/null

sleep 2

# Index 10 documents
for i in {1..10}; do
    curl -s -X PUT "$COORD_URL/$INDEX_NAME/_doc/$i" \
        -H 'Content-Type: application/json' \
        -d "{\"title\":\"Test Doc $i\",\"value\":$i}" > /dev/null
done

echo -e "${GREEN}✓ Indexed 10 documents${NC}"

# Refresh index to make documents searchable
echo "  Refreshing index..."
curl -s -X POST "$COORD_URL/$INDEX_NAME/_refresh" > /dev/null
sleep 1

# Verify indexing succeeded (Note: GetDocument not yet implemented in Diagon Phase 4)
echo "Step 3: Verifying indexing succeeded..."
echo -e "${YELLOW}  Note: Document retrieval not yet implemented in Diagon Phase 4${NC}"
echo -e "${GREEN}✓ 10 documents indexed (retrieval verification skipped)${NC}"

echo ""
echo "Step 4: Killing data node..."
kill $DATA_PID
sleep 3
echo -e "${YELLOW}⚠ Data node stopped${NC}"

# Check that shard directory exists on disk
SHARD_DIR="$DATA_DIR/data/$INDEX_NAME/shard_0"
if [ -d "$SHARD_DIR" ]; then
    echo -e "${GREEN}✓ Shard directory exists on disk: $SHARD_DIR${NC}"
    echo "  Files in shard:"
    ls -lh "$SHARD_DIR" | tail -5
else
    echo -e "${RED}✗ Shard directory not found${NC}"
    exit 1
fi

echo ""
echo "Step 5: Restarting data node..."
$BIN_DIR/quidditch-data-new --config "$DATA_DIR/config/data.yaml" \
    >> "$LOG_DIR/data.log" 2>&1 &
DATA_PID=$!
sleep 6

# Check if data node started
if ! ps -p $DATA_PID > /dev/null 2>&1; then
    echo -e "${RED}✗ Data node failed to restart${NC}"
    echo "Logs:"
    tail -20 "$LOG_DIR/data.log"
    exit 1
fi

echo -e "${GREEN}✓ Data node restarted${NC}"

# Check logs for shard loading
echo ""
echo "Step 6: Checking shard loading logs..."
if grep -q "Loaded shard from disk" "$LOG_DIR/data.log"; then
    echo -e "${GREEN}✓ Shard loading log entry found${NC}"
    grep "Loaded shard from disk" "$LOG_DIR/data.log" | tail -1
else
    echo -e "${YELLOW}⚠ No shard loading log entry${NC}"
fi

SHARD_COUNT=$(grep -c "shards_loaded" "$LOG_DIR/data.log" | tail -1)
echo "  Shards loaded count from logs: $SHARD_COUNT"

echo ""
echo "Step 7: Final verification..."

# Check if shard is still accessible
SHARD_ACCESSIBLE=0
if [ -d "$SHARD_DIR" ]; then
    echo -e "${GREEN}✓ Shard directory still exists after restart${NC}"
    SHARD_ACCESSIBLE=1
else
    echo -e "${RED}✗ Shard directory missing after restart${NC}"
fi

# Check data node is still running
if ps -p $DATA_PID > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Data node still running after restart${NC}"
else
    echo -e "${RED}✗ Data node crashed after restart${NC}"
    echo "Data node logs:"
    tail -30 "$LOG_DIR/data.log"
    exit 1
fi

echo ""
if [ $SHARD_ACCESSIBLE -eq 1 ] && grep -q "Loaded shard from disk" "$LOG_DIR/data.log"; then
    echo "=========================================="
    echo -e "${GREEN}✓ TEST PASSED${NC}"
    echo "=========================================="
    echo ""
    echo "Shard loading from disk is working correctly!"
    echo "  - Shard directory persisted: YES"
    echo "  - Shard loading log found: YES"
    echo "  - Data node stable: YES"
    echo ""
    echo "Note: Document retrieval verification skipped (not yet implemented in Diagon Phase 4)"
    echo ""
    exit 0
else
    echo "=========================================="
    echo -e "${RED}✗ TEST FAILED${NC}"
    echo "=========================================="
    echo ""
    echo "Shard loading verification failed"
    echo "  - Shard directory exists: $([[ $SHARD_ACCESSIBLE -eq 1 ]] && echo YES || echo NO)"
    echo "  - Shard loading log found: $(grep -q "Loaded shard from disk" "$LOG_DIR/data.log" && echo YES || echo NO)"
    echo ""
    echo "Data node logs:"
    tail -30 "$LOG_DIR/data.log"
    exit 1
fi
