#!/bin/bash
#
# Simple Query Latency Benchmark - Document Retrieval
# Tests document GET performance (search queries have known format issues)
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
NUM_QUERIES="${NUM_QUERIES:-1000}"
WARMUP_QUERIES="${WARMUP_QUERIES:-100}"
MAX_DOC_ID="${MAX_DOC_ID:-9999}"
RESULTS_FILE="/tmp/query_latency_results.txt"

echo "=========================================="
echo " Quidditch Query Latency Benchmark"
echo " (Document Retrieval)"
echo "=========================================="
echo ""
echo "Configuration:"
echo "  Coordination URL: $COORD_URL"
echo "  Index Name: $INDEX_NAME"
echo "  Total Queries: $NUM_QUERIES"
echo "  Warmup Queries: $WARMUP_QUERIES"
echo "  Document ID Range: 0-$MAX_DOC_ID"
echo ""

# Check if cluster is running
echo "Step 1: Checking cluster health..."
HEALTH=$(curl -s --max-time 5 "$COORD_URL/_cluster/health" 2>&1 || echo "{}")
if echo "$HEALTH" | grep -q "curl:"; then
    echo -e "${RED}✗ Cannot connect to cluster at $COORD_URL${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Cluster is reachable${NC}"
echo ""

# Check if index exists
echo "Step 2: Checking if benchmark index exists..."
DOC_CHECK=$(curl -s -o /dev/null -w "%{http_code}" "$COORD_URL/$INDEX_NAME/_doc/1" 2>&1)
if [ "$DOC_CHECK" != "200" ]; then
    echo -e "${RED}✗ Benchmark index does not exist or is empty${NC}"
    echo "  Please run benchmark_indexing.sh first"
    exit 1
fi
echo -e "${GREEN}✓ Index exists with documents${NC}"
echo ""

# Clean results file
> "$RESULTS_FILE"

# Function to execute a query and measure latency
execute_query() {
    local doc_id=$((RANDOM % (MAX_DOC_ID + 1)))
    local start=$(date +%s.%N)

    local response=$(curl -s "$COORD_URL/$INDEX_NAME/_doc/$doc_id" 2>&1)

    local end=$(date +%s.%N)
    local latency=$(echo "($end - $start) * 1000" | bc)

    # Check if query was successful
    if echo "$response" | grep -q "\"found\":true"; then
        echo "$latency"
    else
        echo "ERROR"
    fi
}

# Warmup phase
echo "Step 3: Warmup phase ($WARMUP_QUERIES queries)..."
warmup_start=$(date +%s.%N)

for i in $(seq 1 $WARMUP_QUERIES); do
    execute_query > /dev/null 2>&1
done

warmup_end=$(date +%s.%N)
warmup_duration=$(echo "$warmup_end - $warmup_start" | bc)
warmup_qps=$(echo "scale=2; $WARMUP_QUERIES / $warmup_duration" | bc)

echo -e "${GREEN}✓ Warmup complete${NC}"
echo "  Duration: ${warmup_duration}s"
echo "  QPS: ${warmup_qps} queries/sec"
echo ""

# Main benchmark
echo "Step 4: Running main benchmark ($NUM_QUERIES queries)..."
echo -e "${BLUE}Progress:${NC}"

benchmark_start=$(date +%s.%N)
progress_interval=$((NUM_QUERIES / 20))
if [ $progress_interval -eq 0 ]; then progress_interval=1; fi

for i in $(seq 1 $NUM_QUERIES); do
    latency=$(execute_query)
    if [ "$latency" != "ERROR" ]; then
        echo "$latency" >> "$RESULTS_FILE"
    fi

    # Progress indicator
    if [ $((i % progress_interval)) -eq 0 ]; then
        percent=$(echo "scale=1; ($i * 100) / $NUM_QUERIES" | bc)
        printf "\r  [%-50s] %.1f%%" $(printf '#%.0s' $(seq 1 $((i * 50 / NUM_QUERIES)))) "$percent"
    fi
done

benchmark_end=$(date +%s.%N)
echo ""
echo ""

# Calculate statistics
echo "Step 5: Calculating statistics..."

# Sort results
sort -n "$RESULTS_FILE" -o "$RESULTS_FILE"

