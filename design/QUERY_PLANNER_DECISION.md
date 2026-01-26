# Query Planner & UDF Pushdown: Design Decision

**Date**: 2026-01-25
**Status**: Decided - WASM UDF + Multi-Tiered Approach
**Decision Owner**: Architecture Team

---

## Executive Summary

**Decision**: Use **WebAssembly (WASM) UDFs** as the primary script pushdown mechanism, combined with **Expression Trees** for simple cases and **Python UDFs** for ML workloads.

**Replaces**: Original plan to use Apache Calcite (Java) for UDF pushdown

**Rationale**: WASM provides near-native performance (20ns/call), sandboxed security, language flexibility, and eliminates Java dependency while solving the script pushdown challenge in Go/C++ systems.

---

## Problem Statement

### The Challenge

**Requirement**: Users need to push custom logic (scoring, filtering, transformations) to data nodes for execution during query processing.

**Constraint**: Go/C++ systems cannot easily serialize and execute arbitrary code like JVM systems (Apache Calcite) can.

**Original Solution**: Use Apache Calcite for query planning with Java UDF pushdown.

**Issue Identified**: Adding Java/JVM to the stack increases complexity and goes against the Go+C++Python design philosophy.

---

## Alternatives Evaluated

### 1. Apache Calcite (Java) - Original Plan

**Pros**:
- ✅ Mature query optimizer
- ✅ Built-in Java UDF support
- ✅ Cost-based planning

**Cons**:
- ❌ Adds JVM to tech stack (Go + C++ + Python + **Java**)
- ❌ JVM startup overhead (~1-2s)
- ❌ Higher memory footprint
- ❌ Java UDFs less familiar than Rust/Python for users

**Score**: 13/20

---

### 2. Apache Arrow DataFusion (Rust)

**Pros**:
- ✅ Modern, high-performance
- ✅ Arrow-native (good for columnar)
- ✅ No JVM overhead
- ✅ Rust fits well with C++

**Cons**:
- ⚠️ Adds Rust to tech stack
- ⚠️ Doesn't solve UDF pushdown problem directly
- ⚠️ Still need separate UDF mechanism

**Score**: Not directly comparable (query planner only, not UDF solution)

---

### 3. Custom Go Query Planner

**Pros**:
- ✅ Pure Go stack
- ✅ Full control
- ✅ No external dependencies

**Cons**:
- ⚠️ 4-6 months development time
- ⚠️ Requires query optimization expertise
- ⚠️ Still doesn't solve UDF pushdown

**Score**: Good for planner, doesn't address UDF pushdown

---

### 4. Expression Tree Pushdown

**Pros**:
- ✅ Fast (native C++ evaluation)
- ✅ Safe (no arbitrary code)
- ✅ Simple to implement

**Cons**:
- ❌ Limited to predefined operations
- ❌ Can't express complex logic
- ❌ No loops, conditionals beyond ternary

**Score**: 16/20 (excellent for simple cases, insufficient alone)

---

### 5. Python UDF Pushdown

**Pros**:
- ✅ Maximum flexibility
- ✅ Rich ML ecosystem
- ✅ Familiar to data scientists

**Cons**:
- ❌ Slow (~500ns/call, 25× slower than native)
- ❌ Not suitable for performance-critical paths

**Score**: 14/20 (good for ML, too slow for general use)

---

### 6. WebAssembly (WASM) UDFs ⭐ Winner

**Pros**:
- ✅ Near-native performance (20ns/call, only 10× native C++)
- ✅ Sandboxed security (cannot access arbitrary memory)
- ✅ Language-agnostic (Rust, C, AssemblyScript, Go)
- ✅ Industry-proven (Cloudflare, Fastly, Shopify)
- ✅ Portable (x86_64, ARM64)
- ✅ Zero per-query compilation (compile once at deployment)
- ✅ Instant deployment with tiered compilation

**Cons**:
- ⚠️ Users need to compile to WASM (but tooling is mature)
- ⚠️ Requires embedding WASM runtime (~5MB)

**Score**: 17/20 (best overall solution)

---

## Final Decision: Multi-Tiered Approach

### Architecture

Use **different mechanisms** based on complexity:

```
┌─────────────────────────────────────────────┐
│         UDF Complexity Decision             │
├─────────────────────────────────────────────┤
│                                             │
│  Simple Math (price * 1.2 > 100)           │
│    → Expression Tree Pushdown              │
│    → Native C++ evaluation (fastest)       │
│    → Implementation: Phase 2               │
│                                             │
│  Performance-Critical Custom Logic         │
│    → WASM UDF                              │
│    → Near-native speed (20ns/call)         │
│    → Implementation: Phase 2-3             │
│                                             │
│  ML Models / Complex Python Logic          │
│    → Python UDF                            │
│    → Embedded CPython (500ns/call)         │
│    → Implementation: Phase 3               │
│                                             │
└─────────────────────────────────────────────┘
```

### Coverage Estimates

- **Expression Tree**: 75-80% of use cases (simple filters, math)
- **WASM UDF**: 15-20% of use cases (custom scoring, complex logic)
- **Python UDF**: 5% of use cases (ML models, data science)

---

## Query Planner Decision

### For Phase 2-3: Custom Go Planner ⭐ Recommended

**Decision**: Build a **lightweight query planner in Go** rather than integrate Calcite or DataFusion.

**Rationale**:
1. **Simpler stack**: Pure Go (no Java, no Rust)
2. **Sufficient for needs**: Most queries are simple (term, boolean, aggregations)
3. **Faster integration**: Direct integration with Go coordination nodes
4. **Full control**: Can optimize specifically for search workloads
5. **Lower latency**: No gRPC call to external optimizer

**Scope**:
- Basic rule-based optimizer (Phase 2)
- Cost model for index selection (Phase 2)
- Advanced optimizations (Phase 3-4)

### For Phase 5+ (Optional): Consider DataFusion

If advanced SQL/PPL features require sophisticated optimization, evaluate DataFusion integration in Phase 5+.

---

## Implementation Plan

### Phase 2 (Months 6-8): Foundation

**Query Planner**:
- [ ] Basic Go query planner
- [ ] Rule-based optimization (predicate pushdown, filter merging)
- [ ] Index selection with cost model
- [ ] Physical plan generation

**UDFs**:
- [ ] Expression Tree pushdown (for simple cases)
- [ ] WASM UDF framework (tiered compilation)
- [ ] wasm3 interpreter integration
- [ ] Wasmtime JIT integration

**Deliverables**:
- 80% of queries optimized (expression trees)
- WASM UDF support with 20ns/call performance
- Zero deployment latency (tiered compilation)

---

### Phase 3 (Months 9-10): Advanced UDFs

**Enhancements**:
- [ ] Python UDF pushdown (for ML workloads)
- [ ] Native code cache (5ms re-deploys)
- [ ] UDF registry and versioning
- [ ] Advanced query optimizations

**Deliverables**:
- Python UDF support for ML models
- Cached WASM UDFs (5ms startup)
- 95% of use cases covered

---

### Phase 4+ (Optional): AOT and Optimization

**Optional**:
- [ ] AOT WASM compilation support
- [ ] SIMD vectorized UDF execution
- [ ] Cost-based query optimization
- [ ] (Consider DataFusion if needed)

---

## Performance Comparison

### UDF Execution Performance

| Approach | Latency per Call | Throughput | Notes |
|----------|------------------|------------|-------|
| **Expression Tree (C++)** | 5ns | 200M/sec | Predefined ops only |
| **WASM UDF (JIT)** | 20ns | 50M/sec | Custom logic, near-native |
| **Python UDF** | 500ns | 2M/sec | ML models, flexible |
| **Calcite Java UDF** | 50ns | 20M/sec | JVM overhead |

### Query Impact (10,000 documents)

| Scenario | Total Time | vs Baseline | Notes |
|----------|------------|-------------|-------|
| **No UDF** | 0.5ms | 1.0× | BM25 only |
| **Expression Tree** | 0.55ms | 1.1× | Simple math |
| **WASM (native)** | 0.7ms | 1.4× | Custom logic |
| **WASM (interpreter)** | 2.5ms | 5.0× | Warmup period |
| **Python UDF** | 5.5ms | 11.0× | ML inference |
| **Calcite Java** | 1.0ms | 2.0× | JVM call |

**Verdict**: WASM is 3× faster than Calcite, 10× faster than Python, only 1.4× slower than no UDF.

---

## Compilation Efficiency

### WASM Compilation Timeline

```
T=0ms:      User deploys WASM UDF
T=1ms:      wasm3 interpreter loaded, accepting queries
T=0-250ms:  ~80 queries execute with interpreter
T=250ms:    Wasmtime JIT compilation completes
T=250ms+:   All queries use native code (20ns/call)
```

**Key Point**: 200ms JIT happens **ONCE per deployment**, NOT per query!

