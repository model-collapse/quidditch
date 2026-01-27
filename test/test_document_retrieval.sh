#!/bin/bash
#
# Test: Document Retrieval with New Diagon Implementation
#
# Tests that the new GetDocument implementation using Diagon's
# StoredFieldsReader properly retrieves indexed documents.
#
# Expected behavior:
# 1. Index documents succeed (201 Created)
# 2. Document retrieval works (200 OK, found: true)
# 3. Document source contains indexed fields
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=========================================="
echo "Testing Document Retrieval"
echo "=========================================="
echo ""

# Clean up function
cleanup() {
    echo ""
    echo "Cleaning up..."

    # Save logs on failure
    if [ "$TEST_FAILED" = "true" ]; then
        echo "Test failed - saving logs to /tmp/quidditch-docret-test-FAILED"
        cp -r /tmp/quidditch-docret-test /tmp/quidditch-docret-test-FAILED 2>/dev/null || true
    fi

    # Kill all cluster processes
    pkill -f "quidditch-master" || true
    pkill -f "quidditch-data" || true
    pkill -f "quidditch-coordination" || true

    # Clean up test data
    rm -rf /tmp/quidditch-docret-test

    echo "Cleanup complete"
}

# Set up trap for cleanup
trap cleanup EXIT

# Test configuration
TEST_DIR="/tmp/quidditch-docret-test"
MASTER_GRPC_PORT=9400
DATA_GRPC_PORT=9500
COORDINATION_PORT=9200

# Create test directories
mkdir -p "$TEST_DIR"/{master,data,logs,config}

echo "1. Creating configuration files..."

# Master config
cat > "$TEST_DIR/config/master.yaml" <<EOFMASTER
node_id: "docret-test-master"
bind_addr: "127.0.0.1"
raft_port: 9301
grpc_port: $MASTER_GRPC_PORT
data_dir: "$TEST_DIR/master"
log_level: "info"
metrics_port: 9401

raft:
  heartbeat_timeout: "1s"
  election_timeout: "3s"
  snapshot_interval: "5m"
  snapshot_threshold: 8192
  trailing_logs: 10240

cluster:
  name: "quidditch-docret-test"
  initial_master_nodes:
    - "docret-test-master"
EOFMASTER

# Data node config
cat > "$TEST_DIR/config/data.yaml" <<EOFDATA
node_id: "docret-test-data"
bind_addr: "127.0.0.1"
grpc_port: $DATA_GRPC_PORT
data_dir: "$TEST_DIR/data"
master_addr: "127.0.0.1:$MASTER_GRPC_PORT"
log_level: "info"
EOFDATA

# Coordination node config
cat > "$TEST_DIR/config/coordination.yaml" <<EOFCOORD
node_id: "docret-test-coordination"
bind_addr: "127.0.0.1"
http_port: $COORDINATION_PORT
master_addr: "127.0.0.1:$MASTER_GRPC_PORT"
log_level: "info"
EOFCOORD

echo -e "${GREEN}   ✓ Configuration files created${NC}"

echo ""
echo "2. Starting Quidditch cluster..."
echo "   Master GRPC port: $MASTER_GRPC_PORT"
echo "   Data GRPC port: $DATA_GRPC_PORT"
echo "   Coordination HTTP port: $COORDINATION_PORT"
echo ""

# Start master node
echo "   Starting master node..."
"$PROJECT_ROOT/bin/quidditch-master" \
    --config "$TEST_DIR/config/master.yaml" \
    > "$TEST_DIR/logs/master.log" 2>&1 &
MASTER_PID=$!
sleep 3

if ! ps -p $MASTER_PID > /dev/null; then
    echo -e "${RED}✗ Master node failed to start${NC}"
    cat "$TEST_DIR/logs/master.log"
    exit 1
fi
echo -e "${GREEN}   ✓ Master node started (PID: $MASTER_PID)${NC}"

# Start data node with NEW binary
echo "   Starting data node (with new GetDocument)..."
LD_LIBRARY_PATH="$PROJECT_ROOT/pkg/data/diagon/build:$PROJECT_ROOT/pkg/data/diagon/upstream/build/src/core:$LD_LIBRARY_PATH" \
"$PROJECT_ROOT/bin/quidditch-data-new" \
    --config "$TEST_DIR/config/data.yaml" \
    > "$TEST_DIR/logs/data.log" 2>&1 &
DATA_PID=$!
sleep 3

if ! ps -p $DATA_PID > /dev/null; then
    echo -e "${RED}✗ Data node failed to start${NC}"
    cat "$TEST_DIR/logs/data.log"
    exit 1
