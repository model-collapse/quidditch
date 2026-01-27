#!/bin/bash
#
# Indexing Performance Benchmark for Quidditch
# Target: 50,000 documents/second
#

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
COORD_URL="${COORD_URL:-http://localhost:9200}"
INDEX_NAME="benchmark_index"
NUM_DOCS="${NUM_DOCS:-100000}"
BATCH_SIZE="${BATCH_SIZE:-1000}"
WARMUP_DOCS="${WARMUP_DOCS:-10000}"

echo "=========================================="
echo " Quidditch Indexing Performance Benchmark"
echo "=========================================="
echo ""
echo "Configuration:"
echo "  Coordination URL: $COORD_URL"
echo "  Index Name: $INDEX_NAME"
echo "  Total Documents: $NUM_DOCS"
echo "  Batch Size: $BATCH_SIZE"
echo "  Warmup Documents: $WARMUP_DOCS"
echo ""

# Check if cluster is running
echo "Step 1: Checking cluster health..."
HEALTH=$(curl -s --max-time 5 "$COORD_URL/_cluster/health" 2>&1 || echo "{}")
if echo "$HEALTH" | grep -q "curl:"; then
    echo -e "${RED}✗ Cannot connect to cluster at $COORD_URL${NC}"
    echo "  Please start the cluster first with: test/e2e_test.sh"
    exit 1
fi
echo -e "${GREEN}✓ Cluster is reachable${NC}"
echo ""

# Delete existing benchmark index if it exists
echo "Step 2: Cleaning up existing benchmark index..."
curl -s -X DELETE "$COORD_URL/$INDEX_NAME" > /dev/null 2>&1 || true
sleep 1
echo -e "${GREEN}✓ Cleanup complete${NC}"
echo ""

# Create index with optimized settings
echo "Step 3: Creating benchmark index..."
CREATE_RESPONSE=$(curl -s -X PUT "$COORD_URL/$INDEX_NAME" \
    -H 'Content-Type: application/json' \
    -d '{
        "settings": {
            "number_of_shards": 1,
            "number_of_replicas": 0,
            "refresh_interval": "30s"
        },
        "mappings": {
            "properties": {
                "id": {"type": "keyword"},
                "title": {"type": "text"},
                "description": {"type": "text"},
                "price": {"type": "float"},
                "category": {"type": "keyword"},
                "tags": {"type": "keyword"},
                "timestamp": {"type": "long"},
                "metadata": {
                    "type": "object",
                    "properties": {
                        "author": {"type": "keyword"},
                        "version": {"type": "keyword"}
                    }
                }
            }
        }
    }' 2>&1)

if echo "$CREATE_RESPONSE" | grep -qiE "(acknowledged|created|true)"; then
    echo -e "${GREEN}✓ Index created successfully${NC}"
else
    echo -e "${RED}✗ Failed to create index: $CREATE_RESPONSE${NC}"
    exit 1
fi
sleep 2
echo ""

# Generate and index documents
echo "Step 4: Generating test documents..."

# Function to generate a batch of documents
generate_batch() {
    local start_id=$1
    local count=$2
    local batch=""

    for i in $(seq 0 $((count - 1))); do
        local doc_id=$((start_id + i))
        local doc=$(cat <<EOF
{"index":{"_index":"$INDEX_NAME","_id":"$doc_id"}}
{"id":"doc_$doc_id","title":"Product $doc_id","description":"This is a test product description for benchmarking purposes. It contains enough text to be realistic. Product ID: $doc_id","price":$(awk -v min=9.99 -v max=999.99 'BEGIN{srand(); print min+rand()*(max-min)}'),"category":"category_$((doc_id % 10))","tags":["tag1","tag2","tag3"],"timestamp":$(date +%s),"metadata":{"author":"benchmarker","version":"1.0"}}
EOF
)
        batch="${batch}${doc}"$'\n'
    done

    echo -n "$batch"
}

# Function to index a batch
index_batch() {
    local batch_data=$1
    local temp_file=$(mktemp)
    echo -n "$batch_data" > "$temp_file"
    curl -s -X POST "$COORD_URL/$INDEX_NAME/_bulk" \
        -H 'Content-Type: application/x-ndjson' \
        --data-binary "@$temp_file" > /dev/null
    local result=$?
    rm -f "$temp_file"
    return $result
}

# Warmup phase
echo "Step 5: Warmup phase ($WARMUP_DOCS documents)..."
warmup_start=$(date +%s.%N)
warmup_batches=$((WARMUP_DOCS / BATCH_SIZE))

