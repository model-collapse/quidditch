# C++ Expression Evaluator Integration Guide

**Date**: 2026-01-25
**Phase**: Phase 2 - Week 2 - Day 3
**Status**: 📋 Implementation Guide

---

## Overview

This guide explains how to integrate the expression evaluator into the Diagon C++ search engine core. The integration enables native C++ evaluation of filter expressions at ~5ns per document.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Go Layer (Already Complete)                                 │
│                                                              │
│  gRPC Request → Shard.Search(query, filterExpression)      │
│                       ↓                                      │
│  DiagonBridge.Search(query, filterExpression)              │
│                       ↓                                      │
│  C API Call (when CGO enabled)                             │
└──────────────────────┬──────────────────────────────────────┘
                       ↓ CGO
┌─────────────────────────────────────────────────────────────┐
│ C API Layer (search_integration.cpp)                       │
│                                                              │
│  diagon_search_with_filter(                                 │
│    shard, query_json, filter_expr, filter_expr_len          │
│  )                                                          │
│                       ↓                                      │
│  Shard::search(queryJson, options)                         │
└──────────────────────┬──────────────────────────────────────┘
                       ↓
┌─────────────────────────────────────────────────────────────┐
│ Search Loop (search_integration.cpp)                       │
│                                                              │
│  1. Execute base query → candidates                         │
│  2. If filter_expr provided:                                │
│       - Deserialize expression                              │
│       - For each candidate:                                 │
│           if (filter.matches(doc)) {                        │
│             include in results                              │
│           }                                                  │
│  3. Return filtered results                                 │
└──────────────────────┬──────────────────────────────────────┘
                       ↓
┌─────────────────────────────────────────────────────────────┐
│ Expression Evaluator (expression_evaluator.cpp)            │
│                                                              │
│  ExpressionFilter::matches(doc):                            │
│    1. expr->evaluate(doc) → ExprValue                       │
│    2. to_bool(result) → bool                                │
│    3. Return match status                                   │
│                       ↓                                      │
│  Expression::evaluate(doc):                                 │
│    - BinaryOp: evaluate(left) OP evaluate(right)           │
│    - FieldExpression: doc.getField(fieldPath)              │
│    - ConstExpression: return constant value                │
│    - Function: execute function(args)                       │
└──────────────────────┬──────────────────────────────────────┘
                       ↓
┌─────────────────────────────────────────────────────────────┐
│ Document Interface (document.cpp)                          │
│                                                              │
│  JSONDocument::getField(fieldPath):                        │
│    1. Parse field path ("a.b.c" → ["a","b","c"])          │
│    2. Navigate JSON structure                               │
│    3. Convert JSON value → ExprValue                       │
│    4. Return std::optional<ExprValue>                      │
└─────────────────────────────────────────────────────────────┘
```

---

## Implementation Steps

### Step 1: Document Interface Implementation

**File**: `pkg/data/diagon/document.cpp`
**Status**: ✅ Header created, ⏳ Implementation needed

#### Requirements:
1. Choose JSON library (recommended: nlohmann/json)
2. Implement `getField()` method
3. Implement field path navigation
4. Implement type conversion (JSON → ExprValue)

#### Example Implementation:

```cpp
#include <nlohmann/json.hpp>

