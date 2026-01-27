# Shard State Fix - Search Returns 0 Results

## Problem

All search queries were returning 0 results even though documents were successfully indexed. The issue was in the query execution path:

1. Documents were indexed successfully on data nodes
2. Searches reached the coordination node
3. **QueryExecutor skipped all shards** because they were not in `SHARD_STATE_STARTED`
4. No queries were sent to data nodes
5. Searches returned 0 results

## Root Cause

In `pkg/master/master.go`, the `createShardOnDataNode()` function:
- Successfully created shards on data nodes
- Logged "Successfully created shard on data node"
- **BUT NEVER UPDATED THE SHARD STATE FROM "initializing" TO "started"**

The `QueryExecutor` in `pkg/coordination/executor/executor.go` has this check:

```go
if shard.Allocation == nil || shard.Allocation.State != pb.ShardAllocation_SHARD_STATE_STARTED {
    qe.logger.Debug("Skipping shard - not started", ...)
    continue  // SKIPS THE SHARD!
}
```

Since shards stayed in "initializing" state forever, the executor skipped them all.

## Fix Applied

Updated `pkg/master/master.go` line 401-412 to add state update after successful shard creation:

```go
if resp.Acknowledged {
    m.logger.Info("Successfully created shard on data node", ...)

    // CRITICAL FIX: Update shard state to STARTED so executor can query it
    m.logger.Info("Updating shard state to STARTED", ...)

    // Update shard state to "started" through Raft
    updateRouting := raft.ShardRouting{
        IndexName: indexName,
        ShardID:   shardID,
        NodeID:    nodeID,
        State:     "started",
        Version:   2,
    }

    payload, err := json.Marshal(updateRouting)
    if err != nil {
        m.logger.Error("Failed to marshal shard state update", ...)
        return
    }

    cmd := raft.Command{
        Type:    raft.CommandUpdateShard,
        Payload: payload,
    }

    if err := m.raftNode.Apply(cmd, 5*time.Second); err != nil {
        m.logger.Error("Failed to update shard state to STARTED", ...)
        return
    }

    m.logger.Info("Shard state updated to STARTED - now searchable", ...)
}
```

## How It Works

1. Master allocates shard with state="initializing"
2. Master calls `createShardOnDataNode()` in background goroutine
3. Data node creates shard and returns success
4. **NEW:** Master updates shard state to "started" via Raft
5. QueryExecutor can now query the shard (passes state check)
6. Searches work!

## Verification

After applying this fix:
1. Shards transition from "initializing" → "started"
2. `QueryExecutor.ExecuteSearch()` includes shards in query loop
3. Searches reach data nodes
4. Results are returned

## Files Modified

- `pkg/master/master.go` - Added shard state update in `createShardOnDataNode()`

## Related Files

- `pkg/coordination/executor/executor.go` - Executor that checks shard state
- `pkg/master/raft/fsm.go` - FSM that handles `CommandUpdateShard`
- `pkg/common/proto/master.proto` - Shard state enum definition

## Testing

Run the range/boolean query test:
```bash
./test/test_range_bool_queries.sh
```

Expected: All 10 tests should pass with documents being found.

## Remaining Work

The indexed numeric field fix (using `diagon_create_indexed_long_field` instead of `diagon_create_long_field`) is also needed for range queries on numeric fields to work properly. Both fixes together should make all tests pass.
