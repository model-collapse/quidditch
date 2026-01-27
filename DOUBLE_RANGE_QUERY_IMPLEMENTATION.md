# Task: Implement DoubleRangeQuery in Diagon Search Engine

## Context

The Diagon search engine currently has `NumericRangeQuery` that only works with `int64_t` fields. This causes a type mismatch when searching double-indexed fields (created with `diagon_create_indexed_double_field`), resulting in 0 search results.

**Root cause**: Fields indexed as doubles can't be searched with range queries because `NumericRangeQuery` only supports int64_t.

## Objective

Implement a new `DoubleRangeQuery` class that works with double-precision floating-point fields, following the same pattern as `NumericRangeQuery`.

## Reference Implementation

Study the existing implementation:
- **Header**: `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/include/diagon/search/NumericRangeQuery.h`
- **Implementation**: `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/src/search/NumericRangeQuery.cpp`

The new `DoubleRangeQuery` should follow the exact same structure but use `double` instead of `int64_t`.

## Requirements

### 1. Create Header File

**Location**: `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/include/diagon/search/DoubleRangeQuery.h`

```cpp
#pragma once

#include "diagon/search/Query.h"
#include "diagon/search/Weight.h"
#include <limits>
#include <memory>
#include <string>

namespace diagon {
namespace search {

/**
 * DoubleRangeQuery - Query matching documents with double field values in a range
 *
 * Matches documents where field value is in range [lowerValue, upperValue].
 * Endpoints can be excluded via includeLower/includeUpper flags.
 *
 * Uses NumericDocValues (double) for filtering - efficient O(1) per document check.
 *
 * Examples:
 *   price:[99.99 TO 999.99]     -> DoubleRangeQuery("price", 99.99, 999.99, true, true)
 *   score:{0.5 TO 1.0}          -> DoubleRangeQuery("score", 0.5, 1.0, false, false)
 *   temperature:[0.0 TO *]      -> DoubleRangeQuery("temperature", 0.0, MAX, true, true)
 */
class DoubleRangeQuery : public Query {
public:
    /**
     * Constructor for bounded range
     *
     * @param field Field name
     * @param lowerValue Lower bound (inclusive if includeLower=true)
     * @param upperValue Upper bound (inclusive if includeUpper=true)
     * @param includeLower Include lower bound?
     * @param includeUpper Include upper bound?
     */
    DoubleRangeQuery(const std::string& field, double lowerValue, double upperValue,
                     bool includeLower, bool includeUpper);

    /**
     * Create unbounded lower range: field <= upperValue
     */
    static std::unique_ptr<DoubleRangeQuery> newUpperBoundQuery(const std::string& field,
                                                                 double upperValue,
                                                                 bool includeUpper);

    /**
     * Create unbounded upper range: field >= lowerValue
     */
    static std::unique_ptr<DoubleRangeQuery> newLowerBoundQuery(const std::string& field,
                                                                 double lowerValue,
                                                                 bool includeLower);

    /**
     * Create exact value query: field == value
     */
    static std::unique_ptr<DoubleRangeQuery> newExactQuery(const std::string& field,
                                                            double value);

    // ==================== Accessors ====================

    const std::string& getField() const { return field_; }
    double getLowerValue() const { return lowerValue_; }
    double getUpperValue() const { return upperValue_; }
    bool getIncludeLower() const { return includeLower_; }
    bool getIncludeUpper() const { return includeUpper_; }

    // ==================== Query Interface ====================

    std::unique_ptr<Weight> createWeight(IndexSearcher& searcher, ScoreMode scoreMode,
                                         float boost) const override;

    std::string toString(const std::string& field) const override;

    bool equals(const Query& other) const override;

    size_t hashCode() const override;

    std::unique_ptr<Query> clone() const override;

private:
    std::string field_;
    double lowerValue_;
    double upperValue_;
    bool includeLower_;
    bool includeUpper_;
};

}  // namespace search
}  // namespace diagon
```

### 2. Create Implementation File

**Location**: `/home/ubuntu/quidditch/pkg/data/diagon/upstream/src/core/src/search/DoubleRangeQuery.cpp`

- Copy the structure from `NumericRangeQuery.cpp`
- Replace all `int64_t` with `double`
- Update the Weight implementation to work with double NumericDocValues
- Handle double comparisons correctly (inclusive/exclusive bounds)
- Use appropriate double limits for unbounded queries

**Key changes from NumericRangeQuery**:
- Use `std::numeric_limits<double>::lowest()` instead of `std::numeric_limits<int64_t>::min()`
- Use `std::numeric_limits<double>::max()` for upper bounds
- Handle floating-point comparison edge cases (NaN, infinity)