std::optional<ExprValue> JSONDocument::getField(const std::string& fieldPath) const {
    auto* json = static_cast<const nlohmann::json*>(jsonData_);

    // Parse field path
    FieldPath path(fieldPath);
    const nlohmann::json* current = json;

    // Navigate nested structure
    for (const auto& component : path.components()) {
        if (!current->is_object() || !current->contains(component)) {
            return std::nullopt;  // Field not found
        }
        current = &(*current)[component];
    }

    // Convert to ExprValue
    if (current->is_boolean()) {
        return current->get<bool>();
    }
    if (current->is_number_integer()) {
        return current->get<int64_t>();
    }
    if (current->is_number_float()) {
        return current->get<double>();
    }
    if (current->is_string()) {
        return current->get<std::string>();
    }

    return std::nullopt;
}
```

**Performance Target**: <10ns per field access

---

### Step 2: Expression Evaluator Integration

**File**: `pkg/data/diagon/expression_evaluator.cpp`
**Status**: ✅ Complete (from Week 1)

**Already Implemented:**
- ✅ Expression AST classes
- ✅ Binary operators (12 types)
- ✅ Unary operators (2 types)
- ✅ Functions (14 types)
- ✅ Deserialization from Go format
- ✅ Evaluation logic

**Verify:**
```cpp
// Test expression evaluation
auto expr = evaluator.deserialize(exprBytes, exprLen);
ExprValue result = expr->evaluate(document);
bool matches = to_bool(result);
```

**Performance Target**: ~5ns per evaluation

---

### Step 3: Search Loop Integration

**File**: `pkg/data/diagon/search_integration.cpp`
**Status**: ✅ Skeleton created, ⏳ Search loop needed

#### Core Search Loop:

```cpp
SearchResult Shard::search(
    const std::string& queryJson,
    const SearchOptions& options
) {
    auto startTime = std::chrono::high_resolution_clock::now();

    // 1. Execute base query (full-text search, etc.)
    auto candidates = executeBaseQuery(queryJson);

    // 2. Apply expression filter if provided
    if (options.filterExpr && options.filterExprLen > 0) {
        auto filter = ExpressionFilter::create(
            options.filterExpr,
            options.filterExprLen
        );

        if (filter) {
            candidates = applyFilter(candidates, *filter);
        }
    }

    // 3. Apply pagination
    int64_t totalHits = candidates.size();
    int from = std::max(0, options.from);
    int size = std::min(options.size, static_cast<int>(candidates.size() - from));

    std::vector<std::shared_ptr<Document>> hits;
    if (from < candidates.size()) {
        hits.assign(
            candidates.begin() + from,
            candidates.begin() + from + size
        );
    }

    // 4. Build result
    SearchResult result;
    result.totalHits = totalHits;
    result.hits = hits;
    result.maxScore = calculateMaxScore(hits);
    result.took = calculateDuration(startTime);

    return result;
}
```

#### Filter Application (Hot Path):

```cpp
std::vector<std::shared_ptr<Document>> Shard::applyFilter(
    const std::vector<std::shared_ptr<Document>>& candidates,
    const ExpressionFilter& filter
) {
    std::vector<std::shared_ptr<Document>> filtered;
    filtered.reserve(candidates.size());

    // Hot path: evaluate expression on each document
    for (const auto& doc : candidates) {
        if (filter.matches(*doc)) {  // ~5ns per call
            filtered.push_back(doc);
        }
    }

    return filtered;
}
```

**Performance Optimization:**
- Early termination if `size` results found
- SIMD batch evaluation (future)
- Lazy document loading
- Score calculation only for matched docs

---

### Step 4: C API Implementation

**File**: `pkg/data/diagon/search_integration.cpp`
**Status**: ✅ Signatures defined, ⏳ Implementation needed

#### Primary C API Function:

```c
char* diagon_search_with_filter(
    diagon_shard_t* shard,
    const char* query_json,
    const uint8_t* filter_expr,
    size_t filter_expr_len,
    int from,
    int size
) {
    // 1. Convert C types to C++
    auto* s = reinterpret_cast<Shard*>(shard);

    SearchOptions options;
    options.from = from;
    options.size = size;
    options.filterExpr = filter_expr;
    options.filterExprLen = filter_expr_len;

    // 2. Execute search
    SearchResult result = s->search(query_json, options);

    // 3. Convert result to JSON
    nlohmann::json resultJson;
    resultJson["took"] = result.took;
    resultJson["total_hits"] = result.totalHits;
    resultJson["max_score"] = result.maxScore;

    nlohmann::json hitsArray = nlohmann::json::array();
    for (const auto& doc : result.hits) {
        nlohmann::json hit;
        hit["_id"] = doc->getDocumentId();
        hit["_score"] = doc->getScore();
        // hit["_source"] = ... (convert document to JSON)
        hitsArray.push_back(hit);
    }
    resultJson["hits"] = hitsArray;

    // 4. Return as C string (caller must free)
    std::string jsonStr = resultJson.dump();
    return strdup(jsonStr.c_str());
}
```

**Memory Management:**
- Caller (Go) must free returned string
- Use `strdup()` for C string allocation
- Clean up temporary objects properly

---

### Step 5: CGO Integration

**File**: `pkg/data/diagon/bridge.go`
**Status**: ✅ Prepared, ⏳ Needs uncommenting

#### Enable CGO:

```go
// In bridge.go constructor
bridge := &DiagonBridge{
    config:     cfg,
    logger:     cfg.Logger,
    shards:     make(map[string]*Shard),
    cgoEnabled: true,  // SET TO TRUE when C++ is ready
}
```

#### Uncomment C API Calls:

```go
func (s *Shard) Search(query []byte, filterExpression []byte) (*SearchResult, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    if s.cgoEnabled {
        // Prepare filter expression pointer
        var filterPtr *C.uint8_t
        var filterLen C.size_t

        if len(filterExpression) > 0 {
            filterPtr = (*C.uint8_t)(unsafe.Pointer(&filterExpression[0]))
            filterLen = C.size_t(len(filterExpression))
        }

        // Call C++ API
        resultJSON := C.diagon_search_with_filter(
            (*C.diagon_shard_t)(s.shardPtr),
            C.CString(string(query)),
            filterPtr,
            filterLen,
            C.int(0),   // from
            C.int(10),  // size
        )
        defer C.free(unsafe.Pointer(resultJSON))

        // Parse JSON result
        var result SearchResult
        if err := json.Unmarshal([]byte(C.GoString(resultJSON)), &result); err != nil {
            return nil, fmt.Errorf("failed to parse search results: %w", err)
        }

        return &result, nil
    }

    // Stub mode fallback
    // ...
}
```

---

## Dependencies

### JSON Library (Required)

**Recommended**: nlohmann/json
```bash
# Install via package manager
apt-get install nlohmann-json3-dev