# Count successful queries
success_count=$(wc -l < "$RESULTS_FILE")
error_count=$((NUM_QUERIES - success_count))

if [ $success_count -eq 0 ]; then
    echo -e "${RED}✗ No successful queries${NC}"
    exit 1
fi

# Calculate percentiles
p50_line=$((success_count * 50 / 100))
p95_line=$((success_count * 95 / 100))
p99_line=$((success_count * 99 / 100))

if [ $p50_line -eq 0 ]; then p50_line=1; fi
if [ $p95_line -eq 0 ]; then p95_line=1; fi
if [ $p99_line -eq 0 ]; then p99_line=1; fi

p50=$(sed -n "${p50_line}p" "$RESULTS_FILE" | awk '{printf "%.2f", $1}')
p95=$(sed -n "${p95_line}p" "$RESULTS_FILE" | awk '{printf "%.2f", $1}')
p99=$(sed -n "${p99_line}p" "$RESULTS_FILE" | awk '{printf "%.2f", $1}')
min=$(head -1 "$RESULTS_FILE" | awk '{printf "%.2f", $1}')
max=$(tail -1 "$RESULTS_FILE" | awk '{printf "%.2f", $1}')

# Calculate average
avg=$(awk '{sum+=$1} END {printf "%.2f", sum/NR}' "$RESULTS_FILE")

# Calculate throughput
total_duration=$(echo "$benchmark_end - $benchmark_start" | bc)
qps=$(echo "scale=2; $NUM_QUERIES / $total_duration" | bc)

echo ""

# Display results
echo "=========================================="
echo -e "${GREEN}BENCHMARK RESULTS${NC}"
echo "=========================================="
echo ""
echo "Test Configuration:"
echo "  Total Queries: $(printf "%'d" $NUM_QUERIES)"
echo "  Successful Queries: $(printf "%'d" $success_count)"
echo "  Failed Queries: $error_count"
echo "  Query Type: Document GET by ID"
echo ""
echo "Latency Statistics (milliseconds):"
echo "  Min:     ${min} ms"
echo "  Average: ${avg} ms"
echo "  Median:  ${p50} ms"
echo "  P95:     ${p95} ms"
echo "  P99:     ${p99} ms"
echo "  Max:     ${max} ms"
echo ""
echo "Throughput:"
echo "  Total Duration: ${total_duration}s"
echo "  Queries/Second: ${qps} qps"
echo ""

# Target comparison
TARGET_P99=100.0
if [ $(echo "$p99 < $TARGET_P99" | bc) -eq 1 ]; then
    echo -e "${GREEN}✓ PASSED: P99 latency ${p99}ms is below target of ${TARGET_P99}ms${NC}"
else
    percent_over=$(echo "scale=1; (($p99 - $TARGET_P99) * 100) / $TARGET_P99" | bc)
    echo -e "${YELLOW}⚠ Target P99 latency: ${TARGET_P99}ms${NC}"
    echo -e "${YELLOW}  Current P99: ${p99}ms (${percent_over}% over target)${NC}"
fi
echo ""

# Latency distribution
echo "Latency Distribution:"
echo "  0-10ms:     $(awk '$1 <= 10 {count++} END {print count+0}' "$RESULTS_FILE") queries"
echo "  10-50ms:    $(awk '$1 > 10 && $1 <= 50 {count++} END {print count+0}' "$RESULTS_FILE") queries"
echo "  50-100ms:   $(awk '$1 > 50 && $1 <= 100 {count++} END {print count+0}' "$RESULTS_FILE") queries"
echo "  100-200ms:  $(awk '$1 > 100 && $1 <= 200 {count++} END {print count+0}' "$RESULTS_FILE") queries"
echo "  200-500ms:  $(awk '$1 > 200 && $1 <= 500 {count++} END {print count+0}' "$RESULTS_FILE") queries"
echo "  >500ms:     $(awk '$1 > 500 {count++} END {print count+0}' "$RESULTS_FILE") queries"
echo ""

echo "=========================================="
echo ""
echo "Note: This benchmark measures document GET performance."
echo "Search query benchmarks require query format fixes (known issue)."
echo ""
echo "Raw results saved to: $RESULTS_FILE"
echo ""

exit 0
