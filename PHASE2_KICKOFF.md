# Phase 2: Query Optimization & WASM UDFs - Kickoff

**Date**: 2026-01-25
**Status**: Starting
**Approach**: Custom Go Planner + WASM UDFs + Expression Trees

## Overview

Phase 2 builds advanced query optimization capabilities on top of the solid Phase 1 distributed foundation. We're implementing a **multi-tiered UDF pushdown system** combined with a **custom Go query planner** to achieve near-native performance without adding Java/JVM complexity.

## Architecture Decision

### ✅ Chosen Approach: Multi-Tiered System

```
┌─────────────────────────────────────────────┐
│         UDF Complexity Decision             │
├─────────────────────────────────────────────┤
│                                             │
│  Simple Math (price * 1.2 > 100)           │
│    → Expression Tree Pushdown              │
│    → Native C++ evaluation (5ns/call)      │
│    → Coverage: 75-80% of queries           │
│                                             │
│  Performance-Critical Custom Logic         │
│    → WASM UDF                              │
│    → Near-native speed (20ns/call)         │
│    → Coverage: 15-20% of queries           │
│                                             │
│  ML Models / Complex Python Logic          │
│    → Python UDF (Phase 3)                  │
│    → Embedded CPython (500ns/call)         │
│    → Coverage: 5% of queries               │
│                                             │
└─────────────────────────────────────────────┘
```

### ❌ Rejected: Apache Calcite (Java)

**Rationale**: Adds JVM to tech stack, increases complexity, slower than WASM for UDF pushdown.

**Details**: See [QUERY_PLANNER_DECISION.md](design/QUERY_PLANNER_DECISION.md)

## Objectives

1. **Custom Go Query Planner**: Lightweight, integrated, optimized for search workloads
2. **Expression Trees**: Handle 80% of queries (simple filters/math) with native C++ speed
3. **WASM UDF Framework**: Near-native performance (20ns/call) for custom logic
4. **Tiered Compilation**: Zero deployment latency with background JIT

## Phase 2 Components

### 1. Custom Go Query Planner

**Goal**: Rule-based query optimization without external dependencies

**Features**:
- Predicate pushdown (filter execution on data nodes)
- Filter merging and reordering
- Index selection with cost model
- Physical plan generation
- Integration with expression trees and WASM UDFs

**Implementation**:
- `pkg/coordination/planner/optimizer.go` - Rule-based optimizer
- `pkg/coordination/planner/cost_model.go` - Cost estimation
- `pkg/coordination/planner/physical_plan.go` - Physical plan generator

**Estimated Lines**: ~1,500 lines

---

### 2. Expression Tree Pushdown

**Goal**: Fast evaluation of simple expressions (80% coverage)

**Expression Types**:
- **Arithmetic**: `+`, `-`, `*`, `/`, `%`, `pow`
- **Comparison**: `==`, `!=`, `<`, `<=`, `>`, `>=`
- **Logical**: `&&`, `||`, `!`
- **Ternary**: `condition ? true_val : false_val`
- **Field Access**: `doc.price`, `doc.metadata.category`
- **Functions**: `abs`, `sqrt`, `min`, `max`, `floor`, `ceil`

**Example**:
```json
{
  "filter": {
    "expr": {
      "op": ">",
      "left": {
        "op": "*",
        "left": {"field": "price"},
        "right": {"const": 1.2}
      },
      "right": {"const": 100}
    }
  }
}
```

**Implementation**:
- `pkg/coordination/expressions/ast.go` - Expression AST
- `pkg/coordination/expressions/parser.go` - Parser
- `pkg/coordination/expressions/validator.go` - Type checking
- `pkg/data/diagon/expression_evaluator.cpp` - Native C++ evaluator

**Estimated Lines**: ~800 lines (Go) + 600 lines (C++)

---

### 3. WASM UDF Framework

**Goal**: Near-native UDF execution with sandboxed security

**Components**:

#### 3a. WASM Runtime Integration (C++ Data Node)

**Runtime Strategy**: **Tiered Compilation** (wasm3 + Wasmtime)

```cpp
// Phase 1: Interpreter (instant - 0ms startup)
wasm3_interpreter_t* interp = wasm3_load(wasm_bytes);
// Queries execute immediately at ~200ns/call

// Phase 2: Background JIT (200-250ms)
std::thread([=]() {
    wasmtime_module_t* jit = wasmtime_compile(wasm_bytes);
    hot_swap(interp, jit);  // Atomic swap
});
// After 250ms: queries use native code at ~20ns/call
```

