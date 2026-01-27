# Range Query Root Cause Analysis

## Problem Statement

Range queries and boolean queries with range clauses return 0 results even though matching documents exist in the index.

## Investigation Summary

### What Works ✓

1. **Document Indexing**: Documents are successfully indexed, committed, and refreshed
2. **Query Generation**: Coordinator generates correct JSON queries without null values
3. **Query Transmission**: Queries reach the data node Search RPC handler
4. **Query Parsing**: Range parameters are correctly parsed (gte=100, lte=300)
5. **Diagon Query Creation**: `diagon_create_numeric_range_query` succeeds
6. **Field Indexing**: Price field is properly indexed as `indexed_double_field` with value 150

### The Root Cause ❌

**TYPE MISMATCH between indexed field type and query type**

#### Indexing Side (Go → C++)

When indexing documents:
```go
// pkg/data/diagon/bridge.go:211-222
case float32, float64:
    // JSON numbers become float64
    val := float64(150)  // price field
    field := C.diagon_create_indexed_double_field(cFieldName, C.double(val))
    // Creates a DOUBLE field in Diagon
```

**Result**: The "price" field is indexed as a **double (float64)** field.

#### Search Side (C++)

When searching:
```cpp
// pkg/data/diagon/upstream/src/core/include/diagon/search/NumericRangeQuery.h:49
NumericRangeQuery(const std::string& field,
                  int64_t lowerValue,    // NOT double!
                  int64_t upperValue,    // NOT double!
                  bool includeLower,
                  bool includeUpper);
```

**Result**: `NumericRangeQuery` only works with **int64** fields, NOT double fields!

#### The C API Bridge

```cpp
// pkg/data/diagon/c_api_src/diagon_c_api.h
DiagonQuery diagon_create_numeric_range_query(
    const char* field_name,
    double lower_value,    // Accepts double...
    double upper_value,    // Accepts double...
    bool include_lower,
    bool include_upper
);
// But internally creates NumericRangeQuery with int64_t!
```

The C API misleadingly accepts `double` parameters but internally creates an `int64_t`-based `NumericRangeQuery`, causing the mismatch.

## Why It Fails

1. Document has field: `price` (double) = 150.0
2. Query searches for: `price` (int64) in range [100, 300]
3. Diagon's `NumericRangeQuery` looks for int64 doc values
4. Field is indexed as double doc values
5. **No match** because query and field types don't align

## Evidence

### Debug Logs

**Indexing**:
```json
{"msg":"DEBUG: Indexing field","field":"price","type":"float64","value":150}
{"msg":"DEBUG: Created indexed double field","field":"price","value":150}
```

**Searching**:
```json
{"msg":"DEBUG: Range query params","field":"price","params":{"gte":100,"lte":300}}
{"msg":"DEBUG: Found gte (float64)","value":100}
{"msg":"DEBUG: Found lte (float64)","value":300}
{"msg":"DEBUG: Creating Diagon range query","field":"price","lower":100,"upper":300}
{"msg":"DEBUG: Diagon range query created successfully"}
{"msg":"DEBUG: Search result details","total_hits":0,"num_hits":0}
```

Query succeeds but finds 0 hits because of type mismatch.

## Solutions

### Option 1: Implement DoubleRangeQuery in Diagon (RECOMMENDED)

Add a new `DoubleRangeQuery` class in Diagon C++ that works with double fields:

```cpp
// New class in diagon/search/DoubleRangeQuery.h
class DoubleRangeQuery : public Query {
public:
    DoubleRangeQuery(const std::string& field,
                     double lowerValue,
                     double upperValue,
                     bool includeLower,
                     bool includeUpper);
    // ... implementation that works with double doc values
};
```

Add C API wrapper:
```cpp
DiagonQuery diagon_create_double_range_query(
    const char* field_name,
    double lower_value,
    double upper_value,
    bool include_lower,
    bool include_upper
);
```

Update Go bridge to use correct query type based on field type.

**Pros**: Clean solution, maintains precision, follows Lucene pattern
**Cons**: Requires C++ implementation in Diagon

### Option 2: Index Numeric Fields as Int64 Only

Convert all numeric fields to int64 during indexing:

```go
case float32, float64:
    val := int64(f)  // LOSES PRECISION!
    field := C.diagon_create_indexed_long_field(cFieldName, C.int64_t(val))
```

**Pros**: Works with existing `NumericRangeQuery`
**Cons**:
- Loses fractional values (e.g., price: 99.99 → 99)
- Not suitable for decimal fields
- Breaks Elasticsearch compatibility

### Option 3: Scale Doubles to Int64

Scale doubles by a factor (e.g., ×100 for currency):

```go
case float32, float64:
    // Scale by 100 for 2 decimal places
    val := int64(f * 100)  // 99.99 → 9999
    field := C.diagon_create_indexed_long_field(cFieldName, C.int64_t(val))
```

Adjust queries similarly:
```go
lowerValue := int64(100.0 * 100)  // 100.00 → 10000
upperValue := int64(300.0 * 100)  // 300.00 → 30000
```

**Pros**: Preserves fixed precision, works with existing code
**Cons**:
- Requires query-time scaling
- Limited to fixed decimal places
- User confusion about scaling

## Recommended Fix

**Implement Option 1**: Add `DoubleRangeQuery` to Diagon.

This is the cleanest solution that:
1. Maintains full precision for floating-point values
2. Follows Lucene's pattern (Lucene has both IntPoint and DoublePoint)
3. Properly separates int64 and double range queries
4. Maintains Elasticsearch compatibility

### Implementation Steps

1. **Diagon C++**: Implement `DoubleRangeQuery` class
2. **Diagon C API**: Add `diagon_create_double_range_query`
3. **Go Bridge**: Detect field type and use correct query type
4. **Query Conversion**: Update `convertQueryToDiagon` to check field type

## Temporary Workaround

Until `DoubleRangeQuery` is implemented, use Option 3 (scaling) for numeric fields:

```go
// Temporary: scale all numeric fields by 100
func scaleNumeric(val float64) int64 {
    return int64(val * 100)
}
```

This preserves 2 decimal places and allows queries to work.

## Related Files

- `pkg/data/diagon/bridge.go:211-222` - Field indexing (creates double fields)
- `pkg/data/diagon/bridge.go:432-484` - Query conversion (creates range queries)
- `pkg/data/diagon/upstream/src/core/include/diagon/search/NumericRangeQuery.h:49` - int64-only constructor
- `pkg/data/diagon/c_api_src/diagon_c_api.h` - C API (misleading double params)

## Next Actions

1. Decide on solution approach (recommend Option 1)
2. If Option 1: Implement `DoubleRangeQuery` in Diagon C++
3. If Option 3 (temp): Implement scaling in Go bridge
4. Add integration tests for numeric range queries
5. Document behavior in API docs
