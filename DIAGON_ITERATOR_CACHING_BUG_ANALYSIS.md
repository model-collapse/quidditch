# Diagon Iterator Caching Bug - Detailed Analysis

## Date
January 27, 2026

## Summary
Range and boolean queries fail with "Invalid docID: -2147483648" error due to NumericDocValues iterator state persistence across queries.

## Root Causes Identified

### 1. Integer Overflow in nextDoc() (PRIMARY CAUSE)
**Location**: `NumericDocValuesReader.cpp:127-133`

**Problem**: When `docID_` reaches `NO_MORE_DOCS` (INT_MAX) and `nextDoc()` is called again, it overflows to INT_MIN.

```cpp
// BUGGY CODE
int MemoryNumericDocValues::nextDoc() {
    docID_++;  // INT_MAX + 1 = INT_MIN (overflow!)
    if (docID_ >= maxDoc_) {
        docID_ = search::DocIdSetIterator::NO_MORE_DOCS;
    }
    return docID_;
}
```

**Fix Applied**:
```cpp
int MemoryNumericDocValues::nextDoc() {
    // Guard against incrementing past NO_MORE_DOCS
    if (docID_ == search::DocIdSetIterator::NO_MORE_DOCS) {
        return docID_;
    }

    docID_++;
    if (docID_ >= maxDoc_) {
        docID_ = search::DocIdSetIterator::NO_MORE_DOCS;
    }
    return docID_;
}
```

### 2. Iterator Caching Without Reset (SECONDARY CAUSE)
**Location**: `SegmentReader.cpp:84-117`

**Problem**: NumericDocValues iterators are cached and reused across queries without resetting state.

**Attempted Fix**:
- Added `reset()` method to `MemoryNumericDocValues`
- Made `reset()` virtual in base class `DocIdSetIterator`
- Call `reset()` in `SegmentReader::getNumericDocValues()` before returning cached iterator

**Status**: Fix partially works but still has issues

## Test Results

### Working
- ✅ First query in a session works correctly
- ✅ No crashes or segfaults
- ✅ Queries with both bounds (gte + lte) work on first execution

### Not Working
- ❌ Second and subsequent queries fail with "Invalid docID: -2147483644, -2147483643..." (incrementing)
- ❌ Pattern suggests iterator state is still persisting despite reset() being called

## Debugging Attempts

1. **Added debug logging** to track reset() calls
   - fprintf() output not captured in Go logs
   - Unable to verify if reset() is actually being called

2. **Removed dynamic_cast** in favor of virtual reset()
   - Made reset() virtual in DocIdSetIterator base class
   - Still fails on subsequent queries

3. **Clean rebuilds** multiple times
   - Verified library timestamps
   - Confirmed new code is being used

## Hypothesis for Remaining Issue

The incrementing pattern (-2147483644, -2147483643...) suggests one of:

1. **reset() not being called**: Despite the code change, something prevents reset() execution
2. **Multiple iterator instances**: Different iterators for same field (shouldn't happen based on cache)
3. **Scorer-level caching**: NumericRangeScorer or Weight might be cached somewhere
4. **Reader recreation**: If SegmentReader is recreated per query, cache would be empty

## Files Modified

1. `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/src/codecs/NumericDocValuesReader.cpp`
   - Added NO_MORE_DOCS guard in nextDoc()

2. `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/include/diagon/codecs/NumericDocValuesReader.h`
   - Added reset() method to MemoryNumericDocValues

3. `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/include/diagon/search/DocIdSetIterator.h`
   - Added virtual reset() method to base class

4. `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/src/index/SegmentReader.cpp`
   - Call reset() on cached iterators before returning

## Recommended Next Steps

### Option A: Complete Fresh Iterator Creation (RECOMMENDED)
Remove caching entirely and create fresh iterators for each query:

```cpp
std::unique_ptr<index::NumericDocValues> SegmentReader::getNumericDocValues(const std::string& field) const {
    // Don't cache - create fresh iterator every time
    loadDocValuesReader();

    if (docValuesReader_) {
        return docValuesReader_->getNumeric(field);
    }

    return nullptr;
}
```

**Pros**:
- Simple, no state management
- Guaranteed fresh state
- No overflow risk

**Cons**:
- Performance hit from recreating iterators
- More allocations

### Option B: Fix Weight/Scorer Caching
Investigate if Weight or Scorer instances are being cached and reused.

### Option C: Add Explicit Logging via Go
Create C API function to log from C++ through Go logger for visibility.

## Related Issues

- Original bug report: `DIAGON_UNIFIED_RANGE_QUERY_BUG.md`
- Type mismatch: `RANGE_QUERY_ROOT_CAUSE.md`
- Implementation guide: `DOUBLE_RANGE_QUERY_IMPLEMENTATION.md`

## Impact

- All range queries with single bound (gte-only or lte-only) fail after first query
- Boolean queries combining range queries fail
- First query per field works, subsequent fail
- Makes unified NumericRangeQuery unusable in production

## Workaround

Revert to commit `6e36687` (DoubleRangeQuery) which worked before unified implementation.