**Cache Strategy**:
```cpp
// Check cache first
std::string hash = sha256(wasm_bytes);
if (cache.exists(hash)) {
    wasmtime_module_t* module = cache.load(hash);  // 5ms
    return module;  // Native code immediately
}

// Not in cache - tiered compilation
return tiered_compile_and_cache(wasm_bytes, hash);
```

**Implementation Files**:
- `pkg/data/diagon/wasm_runtime.h` - Runtime interface
- `pkg/data/diagon/wasm_runtime.cpp` - Tiered compilation
- `pkg/data/diagon/wasm_cache.cpp` - Native code caching
- `pkg/data/diagon/wasm3_bridge.cpp` - wasm3 interpreter
- `pkg/data/diagon/wasmtime_bridge.cpp` - Wasmtime JIT

**Estimated Lines**: ~1,500 lines (C++)

#### 3b. UDF Registry (Go Coordination Node)

**Goal**: Manage UDF deployments, versions, and metadata

```go
type UDFRegistry struct {
    udfs map[string]*UDFMetadata
    store UDFStore  // Persistent storage
}

type UDFMetadata struct {
    Name        string
    Version     string
    WASMBytes   []byte
    Hash        string
    CreatedAt   time.Time
    Signature   FunctionSignature
}

type FunctionSignature struct {
    Params []ParamType  // e.g., [Float64, Float64]
    Return ReturnType   // e.g., Float64
}
```

**API Endpoints**:
- `POST /_udf/:name` - Deploy UDF
- `GET /_udf/:name` - Get UDF metadata
- `DELETE /_udf/:name` - Delete UDF
- `GET /_udf` - List all UDFs

**Implementation Files**:
- `pkg/coordination/udf/registry.go` - UDF management
- `pkg/coordination/udf/store.go` - Persistent storage
- `pkg/coordination/udf/api_handlers.go` - REST API

**Estimated Lines**: ~600 lines (Go)

#### 3c. UDF Deployment Protocol

**gRPC Service** (added to DataService):
```protobuf
service DataService {
    // ... existing methods ...

    rpc DeployUDF(DeployUDFRequest) returns (DeployUDFResponse);
    rpc DeleteUDF(DeleteUDFRequest) returns (DeleteUDFResponse);
    rpc ListUDFs(ListUDFsRequest) returns (ListUDFsResponse);
}

message DeployUDFRequest {
    string name = 1;
    bytes wasm_bytecode = 2;
    FunctionSignature signature = 3;
    optional bytes native_binary_x86_64 = 4;
    optional bytes native_binary_arm64 = 5;
}
```

**Implementation Files**:
- `pkg/common/proto/udf.proto` - UDF protocol
- `pkg/data/udf_service.go` - gRPC handlers

**Estimated Lines**: ~400 lines

---

### 4. Query Plan Enhancement

**Goal**: Integrate expression trees and WASM UDFs into query execution

**Enhanced Search Request**:
```json
{
  "query": {
    "bool": {
      "must": [{"match": {"title": "search"}}],
      "filter": [
        {
          "expr": {
            "op": ">",
            "left": {"field": "price"},
            "right": {"const": 100}
          }
        }
      ]
    }
  },
  "rescore": {
    "wasm_udf": {
      "name": "custom_score",
      "params": [
        {"field": "_score"},
        {"field": "popularity"},
        {"const": 1.5}
      ]
    }
  }
}
```

**Physical Plan**:
```
ShardScan
  ├─ MatchQuery(title="search")
  ├─ ExpressionFilter(price > 100)    ← Native C++ evaluation
  └─ WASMRescore(custom_score)         ← WASM UDF execution
```

**Implementation Files**:
- `pkg/coordination/parser/expression_parser.go` - Parse expressions
- `pkg/coordination/parser/udf_parser.go` - Parse UDF references
- `pkg/coordination/planner/pushdown.go` - Pushdown decision logic

**Estimated Lines**: ~500 lines

---

## Performance Targets

### Expression Trees

| Metric | Target | Notes |
|--------|--------|-------|
| Evaluation time | 5ns/call | Native C++ |
| Overhead per 10k docs | 0.05ms | Negligible |
| Coverage | 75-80% | Simple filters/math |

### WASM UDFs

