# Week 3 - WASM Runtime Integration Kickoff

**Date**: 2026-01-25
**Phase**: Phase 2 - Week 3
**Goal**: Enable WASM-based User-Defined Functions (UDFs)

---

## Objectives

Integrate WebAssembly runtime to support custom UDFs that cover ~15% of use cases beyond expression trees.

### Week 3 Deliverables

1. **WASM Runtime Integration** (Days 1-2)
   - Choose runtime (wazero vs wasmtime)
   - Integrate into Go data node
   - Basic function calling

2. **Document Context API** (Day 3)
   - Pass document fields to WASM
   - Memory management for WASM
   - Type conversion (Go ↔ WASM)

3. **UDF Registry** (Day 4)
   - Register WASM modules
   - Function lookup
   - Validation and caching

4. **Integration & Testing** (Day 5)
   - End-to-end tests
   - Example UDFs
   - Performance benchmarks
   - Documentation

---

## Architecture Overview

### Use Case Distribution

```
Expression Trees (75-80%)
├─ Simple filters: price > 100
├─ Arithmetic: price * (1 - discount)
├─ Boolean logic: AND, OR, NOT
└─ Built-in functions: ABS, SQRT, MIN, MAX
    ↓ ~5ns per evaluation

WASM UDFs (15%)
├─ Complex string operations
├─ Custom business logic
├─ Regex matching
└─ JSON path queries
    ↓ ~100ns-1μs per call

Python UDFs (5%)
└─ ML models, complex algorithms
    ↓ ~10ms+ per call
```

### WASM Integration Points

```
┌─────────────────────────────────────────┐
│ Query Parser (Go)                       │
│  - Parse "wasm_udf" query type          │
│  - Extract UDF name + parameters        │
└─────────────┬───────────────────────────┘
              ↓
┌─────────────────────────────────────────┐
│ UDF Registry (Go)                       │
│  - Lookup WASM module                   │
│  - Get function instance                │
└─────────────┬───────────────────────────┘
              ↓
┌─────────────────────────────────────────┐
│ WASM Runtime (wazero/wasmtime)          │
│  - Execute WASM function                │
│  - Pass document context                │
└─────────────┬───────────────────────────┘
              ↓
┌─────────────────────────────────────────┐
│ WASM Function (user-provided)           │
│  - Access document fields               │
│  - Perform computation                  │
│  - Return result                        │
└─────────────────────────────────────────┘
```

---

## Runtime Choice: wazero vs wasmtime

### wazero (Recommended ✅)

**Pros**:
- Pure Go implementation (no CGO!)
- Easy integration
- Good performance
- Active development
- Well-documented

**Cons**:
- Slightly slower than wasmtime
- Less mature than wasmtime

**Performance**: ~100-500ns per call (acceptable for 15% use cases)

### wasmtime

**Pros**:
- Fastest WASM runtime
- Production-proven
- Excellent optimization

**Cons**:
- Requires CGO (complicates build)
- Another C dependency
- More complex integration

**Performance**: ~50-100ns per call

### Decision: wazero ✅

**Rationale**:
1. No CGO = simpler builds
2. Pure Go = easier debugging
3. Performance adequate for use case
4. Better Go ecosystem integration
5. We already have CGO for C++ - avoid more complexity

---

## Implementation Plan

### Day 1: Runtime Integration

**Tasks**:
1. Install wazero dependency
2. Create WASM runtime manager
3. Load and compile WASM modules
4. Basic function calling (hello world)

**Deliverables**:
- `pkg/wasm/runtime.go` - Runtime manager
- `pkg/wasm/module.go` - Module loading
- Basic tests

**Lines**: ~300

### Day 2: Document Context API

**Tasks**:
1. Design document field access API for WASM
2. Implement host functions (getField, hasField)
3. Memory management for WASM ↔ Go
4. Type conversion helpers

**Deliverables**:
- `pkg/wasm/context.go` - Document context
- `pkg/wasm/hostfunctions.go` - Host function implementations
- Type conversion tests

**Lines**: ~400

### Day 3: UDF Registry

**Tasks**:
1. Create UDF registry
2. Module registration and validation
3. Function lookup and caching
4. UDF metadata management

