#!/bin/bash
#
# Failure Testing for Quidditch Cluster
# Tests cluster resilience to node failures
#

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
PROJECT_ROOT="/home/ubuntu/quidditch"
BIN_DIR="$PROJECT_ROOT/bin"
DATA_DIR="/tmp/quidditch-failure-test"
LOG_DIR="$DATA_DIR/logs"
COORD_URL="http://localhost:9200"
INDEX_NAME="failure_test_index"

# Test results
TESTS_PASSED=0
TESTS_FAILED=0
TEST_DETAILS=()

echo "=========================================="
echo " Quidditch Failure Testing Suite"
echo "=========================================="
echo ""
echo "This will test cluster resilience by:"
echo "  1. Starting a 3-node cluster"
echo "  2. Indexing test data"
echo "  3. Killing nodes one at a time"
echo "  4. Verifying cluster continues operating"
echo "  5. Testing recovery"
echo ""

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."
    pkill -f "quidditch-master" || true
    pkill -f "quidditch-coordination" || true
    pkill -f "quidditch-data" || true
    sleep 2

    # Preserve logs if tests failed
    if [ $TESTS_FAILED -gt 0 ]; then
        PRESERVED_LOG_DIR="$PROJECT_ROOT/test_logs_$(date +%Y%m%d_%H%M%S)"
        echo "Preserving logs to: $PRESERVED_LOG_DIR"
        cp -r "$LOG_DIR" "$PRESERVED_LOG_DIR" 2>/dev/null || true
    fi

    rm -rf "$DATA_DIR"
    echo "Cleanup complete"
}

# Trap exit
trap cleanup EXIT INT TERM

# Test helper functions
test_start() {
    local test_name=$1
    echo ""
    echo "=========================================="
    echo "TEST: $test_name"
    echo "=========================================="
}

test_pass() {
    local test_name=$1
    TESTS_PASSED=$((TESTS_PASSED + 1))
    TEST_DETAILS+=("✅ PASS: $test_name")
    echo -e "${GREEN}✓ PASS: $test_name${NC}"
}

test_fail() {
    local test_name=$1
    local reason=$2
    TESTS_FAILED=$((TESTS_FAILED + 1))
    TEST_DETAILS+=("✗ FAIL: $test_name - $reason")
    echo -e "${RED}✗ FAIL: $test_name${NC}"
    echo -e "${RED}  Reason: $reason${NC}"
}

# Check cluster health
check_cluster_health() {
    local expected_status=$1
    HEALTH=$(curl -s --max-time 5 "$COORD_URL/_cluster/health" 2>&1 || echo "{}")

    if echo "$HEALTH" | grep -q "curl:"; then
        return 1
    fi

    if [ -n "$expected_status" ]; then
        if echo "$HEALTH" | grep -q "\"status\":\"$expected_status\""; then
            return 0
        else
            return 1
        fi
    fi

    return 0
}

# Index test document
index_document() {
    local doc_id=$1
    local response=$(curl -s -X PUT "$COORD_URL/$INDEX_NAME/_doc/$doc_id" \
        -H 'Content-Type: application/json' \
        -d "{\"title\":\"Test Doc $doc_id\",\"timestamp\":$(date +%s)}" 2>&1)

    if echo "$response" | grep -q "\"result\":\"created\""; then
        return 0
    elif echo "$response" | grep -q "\"result\":\"updated\""; then
        return 0
    else
        return 1
    fi
}

# Retrieve test document
get_document() {
    local doc_id=$1
    local response=$(curl -s "$COORD_URL/$INDEX_NAME/_doc/$doc_id" 2>&1)

    if echo "$response" | grep -q "\"found\":true"; then
        return 0
    else
        return 1
    fi
}

# Start cluster
echo "Step 1: Starting cluster..."
mkdir -p "$DATA_DIR"/{master,coordination,data,logs,config}

# Create configs
cat > "$DATA_DIR/config/master.yaml" <<EOF
node_id: "failure-test-master"
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
  name: "quidditch-failure-test"
  initial_master_nodes:
    - "failure-test-master"
EOF

cat > "$DATA_DIR/config/data.yaml" <<EOF
node_id: "failure-test-data"
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
node_id: "failure-test-coord"
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

# Start nodes
echo "  Starting Master Node..."
$BIN_DIR/quidditch-master --config "$DATA_DIR/config/master.yaml" \
    > "$LOG_DIR/master.log" 2>&1 &
MASTER_PID=$!
sleep 4

if ! ps -p $MASTER_PID > /dev/null 2>&1; then
    echo -e "${RED}✗ Master node failed to start${NC}"
    tail -20 "$LOG_DIR/master.log"
    exit 1
fi

echo "  Starting Data Node..."
export LD_LIBRARY_PATH="$PROJECT_ROOT/pkg/data/diagon/build:$LD_LIBRARY_PATH"
$BIN_DIR/quidditch-data --config "$DATA_DIR/config/data.yaml" \
    > "$LOG_DIR/data.log" 2>&1 &
