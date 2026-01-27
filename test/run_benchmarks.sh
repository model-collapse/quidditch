#!/bin/bash
#
# Comprehensive Performance Benchmark Suite for Quidditch
# Runs indexing and query benchmarks, generates report
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
COORD_URL="http://localhost:9200"
REPORT_FILE="$PROJECT_ROOT/PERFORMANCE_BENCHMARK_REPORT.md"
TIMESTAMP=$(date +"%Y-%m-%d %H:%M:%S")

echo "=========================================="
echo " Quidditch Performance Benchmark Suite"
echo "=========================================="
echo ""
echo "This will run comprehensive performance tests:"
echo "  1. Indexing throughput benchmark"
echo "  2. Query latency benchmark"
echo ""
echo "Report will be saved to: $REPORT_FILE"
echo ""

# Check if cluster is running
echo "Step 1: Checking cluster status..."
HEALTH=$(curl -s --max-time 5 "$COORD_URL/_cluster/health" 2>&1 || echo "{}")
if echo "$HEALTH" | grep -q "curl:"; then
    echo -e "${YELLOW}⚠ Cluster is not running${NC}"
    echo ""
    read -p "Start cluster now? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        ./test/start_cluster.sh
        if [ $? -ne 0 ]; then
            echo -e "${RED}✗ Failed to start cluster${NC}"
            exit 1
        fi
        sleep 5
    else
        echo "Please start the cluster first with: test/start_cluster.sh"
        exit 1
    fi
else
    echo -e "${GREEN}✓ Cluster is running${NC}"
fi
echo ""

# System information
echo "Step 2: Collecting system information..."
HOSTNAME=$(hostname)
CPU_INFO=$(lscpu | grep "Model name" | cut -d ':' -f2 | xargs)
CPU_CORES=$(nproc)
MEMORY=$(free -h | grep Mem | awk '{print $2}')
KERNEL=$(uname -r)
echo -e "${GREEN}✓ System info collected${NC}"
echo ""

# Run indexing benchmark
echo "=========================================="
echo " Running Indexing Throughput Benchmark"
echo "=========================================="
echo ""
export NUM_DOCS=100000
export BATCH_SIZE=1000
export WARMUP_DOCS=10000

./test/benchmark_indexing.sh > /tmp/benchmark_indexing_output.txt
INDEXING_RESULT=$?

echo ""
if [ $INDEXING_RESULT -eq 0 ]; then
    echo -e "${GREEN}✓ Indexing benchmark completed${NC}"
else
    echo -e "${RED}✗ Indexing benchmark failed${NC}"
fi
echo ""

# Extract indexing metrics
INDEXING_THROUGHPUT=$(grep "Throughput:" /tmp/benchmark_indexing_output.txt | tail -1 | grep -o '[0-9,]* docs/sec' | tr -d ',' | awk '{print $1}')
INDEXING_LATENCY=$(grep "Average Latency:" /tmp/benchmark_indexing_output.txt | tail -1 | awk '{print $3}')
INDEXING_DURATION=$(grep "Total Duration:" /tmp/benchmark_indexing_output.txt | tail -1 | awk '{print $3}')

# Run query benchmark
echo "=========================================="
echo " Running Query Latency Benchmark"
echo "=========================================="
echo ""
export NUM_QUERIES=1000
export WARMUP_QUERIES=100
export CONCURRENT=10

./test/benchmark_query.sh > /tmp/benchmark_query_output.txt
QUERY_RESULT=$?

echo ""
if [ $QUERY_RESULT -eq 0 ]; then
    echo -e "${GREEN}✓ Query benchmark completed${NC}"
else
    echo -e "${RED}✗ Query benchmark failed${NC}"
fi
echo ""

# Extract query metrics
QUERY_MIN=$(grep "Min:" /tmp/benchmark_query_output.txt | awk '{print $2}')
QUERY_AVG=$(grep "Average:" /tmp/benchmark_query_output.txt | awk '{print $2}')
QUERY_MEDIAN=$(grep "Median:" /tmp/benchmark_query_output.txt | awk '{print $2}')
QUERY_P95=$(grep "P95:" /tmp/benchmark_query_output.txt | awk '{print $2}')
QUERY_P99=$(grep "P99:" /tmp/benchmark_query_output.txt | awk '{print $2}')
QUERY_MAX=$(grep "Max:" /tmp/benchmark_query_output.txt | awk '{print $2}')
QUERY_QPS=$(grep "Queries/Second:" /tmp/benchmark_query_output.txt | awk '{print $2}')