| Metric | Target | Notes |
|--------|--------|-------|
| JIT compilation | <250ms | Background, one-time |
| Cached load | <5ms | From native code cache |
| Interpreter | 200ns/call | During warmup |
| JIT native | 20ns/call | After compilation |
| Deployment latency | 0ms | Tiered compilation |

### Query Planner

| Metric | Target | Notes |
|--------|--------|-------|
| Planning time | <10ms | For complex queries |
| Planning time | <1ms | For simple queries |
| Memory overhead | <100KB | Per query plan |

---

## Implementation Plan

### Week 1-2: Expression Tree Foundation
- [ ] Expression AST design (`ast.go`)
- [ ] Expression parser (`parser.go`)
- [ ] Type checking and validation (`validator.go`)
- [ ] C++ evaluator skeleton (`expression_evaluator.cpp`)
- [ ] Integration with query parser
- [ ] Unit tests (50+ tests)

### Week 3: WASM Runtime Integration
- [ ] wasm3 interpreter integration (`wasm3_bridge.cpp`)
- [ ] Basic WASM loading and execution
- [ ] Function signature validation
- [ ] Memory management
- [ ] Error handling
- [ ] Basic tests with simple UDFs

### Week 4: Tiered Compilation
- [ ] Wasmtime JIT integration (`wasmtime_bridge.cpp`)
- [ ] Hot-swap mechanism (atomic switch)
- [ ] Background compilation thread
- [ ] Native code caching (`wasm_cache.cpp`)
- [ ] Cache management (LRU eviction)

### Week 5: UDF Registry & API
- [ ] UDF registry service (`registry.go`)
- [ ] Persistent storage (`store.go`)
- [ ] REST API endpoints (`api_handlers.go`)
- [ ] gRPC protocol (`udf.proto`)
- [ ] Data node gRPC handlers
- [ ] End-to-end deployment test

### Week 6: Query Planner
- [ ] Basic optimizer (`optimizer.go`)
- [ ] Predicate pushdown rules
- [ ] Filter merging logic
- [ ] Cost model (`cost_model.go`)
- [ ] Physical plan generator
- [ ] Integration with executors

### Week 7: Integration & Testing
- [ ] Expression tree → C++ integration
- [ ] WASM UDF → query execution
- [ ] End-to-end query tests
- [ ] Performance benchmarks
- [ ] Error handling paths

### Week 8: Polish & Documentation
- [ ] User guide (writing UDFs)
- [ ] Examples (Rust, C, AssemblyScript)
- [ ] Performance tuning
- [ ] Monitoring integration
- [ ] Release preparation

---

## User Experience

### Writing Expression Filters

**Before (limited to DSL)**:
```json
{
  "range": {"price": {"gt": 100, "lt": 1000}}
}
```

**After (arbitrary expressions)**:
```json
{
  "expr": {
    "op": "&&",
    "left": {
      "op": ">",
      "left": {"op": "*", "left": {"field": "price"}, "right": {"const": 1.2}},
      "right": {"const": 100}
    },
    "right": {
      "op": "<",
      "left": {"field": "price"},
      "right": {"const": 1000}
    }
  }
}
```

### Writing WASM UDFs

**Example: Custom Scoring (Rust)**:
```rust
#[no_mangle]
pub extern "C" fn custom_score(bm25_score: f64, popularity: f64, boost: f64) -> f64 {
    let base = bm25_score * boost;
    let pop_factor = (popularity / 100.0).min(2.0);
    base * pop_factor
}
```

**Compile**:
```bash
rustc --target wasm32-wasi --release custom_score.rs -o custom_score.wasm
```

**Deploy**:
```bash
curl -X POST http://localhost:8080/_udf/custom_score \
  -H "Content-Type: application/wasm" \
  --data-binary @custom_score.wasm
```

**Use in Query**:
```json
{
  "query": {"match": {"title": "laptop"}},
  "rescore": {
    "wasm_udf": {
      "name": "custom_score",
      "params": [
        {"field": "_score"},
        {"field": "popularity"},
        {"const": 1.5}
      ]
    }
  }
}
```

---

## Technology Stack

### Phase 2 Additions

**Go Libraries**:
- None! Pure Go implementation for planner and expression parser

**C++ Libraries**:
- **wasm3** (~15,000 lines) - Fast WASM interpreter (MIT license)
- **Wasmtime C API** (~5MB) - Production-grade JIT (Apache 2.0 license)