### Re-deployment Performance

| Scenario | Time | Notes |
|----------|------|-------|
| **First deployment** | 250ms | Interpreter (0ms) + JIT (250ms) |
| **Re-deployment (cached)** | 5ms | Load from native cache |
| **AOT deployment** | 5ms | Pre-compiled native .so |
| **Node restart (cached)** | 5ms | Load from disk |

---

## Technology Stack Impact

### Before Decision

```
Languages:
  - Go (master, coordination)
  - C++ (data nodes)
  - Python (pipelines)
  - Java (Calcite optimizer) ← Adds complexity
```

### After Decision

```
Languages:
  - Go (master, coordination, query planner)
  - C++ (data nodes)
  - Python (pipelines, ML UDFs)
  - WASM (user UDFs - compiled from Rust/C/AssemblyScript)

Runtimes in Data Nodes:
  - C++ native
  - Embedded CPython (for Python UDFs)
  - Embedded WASM (wasm3 + Wasmtime)
```

**Result**: **Simpler stack** (no JVM), **better performance**, **more flexibility** for users.

---

## User Experience

### Writing UDFs

**Before (Calcite/Java)**:
```java
// Java UDF (unfamiliar to most users)
public class CustomScorer extends ScalarFunction {
    public Double eval(Double score, Double boost) {
        return score * boost * 1.5;
    }
}
```

**After (WASM/Rust)**:
```rust
// Rust UDF (modern, type-safe)
#[no_mangle]
pub extern "C" fn custom_score(score: f64, boost: f64) -> f64 {
    score * boost * 1.5
}

// Compile: rustc --target wasm32-wasi custom_score.rs
```

**Benefits**:
- ✅ More familiar languages (Rust, C, AssemblyScript)
- ✅ Better performance (20ns vs 50ns)
- ✅ Safer (sandboxed WASM vs JVM)
- ✅ Smaller artifacts (50-200KB vs MB-sized JARs)

---

## Risk Assessment

### Risks Mitigated

| Risk | Mitigation |
|------|------------|
| **JVM complexity** | Eliminated (no Java/JVM) |
| **Slow UDF execution** | WASM JIT achieves 20ns/call |
| **Deployment latency** | Tiered compilation (0ms start) |
| **Security concerns** | WASM sandboxing |
| **Platform portability** | WASM is cross-platform |

### New Risks Introduced

| Risk | Mitigation | Severity |
|------|------------|----------|
| **WASM runtime bugs** | Use mature runtimes (wasm3, Wasmtime) | Low |
| **User learning curve** | Provide templates, examples | Low |
| **Compilation overhead** | Cache native code (5ms re-deploy) | Low |

---

## Success Criteria

### Phase 2 Success

- [ ] Expression Tree supports 80% of use cases
- [ ] WASM UDFs achieve <30ns/call execution
- [ ] Zero-latency deployment (tiered compilation)
- [ ] Query planner handles all DSL queries
- [ ] No Java/JVM in production stack

### Phase 3 Success

- [ ] Python UDF support for ML workloads
- [ ] Native code cache (5ms re-deploys)
- [ ] 95% of use cases covered
- [ ] Production-ready monitoring

---

## References

- **Detailed Design**: See [WASM_UDF_DESIGN.md](WASM_UDF_DESIGN.md)
- **Architecture**: See [QUIDDITCH_ARCHITECTURE.md](QUIDDITCH_ARCHITECTURE.md) §7 (Python Integration)
- **Implementation**: See [IMPLEMENTATION_ROADMAP.md](IMPLEMENTATION_ROADMAP.md) Phase 2-3
- **Query Processing**: See [QUIDDITCH_ARCHITECTURE.md](QUIDDITCH_ARCHITECTURE.md) §4 (Query Processing Pipeline)

---

## Decision Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-01-15 | Initial plan: Calcite for query planning | Mature optimizer, Java UDF support |
| 2026-01-25 | **Revised**: WASM UDF + Go planner | Better performance, simpler stack, no JVM |
| 2026-01-25 | Add Expression Tree for simple cases | Cover 80% of queries with native speed |
| 2026-01-25 | Add Python UDF for ML workloads | Support data science use cases |

---

## Approval

**Approved by**: Architecture Team
**Date**: 2026-01-25
**Next Review**: Before Phase 2 implementation

---

**Status**: ✅ **Decided and Documented**
**Implementation**: Phase 2-3 (Months 6-10)

---

Made with ❤️ by the Quidditch team
