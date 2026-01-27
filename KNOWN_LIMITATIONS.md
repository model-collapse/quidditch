# Quidditch Known Limitations

This document tracks known limitations and workarounds for the Quidditch distributed search engine.

---

## 1. Data Node Shard Loading (HIGH PRIORITY)

**Status**: 🔴 Not Implemented

**Description**: Data nodes do not automatically load shards from disk on restart.

**Technical Details**:
- Location: `pkg/data/shard.go:190-194`
- Function: `loadShards()` contains only a TODO comment
- Current behavior: Returns nil without loading any shards
- Impact: After data node restart, shards must be manually reassigned

**Code Location**:
```go
func (sm *ShardManager) loadShards() error {
    // TODO: Scan data directory and load existing shards
    sm.logger.Info("Loading shards from disk", zap.String("data_dir", sm.cfg.DataDir))
    return nil
}
```

**Impact**:
- **Severity**: HIGH - Data temporarily unavailable after restart
- **Data Loss**: None - data persists on disk
- **Workaround**: Available (see below)
- **User Facing**: Yes - affects availability

**Workaround**:
1. Avoid restarting data nodes in production
2. If restart required, master will reassign shards (manual trigger may be needed)
3. Monitor shard health after data node restart
4. Use coordination/master node restarts instead (work perfectly)

**Affected Scenarios**:
- ✅ Data node **process crash**: Restart triggers shard reassignment
- ❌ Data node **planned restart**: Requires manual intervention
- ✅ Coordination node restart: Not affected (data node continues)
- ✅ Master node restart: Not affected (data node continues)

**Testing Results**:
- Coordination node restart: ✅ All tests passed (14/14)
- Master node restart: ✅ All tests passed (14/14)
- Data node restart: ⚠️ Skipped due to this limitation

**Recommended Fix**:
```go
func (sm *ShardManager) loadShards() error {
    sm.logger.Info("Loading shards from disk", zap.String("data_dir", sm.cfg.DataDir))

    // Scan data directory for index directories
    entries, err := os.ReadDir(sm.cfg.DataDir)
    if err != nil {
        if os.IsNotExist(err) {
            sm.logger.Info("Data directory does not exist yet")
            return nil
        }
        return fmt.Errorf("failed to read data directory: %w", err)
    }

    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }

        indexName := entry.Name()
        indexPath := filepath.Join(sm.cfg.DataDir, indexName)

        // Scan for shard directories (format: shard_0, shard_1, etc.)
        shardEntries, err := os.ReadDir(indexPath)
        if err != nil {
            sm.logger.Warn("Failed to read index directory",
                zap.String("index", indexName),
                zap.Error(err))
            continue
        }

        for _, shardEntry := range shardEntries {
            if !shardEntry.IsDir() || !strings.HasPrefix(shardEntry.Name(), "shard_") {
                continue
            }

            // Extract shard ID from directory name (e.g., "shard_0" -> 0)
            shardIDStr := strings.TrimPrefix(shardEntry.Name(), "shard_")
            shardID, err := strconv.ParseInt(shardIDStr, 10, 32)
            if err != nil {
                sm.logger.Warn("Invalid shard directory name",
                    zap.String("name", shardEntry.Name()))
                continue
            }

            // Load shard from disk
            shardPath := filepath.join(indexPath, shardEntry.Name())
            shard, err := sm.loadShardFromDisk(indexName, int32(shardID), shardPath)
            if err != nil {
                sm.logger.Error("Failed to load shard",
                    zap.String("index", indexName),
                    zap.Int64("shard_id", shardID),
                    zap.Error(err))
                continue
            }

            // Register shard
            key := shardKey(indexName, int32(shardID))
            sm.shards[key] = shard
            sm.logger.Info("Loaded shard from disk",
                zap.String("index", indexName),
                zap.Int32("shard_id", int32(shardID)),
                zap.String("path", shardPath))
        }
    }

    sm.logger.Info("Shard loading complete",
        zap.Int("shards_loaded", len(sm.shards)))
    return nil
}

func (sm *ShardManager) loadShardFromDisk(indexName string, shardID int32, path string) (*Shard, error) {
    // Open Diagon shard from disk
    diagonShard, err := diagon.OpenShard(path)
    if err != nil {
        return nil, fmt.Errorf("failed to open Diagon shard: %w", err)
    }

    shard := &Shard{
        IndexName:   indexName,
        ShardID:     shardID,
        Path:        path,
        State:       ShardStateActive,
        DiagonShard: diagonShard,
        logger:      sm.logger.With(zap.String("shard", shardKey(indexName, shardID))),
    }

    return shard, nil
}
```

**Priority**: HIGH - Required for production data node restarts

**Effort Estimate**: 2-4 hours implementation + testing

---

## 2. Search Query Format Conversion (MEDIUM PRIORITY)

**Status**: 🟡 Workaround Available

**Description**: Full-text search queries fail with query format errors.

**Technical Details**:
- Query planner doesn't properly format queries for Diagon C++ engine
- Error message: "query is required" or "query execution failed"
- Affects: `match_all`, `match`, `term`, `range` queries
- Does NOT affect: Document GET by ID (works perfectly)

**Impact**:
- **Severity**: MEDIUM - User-facing search API affected
- **Data Loss**: None
- **Workaround**: Available (use document GET by ID)
- **User Facing**: Yes - affects search queries

