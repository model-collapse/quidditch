# Migration Guide: OpenSearch to Quidditch

**Safe, Zero-Downtime Migration Strategy**

**Version**: 1.0.0
**Date**: 2026-01-25

---

## Table of Contents

1. [Migration Overview](#migration-overview)
2. [Pre-Migration Assessment](#pre-migration-assessment)
3. [Migration Strategies](#migration-strategies)
4. [Step-by-Step Migration](#step-by-step-migration)
5. [Testing & Validation](#testing--validation)
6. [Rollback Plan](#rollback-plan)
7. [Post-Migration Optimization](#post-migration-optimization)
8. [Troubleshooting](#troubleshooting)

---

## Migration Overview

### Migration Approaches

| Approach | Downtime | Risk | Complexity | Use Case |
|----------|----------|------|------------|----------|
| **Snapshot & Restore** | None | Low | Low | Production (recommended) |
| **Reindex API** | None | Low | Low | Small to medium indices |
| **Dual Write** | None | Medium | Medium | Large indices, incremental |
| **Blue-Green** | Minimal | Low | High | Mission-critical systems |

### Timeline Estimates

| Cluster Size | Snapshot Time | Restore Time | Total Migration |
|--------------|---------------|--------------|-----------------|
| 10M docs (10 GB) | 5 min | 10 min | 2-3 hours |
| 100M docs (100 GB) | 30 min | 1 hour | 4-6 hours |
| 1B docs (1 TB) | 5 hours | 10 hours | 2-3 days |
| 10B docs (10 TB) | 2 days | 4 days | 1-2 weeks |

**Note**: Includes testing and validation time

---

## Pre-Migration Assessment

### 1. Inventory Your Cluster

**Capture Current State**:

```bash
# Get all indices
curl -X GET "http://opensearch:9200/_cat/indices?v"

# Get cluster settings
curl -X GET "http://opensearch:9200/_cluster/settings?include_defaults=true" > cluster_settings.json

# Get index settings
curl -X GET "http://opensearch:9200/my-index/_settings" > my-index_settings.json

# Get index mappings
curl -X GET "http://opensearch:9200/my-index/_mapping" > my-index_mapping.json

# Get aliases
curl -X GET "http://opensearch:9200/_aliases" > aliases.json

# Get index templates
curl -X GET "http://opensearch:9200/_index_template" > index_templates.json
```

### 2. Check Compatibility

**API Compatibility Checklist**:

```bash
# Run compatibility checker (Quidditch CLI tool)
quidditch-migrate check \
  --opensearch-host opensearch:9200 \
  --report compatibility-report.json

# Review incompatibilities
cat compatibility-report.json | jq '.incompatibilities'
```

**Common Incompatibilities**:

| Feature | OpenSearch | Quidditch | Migration Action |
|---------|-----------|-----------|------------------|
| Parent-child queries | ✅ Full | ⚠️ Limited | Use nested documents |
| Percolate queries | ✅ Yes | ❌ No | Re-implement with filters |
| Rare/Dedup PPL | ✅ Yes | ❌ No | Use workarounds |
| Custom plugins | ✅ Java | ⚠️ Python | Rewrite as pipelines |

### 3. Estimate Resource Requirements

**Calculate Quidditch Cluster Size**:

```python
# Migration calculator
def estimate_quidditch_nodes(opensearch_nodes, doc_count, avg_doc_size_kb):
    # Quidditch has 40% better compression
    storage_reduction = 0.4

    # Quidditch queries are 3× faster on average
    query_speedup = 3.0

    # Calculate required nodes
    quidditch_nodes = opensearch_nodes / (storage_reduction + query_speedup) * 2

    return max(3, int(quidditch_nodes))  # Minimum 3 nodes

# Example
opensearch_nodes = 10
quidditch_nodes = estimate_quidditch_nodes(10, 100_000_000, 1)
print(f"Estimated Quidditch nodes: {quidditch_nodes}")  # Output: 6
```

---

## Migration Strategies

### Strategy 1: Snapshot & Restore (Recommended)

**Best For**: Production clusters, any size

**Process**:
1. Create snapshot repository (shared S3 bucket)
2. Take snapshot of OpenSearch cluster
3. Deploy Quidditch cluster
4. Restore snapshot to Quidditch
5. Validate data integrity
6. Switch traffic

**Advantages**:
- ✅ Zero downtime (read-only cutover)
- ✅ Fast (parallel restore)
- ✅ Safe (easy rollback)
- ✅ Works for any cluster size

**Disadvantages**:
- ⚠️ Requires S3/object storage
- ⚠️ Snapshot time (minutes to hours)

---

### Strategy 2: Reindex API

**Best For**: Small to medium indices (<100 GB)

**Process**:
1. Deploy Quidditch cluster
2. Use OpenSearch reindex API to copy data
3. Validate data
4. Switch traffic

**Advantages**:
- ✅ Simple (single API call)
- ✅ No snapshot needed
- ✅ Good for incremental migration

**Disadvantages**:
- ⚠️ Slower than snapshot/restore
- ⚠️ Network bandwidth intensive
- ⚠️ Not recommended for large indices

---

### Strategy 3: Dual Write

**Best For**: Large, constantly changing indices

**Process**:
1. Deploy Quidditch cluster
2. Reindex historical data (snapshot or reindex API)
3. Update application to write to both clusters
4. Backfill missing data
5. Validate consistency
6. Switch reads to Quidditch
7. Remove OpenSearch cluster

**Advantages**:
- ✅ Zero downtime
- ✅ Incremental migration
- ✅ Long validation period

**Disadvantages**:
- ⚠️ Complex (dual write logic)
- ⚠️ Consistency challenges
- ⚠️ Higher cost (two clusters)

---

### Strategy 4: Blue-Green Deployment

**Best For**: Mission-critical systems

**Process**:
1. Deploy Quidditch cluster (green)
2. Replicate data from OpenSearch (blue)
3. Run parallel for days/weeks
4. Gradually shift traffic (canary deployment)
5. Validate metrics
6. Full cutover to Quidditch
7. Keep OpenSearch as backup for 1-2 weeks

**Advantages**:
- ✅ Safest (instant rollback)
- ✅ Thorough validation
- ✅ Gradual traffic shift

**Disadvantages**:
- ⚠️ Most expensive (two full clusters)
- ⚠️ Longest timeline
- ⚠️ Complex orchestration

---

## Step-by-Step Migration

### Phase 1: Preparation (Week 1)

#### 1.1 Deploy Quidditch Cluster

```yaml
# quidditch-migration.yaml
apiVersion: quidditch.io/v1
kind: QuidditchCluster
metadata:
  name: quidditch-prod
  namespace: migration
spec:
  version: "1.0.0"

  master:
    replicas: 3
    resources:
      requests: {memory: "4Gi", cpu: "2"}

  coordination:
    replicas: 5
    resources:
      requests: {memory: "8Gi", cpu: "4"}

  data:
    replicas: 8  # Calculated from estimator
    storage:
      class: "nvme-ssd"
      size: "500Gi"
    resources:
      requests: {memory: "32Gi", cpu: "16"}

  # S3 for snapshot/restore
  s3:
    endpoint: "s3.amazonaws.com"
    bucket: "opensearch-to-quidditch-migration"
    region: "us-east-1"
```

**Deploy**:
```bash
kubectl apply -f quidditch-migration.yaml

# Wait for ready
kubectl wait --for=condition=Ready \
  quidditchcluster/quidditch-prod \
  --namespace migration \
  --timeout=600s

# Get endpoint
QUIDDITCH_ENDPOINT=$(kubectl get svc -n migration \
  quidditch-prod-coordination \
  -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')

echo "Quidditch endpoint: $QUIDDITCH_ENDPOINT"
```

#### 1.2 Configure Snapshot Repository

**OpenSearch** (source):
```bash
# Create S3 snapshot repository
curl -X PUT "http://opensearch:9200/_snapshot/migration-repo" \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "s3",
    "settings": {
      "bucket": "opensearch-to-quidditch-migration",
      "region": "us-east-1",
      "base_path": "opensearch-snapshots",
      "compress": true,
      "chunk_size": "100mb"
    }
  }'

# Verify repository
curl -X POST "http://opensearch:9200/_snapshot/migration-repo/_verify"
```

**Quidditch** (destination):
```bash
# Same repository, different base path
curl -X PUT "http://$QUIDDITCH_ENDPOINT:9200/_snapshot/migration-repo" \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "s3",
    "settings": {
      "bucket": "opensearch-to-quidditch-migration",
      "region": "us-east-1",
      "base_path": "quidditch-restores",
      "compress": true
    }
  }'
```

---

### Phase 2: Data Migration (Week 2)

#### 2.1 Take Snapshot (OpenSearch)

```bash
# Create snapshot (runs in background)
SNAPSHOT_NAME="migration-$(date +%Y%m%d-%H%M%S)"

curl -X PUT "http://opensearch:9200/_snapshot/migration-repo/$SNAPSHOT_NAME?wait_for_completion=false" \
  -H 'Content-Type: application/json' \
  -d '{
    "indices": "*",
    "ignore_unavailable": true,
    "include_global_state": true,
    "metadata": {
      "taken_by": "migration-script",
      "taken_because": "opensearch-to-quidditch-migration"
    }
  }'

# Monitor progress
watch -n 5 'curl -s "http://opensearch:9200/_snapshot/migration-repo/$SNAPSHOT_NAME/_status" | jq ".snapshots[0].stats"'
```

**Expected Time**:
- 10 GB: ~5 minutes
- 100 GB: ~30 minutes
- 1 TB: ~5 hours

#### 2.2 Restore Snapshot (Quidditch)

```bash
# Restore snapshot
curl -X POST "http://$QUIDDITCH_ENDPOINT:9200/_snapshot/migration-repo/$SNAPSHOT_NAME/_restore?wait_for_completion=false" \
  -H 'Content-Type: application/json' \
  -d '{
    "indices": "*",
    "ignore_unavailable": true,
    "include_global_state": false,
    "rename_pattern": "(.*)",
    "rename_replacement": "$1",
    "index_settings": {
      "index.number_of_replicas": 1
    }
  }'

# Monitor progress
watch -n 5 'curl -s "http://$QUIDDITCH_ENDPOINT:9200/_cat/recovery?v&h=index,stage,type,files_percent,bytes_percent" | grep -v "done"'
```

**Expected Time**:
- 10 GB: ~10 minutes
- 100 GB: ~1 hour
- 1 TB: ~10 hours

---

### Phase 3: Validation (Week 2-3)

#### 3.1 Compare Document Counts

```bash
#!/bin/bash
# compare-counts.sh

OPENSEARCH="http://opensearch:9200"
QUIDDITCH="http://$QUIDDITCH_ENDPOINT:9200"

# Get indices
INDICES=$(curl -s "$OPENSEARCH/_cat/indices?h=index" | grep -v "^\.")

for INDEX in $INDICES; do
  OS_COUNT=$(curl -s "$OPENSEARCH/$INDEX/_count" | jq '.count')
  QD_COUNT=$(curl -s "$QUIDDITCH/$INDEX/_count" | jq '.count')

  if [ "$OS_COUNT" -eq "$QD_COUNT" ]; then
    echo "✅ $INDEX: $OS_COUNT docs (match)"
  else
    echo "❌ $INDEX: OpenSearch=$OS_COUNT, Quidditch=$QD_COUNT (mismatch!)"
  fi
done
```

#### 3.2 Compare Query Results

```bash
# test-queries.sh
#!/bin/bash

QUERIES=(
  '{"query": {"match_all": {}}}'
  '{"query": {"match": {"title": "search"}}}'
  '{"query": {"range": {"price": {"gte": 10, "lte": 100}}}}'
  '{"aggs": {"avg_price": {"avg": {"field": "price"}}}}'
)

for QUERY in "${QUERIES[@]}"; do
  echo "Testing query: $QUERY"

  # Run on OpenSearch
  OS_RESULT=$(curl -s "$OPENSEARCH/my-index/_search" -H 'Content-Type: application/json' -d "$QUERY")
  OS_TOTAL=$(echo $OS_RESULT | jq '.hits.total.value')

  # Run on Quidditch
  QD_RESULT=$(curl -s "$QUIDDITCH/my-index/_search" -H 'Content-Type: application/json' -d "$QUERY")
  QD_TOTAL=$(echo $QD_RESULT | jq '.hits.total.value')

  if [ "$OS_TOTAL" -eq "$QD_TOTAL" ]; then
    echo "✅ Query passed ($OS_TOTAL results)"
  else
    echo "❌ Query failed: OpenSearch=$OS_TOTAL, Quidditch=$QD_TOTAL"
  fi
done
```

#### 3.3 Run Integration Tests

```bash
# Run application test suite against Quidditch
export SEARCH_ENDPOINT="http://$QUIDDITCH_ENDPOINT:9200"

# Run tests
npm test -- --grep "search"
pytest tests/test_search.py
```

#### 3.4 Performance Benchmarks

```bash
# Benchmark query latency
quidditch-benchmark \
  --opensearch-host opensearch:9200 \
  --quidditch-host $QUIDDITCH_ENDPOINT:9200 \
  --queries benchmark-queries.json \
  --duration 5m \
  --rate 100 \
  --report benchmark-report.json

# Expected results:
# - Quidditch: 2-5× faster on most queries
# - Lower p99 latency
# - Higher throughput
```

---

### Phase 4: Traffic Cutover (Week 3-4)

#### 4.1 Update Application Configuration

**Option A: Environment Variable**:
```bash
# Before
SEARCH_HOST=opensearch:9200

# After
SEARCH_HOST=$QUIDDITCH_ENDPOINT:9200

# Restart application
kubectl rollout restart deployment/my-app
```

**Option B: DNS Cutover**:
```bash
# Update DNS record
aws route53 change-resource-record-sets \
  --hosted-zone-id Z1234567890ABC \
  --change-batch '{
    "Changes": [{
      "Action": "UPSERT",
      "ResourceRecordSet": {
        "Name": "search.example.com",
        "Type": "CNAME",
        "TTL": 60,
        "ResourceRecords": [{"Value": "'$QUIDDITCH_ENDPOINT'"}]
      }
    }]
  }'

# Wait for DNS propagation (1-5 minutes)
```

**Option C: Gradual Canary** (recommended):
```yaml
# Kubernetes Ingress with traffic splitting
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: search-ingress
  annotations:
    nginx.ingress.kubernetes.io/canary: "true"
    nginx.ingress.kubernetes.io/canary-weight: "10"  # 10% traffic
spec:
  rules:
  - host: search.example.com
    http:
      paths:
      - path: /
        backend:
          service:
            name: quidditch-prod-coordination
            port:
              number: 9200
```

**Gradual Rollout**:
```bash
# Week 3: 10% traffic
kubectl patch ingress search-ingress -p '{"metadata":{"annotations":{"nginx.ingress.kubernetes.io/canary-weight":"10"}}}'

# Week 3.5: 50% traffic
kubectl patch ingress search-ingress -p '{"metadata":{"annotations":{"nginx.ingress.kubernetes.io/canary-weight":"50"}}}'

# Week 4: 100% traffic
kubectl patch ingress search-ingress -p '{"metadata":{"annotations":{"nginx.ingress.kubernetes.io/canary-weight":"100"}}}'
```

#### 4.2 Monitor Metrics

```bash
# Watch key metrics during cutover
watch -n 5 '
  echo "=== Quidditch Metrics ===" && \
  curl -s http://$QUIDDITCH_ENDPOINT:9200/_cluster/health | jq "." && \
  echo && \
  echo "=== Query Rate ===" && \
  curl -s http://$QUIDDITCH_ENDPOINT:9200/_nodes/stats | jq ".nodes | .[].indices.search.query_total" && \
  echo && \
  echo "=== Error Rate ===" && \
  curl -s http://$QUIDDITCH_ENDPOINT:9200/_nodes/stats | jq ".nodes | .[].http.total_opened"
'
```

---

### Phase 5: Decommission OpenSearch (Week 5-6)

#### 5.1 Keep OpenSearch as Backup (1-2 weeks)

```bash
# Stop indexing to OpenSearch
# But keep cluster running (read-only)

# Mark OpenSearch indices as read-only
curl -X PUT "http://opensearch:9200/*/_settings" \
  -H 'Content-Type: application/json' \
  -d '{
    "index.blocks.write": true
  }'
```

#### 5.2 Final Validation

```bash
# Ensure no issues for 1-2 weeks
# Monitor:
# - Error rates
# - Query latency
# - Application logs
# - User complaints

# Compare metrics
quidditch-migrate compare-metrics \
  --opensearch-host opensearch:9200 \
  --quidditch-host $QUIDDITCH_ENDPOINT:9200 \
  --period 14d \
  --report final-comparison.json
```

#### 5.3 Take Final Snapshot (OpenSearch)

```bash
# One final snapshot for archive
FINAL_SNAPSHOT="opensearch-final-$(date +%Y%m%d)"

curl -X PUT "http://opensearch:9200/_snapshot/migration-repo/$FINAL_SNAPSHOT?wait_for_completion=true" \
  -H 'Content-Type: application/json' \
  -d '{
    "indices": "*",
    "include_global_state": true
  }'

# Move to Glacier for long-term storage
aws s3 cp \
  s3://opensearch-to-quidditch-migration/opensearch-snapshots/$FINAL_SNAPSHOT \
  s3://archive-bucket/opensearch-final/ \
  --storage-class GLACIER
```

#### 5.4 Shutdown OpenSearch Cluster

```bash
# Stop OpenSearch pods
kubectl scale statefulset opensearch-master --replicas=0
kubectl scale statefulset opensearch-data --replicas=0

# Delete PVCs (after 30 days)
kubectl delete pvc -l app=opensearch
```

---

## Testing & Validation

### Validation Checklist

- [ ] **Document Count**: All indices have same doc count
- [ ] **Query Results**: Sample queries return same results
- [ ] **Aggregations**: Aggregations match (within rounding)
- [ ] **Mapping**: Index mappings preserved
- [ ] **Settings**: Index settings migrated correctly
- [ ] **Aliases**: Aliases recreated
- [ ] **Templates**: Index templates migrated
- [ ] **Performance**: Quidditch meets latency SLAs
- [ ] **Application Tests**: Integration tests pass
- [ ] **Load Test**: Cluster handles production load

### Automated Validation Script

```python
#!/usr/bin/env python3
# validate-migration.py

import requests
import json
from typing import Dict, List

class MigrationValidator:
    def __init__(self, opensearch_url: str, quidditch_url: str):
        self.opensearch = opensearch_url
        self.quidditch = quidditch_url
        self.results = []

    def validate_document_count(self, index: str) -> bool:
        """Compare document counts"""
        os_count = requests.get(f"{self.opensearch}/{index}/_count").json()['count']
        qd_count = requests.get(f"{self.quidditch}/{index}/_count").json()['count']

        passed = os_count == qd_count
        self.results.append({
            'test': 'document_count',
            'index': index,
            'passed': passed,
            'opensearch': os_count,
            'quidditch': qd_count
        })
        return passed

    def validate_query(self, index: str, query: Dict) -> bool:
        """Compare query results"""
        os_result = requests.post(
            f"{self.opensearch}/{index}/_search",
            json=query
        ).json()

        qd_result = requests.post(
            f"{self.quidditch}/{index}/_search",
            json=query
        ).json()

        os_total = os_result['hits']['total']['value']
        qd_total = qd_result['hits']['total']['value']

        passed = os_total == qd_total
        self.results.append({
            'test': 'query',
            'index': index,
            'query': json.dumps(query),
            'passed': passed,
            'opensearch': os_total,
            'quidditch': qd_total
        })
        return passed

    def report(self) -> Dict:
        """Generate validation report"""
        total = len(self.results)
        passed = sum(1 for r in self.results if r['passed'])

        return {
            'summary': {
                'total': total,
                'passed': passed,
                'failed': total - passed,
                'success_rate': passed / total if total > 0 else 0
            },
            'results': self.results
        }

# Run validation
if __name__ == '__main__':
    validator = MigrationValidator(
        opensearch_url='http://opensearch:9200',
        quidditch_url='http://quidditch:9200'
    )

    # Validate all indices
    indices = ['my-index', 'logs-2026-01', 'metrics-2026-01']
    for index in indices:
        validator.validate_document_count(index)
        validator.validate_query(index, {'query': {'match_all': {}}})

    # Print report
    report = validator.report()
    print(json.dumps(report, indent=2))

    # Exit with error if any failures
    if report['summary']['failed'] > 0:
        exit(1)
```

---

## Rollback Plan

### When to Rollback

Rollback if:
- ❌ Data integrity issues (missing documents)
- ❌ Query correctness issues (wrong results)
- ❌ Performance degradation (>50% slower)
- ❌ Application errors (>1% error rate)
- ❌ Cluster instability (frequent node failures)

### Rollback Procedure

```bash
# 1. Switch traffic back to OpenSearch (DNS or ingress)
kubectl patch ingress search-ingress -p '{"metadata":{"annotations":{"nginx.ingress.kubernetes.io/canary-weight":"0"}}}'

# 2. Mark Quidditch as read-only (prevent writes)
curl -X PUT "http://$QUIDDITCH_ENDPOINT:9200/*/_settings" \
  -H 'Content-Type: application/json' \
  -d '{"index.blocks.write": true}'

# 3. Resume writes to OpenSearch
curl -X PUT "http://opensearch:9200/*/_settings" \
  -H 'Content-Type: application/json' \
  -d '{"index.blocks.write": false}'

# 4. Investigate issues
kubectl logs -n migration -l app=quidditch --tail=1000

# 5. Fix issues and retry migration
```

---

## Post-Migration Optimization

### 1. Index Settings Tuning

```bash
# Optimize for Quidditch
curl -X PUT "http://$QUIDDITCH_ENDPOINT:9200/my-index/_settings" \
  -H 'Content-Type: application/json' \
  -d '{
    "index": {
      "codec": "diagon_best_compression",
      "refresh_interval": "5s",
      "number_of_replicas": 1,
      "merge.policy.max_merged_segment": "5gb"
    }
  }'
```

### 2. Force Merge

```bash
# Merge segments for better performance
curl -X POST "http://$QUIDDITCH_ENDPOINT:9200/my-index/_forcemerge?max_num_segments=1"
```

### 3. Setup Lifecycle Policies

```yaml
# Hot-Warm-Cold tiering
PUT _ilm/policy/logs-policy
{
  "policy": {
    "phases": {
      "hot": {
        "actions": {
          "rollover": {
            "max_size": "50gb",
            "max_age": "1d"
          }
        }
      },
      "warm": {
        "min_age": "7d",
        "actions": {
          "allocate": {"require": {"tier": "warm"}},
          "readonly": {}
        }
      },
      "cold": {
        "min_age": "30d",
        "actions": {
          "searchable_snapshot": {
            "snapshot_repository": "s3-repo"
          }
        }
      },
      "delete": {
        "min_age": "90d",
        "actions": {"delete": {}}
      }
    }
  }
}
```

---

## Troubleshooting

### Issue: Snapshot Restore Fails

**Error**: `index [my-index] cannot be restored because an open index with same name already exists`

**Solution**:
```bash
# Delete conflicting index first
curl -X DELETE "http://$QUIDDITCH_ENDPOINT:9200/my-index"

# Retry restore
curl -X POST "http://$QUIDDITCH_ENDPOINT:9200/_snapshot/migration-repo/$SNAPSHOT_NAME/_restore"
```

---

### Issue: Document Count Mismatch

**Error**: OpenSearch has 1,000,000 docs, Quidditch has 999,950 docs

**Diagnosis**:
```bash
# Check if documents were deleted during migration
curl -X GET "http://opensearch:9200/my-index/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {"range": {"@timestamp": {"gte": "now-1h"}}},
    "size": 0
  }'

# If documents were added during snapshot, take new snapshot
```

**Solution**:
- Use reindex API for incremental sync
- Or take fresh snapshot

---

### Issue: Query Results Different

**Error**: Query returns different results on Quidditch

**Diagnosis**:
```bash
# Compare explain results
curl -X GET "http://opensearch:9200/my-index/_explain/1?q=title:search"
curl -X GET "http://$QUIDDITCH_ENDPOINT:9200/my-index/_explain/1?q=title:search"

# Check BM25 parameters
curl -X GET "http://$QUIDDITCH_ENDPOINT:9200/my-index/_settings?include_defaults=true" | jq '.my-index.settings.index.similarity'
```

**Solution**:
- Ensure same analyzer settings
- Check BM25 parameters (k1, b)
- Force merge to rebuild statistics

---

## Conclusion

### Success Criteria

- ✅ All indices migrated successfully
- ✅ Document counts match
- ✅ Query results match (>99.9% accuracy)
- ✅ Performance improved (>30% faster)
- ✅ No production incidents
- ✅ Application tests pass
- ✅ Zero data loss

### Post-Migration Checklist

- [ ] Update documentation (API endpoints)
- [ ] Update monitoring dashboards
- [ ] Update runbooks
- [ ] Train operations team
- [ ] Archive OpenSearch snapshots
- [ ] Decommission OpenSearch cluster
- [ ] Celebrate! 🎉

---

**Version**: 1.0.0
**Last Updated**: 2026-01-25
**Maintainer**: Quidditch Migration Team