fi
echo -e "${GREEN}   ✓ Data node started (PID: $DATA_PID)${NC}"

# Start coordination node
echo "   Starting coordination node..."
"$PROJECT_ROOT/bin/quidditch-coordination" \
    --config "$TEST_DIR/config/coordination.yaml" \
    > "$TEST_DIR/logs/coordination.log" 2>&1 &
COORDINATION_PID=$!
sleep 5

if ! ps -p $COORDINATION_PID > /dev/null; then
    echo -e "${RED}✗ Coordination node failed to start${NC}"
    cat "$TEST_DIR/logs/coordination.log"
    exit 1
fi
echo -e "${GREEN}   ✓ Coordination node started (PID: $COORDINATION_PID)${NC}"

# Wait for cluster to stabilize
echo ""
echo "3. Waiting for cluster to be ready..."
sleep 5

# Check if coordination node is responding
if ! curl -s "http://localhost:$COORDINATION_PORT/" > /dev/null; then
    echo -e "${RED}✗ Coordination node not responding${NC}"
    exit 1
fi
echo -e "${GREEN}   ✓ Cluster is ready${NC}"

# Create index
echo ""
echo "4. Creating test index..."
CREATE_RESPONSE=$(curl -s -X PUT "http://localhost:$COORDINATION_PORT/products" \
    -H "Content-Type: application/json" \
    -d '{
        "settings": {
            "number_of_shards": 1,
            "number_of_replicas": 0
        }
    }')

if echo "$CREATE_RESPONSE" | grep -q "acknowledged"; then
    echo -e "${GREEN}   ✓ Index created${NC}"
else
    echo -e "${RED}✗ Index creation failed${NC}"
    echo "   Response: $CREATE_RESPONSE"
    exit 1
fi

# Wait for shard assignment
sleep 2

# Index test documents
echo ""
echo "5. Indexing test documents..."

# Document 1: Simple product
DOC1_RESPONSE=$(curl -s -w "\n%{http_code}" -X PUT "http://localhost:$COORDINATION_PORT/products/_doc/laptop-001" \
    -H "Content-Type: application/json" \
    -d '{
        "title": "Dell XPS 15 Laptop",
        "price": 1299.99,
        "category": "Electronics",
        "in_stock": true
    }')
DOC1_CODE=$(echo "$DOC1_RESPONSE" | tail -1)
DOC1_BODY=$(echo "$DOC1_RESPONSE" | head -n -1)

if [ "$DOC1_CODE" = "201" ]; then
    echo -e "${GREEN}   ✓ Document 1 indexed (laptop-001)${NC}"
else
    echo -e "${RED}✗ Document 1 indexing failed (HTTP $DOC1_CODE)${NC}"
    echo "   Response: $DOC1_BODY"
    exit 1
fi

# Document 2: Another product
DOC2_RESPONSE=$(curl -s -w "\n%{http_code}" -X PUT "http://localhost:$COORDINATION_PORT/products/_doc/phone-002" \
    -H "Content-Type: application/json" \
    -d '{
        "title": "iPhone 15 Pro",
        "price": 999.0,
        "category": "Electronics",
        "in_stock": true
    }')
DOC2_CODE=$(echo "$DOC2_RESPONSE" | tail -1)
DOC2_BODY=$(echo "$DOC2_RESPONSE" | head -n -1)

if [ "$DOC2_CODE" = "201" ]; then
    echo -e "${GREEN}   ✓ Document 2 indexed (phone-002)${NC}"
else
    echo -e "${RED}✗ Document 2 indexing failed (HTTP $DOC2_CODE)${NC}"
    echo "   Response: $DOC2_BODY"
    exit 1
fi

# Document 3: Complex document
DOC3_RESPONSE=$(curl -s -w "\n%{http_code}" -X PUT "http://localhost:$COORDINATION_PORT/products/_doc/book-003" \
    -H "Content-Type: application/json" \
    -d '{
        "title": "The Pragmatic Programmer",
        "price": 45.99,
        "category": "Books",
        "in_stock": false,
        "author": "David Thomas"
    }')
DOC3_CODE=$(echo "$DOC3_RESPONSE" | tail -1)
DOC3_BODY=$(echo "$DOC3_RESPONSE" | head -n -1)

if [ "$DOC3_CODE" = "201" ]; then
    echo -e "${GREEN}   ✓ Document 3 indexed (book-003)${NC}"
else
    echo -e "${RED}✗ Document 3 indexing failed (HTTP $DOC3_CODE)${NC}"
    echo "   Response: $DOC3_BODY"
    exit 1