# Or include as header-only library
# Download from: https://github.com/nlohmann/json
```

**Why nlohmann/json:**
- Header-only (easy integration)
- Modern C++ API
- Excellent performance
- Wide adoption

**Alternatives:**
- **rapidjson**: Faster, but more complex API
- **simdjson**: Fastest, but read-only
- **boost::json**: If already using Boost

### Build System

**CMakeLists.txt** (example):
```cmake
cmake_minimum_required(VERSION 3.15)
project(diagon_expression)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Find dependencies
find_package(nlohmann_json REQUIRED)

# Expression evaluator library
add_library(diagon_expression SHARED
    expression_evaluator.cpp
    document.cpp
    search_integration.cpp
)

target_link_libraries(diagon_expression
    nlohmann_json::nlohmann_json
)

# Optimization flags
target_compile_options(diagon_expression PRIVATE
    -O3
    -march=native
    -ffast-math
)
```

---

## Performance Optimization

### 1. Hot Path Optimization

```cpp
// Mark hot functions as inline
inline bool ExpressionFilter::matches(const Document& doc) const {
    evaluationCount_++;
    ExprValue result = expr_->evaluate(doc);
    bool matched = to_bool(result);
    if (matched) matchCount_++;
    return matched;
}
```

### 2. Field Access Caching

```cpp
class DocumentCache {
    std::unordered_map<std::string, ExprValue> cache_;
public:
    std::optional<ExprValue> getField(
        const Document& doc,
        const std::string& path
    ) {
        auto it = cache_.find(path);
        if (it != cache_.end()) {
            return it->second;  // Cache hit
        }

        auto value = doc.getField(path);
        if (value) {
            cache_[path] = *value;  // Cache for next time
        }
        return value;
    }
};
```

### 3. SIMD Batch Evaluation (Future)

```cpp
// Evaluate expression on multiple documents at once
std::vector<bool> evaluateBatch(
    const Expression& expr,
    const std::vector<Document*>& docs
) {
    // Use SIMD intrinsics for parallel evaluation
    // Requires expression to be SIMD-friendly
    // Target: 16 documents per ~80ns = ~5ns each
}
```

### 4. Branch Prediction Hints

```cpp
if (__builtin_expect(filter == nullptr, 0)) {
    // Unlikely path: no filter
    return candidates;
}

// Likely path: with filter
return applyFilter(candidates, *filter);
```

---

## Error Handling

### 1. Deserialization Errors

```cpp
auto filter = ExpressionFilter::create(exprData, exprLen);
if (!filter) {
    // Log: Failed to deserialize expression
    // Fallback: Execute query without filter
    return searchWithoutFilter(queryJson, options);
}
```

### 2. Evaluation Errors

```cpp
bool ExpressionFilter::matches(const Document& doc) const {
    try {
        ExprValue result = expr_->evaluate(doc);
        return to_bool(result);
    } catch (const std::exception& e) {
        // Log: Expression evaluation failed: {e.what()}
        // Treat as non-match for safety
        return false;
    }
}
```

### 3. Field Access Errors

```cpp
std::optional<ExprValue> Document::getField(const std::string& path) const {
    try {
        // Navigate and extract field
        return fieldValue;
    } catch (const std::exception& e) {
        // Log: Field access failed: {path}
        // Return empty optional (field not found)
        return std::nullopt;
    }
}
```

---

## Testing Strategy

### Unit Tests

```cpp
// Test document field access
TEST(DocumentTest, GetField) {
    JSONDocument doc(jsonData, "doc1");

    auto price = doc.getField("price");
    ASSERT_TRUE(price.has_value());
    ASSERT_EQ(std::get<double>(*price), 99.99);
}

