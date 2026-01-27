# Document Retrieval Bug Analysis
## Date: January 27, 2026

## Summary

**Critical bug in C API document retrieval**: `diagon_reader_get_document()` always uses the first segment and doesn't convert global document IDs to segment-local IDs, causing "Document ID out of range" errors.

---

## Bug Location

**File**: `pkg/data/diagon/c_api_src/diagon_c_api.cpp`
**Function**: `diagon_reader_get_document()` (lines 937-1070)
**Problematic code**: Lines 989 and 1017

```cpp
// Line 989: ALWAYS uses first segment
diagon::index::LeafReader* leaf_reader = leaves[0].reader;

// ... later at line 1017 ...

// Line 1017: Uses GLOBAL doc_id without conversion to segment-local ID
auto fields = stored_fields_reader->document(doc_id);
```

---

## Root Cause: Lucene's Two-Level Document ID System

As you correctly described, Lucene (and Diagon) use a two-level document ID system:

### 1. Global Document ID
- User-facing document ID across the entire index
- 0-based, continuous across all segments
- Example: 0, 1, 2, 3, ...

### 2. Segment-Local Document ID
- Internal ID within a specific segment
- 0-based within each segment
- Each segment has `docBase` (starting global ID) and `maxDoc` (document count)

### Mapping Formula
```
global_doc_id = segment.docBase + segment_local_doc_id
segment_local_doc_id = global_doc_id - segment.docBase
```

### Segment Structure
From `LeafReaderContext.h` (lines 22-46):
```cpp
struct LeafReaderContext {
    LeafReader* reader;
    int docBase;  // Starting global doc ID for this segment
    int ord;      // Segment ordinal
};
```

Each segment covers doc IDs in range: `[docBase, docBase + maxDoc)`

---

## Current Behavior

### Test Case
Index 3 documents into `test_match_all`:
- doc1: `{"title": "Document One", "price": 100}`
- doc2: `{"title": "Document Two", "price": 200}`
- doc3: `{"title": "Document Three", "price": 300}`

### Actual Segment Layout (from logs)
```
[C API] Got 3 leaves
```

The index has **3 segments** (one per document committed separately):
- **Segment 0**: docBase=0, maxDoc=1, contains global doc_id 0
- **Segment 1**: docBase=1, maxDoc=1, contains global doc_id 1
- **Segment 2**: docBase=2, maxDoc=1, contains global doc_id 2

### Retrieval Attempts

#### ✅ Global doc_id 0:
```
Function receives: global_doc_id = 0
Code uses: leaves[0] (segment 0, docBase=0, maxDoc=1)
Passes to stored fields reader: doc_id = 0
Segment 0 valid range: [0, 1)
Result: SUCCESS - doc_id 0 is within segment 0's range
```

#### ❌ Global doc_id 1:
```
Function receives: global_doc_id = 1
Code uses: leaves[0] (segment 0, docBase=0, maxDoc=1)  ← WRONG SEGMENT!
Passes to stored fields reader: doc_id = 1
Segment 0 valid range: [0, 1)
Result: ERROR - "Document ID out of range: 1"
```
**Explanation**: Document 1 is actually in segment 1 (docBase=1), not segment 0!

#### ❌ Global doc_id 2:
```
Function receives: global_doc_id = 2
Code uses: leaves[0] (segment 0, docBase=0, maxDoc=1)  ← WRONG SEGMENT!
Passes to stored fields reader: doc_id = 2
Segment 0 valid range: [0, 1)
Result: ERROR - "Document ID out of range: 2"
```
**Explanation**: Document 2 is actually in segment 2 (docBase=2), not segment 0!

---

## Why This Bug Exists

The code was written with the assumption that all documents are in a single segment (segment 0), which works for:
- Fresh indexes with very few documents
- Indexes that haven't been committed/flushed yet

But fails when:
- Multiple commits create multiple segments
- Documents are spread across segments
- Attempting to retrieve any document not in the first segment

---

## The Fix Required

Replace the hardcoded segment selection with proper segment lookup:

### Current (Broken) Code
```cpp
// Always use first segment
diagon::index::LeafReader* leaf_reader = leaves[0].reader;

// Pass global doc_id directly
auto fields = stored_fields_reader->document(doc_id);
```

### Fixed Code (Required)
```cpp
// Find which segment contains the global doc_id
diagon::index::LeafReader* leaf_reader = nullptr;
int segment_local_doc_id = -1;

for (const auto& ctx : leaves) {
    int maxDoc = ctx.reader->maxDoc();

    // Check if global doc_id falls within this segment's range
    if (doc_id >= ctx.docBase && doc_id < ctx.docBase + maxDoc) {
        leaf_reader = ctx.reader;

        // Convert global ID to segment-local ID
        segment_local_doc_id = doc_id - ctx.docBase;
        break;
    }
}

if (!leaf_reader) {
    set_error("Document ID not found in any segment");
    return nullptr;
}

// Use segment-local doc_id
auto fields = stored_fields_reader->document(segment_local_doc_id);
```

---

## Impact

### Current Impact
- ❌ Only first document in index can be retrieved
- ❌ All other documents return "Document ID out of range" error
- ❌ Search results show fallback data: `{"_internal_doc_id": N}`
- ❌ Users cannot retrieve indexed documents
- ❌ Breaks all query functionality

### After Fix
- ✅ All documents can be retrieved regardless of segment
- ✅ Search results show complete `_source` with all fields
- ✅ Works with any number of segments
- ✅ Correctly implements Lucene's two-level ID system

---

## Files to Modify

1. **Primary Fix**:
   - `pkg/data/diagon/c_api_src/diagon_c_api.cpp` (lines 989-1017)
   - Function: `diagon_reader_get_document()`
   - Add segment lookup and ID conversion logic

2. **Related Functions** (may need similar fixes):
   - `diagon_reader_max_doc()` - Verify it returns total across all segments
   - Any other functions that take `doc_id` parameters

---

## Test Validation

After fix, this test should pass:

```bash
# Create index with 3 documents
curl -X PUT http://localhost:9200/test/_doc/doc1 -d '{"title":"One","price":100}'
curl -X PUT http://localhost:9200/test/_doc/doc2 -d '{"title":"Two","price":200}'
curl -X PUT http://localhost:9200/test/_doc/doc3 -d '{"title":"Three","price":300}'

# Query all documents
curl -X POST http://localhost:9200/test/_search -d '{"query":{"match_all":{}}}'

# Expected result:
{
  "hits": {
    "total": {"value": 3},
    "hits": [
      {"_id": "doc1", "_source": {"title": "One", "price": 100}},
      {"_id": "doc2", "_source": {"title": "Two", "price": 200}},
      {"_id": "doc3", "_source": {"title": "Three", "price": 300}}
    ]
  }
}
```

All three documents should have complete `_source`, not fallback data.

---

## Related Context

- **Match-all query bug**: ✅ FIXED in this session
- **Document retrieval bug**: ❌ IDENTIFIED, awaiting approval to fix
- **Your architectural insight**: ✅ CORRECT - two-level ID system is exactly the issue

---

## Conclusion

The bug is a classic Lucene multi-segment handling error. The code assumes a single-segment index but production indexes always have multiple segments. The fix is well-understood and straightforward: iterate through segments, find the right one, and convert global ID to segment-local ID before retrieval.

**Awaiting your approval to implement the fix across the repository.**
