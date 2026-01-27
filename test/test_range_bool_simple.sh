#!/bin/bash
# Simple test for range and boolean queries

set -e

PROJECT_ROOT="/home/ubuntu/quidditch"
DATA_DIR="/tmp/quidditch-test-rb"
LOG_DIR="$DATA_DIR/logs"

echo "=========================================="
echo "Simple Range & Boolean Query Test"
echo "=========================================="
echo ""

# Kill any existing processes
echo "1. Cleaning up old processes..."
pkill -f "quidditch-master" || true
pkill -f "quidditch-data" || true
pkill -f "quidditch-coordination" || true
sleep 3

# Setup directories
echo "2. Setting up directories..."
rm -rf "$DATA_DIR"
mkdir -p "$DATA_DIR"/{master,data,coordination,logs,config}

# Create configs
cat > "$DATA_DIR/config/master.yaml" <<EOF
node_id: "test-master"
bind_addr: "127.0.0.1"
raft_port: 9301
grpc_port: 9400
data_dir: "$DATA_DIR/master"
log_level: "debug"
metrics_port: 9401
EOF

cat > "$DATA_DIR/config/data.yaml" <<EOF
node_id: "test-data"
bind_addr: "127.0.0.1"
grpc_port: 9500
data_dir: "$DATA_DIR/data"
log_level: "debug"
metrics_port: 9501
master_addr: "127.0.0.1:9400"
simd_enabled: true
EOF

cat > "$DATA_DIR/config/coordination.yaml" <<EOF
node_id: "test-coord"
bind_addr: "127.0.0.1"
rest_port: 9200
grpc_port: 9600
data_dir: "$DATA_DIR/coordination"
log_level: "debug"
metrics_port: 9601
master_addr: "127.0.0.1:9400"
EOF

echo "   ✓ Directories and configs created"

# Start cluster
echo "3. Starting cluster..."
export LD_LIBRARY_PATH="$PROJECT_ROOT/pkg/data/diagon/build:$PROJECT_ROOT/pkg/data/diagon/upstream/build/src/core"

$PROJECT_ROOT/bin/quidditch-master --config "$DATA_DIR/config/master.yaml" \
    > "$LOG_DIR/master.log" 2>&1 &
MASTER_PID=$!
echo "   Master started (PID: $MASTER_PID)"
sleep 3

$PROJECT_ROOT/bin/quidditch-data-new --config "$DATA_DIR/config/data.yaml" \
    > "$LOG_DIR/data.log" 2>&1 &
DATA_PID=$!
echo "   Data node started (PID: $DATA_PID)"
sleep 3

$PROJECT_ROOT/bin/quidditch-coordination --config "$DATA_DIR/config/coordination.yaml" \
    > "$LOG_DIR/coordination.log" 2>&1 &
COORD_PID=$!
echo "   Coordination started (PID: $COORD_PID)"
sleep 3

echo "   ✓ Cluster started"

# Wait for cluster to be ready
echo "4. Waiting for cluster..."
sleep 5

# Check if processes are running
if ! kill -0 $MASTER_PID 2>/dev/null; then
    echo "   ✗ Master died! Logs:"
    tail -20 "$LOG_DIR/master.log"
    exit 1
fi
if ! kill -0 $DATA_PID 2>/dev/null; then
    echo "   ✗ Data node died! Logs:"
    tail -20 "$LOG_DIR/data.log"
    exit 1
fi
if ! kill -0 $COORD_PID 2>/dev/null; then
    echo "   ✗ Coordination died! Logs:"
    tail -20 "$LOG_DIR/coordination.log"
    exit 1
fi
echo "   ✓ All processes running"

# Create index
echo "5. Creating index 'products'..."
curl -s -X PUT http://localhost:9200/products \
    -H 'Content-Type: application/json' \
    -d '{
        "settings": {
            "number_of_shards": 1,
            "number_of_replicas": 0
        },
        "mappings": {
            "properties": {
                "title": {"type": "text"},
                "category": {"type": "keyword"},
                "price": {"type": "long"},
                "in_stock": {"type": "boolean"},
                "brand": {"type": "keyword"}
            }
        }
    }' > /tmp/create_resp.json

