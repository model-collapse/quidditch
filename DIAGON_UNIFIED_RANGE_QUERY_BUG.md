# Bug Report: Unified NumericRangeQuery Implementation

## Issue

The recently implemented unified NumericRangeQuery (commit `346d792`) has a bug that causes range queries to fail with:

```
Invalid docID: -2147483648
```

## Reproduction

```bash
# Start cluster
bash test/start_cluster.sh

# Create index and add documents
curl -X PUT http://localhost:9200/test
curl -X PUT http://localhost:9200/test/_doc/1 \
  -H 'Content-Type: application/json' \
  -d '{"id": 1, "price": 150.5}'

# Execute range query
curl -X POST http://localhost:9200/test/_search \
  -H 'Content-Type: application/json' \
  -d '{"query": {"range": {"price": {"gte": 100, "lte": 300}}}}'

# ERROR: Invalid docID: -2147483648
```

## Error Details

**Location**: `NumericDocValuesReader.cpp:longValue()`

```cpp
int64_t MemoryNumericDocValues::longValue() const {
    if (docID_ < 0 || docID_ >= maxDoc_) {
        throw std::runtime_error("Invalid docID: " + std::to_string(docID_));
    }
    return values_[docID_];
}
```

**Value**: `docID_ = -2147483648` (INT32_MIN)

**Root Cause**: The NumericRangeScorer is accessing documents with an uninitialized or invalid docID.

## Analysis

From the debug logs:

```json
{"msg":"DEBUG: Creating Diagon numeric range query","field":"price","lower":100,"upper":300}
{"msg":"DEBUG: Diagon numeric range query created successfully"}
{"msg":"DEBUG: shard.Search returned","has_result":false,"has_error":true}
{"msg":"DEBUG: Search error","error":"Invalid docID: -2147483648"}
```

The query is **created successfully**, but **search execution fails** when the scorer tries to iterate documents.

### Likely Bug Location

Based on the commit message, the unified implementation added type auto-detection in NumericRangeQuery. The bug is likely in one of these areas:

1. **NumericRangeScorer initialization** - docID_ not properly initialized to -1
2. **Field type detection** - Failing to detect field type, causing invalid iteration
3. **NumericDocValues access** - Accessing values before calling nextDoc()

### Suspect Code

In `NumericRangeQuery.cpp`, the scorer should initialize `doc_` to -1:

```cpp
NumericRangeScorer(...)
    : doc_(-1)  // Should be initialized to -1, not INT32_MIN
    , ...
```

Or the scorer might be calling `longValue()` before calling `nextDoc()`:

```cpp
// WRONG:
int64_t value = values_->longValue();  // docID_ still -1!
if (matchesRange(value)) { ... }

// CORRECT:
int doc = values_->nextDoc();
if (doc != NO_MORE_DOCS) {
    int64_t value = values_->longValue();
    if (matchesRange(value)) { ... }
}
```

## Impact

- All numeric range queries fail
- Affects both int64 and double fields
- Makes the unified implementation unusable

## Workaround

**Option 1**: Revert to previous commit before unified implementation

```bash
cd pkg/data/diagon/upstream
git checkout 6e36687  # DoubleRangeQuery implementation (before unified)
cd ../../../..
bash build_c_api.sh
go build -o bin/quidditch-data ./cmd/data
```

**Option 2**: Fix the bug in NumericRangeScorer

Check the scorer initialization and document iteration logic in:
- `NumericRangeQuery.cpp:NumericRangeScorer` constructor
- `NumericRangeQuery.cpp:nextDoc()` method
- Ensure docID is initialized to -1, not INT32_MIN

## Recommended Action

1. **Immediate**: Revert to commit `6e36687` (DoubleRangeQuery) which worked
2. **Fix**: Debug the unified implementation to fix the docID initialization bug
3. **Test**: Add comprehensive tests before merging unified implementation

## Files Affected

- `src/core/src/search/NumericRangeQuery.cpp` - Main implementation
- `src/core/src/codecs/NumericDocValuesReader.cpp` - Where error is thrown
- `src/core/include/diagon/search/NumericRangeQuery.h` - Class definition

## Related

- Original issue: `RANGE_QUERY_ROOT_CAUSE.md` - Type mismatch between double fields and int64 queries
- Implementation guide: `DOUBLE_RANGE_QUERY_IMPLEMENTATION.md` - How to implement DoubleRangeQuery
- This bug appeared when implementing the unified auto-detection approach

## Next Steps

Contact the developer who implemented commit `346d792` to:
1. Debug the scorer initialization
2. Fix the docID handling
3. Add tests for both LONG and DOUBLE field types
4. Verify the fix works for the reproduction case above
