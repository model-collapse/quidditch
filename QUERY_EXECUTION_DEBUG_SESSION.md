# Query Execution Debugging Session
## Date: January 27, 2026 09:30-10:00 UTC

## Summary

Successfully fixed critical cluster startup blockers, but discovered document retrieval issues during query execution testing.

## Problems Identified

### 1. Document _source Retrieval (PARTIALLY FIXED)
**Problem**: Search results only showing `_internal_doc_id` instead of actual document fields
**Root Cause**:
- Search function had placeholder code that didn't retrieve actual documents
- Only indexed fields were created, not stored fields for retrieval

**Fixes Applied**:
1. ✅ Created `getDocumentByInternalID()` helper to retrieve documents by internal docID
2. ✅ Modified Search() to call document retrieval for each hit
3. ✅ Added stored fields for numeric values (in addition to indexed fields)
4. ✅ Updated retrieval code to parse numeric values from stored string fields
5. ✅ Added reader/searcher reopening before each search to see latest changes
6. ✅ Added commit before reopening reader

**Status**: Code changes complete, but still seeing "Document ID out of range" errors

### 2. Stale Reader Issue (IN PROGRESS)
**Problem**: Reader can't find documents that were just indexed
**Error**: "Document ID out of range: 1" (and similar)
**Root Cause**: Reader showing maxDoc=0 even after documents are indexed and committed

**Investigation**:
- Confirmed new indexing code is running (logs show "indexed+stored double field")
- Confirmed documents are being committed and refreshed after indexing
- Confirmed reader is being reopened before each search
- Issue persists: internal docIDs from search results don't exist in reader

**Hypothesis**:
- Possible segment management issue
- Search might be using cached/old segments
- Commit might not be fully flushing to disk
- Reader might need explicit refresh or different open strategy

## Code Changes Made

### Files Modified
1. **pkg/data/diagon/bridge.go**:
   - Lines 687-728: Updated Search() to retrieve actual documents
   - Lines 746-828: Created getDocumentByInternalID() helper
   - Lines 202-223: Added StoredField creation for int64 values
   - Lines 225-245: Added StoredField creation for float64 values
   - Lines 806-823: Updated numeric field retrieval to parse from strings
   - Line 13: Added "strconv" import
   - Lines 635-659: Added commit + reader/searcher reopening in Search()
   - Lines 756-762: Added maxDoc logging for debugging

## Test Results

### What Works ✅
- Cluster startup (all 3 nodes)
- Document indexing with multiple field types
- Query parsing and translation to Diagon
- Search execution (returns correct hit count)
- Partial document retrieval (title field works for some docs)

### What Doesn't Work ❌
- Consistent document retrieval (Document ID out of range errors)
- Complete _source retrieval (missing numeric fields)
- Query filtering (returns wrong documents)

### Example Test
```bash
# Index documents
curl -X PUT localhost:9200/final/_doc/A -d '{"title":"Product A","price":100}'
curl -X PUT localhost:9200/final/_doc/B -d '{"title":"Product B","price":200}'
curl -X PUT localhost:9200/final/_doc/C -d '{"title":"Product C","price":300}'
curl -X PUT localhost:9200/final/_doc/D -d '{"title":"Product D","price":400}'

# Query: price between 200-300 (should return B, C)
curl -X POST localhost:9200/final/_search -d '{"query":{"range":{"price":{"gte":200,"lte":300}}}}'

# Actual result:
{
  "hits": [
    {"_id": "doc_1", "_source": {"_internal_doc_id": 1}},  # Wrong! Fallback data
    {"_id": "doc_2", "_source": {"_internal_doc_id": 2}}   # Wrong! Fallback data
  ]
}
```

## Logs Analysis

### Indexing (Working)
```
{"msg":"DEBUG: Created indexed+stored double field","field":"price","value":100}
{"msg":"DEBUG: Created indexed+stored double field","field":"price","value":200}
```
✅ Documents are being indexed correctly with both indexed and stored fields

### Search Execution (Working)
```
{"msg":"DEBUG: Range query params","field":"price","params":{"gte":200,"lte":300}}
{"msg":"DEBUG: Creating Diagon numeric range query","field":"price","lower":200,"upper":300}
```
✅ Queries are being translated correctly to Diagon

### Document Retrieval (Failing)
```
[C API] diagon_reader_get_document called for doc_id=1
[C API] Reading document fields for doc_id=1
[C API] Exception caught: Document ID out of range: 1
{"msg":"Failed to retrieve document fields","internal_doc_id":1,"error":"...out of range: 1"}
```
❌ Reader can't find documents even though they were just indexed

## Next Steps (Recommended)

### Immediate Priority
1. **Debug maxDoc issue**:
   - Add logging to see reader's maxDoc value
   - Compare maxDoc with internal docIDs from search results
   - Check if segments are being created/merged correctly

2. **Investigate segment management**:
   - Check if Diagon is creating multiple segments
   - Verify searcher and reader are looking at same segments
   - Consider using DirectoryReader.openIfChanged() pattern

3. **Alternative approach - Don't reopen reader every time**:
   - Keep reader open, use refresh mechanism
   - Track when documents are added, only reopen when needed
   - Cache reader across searches for performance

### Testing Strategy
1. Start with single document test (eliminate timing issues)
2. Add explicit waits/sleeps after commit
3. Check Diagon index directory structure on disk
4. Compare with working GetDocument() implementation

## Performance Considerations

### Current Approach Issues
- Reopening reader/searcher on every search is expensive
- Creating fresh reader loses caches and optimization
- Not scalable for production

### Better Approach (Future)
- Use NRT (Near Real-Time) reader with periodic refresh
- Keep reader open, reopen only when needed
- Implement proper segment merging strategy

## Related Documentation

- `SESSION_STATUS_2026-01-27_E2E_PROGRESS.md` - Cluster startup fixes
- `DIAGON_ITERATOR_BUG_FIX_COMPLETE.md` - Iterator overflow bug
- `ROADMAP_STATUS.md` - Overall project status

## Time Spent

- Cluster startup debugging: 30 minutes ✅ RESOLVED
- Query execution debugging: 30 minutes ⏳ IN PROGRESS

## Conclusion

**Major Win**: Fixed critical infrastructure blockers (master node crash, data node compilation)
**Current Challenge**: Document retrieval from reader has staleness/segment issues
**Estimated Time to Fix**: 1-2 hours of focused debugging on reader/segment management

The infrastructure is solid and working. The remaining issue is a Lucene-style reader management problem that requires understanding Diagon's segment lifecycle better.