**Deliverables**:
- `pkg/wasm/registry.go` - UDF registry
- `pkg/wasm/metadata.go` - UDF metadata
- Registration API
- Tests

**Lines**: ~350

### Day 4: Query Integration

**Tasks**:
1. Add "wasm_udf" query type to parser
2. Extract UDF parameters
3. Call WASM functions during search
4. Result handling

**Deliverables**:
- Parser updates for wasm_udf
- Integration with data node
- End-to-end test

**Lines**: ~200

### Day 5: Testing & Examples

**Tasks**:
1. Comprehensive integration tests
2. Example WASM UDFs (Rust, Go, AssemblyScript)
3. Performance benchmarks
4. Documentation

**Deliverables**:
- Integration tests
- Example UDFs (3-5 examples)
- Performance report
- User guide

**Lines**: ~500 (tests + examples)

---

## WASM UDF Examples

### 1. String Distance (Levenshtein)

```json
{
  "query": {
    "wasm_udf": {
      "function": "string_distance",
      "params": {
        "field": "product_name",
        "target": "iPhone 15",
        "max_distance": 3
      }
    }
  }
}
```

**Use Case**: Fuzzy string matching beyond simple prefix/wildcard

### 2. Custom Scoring

```json
{
  "query": {
    "wasm_udf": {
      "function": "custom_score",
      "params": {
        "price_weight": 0.3,
        "rating_weight": 0.5,
        "popularity_weight": 0.2
      }
    }
  }
}
```

**Use Case**: Complex scoring algorithms

### 3. Date Range Business Logic

```json
{
  "query": {
    "wasm_udf": {
      "function": "business_days_between",
      "params": {
        "start_field": "order_date",
        "end_field": "ship_date",
        "min_days": 2,
        "max_days": 5
      }
    }
  }
}
```

**Use Case**: Business logic that expressions can't express

### 4. JSON Path Query

```json
{
  "query": {
    "wasm_udf": {
      "function": "json_path",
      "params": {
        "field": "metadata",
        "path": "$.tags[?(@.type=='category')].value",
        "operator": "contains",
        "value": "electronics"
      }
    }
  }
}
```

**Use Case**: Complex JSON queries

### 5. Regex with Capture Groups

```json
{
  "query": {
    "wasm_udf": {
      "function": "regex_extract",
      "params": {
        "field": "description",
        "pattern": "Model: ([A-Z0-9]+)",
        "group": 1,
        "equals": "ABC123"
      }
    }
  }
}
```

**Use Case**: Advanced regex operations

---

## API Design

### Host Functions (Go → WASM)

```go
// Available to WASM functions
type HostAPI interface {
    // Field access
    GetFieldString(fieldPath string) (string, bool)
    GetFieldInt64(fieldPath string) (int64, bool)
    GetFieldFloat64(fieldPath string) (float64, bool)
    GetFieldBool(fieldPath string) (bool, bool)

    // Document metadata
    GetDocumentID() string
    GetScore() float64

    // Logging (for debugging)
    Log(message string)
}
```

### WASM Function Signature

```rust
// Example WASM function in Rust
#[no_mangle]
pub extern "C" fn string_distance(
    ctx_ptr: i32,           // Document context pointer
    target_ptr: i32,        // Target string pointer
    target_len: i32,        // Target string length
    max_distance: i32       // Max allowed distance
) -> i32 {                 // Returns: 1 = match, 0 = no match
    // Implementation
}
```

### Registry API

```go
// Register UDF
registry.Register("string_distance", &UDFMetadata{
    Name:        "string_distance",
    Description: "Levenshtein distance string matching",
    Version:     "1.0.0",
    Author:      "team@example.com",
    WASMModule:  wasmBytes,
    Function:    "string_distance",
    Parameters: []Parameter{
        {Name: "field", Type: "string", Required: true},
        {Name: "target", Type: "string", Required: true},
        {Name: "max_distance", Type: "int", Required: false, Default: "2"},
    },
})

// Call UDF
result, err := registry.Call("string_distance", doc, params)
```

---

## Performance Targets

| Operation | Target | Notes |
|-----------|--------|-------|
| Module load | <10ms | One-time per startup |
| Module compile | <50ms | One-time per module |
| Function call | <1μs | Per document |
| String parameter | <100ns | Memory copy |
| Field access | <50ns | Via host function |
| Total overhead | <2μs | Acceptable for 15% |

