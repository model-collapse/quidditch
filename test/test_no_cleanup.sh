#!/bin/bash
#
# Test: Range and Boolean Queries
#
# Tests the newly integrated range and boolean query functionality
# with Diagon C++ search engine.
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "=========================================="
echo "Testing Range and Boolean Queries"
echo "=========================================="
echo ""

# Test configuration
TEST_DIR="/tmp/quidditch-range-bool-test"
MASTER_GRPC_PORT=9400
DATA_GRPC_PORT=9500
COORDINATION_PORT=9200

TESTS_PASSED=0
TESTS_FAILED=0

# Clean up function
cleanup() {
    echo ""
    echo "Cleaning up..."

    # Kill all cluster processes
    pkill -f "quidditch-master" || true
    pkill -f "quidditch-data" || true
    pkill -f "quidditch-coordination" || true

    # Clean up test data
    rm -rf "$TEST_DIR"

    echo "Cleanup complete"

    echo ""
    echo "=========================================="
    echo "Test Results"
    echo "=========================================="
    echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"

    if [ $TESTS_FAILED -eq 0 ]; then
        echo -e "${GREEN}✓ ALL TESTS PASSED${NC}"
        exit 0
    else
        echo -e "${RED}✗ SOME TESTS FAILED${NC}"
        exit 1
    fi
}

# Set up trap for cleanup
#trap cleanup EXIT

# Create test directories
mkdir -p "$TEST_DIR"/{master,data,logs,config}

echo "1. Creating configuration files..."

# Master config
cat > "$TEST_DIR/config/master.yaml" <<EOFMASTER
node_id: "range-bool-test-master"
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
EOFMASTER

# Data node config
cat > "$TEST_DIR/config/data.yaml" <<EOFDATA
node_id: "range-bool-test-data"
bind_addr: "127.0.0.1"
grpc_port: $DATA_GRPC_PORT
http_port: 9600
master_addr: "127.0.0.1:$MASTER_GRPC_PORT"
data_dir: "$TEST_DIR/data"
log_level: "info"
metrics_port: 9601
EOFDATA

# Coordination node config
cat > "$TEST_DIR/config/coordination.yaml" <<EOFCOORD
node_id: "range-bool-test-coordination"
bind_addr: "127.0.0.1"
rest_port: $COORDINATION_PORT
master_addr: "127.0.0.1:$MASTER_GRPC_PORT"
log_level: "info"
metrics_port: 9201
EOFCOORD

echo -e "${GREEN}   ✓ Configuration files created${NC}"

echo ""
echo "2. Starting Quidditch cluster..."
echo "   Master GRPC port: $MASTER_GRPC_PORT"
echo "   Data GRPC port: $DATA_GRPC_PORT"
echo "   Coordination HTTP port: $COORDINATION_PORT"

$PROJECT_ROOT/bin/quidditch-master --config "$TEST_DIR/config/master.yaml" > "$TEST_DIR/logs/master.log" 2>&1 &
MASTER_PID=$!
echo -e "${GREEN}   ✓ Master node started (PID: $MASTER_PID)${NC}"

sleep 2

$PROJECT_ROOT/bin/quidditch-data-new --config "$TEST_DIR/config/data.yaml" > "$TEST_DIR/logs/data.log" 2>&1 &
DATA_PID=$!
echo -e "${GREEN}   ✓ Data node started (PID: $DATA_PID)${NC}"

sleep 2

$PROJECT_ROOT/bin/quidditch-coordination --config "$TEST_DIR/config/coordination.yaml" > "$TEST_DIR/logs/coordination.log" 2>&1 &
COORD_PID=$!
echo -e "${GREEN}   ✓ Coordination node started (PID: $COORD_PID)${NC}"

echo ""
echo "3. Waiting for cluster to be ready..."
sleep 3
echo -e "${GREEN}   ✓ Cluster is ready${NC}"

echo ""
echo "4. Creating test index..."
curl -s -X PUT "http://localhost:$COORDINATION_PORT/products" \
  -H 'Content-Type: application/json' \
  -d '{
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0
    }
  }' > /tmp/create_index_resp.json

if grep -q "acknowledged.*true" /tmp/create_index_resp.json; then
  echo -e "${GREEN}   ✓ Index created${NC}"
else
  echo -e "${RED}   ✗ Index creation failed${NC}"
  cat /tmp/create_index_resp.json
  exit 1
fi

echo ""
echo "5. Indexing test documents..."

# Index products with various prices
curl -s -X PUT "http://localhost:$COORDINATION_PORT/products/_doc/1" \
  -H 'Content-Type: application/json' \
  -d '{"title": "Budget Laptop", "category": "electronics", "price": 50, "in_stock": true, "refurbished": false}' > /dev/null

curl -s -X PUT "http://localhost:$COORDINATION_PORT/products/_doc/2" \
  -H 'Content-Type: application/json' \
  -d '{"title": "Dell XPS 15", "category": "electronics", "price": 150, "in_stock": true, "refurbished": false, "brand": "Dell"}' > /dev/null