for batch_num in $(seq 0 $((warmup_batches - 1))); do
    start_id=$((batch_num * BATCH_SIZE))
    batch_data=$(generate_batch $start_id $BATCH_SIZE)
    index_batch "$batch_data" &

    # Limit concurrency to 10 batches
    if [ $(( (batch_num + 1) % 10 )) -eq 0 ]; then
        wait
    fi
done
wait

warmup_end=$(date +%s.%N)
warmup_duration=$(echo "$warmup_end - $warmup_start" | bc)
warmup_throughput=$(echo "scale=2; $WARMUP_DOCS / $warmup_duration" | bc)

echo -e "${GREEN}✓ Warmup complete${NC}"
echo "  Duration: ${warmup_duration}s"
echo "  Throughput: ${warmup_throughput} docs/sec"
echo ""

# Main benchmark
echo "Step 6: Running main benchmark ($NUM_DOCS documents)..."
echo -e "${BLUE}Progress:${NC}"

benchmark_start=$(date +%s.%N)
total_batches=$((NUM_DOCS / BATCH_SIZE))
progress_interval=$((total_batches / 20))  # Update progress 20 times

for batch_num in $(seq 0 $((total_batches - 1))); do
    start_id=$((WARMUP_DOCS + batch_num * BATCH_SIZE))
    batch_data=$(generate_batch $start_id $BATCH_SIZE)
    index_batch "$batch_data" &

    # Limit concurrency
    if [ $(( (batch_num + 1) % 10 )) -eq 0 ]; then
        wait
    fi

    # Progress indicator
    if [ $progress_interval -gt 0 ] && [ $(( (batch_num + 1) % progress_interval )) -eq 0 ]; then
        percent=$(echo "scale=1; (($batch_num + 1) * 100) / $total_batches" | bc)
        printf "\r  [%-50s] %.1f%%" $(printf '#%.0s' $(seq 1 $((batch_num * 50 / total_batches)))) "$percent"
    fi
done
wait

benchmark_end=$(date +%s.%N)
echo ""
echo ""

# Calculate results
total_duration=$(echo "$benchmark_end - $benchmark_start" | bc)
throughput=$(echo "scale=2; $NUM_DOCS / $total_duration" | bc)
avg_latency=$(echo "scale=2; ($total_duration * 1000) / $NUM_DOCS" | bc)
batch_latency=$(echo "scale=2; ($total_duration * 1000) / $total_batches" | bc)

# Get index stats
echo "Step 7: Retrieving index statistics..."
STATS=$(curl -s "$COORD_URL/$INDEX_NAME/_stats" 2>&1)
sleep 1
echo ""

# Display results
echo "=========================================="
echo -e "${GREEN}BENCHMARK RESULTS${NC}"
echo "=========================================="
echo ""
echo "Test Configuration:"
echo "  Total Documents: $(printf "%'d" $NUM_DOCS)"
echo "  Batch Size: $BATCH_SIZE"
echo "  Total Batches: $total_batches"
echo "  Concurrent Batches: 10"
echo ""
echo "Performance Metrics:"
echo "  Total Duration: ${total_duration}s"
echo "  Throughput: $(printf "%'d" ${throughput%.*}) docs/sec"
echo "  Average Latency: ${avg_latency} ms/doc"
echo "  Batch Latency: ${batch_latency} ms/batch"
echo ""

# Target comparison
TARGET_THROUGHPUT=50000
if [ ${throughput%.*} -ge $TARGET_THROUGHPUT ]; then
    echo -e "${GREEN}✓ PASSED: Throughput exceeds target of $(printf "%'d" $TARGET_THROUGHPUT) docs/sec${NC}"
else
    percent_of_target=$(echo "scale=1; (${throughput%.*} * 100) / $TARGET_THROUGHPUT" | bc)
    echo -e "${YELLOW}⚠ Target throughput: $(printf "%'d" $TARGET_THROUGHPUT) docs/sec${NC}"
    echo -e "${YELLOW}  Current: ${percent_of_target}% of target${NC}"
fi
echo ""

# Index size and stats
if echo "$STATS" | grep -q "doc_count"; then
    doc_count=$(echo "$STATS" | grep -o '"doc_count":[0-9]*' | head -1 | grep -o '[0-9]*')
    echo "Index Statistics:"
    echo "  Documents Indexed: $(printf "%'d" $doc_count)"
    echo ""
fi

echo "=========================================="
echo ""
echo "Recommendations:"
echo "  - For higher throughput, increase batch size or concurrency"
echo "  - Monitor data node CPU and memory usage"
echo "  - Check Diagon C++ engine performance metrics"
echo "  - Consider adding more data nodes for horizontal scaling"
echo ""
echo "Cleanup:"
echo "  To remove the benchmark index, run:"
echo "  curl -X DELETE $COORD_URL/$INDEX_NAME"
echo ""

exit 0