### Optimization Strategies

1. **Module Caching**: Compile once, reuse
2. **Instance Pooling**: Pre-allocated instances
3. **Memory Reuse**: Reuse WASM memory across calls
4. **JIT Compilation**: wazero supports JIT
5. **Batch Calling**: Process multiple docs at once

---

## File Structure

```
pkg/wasm/
├── runtime.go          # WASM runtime manager
├── module.go           # Module loading and compilation
├── context.go          # Document context for WASM
├── hostfunctions.go    # Host functions (getField, etc)
├── registry.go         # UDF registry
├── metadata.go         # UDF metadata structures
├── types.go            # Type conversions
└── wasm_test.go        # Tests

pkg/wasm/examples/
├── string_distance/    # Rust example
├── custom_score/       # Go example
└── json_path/          # AssemblyScript example

pkg/coordination/parser/
└── types.go            # Add WasmUDFQuery type
└── parser.go           # Add wasm_udf parsing
```

---

## Testing Strategy

### Unit Tests

1. **Runtime Tests**
   - Module loading
   - Function calling
   - Error handling

2. **Context Tests**
   - Field access from WASM
   - Type conversion
   - Memory management

3. **Registry Tests**
   - Registration
   - Lookup
   - Validation

### Integration Tests

1. **End-to-End**
   - Query → UDF → Result
   - Multiple documents
   - Error cases

2. **Performance**
   - Call latency
   - Memory usage
   - Module compilation time

### Example UDFs

1. String distance (Rust)
2. Custom scoring (Go)
3. JSON path (AssemblyScript)

---

## Documentation Deliverables

1. **WASM_UDF_GUIDE.md** - User guide for creating UDFs
2. **WASM_INTEGRATION.md** - Integration architecture
3. **WASM_API_REFERENCE.md** - Host API reference
4. **WASM_EXAMPLES.md** - Example UDFs with code

---

## Success Criteria

### Week 3 Goals

- [ ] wazero integrated and working
- [ ] Document context API functional
- [ ] UDF registry operational
- [ ] Example UDFs working (3+)
- [ ] Integration tests passing (>90%)
- [ ] Performance <1μs per call
- [ ] Documentation complete

### Deliverables

- [ ] ~1,750 lines of implementation
- [ ] ~500 lines of tests
- [ ] ~1,500 lines of documentation
- [ ] 3-5 example UDFs (Rust, Go, AssemblyScript)

**Total Week 3**: ~3,750 lines

---

## Risk Mitigation

### Risk 1: Performance

**Risk**: WASM calls too slow
**Mitigation**: Module caching, instance pooling, benchmarking
**Fallback**: wasmtime if wazero too slow

### Risk 2: Memory Management

**Risk**: Memory leaks between Go ↔ WASM
**Mitigation**: Careful pointer tracking, automated tests
**Fallback**: Arena allocators

### Risk 3: Type Conversion

**Risk**: Complex type conversions error-prone
**Mitigation**: Comprehensive type tests, clear API
**Fallback**: Limit to simple types initially

---

## Dependencies

### Go Packages

```go
import (
    "github.com/tetratelabs/wazero"
    "github.com/tetratelabs/wazero/api"
    "github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)
```

### Build Tools (for examples)

- Rust + cargo (for Rust examples)
- TinyGo (for Go → WASM)
- AssemblyScript (for TS examples)

---

## Timeline

| Day | Focus | Deliverables | Lines |
|-----|-------|--------------|-------|
| 1 | Runtime integration | runtime.go, module.go | ~300 |
| 2 | Document context | context.go, hostfunctions.go | ~400 |
| 3 | UDF registry | registry.go, metadata.go | ~350 |
| 4 | Query integration | Parser updates, integration | ~200 |
| 5 | Testing & examples | Tests, examples, docs | ~500 |

**Total**: ~1,750 lines code + ~2,000 lines tests/docs = ~3,750 lines

---

## Next Steps

1. Install wazero dependency
2. Create `pkg/wasm/` directory structure
3. Implement runtime manager
4. Write basic "hello world" WASM test
5. Iterate through days 1-5

---

**Ready to start Week 3!** 🚀

**First task**: Install wazero and create basic runtime manager