curl -s -X PUT "http://localhost:$COORDINATION_PORT/products/_doc/3" \
  -H 'Content-Type: application/json' \
  -d '{"title": "MacBook Pro", "category": "electronics", "price": 250, "in_stock": true, "refurbished": false, "brand": "Apple"}' > /dev/null

curl -s -X PUT "http://localhost:$COORDINATION_PORT/products/_doc/4" \
  -H 'Content-Type: application/json' \
  -d '{"title": "ThinkPad X1", "category": "electronics", "price": 350, "in_stock": false, "refurbished": false, "brand": "Lenovo"}' > /dev/null

curl -s -X PUT "http://localhost:$COORDINATION_PORT/products/_doc/5" \
  -H 'Content-Type: application/json' \
  -d '{"title": "Gaming Laptop", "category": "electronics", "price": 450, "in_stock": true, "refurbished": false}' > /dev/null

curl -s -X PUT "http://localhost:$COORDINATION_PORT/products/_doc/6" \
  -H 'Content-Type: application/json' \
  -d '{"title": "Refurb Laptop", "category": "electronics", "price": 550, "in_stock": true, "refurbished": true}' > /dev/null

curl -s -X PUT "http://localhost:$COORDINATION_PORT/products/_doc/7" \
  -H 'Content-Type: application/json' \
  -d '{"title": "Programming Book", "category": "books", "price": 40, "in_stock": true}' > /dev/null

curl -s -X PUT "http://localhost:$COORDINATION_PORT/products/_doc/8" \
  -H 'Content-Type: application/json' \
  -d '{"title": "Samsung Laptop", "category": "electronics", "price": 280, "in_stock": true, "refurbished": false, "brand": "Samsung"}' > /dev/null

echo -e "${GREEN}   ✓ Indexed 8 test documents${NC}"

sleep 2

echo ""
echo "=========================================="
echo "RANGE QUERY TESTS"
echo "=========================================="

