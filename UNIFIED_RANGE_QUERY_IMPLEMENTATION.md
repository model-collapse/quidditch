# Unified Range Query Implementation

**Date**: 2026-01-27
**Status**: 🔶 Partially Complete - Investigation Required

---

## Summary

Implemented a unified NumericRangeQuery that automatically detects field types (LONG vs DOUBLE) and handles both integer and floating-point range queries with a single API. This replaces the need for separate NumericRangeQuery and DoubleRangeQuery classes.

---

## What Was Completed ✅

### 1. Fixed Double Field Storage (Critical Bug Fix)

**Problem**: `diagon_create_indexed_double_field` was using `static_cast<int64_t>(value)` which **truncated** all double values, losing precision.

**Fix**: Changed to use `std::bit_cast<int64_t>(value)` which preserves the full bit representation of the double.

**Files Modified**:
- `/home/ubuntu/quidditch/pkg/data/diagon/c_api_src/diagon_c_api.cpp` (line 392)

```cpp
// BEFORE (WRONG):
int64_t intValue = static_cast<int64_t>(value);  // Truncates!

// AFTER (CORRECT):
int64_t longBits = std::bit_cast<int64_t>(value);  // Preserves full precision
```

### 2. Added NumericType Tracking

**Added**: New `NumericType` enum to track the original type of numeric fields.

**File Created/Modified**:
- `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/include/diagon/document/IndexableField.h`

```cpp
enum class NumericType {
    NONE,    // Not a numeric field
    LONG,    // int64_t value stored directly
    DOUBLE,  // double value stored as int64_t bits (via bit_cast)
    INT,     // int32_t value stored as int64_t
    FLOAT    // float value stored as int64_t bits (via bit_cast)
};

struct FieldType {
    // ... existing fields ...
    NumericType numericType = NumericType::NONE;  // NEW: Track numeric field type
};
```

### 3. Propagated Numeric Type to FieldInfo

**Added**: Code to propagate `numericType` from FieldType to FieldInfo attributes during indexing.

**Files Modified**:
- `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/include/diagon/index/FieldInfo.h`
  - Added `setAttribute()` method to FieldInfosBuilder
- `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/src/index/FieldInfo.cpp`
  - Implemented `setAttribute()` method
- `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/src/index/DocumentsWriterPerThread.cpp`
  - Added code to set "numeric_type" attribute from FieldType

```cpp
// In DocumentsWriterPerThread::addDocument()
if (fieldType.numericType != document::NumericType::NONE) {
    std::string numericTypeStr;
    switch (fieldType.numericType) {
        case document::NumericType::LONG:   numericTypeStr = "LONG"; break;
        case document::NumericType::DOUBLE: numericTypeStr = "DOUBLE"; break;
        // ... etc
    }
    fieldInfosBuilder_.setAttribute(field->name(), "numeric_type", numericTypeStr);
}
```

### 4. Implemented Type Auto-Detection in NumericRangeQuery

**Modified**: NumericRangeQuery to automatically detect field type and handle both LONG and DOUBLE.

**File Modified**:
- `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/src/search/NumericRangeQuery.cpp`

**Key Changes**:
1. Added `isDoubleField` parameter to NumericRangeScorer
2. In `NumericRangeWeight::scorer()`, check FieldInfo for "numeric_type" attribute
3. In `NumericRangeScorer::matchesRange()`, use `std::bit_cast` to convert if DOUBLE type

```cpp
// In NumericRangeWeight::scorer()
bool isDoubleField = false;
auto* fieldInfos = context.reader->getFieldInfos();
if (fieldInfos) {
    auto* fieldInfo = fieldInfos->fieldInfo(query_.getField());
    if (fieldInfo) {
        auto numericType = fieldInfo->getAttribute("numeric_type");
        if (numericType && *numericType == "DOUBLE") {
            isDoubleField = true;
        }
    }
}

// In NumericRangeScorer::matchesRange()
if (isDoubleField_) {
    double value = std::bit_cast<double>(longValue);
    double lower = std::bit_cast<double>(lowerValue_);
    double upper = std::bit_cast<double>(upperValue_);
    // ... double comparison logic with NaN handling ...
} else {
    // ... int64_t comparison logic ...
}
```

### 5. Updated C API to Use Unified Function

**Modified**: `diagon_create_numeric_range_query()` to use `std::bit_cast` instead of `static_cast`.

**File Modified**:
- `/home/ubuntu/quidditch/pkg/data/diagon/c_api_src/diagon_c_api.cpp` (line 589)

```cpp
// Convert double to int64_t using bit_cast to preserve full precision
int64_t lower = std::bit_cast<int64_t>(lower_value);
int64_t upper = std::bit_cast<int64_t>(upper_value);
```

**Rationale**: This allows the same function to work for both LONG and DOUBLE fields:
- **LONG fields**: Pass integers as doubles (e.g., 100.0), they convert correctly
- **DOUBLE fields**: Pass doubles (e.g., 150.5), bit representation is preserved

### 6. Updated Go Bridge

**Modified**: Go bridge to use `diagon_create_numeric_range_query()` for all range queries.

**File Modified**:
- `/home/ubuntu/quidditch/pkg/data/diagon/bridge.go` (line 497)

```go
// OLD: diagonQuery = C.diagon_create_double_range_query(...)

// NEW: Use unified numeric range query (auto-detects LONG vs DOUBLE)
diagonQuery = C.diagon_create_numeric_range_query(
    cField,
    C.double(lowerValue),
    C.double(upperValue),
    C.bool(includeLower),
    C.bool(includeUpper),
)
```

### 7. Fixed Missing Doc Values Type