**User UDF Languages**:
- **Rust** (recommended) - Best performance, safety
- **C/C++** - Maximum control
- **AssemblyScript** - TypeScript-like syntax
- **TinyGo** - Go-like syntax

---

## Monitoring Metrics

### Expression Evaluation

- `expression_evaluations_total` - Total evaluations
- `expression_evaluation_duration_seconds` - Evaluation time
- `expression_errors_total` - Parse/eval errors

### WASM UDFs

- `wasm_udfs_deployed_total` - Total deployments
- `wasm_udf_compile_duration_seconds` - JIT compilation time
- `wasm_udf_call_duration_seconds` - Per-call execution time
- `wasm_udf_cache_hits_total` - Cache hit rate
- `wasm_udf_cache_misses_total` - Cache misses
- `wasm_udf_interpreter_calls_total` - Interpreter usage
- `wasm_udf_jit_calls_total` - JIT native code usage

### Query Planner

- `query_planning_duration_seconds` - Planning time
- `query_plan_complexity` - Plan complexity score
- `predicate_pushdowns_total` - Successful pushdowns
- `filter_merges_total` - Filter optimizations

---

## Success Criteria

### Phase 2 Complete When:

- [ ] Expression trees handle 80% of common filter patterns
- [ ] WASM UDF execution achieves <30ns/call (JIT)
- [ ] Zero deployment latency (tiered compilation working)
- [ ] Native code cache working (5ms re-deploys)
- [ ] Query planner handles all DSL queries
- [ ] End-to-end tests passing (expression + WASM)
- [ ] Performance benchmarks meet targets
- [ ] Documentation complete (user guide + examples)

---

## Files to Create

### Go Files (~3,000 lines)
1. `pkg/coordination/expressions/ast.go` (200 lines)
2. `pkg/coordination/expressions/parser.go` (300 lines)
3. `pkg/coordination/expressions/validator.go` (200 lines)
4. `pkg/coordination/planner/optimizer.go` (400 lines)
5. `pkg/coordination/planner/cost_model.go` (200 lines)
6. `pkg/coordination/planner/physical_plan.go` (300 lines)
7. `pkg/coordination/planner/pushdown.go` (200 lines)
8. `pkg/coordination/udf/registry.go` (300 lines)
9. `pkg/coordination/udf/store.go` (200 lines)
10. `pkg/coordination/udf/api_handlers.go` (200 lines)
11. `pkg/data/udf_service.go` (200 lines)
12. `pkg/common/proto/udf.proto` (200 lines)

### C++ Files (~2,500 lines)
1. `pkg/data/diagon/expression_evaluator.h` (100 lines)
2. `pkg/data/diagon/expression_evaluator.cpp` (500 lines)
3. `pkg/data/diagon/wasm_runtime.h` (100 lines)
4. `pkg/data/diagon/wasm_runtime.cpp` (400 lines)
5. `pkg/data/diagon/wasm3_bridge.cpp` (300 lines)
6. `pkg/data/diagon/wasmtime_bridge.cpp` (400 lines)
7. `pkg/data/diagon/wasm_cache.cpp` (300 lines)
8. `pkg/data/diagon/udf_executor.cpp` (400 lines)

### Test Files (~2,000 lines)
- Expression parser tests
- Expression evaluator tests
- WASM runtime tests
- UDF registry tests
- Query planner tests
- Integration tests

### Documentation (~1,000 lines)
- User guide (writing UDFs)
- Expression syntax reference
- UDF API documentation
- Performance tuning guide
- Example UDFs (Rust, C, AssemblyScript)

**Total New Code**: ~8,500 lines

---

## Risk Mitigation

| Risk | Mitigation | Severity |
|------|------------|----------|
| **WASM runtime bugs** | Use mature runtimes (wasm3, Wasmtime) | Low |
| **Performance regression** | Aggressive benchmarking, caching | Low |
| **User learning curve** | Templates, examples, documentation | Low |
| **Memory leaks** | Careful resource management, RAII | Medium |
| **Cache invalidation bugs** | Content-addressable caching (hash-based) | Low |

---

## Next Steps

**Immediate** (Week 1):
1. Create expression AST and parser
2. Implement basic expression validator
3. Start C++ expression evaluator

**Week 2**:
1. Complete expression evaluator
2. Integration tests
3. Start WASM runtime integration

---

**Author**: Implementation Team
**Date**: 2026-01-25
**Project**: Quidditch Phase 2
**Approach**: WASM UDFs + Custom Go Planner (No Calcite/Java)