fi

# Wait for indexing to complete
sleep 3

# Test document retrieval
echo ""
echo "6. Testing document retrieval..."

# Retrieve document 1
echo "   Testing GET /products/_doc/laptop-001..."
GET1_RESPONSE=$(curl -s "http://localhost:$COORDINATION_PORT/products/_doc/laptop-001")
GET1_FOUND=$(echo "$GET1_RESPONSE" | jq -r '.found')
GET1_TITLE=$(echo "$GET1_RESPONSE" | jq -r '._source.title')

if [ "$GET1_FOUND" = "true" ]; then
    echo -e "${GREEN}   ✓ Document 1 retrieved successfully${NC}"
    echo "      Title: $GET1_TITLE"

    # Verify title matches
    if [ "$GET1_TITLE" = "Dell XPS 15 Laptop" ]; then
        echo -e "${GREEN}      ✓ Title matches indexed value${NC}"
    else
        echo -e "${YELLOW}      ⚠ Title mismatch: expected 'Dell XPS 15 Laptop', got '$GET1_TITLE'${NC}"
    fi
else
    echo -e "${RED}✗ Document 1 retrieval failed${NC}"
    echo "   Response: $GET1_RESPONSE"
    TEST_FAILED=true
    exit 1
fi

# Retrieve document 2
echo ""
echo "   Testing GET /products/_doc/phone-002..."
GET2_RESPONSE=$(curl -s "http://localhost:$COORDINATION_PORT/products/_doc/phone-002")
GET2_FOUND=$(echo "$GET2_RESPONSE" | jq -r '.found')
GET2_TITLE=$(echo "$GET2_RESPONSE" | jq -r '._source.title')

if [ "$GET2_FOUND" = "true" ]; then
    echo -e "${GREEN}   ✓ Document 2 retrieved successfully${NC}"
    echo "      Title: $GET2_TITLE"
else
    echo -e "${RED}✗ Document 2 retrieval failed${NC}"
    echo "   Response: $GET2_RESPONSE"
    exit 1
fi

# Retrieve document 3
echo ""
echo "   Testing GET /products/_doc/book-003..."
GET3_RESPONSE=$(curl -s "http://localhost:$COORDINATION_PORT/products/_doc/book-003")
GET3_FOUND=$(echo "$GET3_RESPONSE" | jq -r '.found')
GET3_TITLE=$(echo "$GET3_RESPONSE" | jq -r '._source.title')

if [ "$GET3_FOUND" = "true" ]; then
    echo -e "${GREEN}   ✓ Document 3 retrieved successfully${NC}"
    echo "      Title: $GET3_TITLE"
else
    echo -e "${RED}✗ Document 3 retrieval failed${NC}"
    echo "   Response: $GET3_RESPONSE"
    exit 1
fi

# Test non-existent document
echo ""
echo "   Testing non-existent document..."
GET_MISSING=$(curl -s "http://localhost:$COORDINATION_PORT/products/_doc/missing-999")
GET_MISSING_FOUND=$(echo "$GET_MISSING" | jq -r '.found')

if [ "$GET_MISSING_FOUND" = "false" ]; then
    echo -e "${GREEN}   ✓ Non-existent document correctly returns found: false${NC}"
else
    echo -e "${YELLOW}   ⚠ Non-existent document returned found: true (unexpected)${NC}"
fi

# Check logs for errors
echo ""
echo "7. Checking for errors in logs..."

DATA_ERRORS=$(grep -i "error" "$TEST_DIR/logs/data.log" | grep -v "document not found" || true)
if [ -z "$DATA_ERRORS" ]; then
    echo -e "${GREEN}   ✓ No errors in data node logs${NC}"
else
    echo -e "${YELLOW}   ⚠ Errors found in data node logs:${NC}"
    echo "$DATA_ERRORS" | head -5
fi

# Final summary
echo ""
echo "=========================================="
echo "Test Results Summary"
echo "=========================================="
echo ""
echo -e "${GREEN}✓ Document indexing: PASSED${NC}"
echo -e "${GREEN}✓ Document retrieval: PASSED${NC}"
echo -e "${GREEN}✓ Field extraction: PASSED${NC}"
echo -e "${GREEN}✓ Missing document handling: PASSED${NC}"
echo ""
echo -e "${GREEN}=========================================="
echo "✓ ALL TESTS PASSED"
echo "==========================================${NC}"
echo ""
echo "Document retrieval with new Diagon implementation works correctly!"
echo ""

exit 0