if grep -q "acknowledged.*true" /tmp/create_resp.json; then
    echo "   ✓ Index created"
else
    echo "   ✗ Index creation failed:"
    cat /tmp/create_resp.json
    exit 1
fi

# Index documents with various prices
echo "6. Indexing test documents..."
curl -s -X PUT http://localhost:9200/products/_doc/1 \
    -H 'Content-Type: application/json' \
    -d '{"title": "Budget Laptop", "category": "electronics", "price": 50, "in_stock": true, "brand": "Generic"}' \
    > /dev/null

curl -s -X PUT http://localhost:9200/products/_doc/2 \
    -H 'Content-Type: application/json' \
    -d '{"title": "Dell XPS 15", "category": "electronics", "price": 150, "in_stock": true, "brand": "Dell"}' \
    > /dev/null

curl -s -X PUT http://localhost:9200/products/_doc/3 \
    -H 'Content-Type: application/json' \
    -d '{"title": "MacBook Pro", "category": "electronics", "price": 250, "in_stock": true, "brand": "Apple"}' \
    > /dev/null

curl -s -X PUT http://localhost:9200/products/_doc/4 \
    -H 'Content-Type: application/json' \
    -d '{"title": "Samsung Laptop", "category": "electronics", "price": 280, "in_stock": true, "brand": "Samsung"}' \
    > /dev/null

echo "   ✓ Indexed 4 documents"

# Wait for indexing to complete
echo "7. Waiting for indexing to complete..."
sleep 5

# Check shard directory
echo "8. Checking shard files..."
SHARD_DIR="$DATA_DIR/data/shards/products/0"
if [ -d "$SHARD_DIR" ]; then
    echo "   Shard directory exists:"
    ls -la "$SHARD_DIR" | head -10
else
    echo "   ✗ Shard directory not found at $SHARD_DIR"
fi

# Test Range Query
echo ""
echo "=========================================="
echo "TEST 1: Range Query (150 <= price <= 280)"
echo "=========================================="
RESULT=$(curl -s -X POST http://localhost:9200/products/_search \
    -H 'Content-Type: application/json' \
    -d '{
        "query": {
            "range": {
                "price": {
                    "gte": 150,
                    "lte": 280
                }
            }
        },
        "size": 10
    }')

echo "Response:"
echo "$RESULT" | jq '.'

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
echo ""
echo "Total hits: $TOTAL (expected: 3)"

if [ "$TOTAL" -eq 3 ]; then
    echo "✓ PASS: Range query works!"
else
    echo "✗ FAIL: Expected 3 hits, got $TOTAL"
fi

# Test Boolean Query
echo ""
echo "=========================================="
echo "TEST 2: Boolean Query (category=electronics AND price<=200)"
echo "=========================================="
RESULT=$(curl -s -X POST http://localhost:9200/products/_search \
    -H 'Content-Type: application/json' \
    -d '{
        "query": {
            "bool": {
                "must": [
                    {"term": {"category": "electronics"}}
                ],
                "filter": [
                    {"range": {"price": {"lte": 200}}}
                ]
            }
        },
        "size": 10
    }')

echo "Response:"
echo "$RESULT" | jq '.'

TOTAL=$(echo "$RESULT" | jq -r '.hits.total.value // 0')
echo ""
echo "Total hits: $TOTAL (expected: 2)"

if [ "$TOTAL" -eq 2 ]; then
    echo "✓ PASS: Boolean query works!"
else
    echo "✗ FAIL: Expected 2 hits, got $TOTAL"
fi

# Cleanup
echo ""
echo "=========================================="
echo "Cleaning up..."
echo "=========================================="
kill $MASTER_PID $DATA_PID $COORD_PID 2>/dev/null || true
sleep 2

echo ""
echo "Test complete!"
echo "Logs saved to: $LOG_DIR"