### 3. Add C API Function

**Location**: `/home/ubuntu/quidditch/pkg/data/diagon/c_api_src/diagon_c_api.h`

Add declaration:
```cpp
/**
 * Create double range query
 * Queries documents where field value is in range [lower_value, upper_value]
 *
 * @param field_name Field to query
 * @param lower_value Lower bound
 * @param upper_value Upper bound
 * @param include_lower Include lower bound?
 * @param include_upper Include upper bound?
 * @return Query handle or NULL on error
 */
DiagonQuery diagon_create_double_range_query(
    const char* field_name,
    double lower_value,
    double upper_value,
    bool include_lower,
    bool include_upper
);
```

**Location**: `/home/ubuntu/quidditch/pkg/data/diagon/c_api_src/diagon_c_api.cpp`

Add implementation:
```cpp
DiagonQuery diagon_create_double_range_query(
    const char* field_name,
    double lower_value,
    double upper_value,
    bool include_lower,
    bool include_upper
) {
    try {
        auto query = std::make_unique<diagon::search::DoubleRangeQuery>(
            field_name, lower_value, upper_value, include_lower, include_upper
        );
        return reinterpret_cast<DiagonQuery>(query.release());
    } catch (const std::exception& e) {
        set_last_error(e.what());
        return nullptr;
    }
}
```

### 4. Update Go Bridge

**Location**: `/home/ubuntu/quidditch/pkg/data/diagon/bridge.go`

Update the range query conversion to use the correct C API function:

```go
// In convertQueryToDiagon method, replace the range query section:

} else if rangeQuery, ok := queryObj["range"].(map[string]interface{}); ok {
    // Range query: {"range": {"field_name": {"gte": 100, "lte": 1000}}}
    for field, rangeParams := range rangeQuery {
        params := rangeParams.(map[string]interface{})

        var lowerValue, upperValue float64
        var includeLower, includeUpper bool

        // Parse bounds (existing code)...

        cField := C.CString(field)
        defer C.free(unsafe.Pointer(cField))

        // Use the double-specific range query function
        diagonQuery = C.diagon_create_double_range_query(
            cField,
            C.double(lowerValue),
            C.double(upperValue),
            C.bool(includeLower),
            C.bool(includeUpper),
        )

        if diagonQuery == nil {
            errMsg := C.GoString(C.diagon_last_error())
            return nil, fmt.Errorf("failed to create double range query: %s", errMsg)
        }
        break
    }
}
```

## Implementation Checklist

- [ ] Create `DoubleRangeQuery.h` header file
- [ ] Implement `DoubleRangeQuery.cpp` source file
- [ ] Implement Weight class for DoubleRangeQuery
- [ ] Implement Scorer that works with double NumericDocValues
- [ ] Add `diagon_create_double_range_query` to C API header
- [ ] Implement `diagon_create_double_range_query` in C API source
- [ ] Update Go bridge to use new C API function
- [ ] Add CMake build rules for new files
- [ ] Write unit tests in C++
- [ ] Write integration tests in Go
- [ ] Verify range queries now return correct results

## Testing

After implementation, test with:

```bash
# Index document with double field
curl -X PUT http://localhost:9200/test/_doc/1 \
  -H 'Content-Type: application/json' \
  -d '{"price": 150.50}'

# Search with range query
curl -X POST http://localhost:9200/test/_search \
  -H 'Content-Type: application/json' \
  -d '{"query": {"range": {"price": {"gte": 100, "lte": 200}}}}'

# Expected: Should return document with price=150.50
```

## Success Criteria

1. DoubleRangeQuery compiles without errors
2. C API function is callable from Go
3. Range queries on double fields return correct results
4. All existing tests still pass
5. No memory leaks in new code

## Additional Notes

- Follow existing Diagon coding style
- Add appropriate error handling
- Consider edge cases: NaN, infinity, -infinity
- Ensure thread safety if needed
- Add comprehensive logging for debugging

## Related Documentation

- **Root Cause Analysis**: See `RANGE_QUERY_ROOT_CAUSE.md` for detailed problem analysis
- **Existing Implementation**: Study `NumericRangeQuery.h` and `NumericRangeQuery.cpp`
- **Field Indexing**: See how `diagon_create_indexed_double_field` works in `bridge.go`

## Timeline Estimate

- C++ implementation: 4-6 hours
- C API wrapper: 1 hour
- Go bridge update: 1 hour
- Testing and debugging: 2-3 hours
- **Total**: 8-11 hours