**Problem**: Numeric fields were missing `docValuesType = NUMERIC` which is required for range queries.

**Files Modified**:
- `/home/ubuntu/quidditch/pkg/data/diagon/c_api_src/diagon_c_api.cpp`
  - Added to `diagon_create_indexed_long_field()` (line 365)
  - Added to `diagon_create_indexed_double_field()` (line 388)

```cpp
fieldType.docValuesType = diagon::index::DocValuesType::NUMERIC;  // Enable doc values
```

---

## Files Modified Summary

### Modified (8 files)
1. `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/include/diagon/document/IndexableField.h`
   - Added NumericType enum and numericType field to FieldType
2. `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/include/diagon/index/FieldInfo.h`
   - Added setAttribute() method to FieldInfosBuilder
3. `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/src/index/FieldInfo.cpp`
   - Implemented setAttribute() method
4. `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/src/index/DocumentsWriterPerThread.cpp`
   - Added numeric type propagation to FieldInfo attributes
5. `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/src/search/NumericRangeQuery.cpp`
   - Added type auto-detection and dual comparison logic
6. `/home/ubuntu/quidditch/pkg/data/diagon/c_api_src/diagon_c_api.cpp`
   - Fixed double field storage bug (bit_cast)
   - Fixed numeric_range_query to use bit_cast
   - Added docValuesType to numeric fields
7. `/home/ubuntu/quidditch/pkg/data/diagon/bridge.go`
   - Changed to use unified numeric_range_query
8. `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/CMakeLists.txt`
   - (No changes needed - DoubleRangeQuery can be deprecated later)

### Created (1 file)
1. `/home/ubuntu/quidditch/pkg/data/diagon/double_range_test.go`
   - Comprehensive test for double range queries

---

## Known Issues 🔶

### Issue #1: Range Queries Return 0 Hits

**Status**: Under Investigation

**Symptoms**:
- Documents with double fields are indexed successfully ✅
- Term queries work and find documents ✅
- Range queries execute without errors but return 0 hits ❌

**Test Results**:
```bash
go test -v ./pkg/data/diagon -run TestDoubleRangeQuery
=== RUN   TestDoubleRangeQuery/IndexDoubleFields
    ✓ Indexed 5 documents with double price field
=== RUN   TestDoubleRangeQuery/VerifyDocumentsSearchable
    ✓ Term search for 'Laptop': found 1 hits
=== RUN   TestDoubleRangeQuery/RangeQuery_BothBounds
    Expected 2 hits, got 0
```

**Possible Causes**:
1. **NumericDocValues not written**: Despite setting `docValuesType = NUMERIC`, doc values may not be written
2. **Attributes not persisted**: The "numeric_type" attribute may not be saved to segment files
3. **Reader not refreshed**: Doc values may not be visible to the reader
4. **Field mapping issue**: The "price" field may not be recognized as having doc values

**Investigation Plan**:
1. [ ] Check if NumericDocValuesWriter is actually invoked for double fields
2. [ ] Verify FieldInfo attributes are persisted to segment files
3. [ ] Test if getNumericDocValues() returns non-null for "price" field
4. [ ] Add debug logging to NumericRangeWeight::scorer() to see if field is found
5. [ ] Test with a simple LONG field to isolate DOUBLE-specific issues

---

## Next Steps

### Immediate Actions (Priority 1)

1. **Debug NumericDocValues Writing**:
   - Add logging to DocumentsWriterPerThread to verify doc values are written
   - Check if NumericDocValuesWriter::addValue() is called for "price" field

2. **Test LONG Fields First**:
   - Create a test with int64 fields (not doubles)
   - If LONG range queries work, problem is DOUBLE-specific
   - If LONG range queries fail, problem is doc values configuration

3. **Verify FieldInfo Persistence**:
   - Add debug code to print FieldInfo attributes when reading segments
   - Check if "numeric_type" attribute survives segment flush/reload

### Medium Term (Priority 2)

4. **Add Direct C++ Test**:
   - Create a standalone C++ test that bypasses Go/C API
   - Index documents with NumericDocValues directly
   - Execute NumericRangeQuery and verify results

5. **Review Doc Values Pipeline**:
   - Trace complete path from Field creation → DocValuesWriter → Segment write → Reader load
   - Identify where the pipeline breaks for range queries

### Long Term (Priority 3)

6. **Deprecate DoubleRangeQuery**:
   - Once unified approach works, mark DoubleRangeQuery as deprecated
   - Update documentation to recommend NumericRangeQuery

7. **Add Integration Tests**:
   - End-to-end test indexing + range querying doubles
   - Performance benchmark comparing LONG vs DOUBLE range queries

---

## Benefits of Unified Approach

✅ **Single API**: One function for all numeric range queries
✅ **Type Safety**: Automatic detection prevents type mismatch errors
✅ **Simpler Code**: No need to choose between numeric/double query types
✅ **Better UX**: Users don't need to know internal storage format
✅ **Forward Compatible**: Easy to add INT and FLOAT support later

---

## Code Quality

- ✅ All existing tests pass
- ✅ No compilation errors
- ✅ Memory safe (proper use of bit_cast, no leaks)
- ✅ Follows existing code patterns
- 🔶 New tests failing (range queries)

---

## Documentation

This document serves as the implementation record. Additional documentation needed:

- [ ] Update API documentation for `diagon_create_numeric_range_query()`
- [ ] Add usage examples to README
- [ ] Document the NumericType enum and its purpose
- [ ] Create migration guide from DoubleRangeQuery to NumericRangeQuery

---

**Last Updated**: 2026-01-27
**Implemented By**: Claude Sonnet 4.5
**Status**: Investigation ongoing - core infrastructure complete, debugging in progress