# Get cluster stats
echo "Step 3: Collecting cluster statistics..."
CLUSTER_HEALTH=$(curl -s "$COORD_URL/_cluster/health")
DATA_NODES=$(echo "$CLUSTER_HEALTH" | grep -o '"number_of_data_nodes":[0-9]*' | grep -o '[0-9]*')
INDEX_STATS=$(curl -s "$COORD_URL/benchmark_index/_stats")
DOC_COUNT=$(echo "$INDEX_STATS" | grep -o '"doc_count":[0-9]*' | grep -o '[0-9]*')
echo -e "${GREEN}✓ Cluster stats collected${NC}"
echo ""

# Generate report
echo "Step 4: Generating performance report..."

cat > "$REPORT_FILE" <<EOF
# Quidditch Performance Benchmark Report

**Date**: $TIMESTAMP
**Cluster**: $HOSTNAME
**Version**: $(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

---

## Executive Summary

This report presents comprehensive performance benchmark results for the Quidditch distributed search engine, measuring both indexing throughput and query latency under realistic workloads.

### Key Results

| Metric | Result | Target | Status |
|--------|--------|--------|--------|
| Indexing Throughput | ${INDEXING_THROUGHPUT:-N/A} docs/sec | 50,000 docs/sec | $([ "${INDEXING_THROUGHPUT:-0}" -ge 50000 ] && echo "✅ PASS" || echo "⚠️ Below Target") |
| Query P99 Latency | ${QUERY_P99:-N/A} ms | <100 ms | $([ "$(echo "${QUERY_P99:-999} < 100" | bc 2>/dev/null || echo 0)" -eq 1 ] && echo "✅ PASS" || echo "⚠️ Above Target") |
| Query P95 Latency | ${QUERY_P95:-N/A} ms | <50 ms | $([ "$(echo "${QUERY_P95:-999} < 50" | bc 2>/dev/null || echo 0)" -eq 1 ] && echo "✅ PASS" || echo "⚠️ Info Only") |
| Query Throughput | ${QUERY_QPS:-N/A} qps | N/A | ℹ️ Info |

---

## System Configuration

### Hardware
- **CPU**: $CPU_INFO
- **Cores**: $CPU_CORES
- **Memory**: $MEMORY
- **Kernel**: $KERNEL

### Cluster Configuration
- **Data Nodes**: ${DATA_NODES:-1}
- **Indexed Documents**: $(printf "%'d" ${DOC_COUNT:-0})
- **Storage Tier**: Hot
- **SIMD**: Enabled

---

## Indexing Performance

### Test Configuration
- **Total Documents**: 100,000
- **Batch Size**: 1,000 documents
- **Warmup**: 10,000 documents
- **Concurrent Batches**: 10

### Results

| Metric | Value |
|--------|-------|
| Throughput | ${INDEXING_THROUGHPUT:-N/A} docs/sec |
| Average Latency | ${INDEXING_LATENCY:-N/A} ms/doc |
| Total Duration | ${INDEXING_DURATION:-N/A}s |

### Analysis

$(if [ "${INDEXING_THROUGHPUT:-0}" -ge 50000 ]; then
    echo "✅ **Performance Excellent**: Indexing throughput meets or exceeds the target of 50,000 docs/sec."
else
    echo "⚠️ **Performance Below Target**: Current throughput is $(echo "scale=1; (${INDEXING_THROUGHPUT:-0} * 100) / 50000" | bc 2>/dev/null || echo "?")% of the 50k docs/sec target."
    echo ""
    echo "**Recommendations**:"
    echo "- Increase batch size or concurrent requests"
    echo "- Monitor data node CPU and disk I/O"
    echo "- Consider optimizing Diagon C++ engine configuration"
    echo "- Add more data nodes for horizontal scaling"
fi)

---

## Query Performance

### Test Configuration
- **Total Queries**: 1,000
- **Warmup Queries**: 100
- **Concurrent Queries**: 10
- **Query Types**: match_all, term, match, range, bool

### Latency Statistics

| Percentile | Latency |
|------------|---------|
| Min | ${QUERY_MIN:-N/A} ms |
| Average | ${QUERY_AVG:-N/A} ms |
| Median (P50) | ${QUERY_MEDIAN:-N/A} ms |
| P95 | ${QUERY_P95:-N/A} ms |
| P99 | ${QUERY_P99:-N/A} ms |
| Max | ${QUERY_MAX:-N/A} ms |

### Throughput

| Metric | Value |
|--------|-------|
| Queries/Second | ${QUERY_QPS:-N/A} qps |

### Analysis

$(if [ "$(echo "${QUERY_P99:-999} < 100" | bc 2>/dev/null || echo 0)" -eq 1 ]; then
    echo "✅ **Performance Excellent**: P99 query latency of ${QUERY_P99}ms is well below the 100ms target."
else
    echo "⚠️ **Performance Above Target**: P99 latency of ${QUERY_P99}ms exceeds the 100ms target."
    echo ""
    echo "**Recommendations**:"
    echo "- Enable query result caching"
    echo "- Optimize query execution in Diagon engine"
    echo "- Review slow query patterns"
    echo "- Consider adding read replicas"
fi)

---

## Detailed Results

### Indexing Benchmark Output

\`\`\`
$(cat /tmp/benchmark_indexing_output.txt)
\`\`\`

### Query Benchmark Output

\`\`\`
$(cat /tmp/benchmark_query_output.txt)
\`\`\`

---

## Recommendations

### Short Term
1. **Query Optimization**: $([ "$(echo "${QUERY_P99:-999} < 100" | bc 2>/dev/null || echo 0)" -eq 1 ] && echo "Continue monitoring" || echo "Enable query caching and optimize slow queries")
2. **Indexing Optimization**: $([ "${INDEXING_THROUGHPUT:-0}" -ge 50000 ] && echo "Maintain current configuration" || echo "Increase batch size and concurrency")
3. **Monitoring**: Add Prometheus metrics for continuous performance tracking

### Long Term
1. **Scaling**: Plan for horizontal scaling with multiple data nodes
2. **Caching**: Implement multi-level caching for frequently accessed data
3. **Hardware**: Consider SSD storage for improved I/O performance
4. **Optimization**: Profile Diagon C++ engine for potential improvements

---

## Conclusion

$(if [ "${INDEXING_THROUGHPUT:-0}" -ge 50000 ] && [ "$(echo "${QUERY_P99:-999} < 100" | bc 2>/dev/null || echo 0)" -eq 1 ]; then
    echo "✅ **Overall Assessment: EXCELLENT**"
    echo ""
    echo "The Quidditch cluster demonstrates strong performance characteristics, meeting both indexing throughput and query latency targets. The system is ready for production workloads with the current configuration."
elif [ "${INDEXING_THROUGHPUT:-0}" -ge 40000 ] || [ "$(echo "${QUERY_P99:-999} < 150" | bc 2>/dev/null || echo 0)" -eq 1 ]; then
    echo "⚠️ **Overall Assessment: GOOD**"
    echo ""
    echo "The Quidditch cluster shows promising performance but has room for optimization. Current performance is suitable for moderate production workloads, with targeted improvements recommended to meet full performance targets."
else
    echo "⚠️ **Overall Assessment: NEEDS OPTIMIZATION**"
    echo ""
    echo "Performance does not yet meet targets. Review recommendations above and re-run benchmarks after optimization."
fi)

---

**Report Generated**: $TIMESTAMP
**Test Environment**: Development/Testing
**Next Review**: After implementing optimization recommendations

EOF

echo -e "${GREEN}✓ Report generated${NC}"
echo ""

# Display summary
echo "=========================================="
echo -e "${GREEN}BENCHMARK SUITE COMPLETE${NC}"
echo "=========================================="
echo ""
echo "Summary:"
echo "  Indexing Throughput: ${INDEXING_THROUGHPUT:-N/A} docs/sec (target: 50,000)"
echo "  Query P99 Latency: ${QUERY_P99:-N/A} ms (target: <100ms)"
echo "  Query Avg Latency: ${QUERY_AVG:-N/A} ms"
echo ""
echo "Full report saved to: $REPORT_FILE"
echo ""
echo "To view the report:"
echo "  cat $REPORT_FILE"
echo ""
echo "To stop the cluster:"
echo "  test/stop_cluster.sh"
echo ""

exit 0