**Workaround**:
```bash
# This fails:
curl -X POST /index/_search -d '{"query":{"match_all":{}}}'

# This works:
curl /index/_doc/document_id
```

**Testing Results**:
- Document GET by ID: ✅ P99 latency 10.34ms (excellent)
- Search queries: ❌ Format conversion issue

**Recommended Fix**: Update query planner to properly format queries for Diagon engine

**Priority**: MEDIUM - Required for full search API functionality

**Effort Estimate**: 4-8 hours investigation + implementation

---

## 3. Indexing Throughput Optimization (MEDIUM PRIORITY)

**Status**: 🟡 Performance Below Target

**Description**: Current indexing throughput is 0.16% of 50k docs/sec target.

**Technical Details**:
- Current: 82 docs/sec
- Target: 50,000 docs/sec
- Gap: 49,918 docs/sec

**Root Causes**:
1. Small batch sizes in benchmark (100 docs)
2. High per-request overhead
3. Sequential processing limitations
4. Unoptimized bulk API usage

**Impact**:
- **Severity**: MEDIUM - Performance not production-ready for high-volume indexing
- **Data Loss**: None - correctness is perfect
- **Workaround**: Use larger batches, optimize bulk API
- **User Facing**: Yes - affects indexing speed

**Mitigation**:
1. Increase batch sizes (1k-10k docs per request)
2. Implement async indexing pipeline
3. Add horizontal scaling (multiple data nodes)
4. Optimize bulk API processing
5. Enable connection pooling

**Testing Results**:
- Throughput: 82 docs/sec
- Data integrity: 100% - all documents indexed correctly
- Latency per doc: 12.11ms (reasonable)

**Priority**: MEDIUM - Required for production high-volume indexing

**Effort Estimate**: 1-2 weeks optimization work

---

## 4. Single-Node Limitations (LOW PRIORITY - BY DESIGN)

**Status**: 🟢 Expected Behavior

**Description**: Current cluster uses single master, single data node.

**Technical Details**:
- Master: Single-node Raft (designed for simplicity)
- Data: Single data node (testing/dev configuration)
- Coordination: Single node (stateless, can scale horizontally)

**Impact**:
- **Severity**: LOW - Expected for current development phase
- **High Availability**: Limited (single points of failure)
- **Scalability**: Limited (vertical scaling only)
- **User Facing**: Not directly

**Mitigation**:
- Multi-node master cluster planned for Phase 4
- Multiple data nodes supported by architecture
- Coordination nodes can be added easily (stateless)

**Priority**: LOW - Architectural expansion, not a bug

**Effort Estimate**: Phase 4 work (multi-week effort)

---

## 5. Replica Support Not Implemented (LOW PRIORITY)

**Status**: 🔴 Not Implemented

**Description**: Shards do not have replicas for redundancy.

**Technical Details**:
- Current: `number_of_replicas: 0` (forced)
- Replica API: Placeholder only
- No automatic failover
- No read load distribution

**Impact**:
- **Severity**: LOW - Single copy of data
- **Data Loss Risk**: Medium (single node failure = data unavailable)
- **Performance**: Read scalability limited
- **User Facing**: Availability during failures

**Workaround**:
- Use single shard with careful data node management
- Backup data directory regularly
- Plan data node restarts carefully

**Priority**: LOW - Nice to have, not critical for initial deployment

**Effort Estimate**: 2-3 weeks implementation

---

## Summary Table

| # | Limitation | Priority | Severity | Status | Workaround |
|---|------------|----------|----------|--------|------------|
| 1 | Data node shard loading | HIGH | HIGH | 🔴 Not implemented | Avoid data node restarts |
| 2 | Search query format | MEDIUM | MEDIUM | 🟡 Partial | Use document GET |
| 3 | Indexing throughput | MEDIUM | MEDIUM | 🟡 Below target | Optimize configuration |
| 4 | Single-node cluster | LOW | LOW | 🟢 By design | Multi-node planned |
| 5 | Replica support | LOW | MEDIUM | 🔴 Not implemented | Careful management |

---

## Production Deployment Considerations

### Safe to Deploy
- ✅ Coordination node (excellent resilience)
- ✅ Master node (good resilience)
- ✅ Query workloads (excellent performance)
- ✅ Data integrity (perfect)

### Not Recommended Yet
- ⚠️ Data node restarts (shard loading issue)
- ⚠️ High-volume indexing (throughput optimization needed)
- ⚠️ Full-text search queries (format conversion issue)

### Deployment Strategy
1. Deploy coordination nodes first (can restart freely)
2. Deploy master node (can restart with brief downtime)
3. Deploy data node carefully (avoid restarts)
4. Use document GET API primarily
5. Monitor and alert on data node health
6. Plan maintenance windows for data node updates

---

## Tracking Updates

| Date | Limitation | Action | Status |
|------|------------|--------|--------|
| 2026-01-26 | #1 Data node shard loading | Documented | Open |
| 2026-01-26 | #2 Search query format | Documented | Open |
| 2026-01-26 | #3 Indexing throughput | Documented | Open |
| 2026-01-26 | #4 Single-node cluster | Documented | Expected |
| 2026-01-26 | #5 Replica support | Documented | Future work |

---

**Last Updated**: 2026-01-26
**Next Review**: After implementing #1 (shard loading)