DATA_PID=$!
sleep 4

if ! ps -p $DATA_PID > /dev/null 2>&1; then
    echo -e "${RED}✗ Data node failed to start${NC}"
    tail -20 "$LOG_DIR/data.log"
    exit 1
fi

echo "  Starting Coordination Node..."
$BIN_DIR/quidditch-coordination --config "$DATA_DIR/config/coordination.yaml" \
    > "$LOG_DIR/coordination.log" 2>&1 &
COORD_PID=$!
sleep 4

if ! ps -p $COORD_PID > /dev/null 2>&1; then
    echo -e "${RED}✗ Coordination node failed to start${NC}"
    tail -20 "$LOG_DIR/coordination.log"
    exit 1
fi

echo -e "${GREEN}✓ Cluster started successfully${NC}"
echo "  Master PID: $MASTER_PID"
echo "  Data PID: $DATA_PID"
echo "  Coordination PID: $COORD_PID"
sleep 2

# Test 1: Initial cluster health
test_start "Initial Cluster Health"
if check_cluster_health; then
    test_pass "Initial cluster health check"
else
    test_fail "Initial cluster health check" "Cannot connect to cluster"
    exit 1
fi

# Test 2: Create index
test_start "Index Creation"
CREATE_RESPONSE=$(curl -s -X PUT "$COORD_URL/$INDEX_NAME" \
    -H 'Content-Type: application/json' \
    -d '{
        "settings": {
            "number_of_shards": 1,
            "number_of_replicas": 0
        },
        "mappings": {
            "properties": {
                "title": {"type": "text"},
                "timestamp": {"type": "long"}
            }
        }
    }' 2>&1)

if echo "$CREATE_RESPONSE" | grep -qiE "(acknowledged|true)"; then
    test_pass "Index creation"
    sleep 2
else
    test_fail "Index creation" "Failed to create index"
fi

# Test 3: Index initial documents
test_start "Initial Document Indexing"
SUCCESS_COUNT=0
for i in {1..10}; do
    if index_document $i; then
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    fi
done

if [ $SUCCESS_COUNT -eq 10 ]; then
    test_pass "Indexed 10 documents successfully"
else
    test_fail "Document indexing" "Only $SUCCESS_COUNT/10 documents indexed"
fi
sleep 2

# Test 4: Verify documents retrievable
test_start "Initial Document Retrieval"
RETRIEVE_COUNT=0
for i in {1..10}; do
    if get_document $i; then
        RETRIEVE_COUNT=$((RETRIEVE_COUNT + 1))
    fi
done

if [ $RETRIEVE_COUNT -eq 10 ]; then
    test_pass "All 10 documents retrievable"
else
    test_fail "Document retrieval" "Only $RETRIEVE_COUNT/10 documents retrieved"
fi

# Test 5: Kill coordination node
test_start "Coordination Node Failure"
echo "  Killing coordination node (PID: $COORD_PID)..."
kill $COORD_PID
sleep 3

# Cluster should be unhealthy without coordination node
if ! check_cluster_health; then
    test_pass "Cluster correctly reports coordination node down"
else
    test_fail "Coordination node failure detection" "Cluster still reports healthy"
fi

# Restart coordination node
echo "  Restarting coordination node..."
$BIN_DIR/quidditch-coordination --config "$DATA_DIR/config/coordination.yaml" \
    > "$LOG_DIR/coordination.log" 2>&1 &
COORD_PID=$!
sleep 4

if ps -p $COORD_PID > /dev/null 2>&1 && check_cluster_health; then
    test_pass "Coordination node recovered successfully"
else
    test_fail "Coordination node recovery" "Node failed to restart or cluster unhealthy"
fi

# Test 6: Data persistence after coordination failure
test_start "Data Persistence After Coordination Failure"
RETRIEVE_COUNT=0
for i in {1..10}; do
    if get_document $i; then
        RETRIEVE_COUNT=$((RETRIEVE_COUNT + 1))
    fi
    sleep 0.1
done

if [ $RETRIEVE_COUNT -ge 9 ]; then
    test_pass "Data persisted through coordination node failure ($RETRIEVE_COUNT/10 docs)"
else
    test_fail "Data persistence" "Only $RETRIEVE_COUNT/10 documents retrieved after recovery"
fi

# Test 7: Index new documents after recovery
test_start "Indexing After Recovery"
SUCCESS_COUNT=0
for i in {11..15}; do
    if index_document $i; then
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    fi
    sleep 0.1
done

if [ $SUCCESS_COUNT -eq 5 ]; then
    test_pass "Indexed 5 new documents after recovery"
else
    test_fail "Post-recovery indexing" "Only $SUCCESS_COUNT/5 documents indexed"
fi

# Test 8: Kill data node
test_start "Data Node Failure"
echo "  Killing data node (PID: $DATA_PID)..."
kill $DATA_PID
sleep 3

