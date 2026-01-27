#!/bin/bash
#
# Query Latency Performance Benchmark for Quidditch
# Target: <100ms p99 latency
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
CONCURRENT="${CONCURRENT:-10}"
RESULTS_FILE="/tmp/query_latency_results.txt"

echo "=========================================="
echo " Quidditch Query Latency Benchmark"
echo "=========================================="
echo ""
echo "Configuration:"
echo "  Coordination URL: $COORD_URL"
echo "  Index Name: $INDEX_NAME"
echo "  Total Queries: $NUM_QUERIES"
echo "  Warmup Queries: $WARMUP_QUERIES"
echo "  Concurrent Queries: $CONCURRENT"
echo ""

# Check if cluster is running
echo "Step 1: Checking cluster health..."
HEALTH=$(curl -s --max-time 5 "$COORD_URL/_cluster/health" 2>&1 || echo "{}")
if echo "$HEALTH" | grep -q "curl:"; then
    echo -e "${RED}✗ Cannot connect to cluster at $COORD_URL${NC}"
    echo "  Please start the cluster first"
    exit 1
fi
echo -e "${GREEN}✓ Cluster is reachable${NC}"
echo ""

# Check if index exists
echo "Step 2: Checking if benchmark index exists..."
INDEX_EXISTS=$(curl -s -o /dev/null -w "%{http_code}" "$COORD_URL/$INDEX_NAME" 2>&1)
if [ "$INDEX_EXISTS" != "200" ]; then
    echo -e "${RED}✗ Benchmark index does not exist${NC}"
    echo "  Please run benchmark_indexing.sh first to create test data"
    exit 1
fi
echo -e "${GREEN}✓ Index exists${NC}"
echo ""

# Get index stats
echo "Step 3: Retrieving index statistics..."
STATS=$(curl -s "$COORD_URL/$INDEX_NAME/_stats" 2>&1)
DOC_COUNT=$(echo "$STATS" | grep -o '"doc_count":[0-9]*' | head -1 | grep -o '[0-9]*' || echo "0")
echo "  Documents in index: $(printf "%'d" $DOC_COUNT)"
echo ""

# Prepare test queries
echo "Step 4: Preparing test queries..."

# Array of query templates
declare -a QUERY_TYPES=(
    "match_all"
    "term"
    "match"
    "range"
    "bool"
)

# Function to generate a random query
generate_query() {
    local query_type=${QUERY_TYPES[$RANDOM % ${#QUERY_TYPES[@]}]}
    local random_term=$((RANDOM % 10))
    local random_id=$((RANDOM % DOC_COUNT))
    local random_price=$(awk -v min=10 -v max=500 'BEGIN{srand(); print int(min+rand()*(max-min))}')

    case $query_type in
        "match_all")
            echo '{"query":{"match_all":{}},"size":10}'
            ;;
        "term")
            echo "{\"query\":{\"term\":{\"category\":\"category_$random_term\"}},\"size\":10}"
            ;;
        "match")
            echo "{\"query\":{\"match\":{\"description\":\"product test\"}},\"size\":10}"
            ;;
        "range")
            echo "{\"query\":{\"range\":{\"price\":{\"gte\":$random_price}}},\"size\":10}"
            ;;
        "bool")
            echo "{\"query\":{\"bool\":{\"must\":[{\"term\":{\"category\":\"category_$random_term\"}},{\"range\":{\"price\":{\"gte\":$random_price}}}]}},\"size\":10}"
            ;;
    esac
}

# Function to execute a query and measure latency
execute_query() {
    local query=$1
    local start=$(date +%s.%N)

    local response=$(curl -s -X POST "$COORD_URL/$INDEX_NAME/_search" \
        -H 'Content-Type: application/json' \
        -d "$query" 2>&1)

    local end=$(date +%s.%N)
    local latency=$(echo "($end - $start) * 1000" | bc)

    # Check if query was successful
    if echo "$response" | grep -q "error"; then
        echo "ERROR"
    else
        echo "$latency"
    fi
}

# Clean results file
> "$RESULTS_FILE"

# Warmup phase
echo "Step 5: Warmup phase ($WARMUP_QUERIES queries)..."
warmup_start=$(date +%s.%N)

for i in $(seq 1 $WARMUP_QUERIES); do
    query=$(generate_query)
    execute_query "$query" > /dev/null 2>&1 &

    # Limit concurrency
    if [ $((i % CONCURRENT)) -eq 0 ]; then
        wait
    fi
done
wait

warmup_end=$(date +%s.%N)
warmup_duration=$(echo "$warmup_end - $warmup_start" | bc)
warmup_qps=$(echo "scale=2; $WARMUP_QUERIES / $warmup_duration" | bc)

echo -e "${GREEN}✓ Warmup complete${NC}"
echo "  Duration: ${warmup_duration}s"
echo "  QPS: ${warmup_qps} queries/sec"
echo ""

# Main benchmark
echo "Step 6: Running main benchmark ($NUM_QUERIES queries)..."
echo -e "${BLUE}Progress:${NC}"

benchmark_start=$(date +%s.%N)
progress_interval=$((NUM_QUERIES / 20))

for i in $(seq 1 $NUM_QUERIES); do
    query=$(generate_query)
    (
        latency=$(execute_query "$query")
        if [ "$latency" != "ERROR" ]; then
            echo "$latency" >> "$RESULTS_FILE"
        fi
    ) &

    # Limit concurrency
    if [ $((i % CONCURRENT)) -eq 0 ]; then
        wait
    fi

    # Progress indicator
    if [ $progress_interval -gt 0 ] && [ $((i % progress_interval)) -eq 0 ]; then
        percent=$(echo "scale=1; ($i * 100) / $NUM_QUERIES" | bc)
        printf "\r  [%-50s] %.1f%%" $(printf '#%.0s' $(seq 1 $((i * 50 / NUM_QUERIES)))) "$percent"
    fi
done
wait

benchmark_end=$(date +%s.%N)
echo ""
echo ""

# Calculate statistics
echo "Step 7: Calculating statistics..."

# Sort results
sort -n "$RESULTS_FILE" -o "$RESULTS_FILE"

# Count successful queries
success_count=$(wc -l < "$RESULTS_FILE")
error_count=$((NUM_QUERIES - success_count))

if [ $success_count -eq 0 ]; then
    echo -e "${RED}✗ No successful queries${NC}"
    echo "  All queries failed. Check cluster status and logs."
    exit 1
fi

# Calculate percentiles
p50_line=$((success_count * 50 / 100))
p95_line=$((success_count * 95 / 100))
p99_line=$((success_count * 99 / 100))

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
echo "  Concurrent Queries: $CONCURRENT"
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
p99_int=${p99%.*}
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
echo "Recommendations:"
echo "  - For lower latency, enable query caching"
echo "  - Monitor data node CPU and disk I/O"
echo "  - Consider adding read replicas for query load distribution"
echo "  - Review slow queries in logs"
echo ""
echo "Raw results saved to: $RESULTS_FILE"
echo ""

exit 0