// Test expression evaluation
TEST(ExpressionFilterTest, SimpleComparison) {
    // price > 100
    uint8_t exprBytes[] = {0x02, 0x05, 0x00, ...};
    auto filter = ExpressionFilter::create(exprBytes, sizeof(exprBytes));

    JSONDocument doc1(json1, "doc1");  // price = 150
    ASSERT_TRUE(filter->matches(doc1));

    JSONDocument doc2(json2, "doc2");  // price = 50
    ASSERT_FALSE(filter->matches(doc2));
}

// Test search integration
TEST(ShardTest, SearchWithFilter) {
    Shard shard("/tmp/test_shard");

    SearchOptions options;
    options.filterExpr = exprBytes;
    options.filterExprLen = sizeof(exprBytes);

    SearchResult result = shard.search(queryJson, options);

    ASSERT_EQ(result.totalHits, 5);
    ASSERT_EQ(result.hits.size(), 5);
}
```

### Performance Benchmarks

```cpp
// Benchmark expression evaluation
BENCHMARK(BM_ExpressionEvaluation) {
    ExpressionFilter filter = ...;
    Document doc = ...;

    for (auto _ : state) {
        bool matches = filter.matches(doc);
        benchmark::DoNotOptimize(matches);
    }
}
// Target: ~5ns per iteration

// Benchmark full search with filter
BENCHMARK(BM_SearchWithFilter) {
    Shard shard = ...;

    for (auto _ : state) {
        SearchResult result = shard.search(query, options);
        benchmark::DoNotOptimize(result);
    }
}
// Target: <10% overhead vs no filter
```

---

## Implementation Checklist

### Phase 1: Document Interface
- [ ] Choose and integrate JSON library
- [ ] Implement `getField()` method
- [ ] Implement field path parsing
- [ ] Implement type conversions
- [ ] Unit tests for field access
- [ ] Performance benchmarks (<10ns/field)

### Phase 2: Search Integration
- [ ] Implement `Shard::search()` method
- [ ] Implement `applyFilter()` method
- [ ] Add filter statistics tracking
- [ ] Error handling for invalid expressions
- [ ] Unit tests for search with filters
- [ ] Performance benchmarks (~5ns/eval)

### Phase 3: C API
- [ ] Implement `diagon_search_with_filter()`
- [ ] Implement result JSON serialization
- [ ] Memory management (strdup/free)
- [ ] Error code handling
- [ ] C API unit tests
- [ ] Memory leak tests (valgrind)

### Phase 4: CGO Integration
- [ ] Set `cgoEnabled = true` in bridge.go
- [ ] Uncomment C API calls
- [ ] Test Go → C → C++ flow
- [ ] Error propagation Go ← C ← C++
- [ ] Integration tests (Go + C++)
- [ ] End-to-end performance tests

### Phase 5: Optimization
- [ ] Profile hot paths
- [ ] Optimize field access
- [ ] Add field caching
- [ ] SIMD evaluation (if beneficial)
- [ ] Branch prediction hints
- [ ] Final performance validation

---

## Expected Performance

### Targets (from Week 1)
- **Deserialization**: ~1 μs (one-time per query)
- **Field access**: <10 ns per field
- **Expression evaluation**: ~5 ns per document
- **10k documents**: ~50 μs total
- **Query overhead**: <10% for filtered queries

### Measurements Needed
1. Field access latency
2. Expression evaluation latency
3. Filter application throughput
4. End-to-end query latency
5. Memory usage per query

---

## Resources

### Files Created
1. `document.h` - Document interface header
2. `document.cpp` - Document implementation
3. `search_integration.h` - Search integration header
4. `search_integration.cpp` - Search implementation
5. `CPP_INTEGRATION_GUIDE.md` - This document

### Existing Files (Week 1)
6. `expression_evaluator.h` - Expression AST and evaluator
7. `expression_evaluator.cpp` - Expression evaluation logic
8. `bridge.go` - Go CGO bridge (ready for C++)

### Total: 8 files, ~3,000 lines of C++ infrastructure

---

## Next Steps

1. **Install JSON library** (nlohmann/json)
2. **Implement Document interface** (document.cpp)
3. **Implement search loop** (search_integration.cpp)
4. **Create unit tests** (Google Test)
5. **Enable CGO** (bridge.go)
6. **Run integration tests**
7. **Performance benchmarks**
8. **Production deployment**

---

**Author**: Implementation Team
**Date**: 2026-01-25
**Phase**: 2 - Week 2 - Day 3
**Status**: 📋 Ready for Implementation