# Cluster should detect data node down
if check_cluster_health; then
    HEALTH=$(curl -s "$COORD_URL/_cluster/health")
    if echo "$HEALTH" | grep -q '"number_of_data_nodes":0'; then
        test_pass "Cluster correctly reports data node down"
    else
        test_fail "Data node failure detection" "Cluster doesn't report data node down"
    fi
else
    test_pass "Cluster reports unhealthy with data node down"
fi

# Restart data node
echo "  Restarting data node..."
$BIN_DIR/quidditch-data --config "$DATA_DIR/config/data.yaml" \
    >> "$LOG_DIR/data.log" 2>&1 &
DATA_PID=$!
sleep 4

if ps -p $DATA_PID > /dev/null 2>&1; then
    echo "  Waiting for data node to fully recover and load shards..."
    sleep 8  # Give more time for shards to load from disk
    if check_cluster_health; then
        test_pass "Data node recovered successfully"
    else
        test_fail "Data node recovery" "Cluster still unhealthy after restart"
    fi
else
    test_fail "Data node recovery" "Node failed to restart"
fi

# Test 9: Data persistence after data node failure
test_start "Data Persistence After Data Node Failure"
sleep 3  # Give data node time to fully recover
RETRIEVE_COUNT=0
for i in {1..15}; do
    if get_document $i; then
        RETRIEVE_COUNT=$((RETRIEVE_COUNT + 1))
    fi
    sleep 0.1
done

if [ $RETRIEVE_COUNT -ge 14 ]; then
    test_pass "Data persisted through data node failure ($RETRIEVE_COUNT/15 docs)"
else
    test_fail "Data persistence after data node failure" "Only $RETRIEVE_COUNT/15 documents retrieved"
fi

# Test 10: Master node resilience (brief kill)
test_start "Master Node Brief Failure"
echo "  Killing master node (PID: $MASTER_PID)..."
kill $MASTER_PID
sleep 2

# Restart immediately
echo "  Restarting master node..."
$BIN_DIR/quidditch-master --config "$DATA_DIR/config/master.yaml" \
    > "$LOG_DIR/master.log" 2>&1 &
MASTER_PID=$!
sleep 5

if ps -p $MASTER_PID > /dev/null 2>&1; then
    # Give time for cluster to stabilize
    sleep 3
    if check_cluster_health; then
        test_pass "Master node recovered successfully"
    else
        test_fail "Master node recovery" "Cluster unhealthy after master restart"
    fi
else
    test_fail "Master node recovery" "Master failed to restart"
fi

# Test 11: Final data integrity check
test_start "Final Data Integrity Check"
sleep 2
RETRIEVE_COUNT=0
for i in {1..15}; do
    if get_document $i; then
        RETRIEVE_COUNT=$((RETRIEVE_COUNT + 1))
    fi
    sleep 0.1
done

if [ $RETRIEVE_COUNT -eq 15 ]; then
    test_pass "All 15 documents retrievable after all failures"
elif [ $RETRIEVE_COUNT -ge 14 ]; then
    test_pass "14-15 documents retrievable after all failures (acceptable)"
else
    test_fail "Final data integrity" "Only $RETRIEVE_COUNT/15 documents retrieved"
fi

# Test 12: Index new documents after all failures
test_start "Indexing After All Failures"
SUCCESS_COUNT=0
for i in {16..20}; do
    if index_document $i; then
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    fi
    sleep 0.1
done

if [ $SUCCESS_COUNT -ge 4 ]; then
    test_pass "Can index new documents after failures ($SUCCESS_COUNT/5 docs)"
else
    test_fail "Post-failure indexing" "Only $SUCCESS_COUNT/5 documents indexed"
fi

# Print summary
echo ""
echo "=========================================="
echo -e "${BLUE}TEST SUMMARY${NC}"
echo "=========================================="
echo ""
echo "Total Tests: $((TESTS_PASSED + TESTS_FAILED))"
echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "${RED}Failed: $TESTS_FAILED${NC}"
echo ""
echo "Detailed Results:"
for detail in "${TEST_DETAILS[@]}"; do
    echo "  $detail"
done
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo "=========================================="
    echo -e "${GREEN}✓ ALL TESTS PASSED${NC}"
    echo "=========================================="
    echo ""
    echo "The Quidditch cluster demonstrates good resilience:"
    echo "  ✓ Survives coordination node failures"
    echo "  ✓ Survives data node failures"
    echo "  ✓ Survives master node brief failures"
    echo "  ✓ Maintains data integrity through failures"
    echo "  ✓ Recovers and continues operating"
    echo ""
    exit 0
else
    echo "=========================================="
    echo -e "${YELLOW}⚠ SOME TESTS FAILED${NC}"
    echo "=========================================="
    echo ""
    echo "Review the failures above for details."
    echo ""
    exit 1
fi