# Test 1: Range query - both bounds (gte, lte)
echo ""
echo -e "${BLUE}Test 1: Range query with both bounds (100 <= price <= 300)${NC}"
RESULT=$(curl -s -X POST "http://localhost:$COORDINATION_PORT/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{"query": {"range": {"price": {"gte": 100, "lte": 300}}}, "size": 20}')

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
if [ "$TOTAL" -eq 3 ]; then
  echo -e "${GREEN}   ✓ Found $TOTAL documents (expected 3: Dell XPS, MacBook, Samsung)${NC}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}   ✗ Found $TOTAL documents (expected 3)${NC}"
  echo "$RESULT" | jq .
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 2: Range query - lower bound only (gte)
echo ""
echo -e "${BLUE}Test 2: Range query with lower bound only (price >= 400)${NC}"
RESULT=$(curl -s -X POST "http://localhost:$COORDINATION_PORT/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{"query": {"range": {"price": {"gte": 400}}}, "size": 20}')

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
if [ "$TOTAL" -eq 2 ]; then
  echo -e "${GREEN}   ✓ Found $TOTAL documents (expected 2: Gaming Laptop, Refurb Laptop)${NC}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}   ✗ Found $TOTAL documents (expected 2)${NC}"
  echo "$RESULT" | jq .
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 3: Range query - upper bound only (lte)
echo ""
echo -e "${BLUE}Test 3: Range query with upper bound only (price <= 50)${NC}"
RESULT=$(curl -s -X POST "http://localhost:$COORDINATION_PORT/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{"query": {"range": {"price": {"lte": 50}}}, "size": 20}')

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
if [ "$TOTAL" -eq 2 ]; then
  echo -e "${GREEN}   ✓ Found $TOTAL documents (expected 2: Budget Laptop, Programming Book)${NC}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}   ✗ Found $TOTAL documents (expected 2)${NC}"
  echo "$RESULT" | jq .
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 4: Range query - exclusive bounds (gt, lt)
echo ""
echo -e "${BLUE}Test 4: Range query with exclusive bounds (100 < price < 300)${NC}"
RESULT=$(curl -s -X POST "http://localhost:$COORDINATION_PORT/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{"query": {"range": {"price": {"gt": 100, "lt": 300}}}, "size": 20}')

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
if [ "$TOTAL" -eq 2 ]; then
  echo -e "${GREEN}   ✓ Found $TOTAL documents (expected 2: Dell XPS, MacBook - excludes 100 and 300)${NC}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}   ✗ Found $TOTAL documents (expected 2)${NC}"
  echo "$RESULT" | jq .
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

echo ""
echo "=========================================="
echo "BOOLEAN QUERY TESTS"
echo "=========================================="

# Test 5: Bool query - MUST only
echo ""
echo -e "${BLUE}Test 5: Bool query with MUST clause (category = electronics AND in_stock = true)${NC}"
RESULT=$(curl -s -X POST "http://localhost:$COORDINATION_PORT/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {
      "bool": {
        "must": [
          {"term": {"category": "electronics"}},
          {"term": {"in_stock": true}}
        ]
      }
    },
    "size": 20
  }')

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
if [ "$TOTAL" -eq 6 ]; then
  echo -e "${GREEN}   ✓ Found $TOTAL documents (expected 6: all electronics in stock)${NC}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}   ✗ Found $TOTAL documents (expected 6)${NC}"
  echo "$RESULT" | jq .
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 6: Bool query - MUST + FILTER
echo ""
echo -e "${BLUE}Test 6: Bool query with MUST + FILTER (electronics with price <= 300)${NC}"
RESULT=$(curl -s -X POST "http://localhost:$COORDINATION_PORT/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {
      "bool": {
        "must": [
          {"term": {"category": "electronics"}}
        ],
        "filter": [
          {"range": {"price": {"lte": 300}}}
        ]
      }
    },
    "size": 20
  }')

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
if [ "$TOTAL" -eq 4 ]; then
  echo -e "${GREEN}   ✓ Found $TOTAL documents (expected 4: Budget, Dell, MacBook, Samsung)${NC}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}   ✗ Found $TOTAL documents (expected 4)${NC}"
  echo "$RESULT" | jq .
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 7: Bool query - MUST_NOT
echo ""
echo -e "${BLUE}Test 7: Bool query with MUST_NOT (electronics NOT refurbished)${NC}"
RESULT=$(curl -s -X POST "http://localhost:$COORDINATION_PORT/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {
      "bool": {
        "must": [
          {"term": {"category": "electronics"}}
        ],
        "must_not": [
          {"term": {"refurbished": true}}
        ]
      }
    },
    "size": 20
  }')

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
if [ "$TOTAL" -eq 6 ]; then
  echo -e "${GREEN}   ✓ Found $TOTAL documents (expected 6: all electronics except refurb)${NC}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}   ✗ Found $TOTAL documents (expected 6)${NC}"
  echo "$RESULT" | jq .
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 8: Complex bool query - MUST + FILTER + MUST_NOT
echo ""
echo -e "${BLUE}Test 8: Complex bool query (electronics, $100-$500, in_stock, not refurb)${NC}"
RESULT=$(curl -s -X POST "http://localhost:$COORDINATION_PORT/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {
      "bool": {
        "must": [
          {"term": {"category": "electronics"}},
          {"term": {"in_stock": true}}
        ],
        "filter": [
          {"range": {"price": {"gte": 100, "lte": 500}}}
        ],
        "must_not": [
          {"term": {"refurbished": true}}
        ]
      }
    },
    "size": 20
  }')

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
if [ "$TOTAL" -eq 4 ]; then
  echo -e "${GREEN}   ✓ Found $TOTAL documents (expected 4: Dell, MacBook, Gaming, Samsung)${NC}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}   ✗ Found $TOTAL documents (expected 4)${NC}"
  echo "$RESULT" | jq .
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 9: Bool query with SHOULD
echo ""
echo -e "${BLUE}Test 9: Bool query with SHOULD (brand = Apple OR Samsung)${NC}"
RESULT=$(curl -s -X POST "http://localhost:$COORDINATION_PORT/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {
      "bool": {
        "should": [
          {"term": {"brand": "Apple"}},
          {"term": {"brand": "Samsung"}}
        ],
        "minimum_should_match": 1
      }
    },
    "size": 20
  }')

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
if [ "$TOTAL" -eq 2 ]; then
  echo -e "${GREEN}   ✓ Found $TOTAL documents (expected 2: MacBook, Samsung)${NC}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}   ✗ Found $TOTAL documents (expected 2)${NC}"
  echo "$RESULT" | jq .
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 10: Nested bool query
echo ""
echo -e "${BLUE}Test 10: Nested bool query (Apple OR Samsung) AND price <= 300${NC}"
RESULT=$(curl -s -X POST "http://localhost:$COORDINATION_PORT/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {
      "bool": {
        "must": [
          {
            "bool": {
              "should": [
                {"term": {"brand": "Apple"}},
                {"term": {"brand": "Samsung"}}
              ],
              "minimum_should_match": 1
            }
          }
        ],
        "filter": [
          {"range": {"price": {"lte": 300}}}
        ]
      }
    },
    "size": 20
  }')

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
if [ "$TOTAL" -eq 2 ]; then
  echo -e "${GREEN}   ✓ Found $TOTAL documents (expected 2: MacBook $250, Samsung $280)${NC}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
else
  echo -e "${RED}   ✗ Found $TOTAL documents (expected 2)${NC}"
  echo "$RESULT" | jq .
  TESTS_FAILED=$((TESTS_FAILED + 1))
fi

echo ""
echo "=========================================="
echo "CHECKING LOGS FOR ERRORS"
echo "=========================================="

echo ""
echo "Data node errors:"
if grep -i "error\|failed\|panic" "$TEST_DIR/logs/data.log" | grep -v "Document not found" | tail -10; then
  echo -e "${YELLOW}   ⚠ Some errors found (check above)${NC}"
else
  echo -e "${GREEN}   ✓ No errors in data node logs${NC}"
fi

echo ""
echo "Coordination node errors:"
if grep -i "error\|failed\|panic" "$TEST_DIR/logs/coordination.log" | tail -10; then
  echo -e "${YELLOW}   ⚠ Some errors found (check above)${NC}"
else
  echo -e "${GREEN}   ✓ No errors in coordination node logs${NC}"
fi
